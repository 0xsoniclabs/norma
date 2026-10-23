package main

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/globalflags"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	nodemon "github.com/0xsoniclabs/norma/driver/monitoring/node"
	txmon "github.com/0xsoniclabs/norma/driver/monitoring/transactions"
	usermon "github.com/0xsoniclabs/norma/driver/monitoring/user"
	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/gdamore/tcell/v2"
	"github.com/navidys/tvxwidgets"
	"github.com/rivo/tview"
	"github.com/urfave/cli/v2"
)

var tuiFlag = cli.BoolFlag{
	Name:  "tui",
	Usage: "shows the scenario steps, live plots and the logs in a terminal UI",
}

// stdout receives the multi-line output of a run that is not logged; the TUI
// redirects it into its log pane.
var stdout io.Writer = os.Stdout

var lineColors = []tcell.Color{
	tcell.ColorGreen, tcell.ColorYellow, tcell.ColorDodgerBlue, tcell.ColorFuchsia,
	tcell.ColorAqua, tcell.ColorOrange, tcell.ColorRed, tcell.ColorWhite,
}

// tui shows the steps of the running scenario, plots of its metrics, the log
// and the log of every node in a tab.
type tui struct {
	app      *tview.Application
	steps    *tview.List
	nodes    *tview.Table
	plots    []*plotTab
	tabs     *tview.TextView
	pages    *tview.Pages
	tabNames []string
	current  int
	stopped  chan struct{}
}

// runWithTUI runs the scenarios while the TUI shows their progress. Quitting
// during the run aborts it; the TUI stays open after the run until quit again.
func runWithTUI(cliCtx *cli.Context, runAll func(context.Context, *tui) error) error {
	ctx, cancel := context.WithCancel(cliCtx.Context)
	defer cancel()

	t := &tui{
		app:   tview.NewApplication(),
		steps: tview.NewList().ShowSecondaryText(false),
		nodes: tview.NewTable().SetFixed(1, 0),
		plots: []*plotTab{
			newPlotTab("Blocks/s", func(m *monitoring.Monitor) map[string]float64 {
				return perNode(m, nodemon.NodeBlockStatus, rate(func(s monitoring.BlockStatus) float64 { return float64(s.BlockHeight) }))
			}),
			newPlotTab("Txs/s", func(m *monitoring.Monitor) map[string]float64 {
				res := map[string]float64{}
				if v, ok := total(m, usermon.SentTransactions, rate(intToFloat)); ok {
					res["sent"] = v
				}
				if v, ok := total(m, txmon.TransactionsIncluded, rate(intToFloat)); ok {
					res["committed"] = v
				}
				return res
			}),
		},
		tabs:    tview.NewTextView().SetRegions(true).SetDynamicColors(true),
		pages:   tview.NewPages(),
		stopped: make(chan struct{}),
	}
	t.steps.SetBorder(true).SetTitle(" Steps ")
	t.nodes.SetBorder(true).SetTitle(" Nodes ")
	log := newLogView()
	log.SetChangedFunc(func() { t.app.Draw() })
	t.addTab("Norma", log)

	logs := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.tabs, 1, 0, false).
		AddItem(t.pages, 0, 1, true)
	logs.SetBorder(true).SetTitle(" Logs ")
	plots := tview.NewFlex()
	for _, plot := range t.plots {
		view := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(plot, 0, 1, false).
			AddItem(plot.legend, 1, 0, false)
		view.SetBorder(true).SetTitle(" " + plot.name + " ")
		plots.AddItem(view, 0, 1, false)
	}
	right := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(logs, 0, 3, true).
		AddItem(plots, 0, 1, false)
	root := tview.NewFlex().
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(t.steps, 0, 3, false).
			AddItem(t.nodes, 0, 1, false), 0, 1, false).
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
		case ev.Key() == tcell.KeyTab:
			t.selectTab((t.current + 1) % len(t.tabNames))
			return nil
		case ev.Key() == tcell.KeyBacktab:
			t.selectTab((t.current + len(t.tabNames) - 1) % len(t.tabNames))
			return nil
		case ev.Rune() >= '1' && ev.Rune() <= '9' && int(ev.Rune()-'1') < len(t.tabNames):
			t.selectTab(int(ev.Rune() - '1'))
			return nil
		}
		return ev
	})

	logOutput := escapeWriter{tview.ANSIWriter(log)}
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
	run := func() {
		defer close(finished)
		if err = runAll(ctx, t); err != nil {
			slog.Error("run failed", "error", err)
		}
		slog.Info("run finished, press q to quit")
	}
	// The run starts once the UI has drawn, so not at all if it fails to start.
	var start sync.Once
	t.app.SetAfterDrawFunc(func(tcell.Screen) { start.Do(func() { go run() }) })
	// A signal also quits the TUI, once the run cleaned up.
	go func() {
		<-cliCtx.Done()
		<-finished
		t.app.Stop()
	}()

	uiErr := t.app.SetRoot(root, true).Run()
	close(t.stopped)
	if uiErr != nil {
		cancel()
		start.Do(func() { close(finished) })
		<-finished
		return uiErr
	}
	return err
}

