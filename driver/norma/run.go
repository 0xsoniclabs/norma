// Copyright 2024 Fantom Foundation
// This file is part of Norma System Testing Infrastructure for Sonic.
//
// Norma is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Norma is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with Norma. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/0xsoniclabs/norma/driver/checking"
	"golang.org/x/exp/maps"

	"github.com/0xsoniclabs/norma/analysis/report"
	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/executor"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	_ "github.com/0xsoniclabs/norma/driver/monitoring/app"
	prometheusmon "github.com/0xsoniclabs/norma/driver/monitoring/prometheus"
	_ "github.com/0xsoniclabs/norma/driver/monitoring/user"
	"github.com/0xsoniclabs/norma/driver/network/local"
	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/urfave/cli/v2"
)

// Run with `go run ./driver/norma run <scenario.yml>`

var runCommand = cli.Command{
	Action: run,
	Name:   "run",
	Usage:  "runs a scenario",
	Flags: []cli.Flag{
		&evalLabel,
		&keepPrometheusRunning,
		&numValidators,
		&skipChecks,
		&skipReportRendering,
		&outputDirectory,
		&openReport,
	},
}

var (
	evalLabel = cli.StringFlag{
		Name:  "label",
		Usage: "define a label for to be added to the monitoring data for this run. If empty, a random label is used.",
		Value: "",
	}
	outputDirectory = cli.StringFlag{
		Name:    "output-directory",
		Usage:   "define a directory at which the monitoring artifact will be saved.",
		Value:   "",
		Aliases: []string{"o"},
	}
	keepPrometheusRunning = cli.BoolFlag{
		Name:    "keep-prometheus-running",
		Usage:   "if set, the Prometheus instance will not be shut down after the run is complete.",
		Aliases: []string{"kpr"},
	}
	numValidators = cli.IntFlag{
		Name:  "num-validators",
		Usage: "overrides the number of validators specified in the scenario file.",
	}
	skipChecks = cli.BoolFlag{
		Name:  "skip-checks",
		Usage: "disables the final network consistency checks",
	}
	skipReportRendering = cli.BoolFlag{
		Name:  "skip-report-rendering",
		Usage: "disables the rendering of the final summary report",
	}
	openReport = cli.BoolFlag{
		Name:  "open-report",
		Usage: "automatically open the rendered report in the default browser after rendering",
	}
)

func run(ctx *cli.Context) (err error) {
	args := ctx.Args()
	if args.Len() < 1 {
		return fmt.Errorf("requires scenario file as an argument")
	}

	outputDir := ctx.String(outputDirectory.Name)
	keepPrometheusRunning := ctx.Bool(keepPrometheusRunning.Name)
	skipChecks := ctx.Bool(skipChecks.Name)
	skipReportRendering := ctx.Bool(skipReportRendering.Name)
	openReport := ctx.Bool(openReport.Name)

	path := args.First()

	// Create a context that is cancelled on SIGINT/SIGTERM so that both
	// network startup and scenario execution can be interrupted cleanly,
	// allowing all deferred shutdowns to execute.
	stoppableCtx, stop := signal.NotifyContext(ctx.Context, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-stoppableCtx.Done()
		slog.Info("stopping...")
		stop() // second Ctrl+C will force-kill
	}()

	files, err := collectScenarioFiles(path)
	if err != nil {
		return fmt.Errorf("failed to collect scenario files: %w", err)
	}
	for _, file := range files {
		if stoppableCtx.Err() != nil {
			return stoppableCtx.Err()
		}
		label := ctx.String(evalLabel.Name)
		if label == "" {
			label = fmt.Sprintf("eval_%d", time.Now().Unix())
		}
		if err := runScenario(stoppableCtx, file, outputDir, label, keepPrometheusRunning, skipChecks, skipReportRendering, openReport); err != nil {
			return fmt.Errorf("failed to run scenario %q: %w", file, err)
		}
	}

	return nil
}

