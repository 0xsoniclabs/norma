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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/0xsoniclabs/norma/driver/checking"

	"github.com/0xsoniclabs/norma/analysis/report"
	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/executor"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	_ "github.com/0xsoniclabs/norma/driver/monitoring/app"
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
	skipChecks := ctx.Bool(skipChecks.Name)
	skipReportRendering := ctx.Bool(skipReportRendering.Name)
	openReport := ctx.Bool(openReport.Name)

	path := args.First()

	files, err := collectScenarioFiles(path)
	if err != nil {
		return fmt.Errorf("failed to collect scenario files: %w", err)
	}

	// When the argument is a directory, scenarios are collected recursively
	// from its subfolders. Print the folder name whenever execution moves
	// into a new subfolder so the output is easy to follow.
	info, statErr := os.Stat(path)
	printFolders := statErr == nil && info.IsDir()
	lastFolder := ""

	for _, file := range files {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if printFolders {
			if folder := filepath.Dir(file); folder != lastFolder {
				lastFolder = folder
				fmt.Printf("=== scenarios in %s ===\n", folder)
			}
		}
		label := ctx.String(evalLabel.Name)
		if label == "" {
			label = fmt.Sprintf("eval_%d", time.Now().Unix())
		}
		if err := runScenario(ctx.Context, file, outputDir, label, skipChecks, skipReportRendering, openReport); err != nil {
			return fmt.Errorf("failed to run scenario %q: %w", file, err)
		}
	}

	return nil
}