// update runs f on the UI and redraws it. It gives up once the UI stopped, as
// nothing would run f anymore.
func (t *tui) update(f func()) {
	done := make(chan struct{})
	go func() {
		t.app.QueueUpdateDraw(f)
		close(done)
	}()
	select {
	case <-done:
	case <-t.stopped:
	}
}

// show lists the steps of scenario, plots the metrics monitor collects and
// shows the log of every node of its network until the returned function is
// called.
func (t *tui) show(scenario *parser.Scenario, monitor *monitoring.Monitor) (stop func()) {
	t.update(func() {
		t.steps.Clear().SetTitle(" " + scenario.Name + " ")
		for i, step := range scenario.Steps {
			t.steps.AddItem(fmt.Sprintf("%d. %s %s", i+1, step.Name(), steps.TargetOf(step)), "", 0, nil)
		}
		for _, plot := range t.plots {
			plot.subjects, plot.lines = nil, nil
		}
		for i := 1; i < len(t.tabNames); i++ {
			t.pages.RemovePage(fmt.Sprint(i))
		}
		t.tabNames = t.tabNames[:1]
		t.selectTab(0)
	})

	ctx, cancel := context.WithCancel(context.Background())
	logs := &nodeLogs{t: t, ctx: ctx, views: map[string]*tview.TextView{}, streamed: map[string]driver.Node{}}
	net := monitor.Network()
	net.RegisterListener(logs)
	for _, node := range net.GetActiveNodes() {
		logs.AfterNodeCreation(node)
	}

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
			// Sampled here, as the network may be locked for minutes while
			// it peers nodes, which must not block the UI.
			samples := make([]map[string]float64, len(t.plots))
			for i, plot := range t.plots {
				samples[i] = plot.sample(monitor)
			}
			rows := nodeRows(monitor)
			t.update(func() {
				for i, plot := range t.plots {
					plot.add(samples[i])
				}
				t.showNodes(rows)
			})
		}
	}()
	return func() {
		net.UnregisterListener(logs)
		cancel()
		close(done)
		<-stopped
	}
}

// addTab adds a tab showing view after the existing ones. Its page is named
// by its index, as a node may be labeled like any other tab.
func (t *tui) addTab(name string, view tview.Primitive) {
	t.pages.AddPage(fmt.Sprint(len(t.tabNames)), view, true, len(t.tabNames) == 0)
	t.tabNames = append(t.tabNames, name)
	t.showTabs()
}

func (t *tui) selectTab(i int) {
	t.current = i
	t.pages.SwitchToPage(fmt.Sprint(i))
	t.showTabs()
}

func (t *tui) showTabs() {
	t.tabs.Clear()
	for i, name := range t.tabNames {
		_, _ = fmt.Fprintf(t.tabs, `["%d"] %d: %s [""] `, i, i+1, tview.Escape(name))
	}
	t.tabs.Highlight(fmt.Sprint(t.current))
}

func newLogView() *tview.TextView {
	return tview.NewTextView().SetDynamicColors(true).SetMaxLines(10_000).ScrollToEnd()
}

// nodeLogs streams the log of every node of a network into a tab of the TUI. A
// node restarted under the same label continues the log of its tab.
// ponytail: node tabs redraw only with the plots, once a second; redraw on
// every write if that lags behind.
type nodeLogs struct {
	t        *tui
	ctx      context.Context
	lock     sync.Mutex
	views    map[string]*tview.TextView
	streamed map[string]driver.Node
}

func (l *nodeLogs) AfterNodeCreation(node driver.Node) {
	label := node.GetLabel()
	l.lock.Lock()
	defer l.lock.Unlock()
	// The stream of a node restarted in place stays open.
	if l.streamed[label] == node {
		return
	}
	l.streamed[label] = node
	view, ok := l.views[label]
	if !ok {
		view = newLogView()
		l.views[label] = view
		l.t.update(func() { l.t.addTab(label, view) })
	}
	go func() {
		defer func() {
			l.lock.Lock()
			if l.streamed[label] == node {
				delete(l.streamed, label)
			}
			l.lock.Unlock()
		}()
		stream, err := node.StreamLog(l.ctx)
		if err != nil {
			_, _ = fmt.Fprintf(view, "failed to stream log: %v\n", err)
			return
		}
		defer func() { _ = stream.Close() }()
		_, _ = io.Copy(escapeWriter{tview.ANSIWriter(view)}, stream)
	}()
}

func (l *nodeLogs) BeforeNodeRemoval(driver.Node) {}

func (l *nodeLogs) AfterApplicationCreation(driver.Application) {}