func runScenario(ctx context.Context, path, outputDir, label string, keepPrometheusRunning, skipChecks, skipReportRendering, openReport bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// if not configured, default to /tmp/norma_data_<label>_<timestamp> else /configured/path/norma_data_<l>_<t>
	outputDir, err := os.MkdirTemp(outputDir, fmt.Sprintf("norma_data_%s_", label))
	if err != nil {
		return fmt.Errorf("couldn't create temp dir for output; %w", err)
	}

	slog.Info("reading scenario file", "path", path)
	scenario, err := parser.ParseFile(path)
	if err != nil {
		return err
	}

	if err := scenario.Check(); err != nil {
		return err
	}

	slog.Info("starting evaluation", "label", label)

	// create symlink as qol (_latest => _####) where #### is the randomly generated name
	symlink := filepath.Join(filepath.Dir(outputDir), fmt.Sprintf("norma_data_%s_latest", label))
	if _, lstatErr := os.Lstat(symlink); lstatErr == nil {
		if err := os.Remove(symlink); err != nil {
			return fmt.Errorf("failed to remove existing _latest symlink: %w", err)
		}
	}
	if err := os.Symlink(outputDir, symlink); err != nil {
		return fmt.Errorf("failed to create _latest symlink: %w", err)
	}

	slog.Info("monitoring data is written", "output", outputDir)

	// Copy scenario yml to outputDir as well to provide context
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(outputDir, filepath.Base(path)), data, 0644)
	if err != nil {
		return err
	}

	clock := executor.NewWallTimeClock()

	// Startup network.
	slog.Info("network RoundTripTime", "value", scenario.GetRoundTripTime())
	for k, v := range scenario.NetworkRules.Genesis {
		slog.Info("network Rule", "key", k, "value", v)
	}

	net, err := local.NewLocalNetwork(ctx, &driver.NetworkConfig{
		Validators:    driver.NewValidators(scenario.Validators),
		RoundTripTime: scenario.GetRoundTripTime(),
		NetworkRules:  driver.NetworkRules(maps.Clone(scenario.NetworkRules.Genesis)),
		OutputDir:     outputDir,
	})
	if err != nil {
		return err
	}
	defer func() {
		slog.Info("shutting down network ...")
		if err := net.Shutdown(); err != nil {
			slog.Error("error during network shutdown", "error", err)
		}
	}()

	// Initialize monitoring environment.
	monitor, err := monitoring.NewMonitor(net, monitoring.MonitorConfig{
		EvaluationLabel: label,
		OutputDir:       outputDir,
	})
	if err != nil {
		return err
	}
	defer func() {
		slog.Info("shutting down data monitor ...")
		if err := monitor.Shutdown(); err != nil {
			slog.Error("error during monitor shutdown", "error", err)
		}
		slog.Info("monitoring data was written", "output", outputDir)
		slog.Info("raw data was exported", "file", monitor.GetMeasurementFileName())

		if !skipReportRendering && ctx.Err() == nil {
			slog.Info("rendering summary report (may take a few minutes the first time if R packages need to be installed) ...")
			if file, err := report.SingleEvalReport.Render(monitor.GetMeasurementFileName(), outputDir, scenario.Name); err != nil {
				slog.Error("report generation failed", "error", err)
			} else {
				slog.Info("summary report was exported", "file", fmt.Sprintf("file://%s/%s", outputDir, file))
				if openReport {
					if err := openBrowser(filepath.Join(outputDir, file)); err != nil {
						slog.Warn("failed to open report in browser", "error", err)
					}
				}
			}
		} else {
			slog.Info("report rendering skipped")
			slog.Info(fmt.Sprintf("To render report run `norma render %s`", monitor.GetMeasurementFileName()))
		}
	}()

	// Install monitoring sensory.
	if err := monitoring.InstallAllRegisteredSources(monitor); err != nil {
		return err
	}

	// Run prometheus.
	slog.Info("starting Prometheus ...")
	prom, err := prometheusmon.Start(net, net.GetDockerNetwork())
	if err != nil {
		slog.Error("error starting Prometheus", "error", err)
	}
	defer func() {
		if !keepPrometheusRunning && prom != nil {
			slog.Info("shutting down Prometheus ...")
			if err := prom.Shutdown(); err != nil {
				slog.Error("error during Prometheus shutdown", "error", err)
			}
		}
	}()

	var checks map[string]checking.Checker
	if !skipChecks {
		// Initialize network consistency checks.
		checks = checking.InitNetworkChecks(net, monitor)
	}

	// Run scenario.
	slog.Info("running scenario", "path", path)
	logger := startProgressLogger(monitor, net)
	defer logger.shutdown()
	err = executor.Run(ctx, clock, net, &scenario, checks)
	if err != nil {
		dumpNodeLogs(net)
		return err
	}
	slog.Info("execution completed successfully")

	return nil
}

func openBrowser(s string) error {

	path, err := exec.LookPath("xdg-open")
	if err != nil {
		return fmt.Errorf("xdg-open not found: %w", err)
	}

	cmd := exec.Command(path, s)
	return cmd.Start()
}

// dumpNodeLogs prints the logs of all active nodes to help diagnose failures.
func dumpNodeLogs(net driver.Network) {
	nodes := net.GetActiveNodes()
	for _, node := range nodes {
		reader, err := node.StreamLog()
		if err != nil {
			slog.Error("failed to stream log", "node", node.GetLabel(), "error", err)
			continue
		}
		data, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			slog.Error("failed to read log", "node", node.GetLabel(), "error", err)
			continue
		}
		slog.Error("node log on failure", "node", node.GetLabel(), "log", string(data))
	}
}
