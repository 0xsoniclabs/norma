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
	"os"

	"github.com/0xsoniclabs/norma/driver/cloud"
	"github.com/urfave/cli/v2"
)

// Run with `go run ./driver/norma estimate <scenario.yml>`

var estimateCommand = cli.Command{
	Action:    estimate,
	Name:      "estimate",
	Usage:     "estimates offline how long scenarios run and what they cost on a GCP VM",
	UsageText: "norma estimate [--machine-type T] [--driver-machine-type T] [--timeout D] <scenario-file-or-dir>",
	Flags: []cli.Flag{
		&machineTypeFlag,
		&driverMachineTypeFlag,
		&estimateTimeoutFlag,
	},
}

var (
	machineTypeFlag = cli.StringFlag{
		Name:  "machine-type",
		Usage: "GCP machine type of the node VMs, one per node",
		Value: "n2-standard-2",
	}
	driverMachineTypeFlag = cli.StringFlag{
		Name:  "driver-machine-type",
		Usage: "GCP machine type of the driver VM; by default sized by the number of nodes",
	}
	estimateTimeoutFlag = cli.DurationFlag{
		Name:  "timeout",
		Usage: "the timeout the scenario will run with; by default twice the expected duration",
	}
)

func estimate(ctx *cli.Context) error {
	if ctx.Args().Len() != 1 {
		return fmt.Errorf("requires one scenario file or directory as argument")
	}
	plan, err := planCloudRun(ctx)
	if err != nil {
		return err
	}
	plan.Fprint(os.Stderr)
	return nil
}

// planCloudRun estimates the scenarios named by the first argument for the
// machine type and timeout of the command's flags.
func planCloudRun(ctx *cli.Context) (cloud.Plan, error) {
	files, err := collectScenarioFiles(ctx.Args().First())
	if err != nil {
		return cloud.Plan{}, err
	}
	est, err := cloud.EstimateFiles(files)
	if err != nil {
		return cloud.Plan{}, err
	}
	return cloud.NewPlan(est, ctx.String(machineTypeFlag.Name), ctx.String(driverMachineTypeFlag.Name),
		ctx.Duration(estimateTimeoutFlag.Name))
}
