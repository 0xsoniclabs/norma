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

package parser

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/0xsoniclabs/norma/driver/steps/wait"
	"github.com/0xsoniclabs/sonic/inter"
)

// stepTimes is the typical wall time of a step, including the wait for block
// production after it, taken from the `duration=` that runs log for every
// step. It is the calibration knob of Estimate.
var stepTimes = map[string]time.Duration{
	"startNode":    8 * time.Second,
	"runApp":       14 * time.Second,
	"advanceEpoch": 15 * time.Second,
	"checks":       5 * time.Second,
	"stopNode":     5 * time.Second,
	"stopApp":      time.Second,
}

const (
	// defaultStepTime is assumed for steps stepTimes has no data on.
	defaultStepTime = 30 * time.Second
	// blockTime is assumed for waitForBlocks; it depends on the load.
	blockTime = time.Second
)

// Estimate is what a scenario is expected to take to run.
type Estimate struct {
	Duration  time.Duration
	PeakNodes int
	PeakApps  int
	// Uncalibrated lists the steps whose time is a guess rather than measured.
	Uncalibrated []string
}

// Estimate walks the steps over a model of the network, as DryRun does, and
// sums their expected durations. It needs no network.
func (s *Scenario) Estimate() (Estimate, error) {
	var res Estimate
	epoch := time.Duration(inter.Timestamp(*s.InitialRules.Epochs.MaxEpochDuration))
	model := steps.NewModel()
	for i, step := range s.Steps {
		if err := step.UpdateModel(model); err != nil {
			return Estimate{}, fmt.Errorf("step %d (%s): %w", i+1, strings.TrimSpace(step.Name()+" "+steps.TargetOf(step)), err)
		}
		res.PeakNodes = max(res.PeakNodes, len(model.Nodes))
		res.PeakApps = max(res.PeakApps, len(model.Apps))

		switch step := step.(type) {
		case *wait.WaitFor:
			res.Duration += step.Duration
		case *wait.WaitForEpoch:
			res.Duration += epoch
		case *wait.WaitForBlocks:
			res.Duration += time.Duration(step.Blocks) * blockTime
			res.Uncalibrated = append(res.Uncalibrated, step.Name())
		default:
			d, ok := stepTimes[step.Name()]
			if !ok {
				d = defaultStepTime
				res.Uncalibrated = append(res.Uncalibrated, step.Name())
			}
			res.Duration += d
		}
	}
	slices.Sort(res.Uncalibrated)
	res.Uncalibrated = slices.Compact(res.Uncalibrated)
	return res, nil
}
