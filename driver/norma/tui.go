package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/0xsoniclabs/norma/driver/globalflags"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	netmon "github.com/0xsoniclabs/norma/driver/monitoring/network"
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
	app   *tview.Application
	steps *tview.List
	plots []*plotTab
	log   *tview.TextView
}

// runWithTUI runs the scenarios while the TUI shows their progress. Quitting
// during the run aborts it; the TUI stays open after the run until quit again.
func runWithTUI(cliCtx *cli.Context, runAll func(context.Context, *tui) error) error {
	ctx, cancel := context.WithCancel(cliCtx.Context)
	defer cancel()

	t := &tui{
		app:   tview.NewApplication(),
		steps: tview.NewList().ShowSecondaryText(false),
		plots: []*plotTab{
			newPlotTab("Block height", func(m *monitoring.Monitor) map[string]float64 {
				return perNode(m, nodemon.NodeBlockStatus, latest(func(s monitoring.BlockStatus) float64 { return float64(s.BlockHeight) }))
			}),
			newPlotTab("Epoch", func(m *monitoring.Monitor) map[string]float64 {
				return perNode(m, nodemon.NodeBlockStatus, latest(func(s monitoring.BlockStatus) float64 { return float64(s.Epoch) }))
			}),
			newPlotTab("Txs committed/s", func(m *monitoring.Monitor) map[string]float64 {
				return perNode(m, txsCommitted, rate)
			}),
			newPlotTab("Stake (S)", func(m *monitoring.Monitor) map[string]float64 {
				return perNode(m, netmon.ValidatorStake, latest(weiToS))
			}),
		},
		log: tview.NewTextView().SetDynamicColors(true).SetMaxLines(10_000).ScrollToEnd(),
	}
	t.steps.SetBorder(true).SetTitle(" Steps ")
	t.log.SetBorder(true).SetTitle(" Log ")
	t.log.SetChangedFunc(func() { t.app.Draw() })

	tabs := tview.NewTextView().SetRegions(true).SetDynamicColors(true)
	pages := tview.NewPages()
	for i, plot := range t.plots {
		id := fmt.Sprint(i + 1)
		view := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(plot, 0, 1, false).
			AddItem(plot.legend, 1, 0, false)
		pages.AddPage(id, view, true, i == 0)
		fmt.Fprintf(tabs, `["%s"] %s: %s [""] `, id, id, plot.name)
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
		for _, plot := range t.plots {
			plot.subjects, plot.lines = nil, nil
		}
	})

	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
			}
			t.app.QueueUpdateDraw(func() {
				for _, plot := range t.plots {
					plot.add(plot.sample(monitor))
				}
			})
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}

func (t *tui) stepStarted(step int) {
	t.app.QueueUpdateDraw(func() { t.steps.SetCurrentItem(step) })
}

// plotTab plots a line per subject of a metric, sampled once a second.
type plotTab struct {
	*tvxwidgets.Plot
	name     string
	sample   func(*monitoring.Monitor) map[string]float64
	legend   *tview.TextView
	subjects []string
	lines    [][]float64
}

func newPlotTab(name string, sample func(*monitoring.Monitor) map[string]float64) *plotTab {
	p := &plotTab{
		Plot:   tvxwidgets.NewPlot(),
		name:   name,
		sample: sample,
		legend: tview.NewTextView().SetDynamicColors(true),
	}
	p.SetMarker(tvxwidgets.PlotMarkerBraille)
	p.SetYAxisLabelDataType(tvxwidgets.PlotYAxisLabelDataInt)
	p.SetYAxisAutoScaleMin(true)
	p.SetDrawXAxisLabel(false)
	return p
}

// add appends the value of every subject to its line, or a gap if it has none.
// The line of a subject seen for the first time starts with gaps, so that all
// lines share the same sample times.
func (p *plotTab) add(values map[string]float64) {
	samples := 0
	if len(p.lines) > 0 {
		samples = len(p.lines[0])
	}
	for _, subject := range slices.Sorted(maps.Keys(values)) {
		if !slices.Contains(p.subjects, subject) {
			p.subjects = append(p.subjects, subject)
			p.lines = append(p.lines, slices.Repeat([]float64{math.NaN()}, samples))
		}
	}
	legend := ""
	for i, subject := range p.subjects {
		value, ok := values[subject]
		if !ok {
			value = math.NaN()
		}
		p.lines[i] = append(p.lines[i], value)
		legend += fmt.Sprintf("[#%06x]■ %s[-]  ", lineColors[i%len(lineColors)].Hex(), subject)
	}
	p.legend.SetText(legend)
}

// Draw plots as much of the end of each line as fits.
func (p *plotTab) Draw(screen tcell.Screen) {
	_, _, width, _ := p.GetPlotRect()
	keep := max(width-1, 1)
	data := make([][]float64, len(p.lines))
	colors := make([]tcell.Color, len(p.lines))
	for i, line := range p.lines {
		data[i] = line[max(0, len(line)-keep):]
		colors[i] = lineColors[i%len(lineColors)]
	}
	p.SetLineColor(colors)
	p.SetData(data)
	p.Plot.Draw(screen)
}

// txsCommitted counts the transactions in the blocks a node committed, read
// from its Prometheus metrics.
var txsCommitted = monitoring.Metric[monitoring.Node, monitoring.Series[monitoring.Time, float64]]{Name: "chain_txs_processed"}

// rateWindow is the time over which rate averages the increase of a counter.
const rateWindow = 5 * time.Second

// perNode converts the series of metric of every node to a value, skipping the
// nodes for which value has none.
func perNode[T any](
	monitor *monitoring.Monitor,
	metric monitoring.Metric[monitoring.Node, monitoring.Series[monitoring.Time, T]],
	value func(monitoring.Series[monitoring.Time, T]) (float64, bool),
) map[string]float64 {
	res := map[string]float64{}
	for _, node := range monitoring.GetSubjects(monitor, metric) {
		series, ok := monitoring.GetData(monitor, node, metric)
		if !ok || series == nil {
			continue
		}
		if v, ok := value(series); ok {
			res[string(node)] = v
		}
	}
	return res
}

// latest converts the latest value of a series with toFloat.
func latest[T any](toFloat func(T) float64) func(monitoring.Series[monitoring.Time, T]) (float64, bool) {
	return func(series monitoring.Series[monitoring.Time, T]) (float64, bool) {
		point := series.GetLatest()
		if point == nil {
			return 0, false
		}
		return toFloat(point.Value), true
	}
}

// rate is the per-second increase of a counter over the last rateWindow. It
// has no value once the counter is no longer collected, as for a stopped node,
// and while the window spans a reset of the counter.
func rate(series monitoring.Series[monitoring.Time, float64]) (float64, bool) {
	last := series.GetLatest()
	if last == nil || time.Since(last.Position.Time()) > rateWindow {
		return 0, false
	}
	points := series.GetRange(last.Position-monitoring.Time(rateWindow), last.Position+1)
	if len(points) < 2 || points[0].Value > last.Value {
		return 0, false
	}
	first := points[0]
	return (last.Value - first.Value) / last.Position.Time().Sub(first.Position.Time()).Seconds(), true
}

func weiToS(wei string) float64 {
	value, err := strconv.ParseFloat(wei, 64)
	if err != nil {
		return math.NaN()
	}
	return value / 1e18
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
