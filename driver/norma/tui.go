package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"slices"
	"time"

	"github.com/0xsoniclabs/norma/driver/globalflags"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	nodemon "github.com/0xsoniclabs/norma/driver/monitoring/node"
	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/gdamore/tcell/v2"
	"github.com/navidys/tvxwidgets"
	"github.com/rivo/tview"
	"github.com/urfave/cli/v2"
)

var tuiFlag = cli.BoolFlag{
	Name:  "tui",
	Usage: "shows the scenario steps, live plots and the log in a terminal UI",
}

// stdout receives the multi-line output of a run that is not logged; the TUI
// redirects it into its log pane.
var stdout io.Writer = os.Stdout

var lineColors = []tcell.Color{
	tcell.ColorGreen, tcell.ColorYellow, tcell.ColorDodgerBlue, tcell.ColorFuchsia,
	tcell.ColorAqua, tcell.ColorOrange, tcell.ColorRed, tcell.ColorWhite,
}

// tui shows the steps of the running scenario, plots of its metrics and the log.
type tui struct {
	app    *tview.Application
	steps  *tview.List
	plot   *tvxwidgets.Plot
	legend *tview.TextView
	log    *tview.TextView
}

// runWithTUI runs the scenarios while the TUI shows their progress. Quitting
// during the run aborts it; the TUI stays open after the run until quit again.
func runWithTUI(cliCtx *cli.Context, runAll func(context.Context, *tui) error) error {
	ctx, cancel := context.WithCancel(cliCtx.Context)
	defer cancel()

	t := &tui{
		app:    tview.NewApplication(),
		steps:  tview.NewList().ShowSecondaryText(false),
		plot:   tvxwidgets.NewPlot(),
		legend: tview.NewTextView().SetDynamicColors(true),
		log:    tview.NewTextView().SetDynamicColors(true).SetMaxLines(10_000).ScrollToEnd(),
	}
	t.steps.SetBorder(true).SetTitle(" Steps ")
	t.log.SetBorder(true).SetTitle(" Log ")
	t.log.SetChangedFunc(func() { t.app.Draw() })
	t.plot.SetMarker(tvxwidgets.PlotMarkerBraille)
	t.plot.SetYAxisLabelDataType(tvxwidgets.PlotYAxisLabelDataInt)
	t.plot.SetYAxisAutoScaleMin(true)
	t.plot.SetDrawXAxisLabel(false)

	tabs := tview.NewTextView().SetRegions(true).SetDynamicColors(true)
	pages := tview.NewPages()
	blockHeight := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.plot, 0, 1, false).
		AddItem(t.legend, 1, 0, false)
	for i, tab := range []struct {
		name string
		view tview.Primitive
	}{{"Block height", blockHeight}} {
		id := fmt.Sprint(i + 1)
		pages.AddPage(id, tab.view, true, i == 0)
		fmt.Fprintf(tabs, `["%s"] %s: %s [""] `, id, id, tab.name)
	}
	tabs.Highlight("1")
	plots := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tabs, 1, 0, false).
		AddItem(pages, 0, 1, false)
	plots.SetBorder(true).SetTitle(" Plots ")

	right := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(plots, 0, 1, false).
		AddItem(t.log, 0, 1, true)
	root := tview.NewFlex().
		AddItem(t.steps, 0, 1, false).
		AddItem(right, 0, 3, true)

	finished := make(chan struct{})
	t.app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch {
		case ev.Key() == tcell.KeyCtrlC || ev.Rune() == 'q':
			select {
			case <-finished:
				t.app.Stop()
			default:
				slog.Info("aborting run ...")
				cancel()
			}
			return nil
		case pages.HasPage(string(ev.Rune())):
			tabs.Highlight(string(ev.Rune()))
			pages.SwitchToPage(string(ev.Rune()))
			return nil
		}
		return ev
	})

	logOutput := escapeWriter{tview.ANSIWriter(t.log)}
	if err := globalflags.SetupLoggerWithOutput(cliCtx, logOutput, true); err != nil {
		return err
	}
	slog.SetDefault(slog.New(stepHandler{slog.Default().Handler(), t.stepStarted}))
	stdout = logOutput
	defer func() {
		stdout = os.Stdout
		_ = globalflags.SetupLogger(cliCtx)
	}()

	var err error
	go func() {
		defer close(finished)
		if err = runAll(ctx, t); err != nil {
			slog.Error("run failed", "error", err)
		}
		slog.Info("run finished, press q to quit")
	}()

	if uiErr := t.app.SetRoot(root, true).Run(); uiErr != nil {
		cancel()
		<-finished
		return uiErr
	}
	return err
}