// nodeRows lists the running nodes with their latest block and epoch.
func nodeRows(monitor *monitoring.Monitor) [][3]string {
	var labels []string
	for _, node := range monitor.Network().GetActiveNodes() {
		labels = append(labels, node.GetLabel())
	}
	slices.Sort(labels)
	rows := make([][3]string, len(labels))
	for i, label := range labels {
		rows[i] = [3]string{label, "-", "-"}
		if series, ok := monitoring.GetData(monitor, monitoring.Node(label), nodemon.NodeBlockStatus); ok && series != nil {
			if point := series.GetLatest(); point != nil {
				rows[i][1], rows[i][2] = fmt.Sprint(point.Value.BlockHeight), fmt.Sprint(point.Value.Epoch)
			}
		}
	}
	return rows
}

// showNodes shows the rows of nodeRows below a header.
func (t *tui) showNodes(rows [][3]string) {
	t.nodes.Clear()
	for column, header := range []string{"Node", "Block", "Epoch"} {
		align := tview.AlignRight
		if column == 0 {
			align = tview.AlignLeft
		}
		t.nodes.SetCell(0, column, tview.NewTableCell(header).SetAttributes(tcell.AttrBold).SetAlign(align).SetExpansion(1))
	}
	for i, row := range rows {
		t.nodes.SetCell(i+1, 0, tview.NewTableCell(row[0]).SetExpansion(1))
		t.nodes.SetCell(i+1, 1, tview.NewTableCell(row[1]).SetAlign(tview.AlignRight).SetExpansion(1))
		t.nodes.SetCell(i+1, 2, tview.NewTableCell(row[2]).SetAlign(tview.AlignRight).SetExpansion(1))
	}
}

func (t *tui) stepStarted(step int) {
	t.update(func() { t.steps.SetCurrentItem(step) })
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
	p.SetYAxisLabelDataType(tvxwidgets.PlotYAxisLabelDataFloat)
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
	var legend strings.Builder
	for i, subject := range p.subjects {
		value, ok := values[subject]
		if !ok {
			value = math.NaN()
		}
		p.lines[i] = append(p.lines[i], value)
		fmt.Fprintf(&legend, "[#%06x]■ %s[-]  ", lineColors[i%len(lineColors)].Hex(), subject)
	}
	p.legend.SetText(legend.String())
}

// Draw plots as much of the end of each line as fits.
func (p *plotTab) Draw(screen tcell.Screen) {
	_, _, width, _ := p.GetPlotRect()
	keep := max(width-1, 1)
	data := make([][]float64, len(p.lines))
	colors := make([]tcell.Color, len(p.lines))
	for i, line := range p.lines {
		data[i] = line[max(0, len(line)-keep):]
		colors[i] = lineColors[i%len(lineColors)].TrueColor()
	}
	p.SetLineColor(colors)
	p.SetData(data)
	p.Plot.Draw(screen)
}

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

// total sums the values of the series of metric of every subject, having none
// if no subject has one.
func total[S, T any](
	monitor *monitoring.Monitor,
	metric monitoring.Metric[S, monitoring.Series[monitoring.Time, T]],
	value func(monitoring.Series[monitoring.Time, T]) (float64, bool),
) (float64, bool) {
	sum, found := 0.0, false
	for _, subject := range monitoring.GetSubjects(monitor, metric) {
		series, ok := monitoring.GetData(monitor, subject, metric)
		if !ok || series == nil {
			continue
		}
		if v, ok := value(series); ok {
			sum, found = sum+v, true
		}
	}
	return sum, found
}

// rate is the per-second increase over the last rateWindow of a counter, read
// from a series with toFloat. It has no value once the counter is no longer
// collected, as for a stopped node, and while the window spans a reset of the
// counter.
func rate[T any](toFloat func(T) float64) func(monitoring.Series[monitoring.Time, T]) (float64, bool) {
	return func(series monitoring.Series[monitoring.Time, T]) (float64, bool) {
		last := series.GetLatest()
		if last == nil || time.Since(last.Position.Time()) > rateWindow {
			return 0, false
		}
		points := series.GetRange(last.Position-monitoring.Time(rateWindow), last.Position+1)
		if len(points) < 2 || !slices.IsSortedFunc(points, func(a, b monitoring.DataPoint[monitoring.Time, T]) int {
			return cmp.Compare(toFloat(a.Value), toFloat(b.Value))
		}) {
			return 0, false
		}
		first := points[0]
		return (toFloat(last.Value) - toFloat(first.Value)) / last.Position.Time().Sub(first.Position.Time()).Seconds(), true
	}
}

func intToFloat(v int) float64 { return float64(v) }

// stepHandler reports the start of every scenario step, read from the record
// the executor logs for it, whether or not the wrapped handler logs it.
// ponytail: couples to the executor's "step"/"phase" log attributes, as the
// operation colouring does; add a start hook to the executor if they change.
type stepHandler struct {
	slog.Handler
	onStart func(step int)
}

func (h stepHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo || h.Handler.Enabled(ctx, level)
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
	if !h.Handler.Enabled(ctx, r.Level) {
		return nil
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
