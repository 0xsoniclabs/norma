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

package executor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/checking"
	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/0xsoniclabs/norma/driver/steps"
)

// Run executes a scenario on the given network.
// Steps are executed one by one in order. The context can be used to abort
// execution, and a default timeout is enforced as a deadline.
func Run(
	ctx context.Context,
	network driver.Network,
	scenario *parser.Scenario,
	checks checking.Checks,
) error {
	return runWithObserver(
		ctx,
		network,
		scenario,
		checks,
		steps.NewValidatorRegistry(network),
		nil,
		nil,
	)
}

// RunAndCaptureEventExecution executes a scenario and
// returns wall-clock start/end intervals for every executed step.
// genesisValidatorIds maps node labels to their pre-assigned validator IDs
// (from genesis configuration); these nodes are started without on-chain
// registration.
func RunAndCaptureEventExecution(
	ctx context.Context,
	network driver.Network,
	scenario *parser.Scenario,
	checks checking.Checks,
	genesisValidatorIds map[string]int,
) ([]EventExecution, error) {
	executions := make([]EventExecution, 0, len(scenario.Steps))
	err := runWithObserver(
		ctx,
		network,
		scenario,
		checks,
		steps.NewValidatorRegistry(network),
		func(execution EventExecution) {
			executions = append(executions, execution)
		},
		genesisValidatorIds,
	)
	return executions, err
}

// A step is logged twice, under the operation's own name, once as it starts and
// once when it is done. These distinguish the two.
const (
	phaseStarted   = "started"
	phaseCompleted = "completed"
)

// DefaultScenarioTimeout is the maximum time a scenario is allowed to run
// before being aborted. It applies only when the caller passes a context
// without a deadline of its own; `norma run --timeout` sets one.
const DefaultScenarioTimeout = 10 * time.Minute

// run is the internal implementation, allowing injection of
// a ValidatorRegistry for testing.
func run(
	ctx context.Context,
	network driver.Network,
	scenario *parser.Scenario,
	checks checking.Checks,
	registry steps.ValidatorRegistry,
) error {
	return runWithObserver(
		ctx,
		network,
		scenario,
		checks,
		registry,
		nil,
		nil,
	)
}

func runWithObserver(
	ctx context.Context,
	network driver.Network,
	scenario *parser.Scenario,
	checks checking.Checks,
	registry steps.ValidatorRegistry,
	onStepExecuted func(EventExecution),
	genesisValidatorIds map[string]int,
) error {
	if err := scenario.Check(); err != nil {
		return err
	}

	// A caller that set its own deadline has already decided how long the
	// scenario may run.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultScenarioTimeout)
		defer cancel()
	}

	env := steps.NewEnv(network, checks, registry)
	for label, id := range genesisValidatorIds {
		env.ValidatorIds[label] = id
	}

	for i, step := range scenario.Steps {
		name, target := step.Name(), steps.TargetOf(step)
		select {
		case <-ctx.Done():
			slog.Warn("scenario aborted", "step", i+1, "reason", ctx.Err())
			return fmt.Errorf("scenario aborted at step %d (%s): %w", i+1, name, ctx.Err())
		default:
		}

		slog.Info(name,
			"step", i+1,
			"phase", phaseStarted,
			"identifier", target,
		)

		start := time.Now()
		err := step.Run(ctx, env)
		end := time.Now()
		if onStepExecuted != nil {
			onStepExecuted(EventExecution{
				Name:  formatStepExecutionName(i+1, step),
				Start: start,
				End:   end,
			})
		}

		if err != nil {
			slog.Error("step failed",
				"step", i+1,
				"function", name,
				"identifier", target,
				"error", err,
				"duration", end.Sub(start),
			)
			return fmt.Errorf("step %d (%s %s) failed: %w", i+1, name, target, err)
		}

		slog.Info(name,
			"step", i+1,
			"phase", phaseCompleted,
			"duration", end.Sub(start),
		)

		// Steps that leave the network expected to be healthy are confirmed
		// to keep it producing blocks before the next one starts.
		if stabilizing, ok := step.(steps.Stabilizing); ok && stabilizing.AwaitsBlockProduction() {
			if err := steps.WaitForBlockProduction(ctx, network); err != nil {
				return fmt.Errorf("network unstable after step %d (%s %s): %w", i+1, name, target, err)
			}
		}
	}

	slog.Info("scenario completed successfully")
	return nil
}

func formatStepExecutionName(stepNum int, step steps.Step) string {
	target := steps.TargetOf(step)
	if target == "" {
		return fmt.Sprintf("step %d: %s", stepNum, step.Name())
	}
	return fmt.Sprintf("step %d: %s %s", stepNum, step.Name(), target)
}
