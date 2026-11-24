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
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/urfave/cli/v2"
)

// Run with `go run ./driver/norma estimate <scenario.yml>`

var estimateCommand = cli.Command{
	Action:    estimate,
	Name:      "estimate",
	Usage:     "estimates offline how long a scenario runs and what it costs on a GCP VM",
	UsageText: "norma estimate [--machine-type T] [--timeout D] [--env] <scenario.yml>",
	Flags: []cli.Flag{
		&machineTypeFlag,
		&estimateTimeoutFlag,
		&envFlag,
	},
}

var (
	machineTypeFlag = cli.StringFlag{
		Name:  "machine-type",
		Usage: "GCP machine type; by default the smallest that fits the scenario",
	}
	estimateTimeoutFlag = cli.DurationFlag{
		Name:  "timeout",
		Usage: "the timeout the scenario will run with; by default twice the expected duration",
	}
	envFlag = cli.BoolFlag{
		Name:  "env",
		Usage: "also print the results as KEY=value lines on stdout, for scripts",
	}
)

type machine struct {
	name   string
	vcpus  int
	memGB  int
	hourly float64
}

// ponytail: static us-central1 on-demand prices as of 2025-11; read them from
// the Cloud Billing Catalog API if they drift or other regions are needed.
var machines = []machine{
	{"n2-standard-4", 4, 16, 0.1942},
	{"n2-standard-8", 8, 32, 0.3885},
	{"n2-standard-16", 16, 64, 0.7769},
	{"n2-standard-32", 32, 128, 1.5539},
	{"n2-standard-48", 48, 192, 2.3308},
	{"n2-standard-64", 64, 256, 3.1078},
	{"n2-standard-80", 80, 320, 3.8847},
	{"n2-standard-128", 128, 512, 6.2156},
}

const (
	// The boot disk (pd-balanced) holds only the OS; Docker and the run's
	// files are on one local NVMe SSD of a fixed 375 GB.
	diskGB          = 10
	diskGBMonthCost = 0.10
	localSSDGB      = 375
	localSSDGBMonth = 0.08
	hoursPerMonth   = 730

	// Resources a node needs (the client image sets GOMEMLIMIT=1GiB) and
	// those the driver and its load generators need.
	nodeVCPUs, nodeMemGB     = 1, 2
	driverVCPUs, driverMemGB = 2, 4

	// setupTime covers booting the VM, installing Docker, copying the sources,
	// building the client image and collecting the results.
	setupTime = 15 * time.Minute
	// ttlMargin is added to the scenario timeout for the VM's lifetime; GCP
	// deletes the VM once it passes.
	ttlMargin  = 30 * time.Minute
	minTimeout = 10 * time.Minute
)

func estimate(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		return fmt.Errorf("requires one scenario file as argument")
	}
	scenario, err := parser.ParseFile(ctx.Args().First())
	if err != nil {
		return err
	}
	est, err := scenario.Estimate()
	if err != nil {
		return err
	}

	vcpus := driverVCPUs + est.PeakNodes*nodeVCPUs
	memGB := driverMemGB + est.PeakNodes*nodeMemGB
	m, err := pickMachine(ctx.String(machineTypeFlag.Name), vcpus, memGB)
	if err != nil {
		return err
	}

	timeout := ctx.Duration(estimateTimeoutFlag.Name)
	if timeout <= 0 {
		timeout = max(minTimeout, 2*est.Duration).Round(time.Minute)
	}
	ttl := timeout + ttlMargin
	cost := func(d time.Duration) float64 {
		return d.Hours() * (m.hourly + (diskGB*diskGBMonthCost+localSSDGB*localSSDGBMonth)/hoursPerMonth)
	}
	expected, worst := cost(setupTime+est.Duration), cost(ttl)

	out := os.Stderr
	fmt.Fprintf(out, "scenario:        %s\n", scenario.Name)
	fmt.Fprintf(out, "expected run:    %s (+%s setup)\n", est.Duration, setupTime)
	fmt.Fprintf(out, "peak:            %d nodes, %d apps (needs %d vCPU, %d GB)\n", est.PeakNodes, est.PeakApps, vcpus, memGB)
	fmt.Fprintf(out, "machine:         %s (%d vCPU, %d GB, $%.4f/h)\n", m.name, m.vcpus, m.memGB, m.hourly)
	fmt.Fprintf(out, "timeout:         %s (VM deleted after %s)\n", timeout, ttl)
	fmt.Fprintf(out, "expected cost:   $%.2f\n", expected)
	fmt.Fprintf(out, "worst-case cost: $%.2f\n", worst)
	if m.vcpus < vcpus || m.memGB < memGB {
		fmt.Fprintf(out, "WARNING:         %s is smaller than the scenario needs\n", m.name)
	}
	if len(est.Uncalibrated) > 0 {
		fmt.Fprintf(out, "uncalibrated:    %s (their times are guesses)\n", strings.Join(est.Uncalibrated, ", "))
	}

	if ctx.Bool(envFlag.Name) {
		fmt.Printf("MACHINE_TYPE=%s\n", m.name)
		fmt.Printf("DISK_GB=%d\n", diskGB)
		fmt.Printf("TIMEOUT=%s\n", timeout)
		fmt.Printf("TTL_MINUTES=%d\n", int(math.Ceil(ttl.Minutes())))
		fmt.Printf("EXPECTED_SECONDS=%d\n", int((setupTime + est.Duration).Seconds()))
		fmt.Printf("EXPECTED_COST=%.2f\n", expected)
		fmt.Printf("WORST_CASE_COST=%.2f\n", worst)
	}
	return nil
}

// pickMachine returns the named machine, or the smallest one that fits.
func pickMachine(name string, vcpus, memGB int) (machine, error) {
	for _, m := range machines {
		if m.name == name || (name == "" && m.vcpus >= vcpus && m.memGB >= memGB) {
			return m, nil
		}
	}
	if name != "" {
		return machine{}, fmt.Errorf("no price known for machine type %q", name)
	}
	return machine{}, fmt.Errorf("no single machine fits %d vCPU and %d GB; the scenario needs more than one host", vcpus, memGB)
}