func runScenario(ctx context.Context, path, outputDir, label string, skipChecks, skipReportRendering, openReport bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// if not configured, default to /tmp/norma_data_<label>_<timestamp> else /configured/path/norma_data_<l>_<t>
	outputDir, err := os.MkdirTemp(outputDir, fmt.Sprintf("norma_data_%s_", label))
	if err != nil {
		return fmt.Errorf("couldn't create temp dir for output; %w", err)
	}

	slog.Info("reading scenario file", "path", path)
	parsed, err := parser.ParseFile(path)
	if err != nil {
		return fmt.Errorf("failed to parse scenario file: %w", err)
	}
	if err := parsed.Check(); err != nil {
		return err
	}
	scenario := &parsed

	slog.Info("starting evaluation", "label", label)
	slog.Info("running scenario", "path", path, "name", scenario.Name)

	scenarioFilePath := path
	if absPath, err := filepath.Abs(path); err == nil {
		scenarioFilePath = absPath
	}

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
	if err := os.WriteFile(filepath.Join(outputDir, filepath.Base(path)), data, 0644); err != nil {
		return err
	}

	// Log initial rules.
	fmt.Println(scenario.InitialRules.PrettyPrint()) // multi line print

	// Startup network. Genesis is configured from the first startNode step,
	// which must be a validator. The step is NOT removed — nodes are started
	// explicitly by the runner so they appear in the report timeline.
	validators, genesisIds, err := extractBootstrapValidators(scenario)
	if err != nil {
		return err
	}
	net, err := local.NewLocalNetwork(ctx, &driver.NetworkConfig{
		Validators:   validators,
		NetworkRules: scenario.InitialRules,
		OutputDir:    outputDir,
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
	var stepExecutions []executor.EventExecution
	defer func() {
		slog.Info("shutting down data monitor ...")
		if err := monitor.Shutdown(); err != nil {
			slog.Error("error during monitor shutdown", "error", err)
		}
		if err := appendScenarioStepTimings(
			monitor.GetMeasurementFileName(),
			label,
			stepExecutions,
		); err != nil {
			slog.Warn("failed to export scenario step execution timings", "error", err)
		}
		slog.Info("monitoring data was written", "output", outputDir)
		slog.Info("raw data was exported", "file", monitor.GetMeasurementFileName())

		if !skipReportRendering && ctx.Err() == nil {
			slog.Info("rendering summary report ...")
			if file, err := report.SingleEvalReport.Render(
				monitor.GetMeasurementFileName(),
				outputDir,
				scenario.Name,
				scenario.Description,
				scenarioFilePath,
			); err != nil {
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

	var checks map[string]checking.Checker
	if !skipChecks {
		checks = checking.InitNetworkChecks(net, monitor)
	}

	// Run the scenario.
	slog.Info("running scenario", "path", path)
	logger := startProgressLogger(monitor, net)
	defer logger.shutdown()
	stepExecutions, err = executor.RunAndCaptureEventExecution(
		ctx,
		net,
		scenario,
		checks,
		genesisIds,
	)
	if err != nil {
		dumpNodeLogs(ctx, net)
		return err
	}
	slog.Info("execution completed successfully")

	return nil
}

// extractBootstrapValidators reads only the first scenario step to configure
// the network genesis validator set. The first step must be startNode.
func extractBootstrapValidators(scenario *parser.Scenario) (driver.Validators, map[string]int, error) {
	if len(scenario.Steps) == 0 {
		return nil, nil, fmt.Errorf("scenario has no steps")
	}

	step := scenario.Steps[0]
	if step.Function != parser.FuncStartNode {
		return nil, nil, fmt.Errorf("first step must be %q, got %q", parser.FuncStartNode, step.Function)
	}
	if step.NodeType != "validator" {
		return nil, nil, fmt.Errorf("first startNode must be validator, got %q", step.NodeType)
	}

	instances := 1
	if step.Instances != nil {
		instances = *step.Instances
	}
	image := driver.ResolveClientImageName(step.ImageName)

	var stake uint64
	if step.Stake != nil {
		stake = *step.Stake
	}

	validators := driver.Validators{{
		Name:      step.Identifier,
		Instances: instances,
		ImageName: image,
		Stake:     stake,
	}}

	// Build the genesis label→validatorId map using the same naming convention
	// as execStartNode: no "-N" suffix for single-instance nodes.
	genesisIds := make(map[string]int)
	for j := range instances {
		label := step.Identifier
		if instances > 1 {
			label = fmt.Sprintf("%s-%d", step.Identifier, j)
		}
		genesisIds[label] = j + 1
	}

	return validators, genesisIds, nil
}

func openBrowser(s string) error {

	path, err := exec.LookPath("xdg-open")
	if err != nil {
		return fmt.Errorf("xdg-open not found: %w", err)
	}

	cmd := exec.Command(path, s)
	return cmd.Start()
}

// logDumpTimeout bounds each node log read, since StreamLog follows the
// container and never reaches EOF for a running node. Var so tests can shorten it.
var logDumpTimeout = 20 * time.Second

// dumpNodeLogs prints the logs of all active nodes to help diagnose failures.
func dumpNodeLogs(ctx context.Context, net driver.Network) {
	nodes := net.GetActiveNodes()
	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(node driver.Node) {
			defer wg.Done()

			logCtx, cancel := context.WithTimeout(ctx, logDumpTimeout)
			defer cancel()

			reader, err := node.StreamLog(logCtx)
			if err != nil {
				slog.Error("failed to stream log", "node", node.GetLabel(), "error", err)
				return
			}
			data, err := io.ReadAll(io.LimitReader(reader, 1<<20))
			_ = reader.Close()

			if len(data) > 0 {
				slog.Error("node log on failure", "node", node.GetLabel(), "log", string(data))
			}
			if err != nil && logCtx.Err() == nil {
				slog.Error("failed to read log", "node", node.GetLabel(), "error", err)
			}
		}(node)
	}
	wg.Wait()
}

func appendScenarioStepTimings(
	measurementFile string,
	label string,
	events []executor.EventExecution,
) (err error) {
	if len(events) == 0 {
		return nil
	}

	file, err := os.OpenFile(measurementFile, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()

	for _, e := range events {
		start := e.Start.UTC().UnixNano()
		duration := e.End.Sub(e.Start).Nanoseconds()
		record := monitoring.CsvRecord{
			Record: monitoring.Record{
				Network: "network",
				App:     e.Name,
				Time:    &start,
				Value:   fmt.Sprintf("%d", duration),
			},
			Metric: "ScenarioStepExecutionDuration",
			Run:    label,
		}

		if _, err := record.WriteTo(file); err != nil {
			return err
		}
	}

	return nil
}
