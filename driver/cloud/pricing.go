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

package cloud

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/0xsoniclabs/norma/driver/parser"
)

// Machine is a GCP machine type with its on-demand price.
type Machine struct {
	Name   string
	VCPUs  int
	MemGB  int
	Hourly float64
}

// ponytail: static us-central1 on-demand prices as of 2025-11; read them from
// the Cloud Billing Catalog API if they drift or other regions are needed.
var machines = []Machine{
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
	bootDiskGB      = 10
	bootDiskGBMonth = 0.10
	localSSDGB      = 375
	localSSDGBMonth = 0.08
	hoursPerMonth   = 730

	// Resources a node needs (the client image sets GOMEMLIMIT=1GiB) and
	// those the driver and its load generators need.
	nodeVCPUs, nodeMemGB     = 1, 2
	driverVCPUs, driverMemGB = 2, 4

	// setupTime covers booting the VM, installing Docker, copying the sources,
	// building the client image and collecting the results.
	setupTime = 10 * time.Minute
	// ttlMargin is added to the scenario timeout for the VM's lifetime; GCP
	// deletes the VM once it passes.
	ttlMargin  = 30 * time.Minute
	minTimeout = 10 * time.Minute
)

// Plan is what running scenarios on one VM takes and costs.
type Plan struct {
	Estimate     parser.Estimate
	Machine      Machine
	VCPUs, MemGB int
	// Timeout is the scenario timeout; GCP deletes the VM after TTL.
	Timeout, TTL time.Duration
	ExpectedCost float64
	// WorstCaseCost is a hard limit: the VM cannot outlive TTL.
	WorstCaseCost float64
}

// EstimateFiles sums the expected durations of the scenarios in files, which
// run one after the other, and takes the largest of their peaks.
func EstimateFiles(files []string) (parser.Estimate, error) {
	var res parser.Estimate
	for _, file := range files {
		scenario, err := parser.ParseFile(file)
		if err != nil {
			return parser.Estimate{}, fmt.Errorf("%s: %w", file, err)
		}
		est, err := scenario.Estimate()
		if err != nil {
			return parser.Estimate{}, fmt.Errorf("%s: %w", file, err)
		}
		res.Duration += est.Duration
		res.PeakNodes = max(res.PeakNodes, est.PeakNodes)
		res.PeakApps = max(res.PeakApps, est.PeakApps)
		res.Uncalibrated = append(res.Uncalibrated, est.Uncalibrated...)
	}
	return res, nil
}

// NewPlan picks the named machine type, or the smallest one that fits the
// estimate, and the timeout, or twice the expected duration when timeout is 0.
func NewPlan(est parser.Estimate, machineType string, timeout time.Duration) (Plan, error) {
	p := Plan{
		Estimate: est,
		VCPUs:    driverVCPUs + est.PeakNodes*nodeVCPUs,
		MemGB:    driverMemGB + est.PeakNodes*nodeMemGB,
		Timeout:  timeout,
	}
	m, err := pickMachine(machineType, p.VCPUs, p.MemGB)
	if err != nil {
		return Plan{}, err
	}
	p.Machine = m
	if p.Timeout <= 0 {
		p.Timeout = max(minTimeout, 2*est.Duration).Round(time.Minute)
	}
	p.TTL = p.Timeout + ttlMargin
	hourly := m.Hourly + (bootDiskGB*bootDiskGBMonth+localSSDGB*localSSDGBMonth)/hoursPerMonth
	p.ExpectedCost = (setupTime + est.Duration).Hours() * hourly
	p.WorstCaseCost = p.TTL.Hours() * hourly
	return p, nil
}

// Fprint writes the plan for people to read.
func (p Plan) Fprint(w io.Writer) {
	fmt.Fprintf(w, "expected run:    %s (+%s setup)\n", p.Estimate.Duration, setupTime)
	fmt.Fprintf(w, "peak:            %d nodes, %d apps (needs %d vCPU, %d GB)\n", p.Estimate.PeakNodes, p.Estimate.PeakApps, p.VCPUs, p.MemGB)
	fmt.Fprintf(w, "machine:         %s (%d vCPU, %d GB, $%.4f/h)\n", p.Machine.Name, p.Machine.VCPUs, p.Machine.MemGB, p.Machine.Hourly)
	fmt.Fprintf(w, "timeout:         %s (VM deleted after %s)\n", p.Timeout, p.TTL)
	fmt.Fprintf(w, "expected cost:   $%.2f\n", p.ExpectedCost)
	fmt.Fprintf(w, "worst-case cost: $%.2f\n", p.WorstCaseCost)
	if p.Machine.VCPUs < p.VCPUs || p.Machine.MemGB < p.MemGB {
		fmt.Fprintf(w, "WARNING:         %s is smaller than the scenario needs\n", p.Machine.Name)
	}
	if u := uncalibrated(p.Estimate); len(u) > 0 {
		fmt.Fprintf(w, "uncalibrated:    %s (their times are guesses)\n", strings.Join(u, ", "))
	}
}

func uncalibrated(est parser.Estimate) []string {
	seen := map[string]bool{}
	var res []string
	for _, name := range est.Uncalibrated {
		if !seen[name] {
			seen[name] = true
			res = append(res, name)
		}
	}
	return res
}

// pickMachine returns the named machine, or the smallest one that fits.
func pickMachine(name string, vcpus, memGB int) (Machine, error) {
	for _, m := range machines {
		if m.Name == name || (name == "" && m.VCPUs >= vcpus && m.MemGB >= memGB) {
			return m, nil
		}
	}
	if name != "" {
		return Machine{}, fmt.Errorf("no price known for machine type %q", name)
	}
	return Machine{}, fmt.Errorf("no single machine fits %d vCPU and %d GB; the scenario needs more than one host", vcpus, memGB)
}