// show lists the steps of scenario and plots the metrics monitor collects until
// the returned function is called.
func (t *tui) show(scenario *parser.Scenario, monitor *monitoring.Monitor) (stop func()) {
	t.app.QueueUpdateDraw(func() {
		t.steps.Clear().SetTitle(" " + scenario.Name + " ")
		for i, step := range scenario.Steps {
			t.steps.AddItem(fmt.Sprintf("%d. %s %s", i+1, step.Function, step.Identifier), "", 0, nil)
		}
	})

	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		var nodes []monitoring.Node
		var lines [][]float64
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
			}
			nodes, lines = sampleBlockHeights(monitor, nodes, lines)
			t.plotLines(slices.Clone(nodes), slices.Clone(lines))
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}

// plotLines plots as much of the end of each node's line as fits the plot.
func (t *tui) plotLines(nodes []monitoring.Node, lines [][]float64) {
	t.app.QueueUpdateDraw(func() {
		_, _, width, _ := t.plot.GetPlotRect()
		keep := max(width-1, 1)
		colors := make([]tcell.Color, len(lines))
		legend := ""
		for i := range lines {
			lines[i] = lines[i][max(0, len(lines[i])-keep):]
			colors[i] = lineColors[i%len(lineColors)]
			legend += fmt.Sprintf("[#%06x]■ %s[-]  ", colors[i].Hex(), nodes[i])
		}
		t.plot.SetLineColor(colors)
		t.plot.SetData(lines)
		t.legend.SetText(legend)
	})
}

func (t *tui) stepStarted(step int) {
	t.app.QueueUpdateDraw(func() { t.steps.SetCurrentItem(step) })
}

// sampleBlockHeights appends the latest block height of every node to its line.
// The line of a node seen for the first time starts with gaps, so that all
// lines share the same sample times.
func sampleBlockHeights(monitor *monitoring.Monitor, nodes []monitoring.Node, lines [][]float64) ([]monitoring.Node, [][]float64) {
	samples := 0
	if len(lines) > 0 {
		samples = len(lines[0])
	}
	for _, node := range monitoring.GetSubjects(monitor, nodemon.NodeBlockStatus) {
		if !slices.Contains(nodes, node) {
			nodes = append(nodes, node)
			lines = append(lines, slices.Repeat([]float64{math.NaN()}, samples))
		}
	}
	for i, node := range nodes {
		height := math.NaN()
		if series, ok := monitoring.GetData(monitor, node, nodemon.NodeBlockStatus); ok {
			if point := series.GetLatest(); point != nil {
				height = float64(point.Value.BlockHeight)
			}
		}
		lines[i] = append(lines[i], height)
	}
	return nodes, lines
}

// stepHandler reports the start of every scenario step, read from the record
// the executor logs for it.
// ponytail: couples to the executor's "step"/"phase" log attributes, as the
// operation colouring does; add a start hook to the executor if they change.
type stepHandler struct {
	slog.Handler
	onStart func(step int)
}

func (h stepHandler) Handle(ctx context.Context, r slog.Record) error {
	phase, step := "", int64(0)
	r.Attrs(func(a slog.Attr) bool {
		switch {
		case a.Key == "phase":
			phase = a.Value.String()
		case a.Key == "step" && a.Value.Kind() == slog.KindInt64:
			step = a.Value.Int64()
		}
		return true
	})
	if phase == "started" && step > 0 {
		h.onStart(int(step) - 1)
	}
	return h.Handler.Handle(ctx, r)
}

// escapeWriter keeps text written to a TextView from being read as style tags.
type escapeWriter struct{ io.Writer }

func (w escapeWriter) Write(p []byte) (int, error) {
	if _, err := w.Writer.Write([]byte(tview.Escape(string(p)))); err != nil {
		return 0, err
	}
	return len(p), nil
}
