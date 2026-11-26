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
	{"n2-standard-2", 2, 8, 0.0971},
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
	// Every VM has a boot disk (pd-balanced) holding only the OS, and one
	// local NVMe SSD of a fixed 375 GB for Docker and the run's files.
	bootDiskGB      = 10
	bootDiskGBMonth = 0.10
	localSSDGB      = 375
	localSSDGBMonth = 0.08
	hoursPerMonth   = 730

	// defaultNodeMachine runs one node, which needs about 1 vCPU and 2 GB
	// (the client image sets GOMEMLIMIT=1GiB).
	defaultNodeMachine = "n2-standard-2"

	// setupTime covers creating the VMs, installing Docker, copying the
	// sources, building the client image and collecting the results.
	setupTime = 10 * time.Minute
	// ttlMargin is added to the scenario timeout for the VMs' lifetime; GCP
	// deletes the VMs once it passes.
	ttlMargin  = 30 * time.Minute
	minTimeout = 10 * time.Minute
)

// Plan is what running scenarios on one VM per node, plus one VM for the
// driver, takes and costs.
type Plan struct {
	Estimate parser.Estimate
	// Hosts is the number of node VMs, RPCHosts of which run nodes that are
	// not validators on RPCMachine; the others run on NodeMachine.
	Hosts, RPCHosts                        int
	NodeMachine, RPCMachine, DriverMachine Machine
	// Timeout is the scenario timeout; GCP deletes the VMs after TTL.
	Timeout, TTL time.Duration
	ExpectedCost float64
	// WorstCaseCost is a hard limit: the VMs cannot outlive TTL.
	WorstCaseCost float64
}

// EstimateFiles sums the expected durations of the scenarios in files, which
// run one after the other on the same VMs, and takes the largest of their
// peaks and host counts.
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
		res.RPCHosts = max(res.RPCHosts, est.RPCHosts)
		res.Hosts = res.RPCHosts + max(res.Hosts-res.RPCHosts, est.Hosts-est.RPCHosts)
		res.Uncalibrated = append(res.Uncalibrated, est.Uncalibrated...)
	}
	return res, nil
}

// NewPlan sizes the VMs for the estimate. Empty machine types pick the
// defaults, the RPC nodes' being the other nodes', and a zero timeout picks
// twice the expected duration.
func NewPlan(est parser.Estimate, nodeMachineType, rpcMachineType, driverMachineType string,
	timeout time.Duration) (Plan, error) {
	if est.Hosts == 0 {
		return Plan{}, fmt.Errorf("the scenario starts no nodes")
	}
	if nodeMachineType == "" {
		nodeMachineType = defaultNodeMachine
	}
	if rpcMachineType == "" {
		rpcMachineType = nodeMachineType
	}
	if driverMachineType == "" {
		driverMachineType = driverMachineFor(est.Hosts)
	}
	p := Plan{Estimate: est, Hosts: est.Hosts, RPCHosts: est.RPCHosts, Timeout: timeout}
	var err error
	if p.NodeMachine, err = lookupMachine(nodeMachineType); err != nil {
		return Plan{}, err
	}
	if p.RPCMachine, err = lookupMachine(rpcMachineType); err != nil {
		return Plan{}, err
	}
	if p.DriverMachine, err = lookupMachine(driverMachineType); err != nil {
		return Plan{}, err
	}
	if p.Timeout <= 0 {
		p.Timeout = max(minTimeout, 2*est.Duration).Round(time.Minute)
	}
	p.TTL = p.Timeout + ttlMargin
	vms := float64(p.Hosts + 1)
	disks := vms * (bootDiskGB*bootDiskGBMonth + localSSDGB*localSSDGBMonth) / hoursPerMonth
	hourly := p.DriverMachine.Hourly + float64(p.Hosts-p.RPCHosts)*p.NodeMachine.Hourly +
		float64(p.RPCHosts)*p.RPCMachine.Hourly + disks
	p.ExpectedCost = (setupTime + est.Duration).Hours() * hourly
	p.WorstCaseCost = p.TTL.Hours() * hourly
	return p, nil
}

// VCPUs is the number of vCPUs of all the VMs.
func (p Plan) VCPUs() int {
	return p.DriverMachine.VCPUs + (p.Hosts-p.RPCHosts)*p.NodeMachine.VCPUs + p.RPCHosts*p.RPCMachine.VCPUs
}

// LocalSSDGB is the local SSD capacity of all the VMs.
func (p Plan) LocalSSDGB() int {
	return (p.Hosts + 1) * localSSDGB
}

// Fprint writes the plan for people to read.
func (p Plan) Fprint(w io.Writer) {
	fmt.Fprintf(w, "expected run:    %s (+%s setup)\n", p.Estimate.Duration, setupTime)
	fmt.Fprintf(w, "nodes:           %d, at most %d at once; %d apps\n", p.Hosts, p.Estimate.PeakNodes, p.Estimate.PeakApps)
	fmt.Fprintf(w, "VMs:             %d × %s ($%.4f/h) for the validators, %d × %s ($%.4f/h) for the other nodes,\n"+
		"                 %s ($%.4f/h) for the driver\n",
		p.Hosts-p.RPCHosts, p.NodeMachine.Name, p.NodeMachine.Hourly, p.RPCHosts, p.RPCMachine.Name,
		p.RPCMachine.Hourly, p.DriverMachine.Name, p.DriverMachine.Hourly)
	fmt.Fprintf(w, "timeout:         %s (VMs deleted after %s)\n", p.Timeout, p.TTL)
	fmt.Fprintf(w, "expected cost:   $%.2f\n", p.ExpectedCost)
	fmt.Fprintf(w, "worst-case cost: $%.2f\n", p.WorstCaseCost)
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

// driverMachineFor sizes the driver VM, which runs the load generators and
// monitors every node.
// ponytail: thresholds are guesses; calibrate them from runs of many nodes.
func driverMachineFor(hosts int) string {
	switch {
	case hosts <= 16:
		return "n2-standard-4"
	case hosts <= 48:
		return "n2-standard-8"
	default:
		return "n2-standard-16"
	}
}

func lookupMachine(name string) (Machine, error) {
	for _, m := range machines {
		if m.Name == name {
			return m, nil
		}
	}
	return Machine{}, fmt.Errorf("no price known for machine type %q", name)
}
