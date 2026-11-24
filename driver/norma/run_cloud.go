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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0xsoniclabs/norma/driver/cloud"
	"github.com/0xsoniclabs/norma/driver/docker"
	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

// Run with `go run ./driver/norma run-cloud --project <p> <scenario.yml>`

var runCloudCommand = cli.Command{
	Action:    runCloud,
	Name:      "run-cloud",
	Usage:     "runs scenarios on short-lived GCP VMs, one per node, that are deleted afterwards",
	UsageText: "norma run-cloud --project P [flags] <scenario-file-or-dir>",
	Flags: []cli.Flag{
		&projectFlag,
		&zoneFlag,
		&machineTypeFlag,
		&driverMachineTypeFlag,
		&maxCostFlag,
		&estimateTimeoutFlag,
		&evalLabel,
		&skipChecks,
		&tuiFlag,
		&outputDirectory,
		&skipReportRendering,
		&openReport,
	},
}

var (
	projectFlag = cli.StringFlag{
		Name:     "project",
		Usage:    "GCP project to create the VM in",
		Required: true,
	}
	zoneFlag = cli.StringFlag{
		Name:  "zone",
		Usage: "GCP zone to create the VM in",
		Value: "us-central1-a",
	}
	maxCostFlag = cli.Float64Flag{
		Name:  "max-cost",
		Usage: "refuse runs whose worst-case cost is higher, in USD",
		Value: 5,
	}
)

func runCloud(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		return fmt.Errorf("requires one scenario file or directory as argument")
	}
	tty := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
	if ctx.Bool(tuiFlag.Name) && !tty {
		return fmt.Errorf("--%s needs a terminal", tuiFlag.Name)
	}
	root, err := docker.ResolveBuildRoot(".")
	if err != nil {
		return err
	}
	plan, err := planCloudRun(ctx)
	if err != nil {
		return err
	}
	plan.Fprint(os.Stderr)

	var flags []string
	if label := ctx.String(evalLabel.Name); label != "" {
		flags = append(flags, "--"+evalLabel.Name, label)
	}
	for _, flag := range []*cli.BoolFlag{&skipChecks, &tuiFlag} {
		if ctx.Bool(flag.Name) {
			flags = append(flags, "--"+flag.Name)
		}
	}

	collected, runErr := cloud.Run(ctx.Context, cloud.Config{
		Project:   ctx.String(projectFlag.Name),
		Zone:      ctx.String(zoneFlag.Name),
		Plan:      plan,
		MaxCost:   ctx.Float64(maxCostFlag.Name),
		Scenario:  ctx.Args().First(),
		OutputDir: ctx.String(outputDirectory.Name),
		RunFlags:  flags,
		TTY:       tty,
		Root:      root,
		Stdin:     os.Stdin,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	})

	// The results are handled as `norma run` handles its own.
	var errs []error
	for _, dir := range collected {
		skipReport := ctx.Bool(skipReportRendering.Name) || ctx.Context.Err() != nil
		if err := finishCollectedRun(dir, skipReport, ctx.Bool(openReport.Name)); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", dir, err))
		}
	}
	return errors.Join(append([]error{runErr}, errs...)...)
}

// finishCollectedRun links and renders the output directory of a run that
// took place on a VM, as runScenario does for a local one.
func finishCollectedRun(dir string, skipReport, openReport bool) error {
	name := filepath.Base(dir)
	label := strings.TrimPrefix(name[:strings.LastIndex(name, "_")], "norma_data_")
	if err := linkLatest(dir, label); err != nil {
		return err
	}
	scenarioFile, err := scenarioFileIn(dir)
	if err != nil {
		return err
	}
	scenario, err := parser.ParseFile(scenarioFile)
	if err != nil {
		return err
	}
	renderReport(filepath.Join(dir, "measurements.csv"), dir, &scenario, scenarioFile, skipReport, openReport)
	return nil
}

// scenarioFileIn finds the copy of its scenario that a run keeps in its
// output directory.
func scenarioFileIn(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() && isYAML(entry.Name()) {
			return filepath.Join(dir, entry.Name()), nil
		}
	}
	return "", fmt.Errorf("no scenario file in %s", dir)
}
