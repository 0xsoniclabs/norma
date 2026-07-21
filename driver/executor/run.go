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
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/checking"
	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/0xsoniclabs/norma/genesis"
	"golang.org/x/sync/errgroup"
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
		&netBasedValidatorRegistry{net: network},
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
		&netBasedValidatorRegistry{net: network},
		func(execution EventExecution) {
			executions = append(executions, execution)
		},
		genesisValidatorIds,
	)
	return executions, err
}

// defaultScenarioTimeout is the maximum time a scenario is
// allowed to run before being aborted.
const defaultScenarioTimeout = 10 * time.Minute

// run is the internal implementation, allowing injection of
// a validatorRegistry for testing.
func run(
	ctx context.Context,
	network driver.Network,
	scenario *parser.Scenario,
	checks checking.Checks,
	registry validatorRegistry,
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
	registry validatorRegistry,
	onStepExecuted func(EventExecution),
	genesisValidatorIds map[string]int,
) error {
	if err := scenario.Check(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, defaultScenarioTimeout)
	defer cancel()

	state := &runState{
		nodes:        make(map[string]driver.Node),
		apps:         make(map[string]driver.Application),
		nodeHistory:  make(map[string]bool),
		validatorIds: make(map[string]int),
	}
	for label, id := range genesisValidatorIds {
		state.validatorIds[label] = id
	}

	for i, step := range scenario.Steps {
		select {
		case <-ctx.Done():
			slog.Warn("scenario aborted", "step", i+1, "reason", ctx.Err())
			return fmt.Errorf("scenario aborted at step %d (%s): %w", i+1, step.Function, ctx.Err())
		default:
		}

		slog.Info("executing step",
			"step", i+1,
			"function", step.Function,
			"identifier", step.Identifier,
		)

		start := time.Now()
		err := executeStep(ctx, &step, network, checks, registry, state)
		end := time.Now()
		if onStepExecuted != nil {
			onStepExecuted(EventExecution{
				Name:  formatStepExecutionName(i+1, &step),
				Start: start,
				End:   end,
			})
		}

		if err != nil {
			slog.Error("step failed",
				"step", i+1,
				"function", step.Function,
				"identifier", step.Identifier,
				"error", err,
				"duration", end.Sub(start),
			)
			return fmt.Errorf("step %d (%s %s) failed: %w", i+1, step.Function, step.Identifier, err)
		}

		slog.Info("step completed",
			"step", i+1,
			"function", step.Function,
			"duration", end.Sub(start),
		)

		// Wait for block production after steps that actively modify the
		// network and expect it to remain healthy. Skip for steps that can
		// legitimately leave the network stalled (stopNode, failing startNode)
		// or that don't affect network state (waitFor, checks).
		if requiresBlockProductionCheck(step) {
			if err := waitForBlockProduction(ctx, network); err != nil {
				return fmt.Errorf("network unstable after step %d (%s %s): %w", i+1, step.Function, step.Identifier, err)
			}
		}
	}

	slog.Info("scenario completed successfully")
	return nil
}

func formatStepExecutionName(stepNum int, step *parser.Step) string {
	if step.Identifier == "" {
		return fmt.Sprintf("step %d: %s", stepNum, step.Function)
	}
	return fmt.Sprintf("step %d: %s %s", stepNum, step.Function, step.Identifier)
}

// runState tracks runtime state during execution.
type runState struct {
	// nodes maps node identifiers to active node instances.
	nodes map[string]driver.Node
	// apps maps app identifiers to active application instances.
	apps map[string]driver.Application
	// nodeHistory tracks names that have been started before (for rejoin detection).
	nodeHistory map[string]bool
	// validatorIds preserves validator IDs for nodes that were stopped,
	// so they can be reused on rejoin.
	validatorIds map[string]int
}

// executeStep dispatches a single step to the appropriate handler.
func executeStep(
	ctx context.Context,
	step *parser.Step,
	net driver.Network,
	checks checking.Checks,
	registry validatorRegistry,
	state *runState,
) error {
	switch step.Function {
	case parser.FuncStartNode:
		return execStartNode(ctx, step, net, registry, state)
	case parser.FuncStopNode:
		return execStopNode(ctx, step, net, state)
	case parser.FuncUndelegate:
		return execUndelegate(ctx, step, net, registry, state)
	case parser.FuncRunApp:
		return execRunApp(ctx, step, net, state)
	case parser.FuncStopApp:
		return execStopApp(step, state)
	case parser.FuncUpdateRules:
		return execUpdateRules(ctx, step, net)
	case parser.FuncAdvanceEpoch:
		if err := waitForBlockProduction(ctx, net); err != nil {
			return err
		}
		if err := net.AdvanceEpoch(ctx, 1); err != nil {
			return err
		}
		return waitForBlockProduction(ctx, net)
	case parser.FuncWaitForEpoch:
		return net.WaitForEpochChange(ctx)
	case parser.FuncChecks:
		for i, spec := range step.SubChecks {
			checkerName, ok := checkFunctionToCheckerName[spec.Function]
			if !ok {
				return fmt.Errorf("unknown check function: %q", spec.Function)
			}
			c := spec
			if err := execCheck(ctx, checkerName, &c, checks); err != nil {
				return fmt.Errorf("check %d (%s): %w", i+1, spec.Function, err)
			}
		}
		return nil
	case parser.FuncWaitFor:
		slog.Info("waiting", "duration", step.Duration)
		select {
		case <-time.After(step.Duration):
			return nil
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during waitFor: %w", ctx.Err())
		}
	default:
		return fmt.Errorf("unknown step function: %q", step.Function)
	}
}

// execStartNode creates a node. If the same name was previously started and
// stopped, this is treated as a rejoin (no new validator registration).
func execStartNode(
	ctx context.Context,
	step *parser.Step,
	net driver.Network,
	registry validatorRegistry,
	state *runState,
) error {
	name := step.Identifier
	isRejoin := state.nodeHistory[name]
	isValidator := step.NodeType == "validator"

	image := driver.DefaultClientDockerImageName
	if step.ImageName != "" {
		image = step.ImageName
	}

	instances := 1
	if step.Instances != nil {
		instances = *step.Instances
	}

	// Get the current block height from an existing node before starting new ones.
	// We'll wait for new nodes to reach this height before proceeding.
	targetBlock, err := getNetworkBlockHeight(ctx, net)
	if err != nil {
		if !errors.Is(err, driver.ErrEmptyNetwork) {
			slog.Warn("failed to get network block height; node sync target defaults to 0", "error", err)
		} else {
			targetBlock = 0
		}
	}

	type nodeResult struct {
		instanceName string
		node         driver.Node
		validatorId  *int
	}
	results := make([]nodeResult, instances)

	// Pre-compute instance names and register validators sequentially.
	// Registration involves on-chain reads (LastValidatorID) that are not
	// safe for concurrent use, so we do this before the parallel fan-out.
	instanceNames := make([]string, instances)
	validatorIds := make([]*int, instances)
	for instance := range instances {
		instanceName := name
		if instances > 1 {
			instanceName = fmt.Sprintf("%s-%d", name, instance)
		}
		instanceNames[instance] = instanceName

		if isValidator {
			var stakeAmount uint64
			if step.Stake != nil {
				stakeAmount = *step.Stake
			}
			if id, ok := state.validatorIds[instanceName]; ok {
				// Reuse existing ID (genesis validator or rejoin).
				validatorIds[instance] = &id
			} else if isRejoin {
				return fmt.Errorf("validator %s is rejoining but has no preserved validator ID",
					instanceName,
				)
			} else {
				id, err := registry.registerNewValidator(ctx, stakeAmount)
				if err != nil {
					return fmt.Errorf("failed to register validator %s: %w",
						instanceName, err,
					)
				}
				validatorIds[instance] = &id
			}
		}
	}

	// Create nodes in parallel — all on-chain state has been settled above.
	g, _ := errgroup.WithContext(ctx)
	for instance := range instances {
		instance := instance
		g.Go(func() error {
			node, err := net.CreateNode(&driver.NodeConfig{
				Name:           instanceNames[instance],
				Failing:        step.Failing,
				Image:          image,
				Validator:      isValidator,
				ValidatorId:    validatorIds[instance],
				DataVolume:     dataVolumePtr(step.DataVolume),
				ExtraArguments: step.ExtraArguments,
			})
			if err != nil {
				return fmt.Errorf(
					"failed to create node %s: %w",
					instanceNames[instance], err,
				)
			}

			results[instance] = nodeResult{
				instanceNames[instance], node, validatorIds[instance],
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return fmt.Errorf("failed to create nodes for %s: %w", name, err)
	}

	var newNodes []driver.Node
	for _, r := range results {
		state.nodes[r.instanceName] = r.node
		state.nodeHistory[r.instanceName] = true
		if r.validatorId != nil {
			state.validatorIds[r.instanceName] = *r.validatorId
		}
		newNodes = append(newNodes, r.node)
	}

	// Also mark the base name in history for single-instance nodes.
	state.nodeHistory[name] = true

	// Wait for newly created nodes to sync to the network's block height.
	// Skip sync wait for nodes expected to fail (they may never reach the target).
	if !step.Failing {
		for _, node := range newNodes {
			slog.Info("waiting for node to sync", "node", node.GetLabel(), "target_block", targetBlock)
			if err := waitForNodeSync(ctx, node, targetBlock+1); err != nil {
				slog.Warn("node sync wait failed", "node", node.GetLabel(), "error", err)
			}
		}
	}

	return nil
}

// getNetworkBlockHeight returns the current block height from an existing network node.
func getNetworkBlockHeight(ctx context.Context, net driver.Network) (uint64, error) {
	client, err := net.DialRandomRpc()
	if err != nil {
		return 0, err
	}
	defer client.Close()
	return client.BlockNumber(ctx)
}

// waitForNodeSync waits until the given node has synced to at least the target block height.
func waitForNodeSync(ctx context.Context, node driver.Node, targetBlock uint64) error {
	client, err := node.DialRpc(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	const syncTimeout = 60 * time.Second
	deadline := time.Now().Add(syncTimeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		block, err := client.BlockNumber(ctx)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if block >= targetBlock {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("node did not sync to block %d within %s", targetBlock, syncTimeout)
}

// execStopNode stops a node.
func execStopNode(
	ctx context.Context,
	step *parser.Step,
	net driver.Network,
	state *runState,
) error {
	base := step.Identifier
	prefix := base + "-"

	type stopTarget struct {
		name string
		node driver.Node
	}
	targets := make([]stopTarget, 0)
	for name, node := range state.nodes {
		if name == base {
			targets = append(targets, stopTarget{name: name, node: node})
		} else if strings.HasPrefix(name, prefix) {
			// Only match instances with a numeric suffix (e.g. "base-0",
			// "base-1"), not unrelated nodes that share the prefix
			// (e.g. "base-extra").
			suffix := name[len(prefix):]
			if _, err := strconv.Atoi(suffix); err == nil {
				targets = append(targets, stopTarget{
					name: name, node: node,
				})
			}
		}
	}

	if len(targets) == 0 {
		return fmt.Errorf("node %q not found in active nodes", base)
	}

	g, gctx := errgroup.WithContext(ctx)
	for _, target := range targets {
		target := target
		g.Go(func() error {
			if err := net.RemoveNode(target.node); err != nil {
				return fmt.Errorf("failed to remove node %s: %w", target.name, err)
			}
			if err := target.node.Stop(gctx); err != nil {
				return fmt.Errorf("failed to stop node %s: %w", target.name, err)
			}
			if err := target.node.Cleanup(gctx); err != nil {
				return fmt.Errorf("failed to cleanup node %s: %w", target.name, err)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	for _, target := range targets {
		delete(state.nodes, target.name)
	}

	return nil
}

// execUndelegate undelegates stake from one or more validator nodes.
func execUndelegate(
	ctx context.Context,
	step *parser.Step,
	net driver.Network,
	registry validatorRegistry,
	state *runState,
) error {
	for _, target := range step.UndelegateTargets {
		node, ok := state.nodes[target.Node]
		if !ok {
			node, ok = state.nodes[target.Node+"-0"]
			if !ok {
				return fmt.Errorf("node %q not found in active nodes", target.Node)
			} else {
				// Ensure it is truly a single-instance node. If
				// other numbered instances exist, require an
				// explicit instance name.
				if hasMultipleInstances(state, target.Node) {
					return fmt.Errorf(
						"node %q has multiple instances; use an explicit instance name (e.g. %q)",
						target.Node, target.Node+"-0",
					)
				}
			}
		}
		id := node.GetValidatorId()
		if id == nil {
			return fmt.Errorf("node %q is not a validator", target.Node)
		}
		var stakeAmount uint64
		if target.Stake != nil {
			stakeAmount = *target.Stake
		}
		// stakeAmount=0 → UnregisterValidatorNode queries self-stake on-chain
		if err := registry.unregisterValidator(ctx, *id, stakeAmount); err != nil {
			return fmt.Errorf("failed to unregister validator %s: %w", target.Node, err)
		}
	}
	return nil
}

// hasMultipleInstances reports whether state contains more than one
// numbered instance for the given base name (e.g. "name-0", "name-1").
func hasMultipleInstances(
	state *runState,
	baseName string,
) bool {
	prefix := baseName + "-"
	count := 0
	for key := range state.nodes {
		if len(key) > len(prefix) &&
			key[:len(prefix)] == prefix {
			count++
			if count > 1 {
				return true
			}
		}
	}
	return false
}

// execRunApp creates and starts an application.
func execRunApp(
	ctx context.Context,
	step *parser.Step,
	net driver.Network,
	state *runState,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	users := 1
	if step.Users != nil {
		users = *step.Users
	}

	app, err := net.CreateApplication(ctx, &driver.ApplicationConfig{
		Name:  step.Identifier,
		Type:  step.AppType,
		Rate:  step.Rate,
		Users: users,
	})
	if err != nil {
		return fmt.Errorf("failed to create application %s: %w", step.Identifier, err)
	}

	if err := app.Start(ctx); err != nil {
		return fmt.Errorf("failed to start application %s: %w", step.Identifier, err)
	}

	state.apps[step.Identifier] = app
	return nil
}

// execStopApp stops a running application.
func execStopApp(step *parser.Step, state *runState) error {
	app, ok := state.apps[step.Identifier]
	if !ok {
		return fmt.Errorf("application %q not found in active apps", step.Identifier)
	}

	if err := app.Stop(); err != nil {
		return fmt.Errorf("failed to stop application %s: %w", step.Identifier, err)
	}

	delete(state.apps, step.Identifier)
	return nil
}

// execUpdateRules applies network rule updates.
func execUpdateRules(ctx context.Context, step *parser.Step, net driver.Network) error {
	rules := driver.NetworkRules(step.Rules)
	if err := net.ApplyNetworkRules(ctx, rules); err != nil {
		return fmt.Errorf("failed to apply network rules: %w", err)
	}

	return nil
}

// checkFunctionToCheckerName maps check step functions to their checker names.
var checkFunctionToCheckerName = map[parser.StepFunction]string{
	parser.FuncCheckBlockGasRate:   "blockGasRate",
	parser.FuncCheckBlockHashes:    "blocksHashes",
	parser.FuncCheckBlockHeights:   "blockHeight",
	parser.FuncCheckBlocksHalted:   "blocksHalted",
	parser.FuncCheckBlocksProduced: "blocksRolling",
	parser.FuncCheckEventThrottled: "eventThrottled",
	parser.FuncCheckNetworkRules:   "networkRules",
	parser.FuncCheckFailingNodes:   "failingNodes",
}

// execCheck runs a named checker with configuration from the check spec.
func execCheck(ctx context.Context, checkerName string, spec *parser.CheckSpec, checks checking.Checks) error {
	if checks == nil {
		slog.Warn("checks skipped (no checker configured)", "check", checkerName)
		return nil
	}

	checker := checks.GetCheckerByName(checkerName)
	if checker == nil {
		return fmt.Errorf("checker %q not found", checkerName)
	}

	// Build configuration from step parameters.
	config := checking.CheckerConfig{}
	if spec.Failing {
		config["failing"] = true
	}
	if spec.Tolerance != nil {
		if spec.Function == parser.FuncCheckBlockHeights {
			config["slack"] = *spec.Tolerance
		} else {
			config["tolerance"] = *spec.Tolerance
		}
	}
	if spec.Duration != nil {
		// Forward the observation duration (in nanoseconds) to the checker,
		// which will actively wait for that long and then verify that block
		// production progressed during the observation window.
		config["duration"] = int64(*spec.Duration)
	}
	if spec.Ceiling != nil {
		config["ceiling"] = int(*spec.Ceiling)
	}
	if spec.Rules != (genesis.NetworkRulesPatch{}) {
		config["rules"] = spec.Rules
	}
	if len(spec.ThrottledNodes) > 0 {
		config["throttledNodes"] = spec.ThrottledNodes
	}

	if len(config) > 0 {
		checker = checking.NewFailingChecker(checker).Configure(config)
	}

	return checker.Check(ctx)
}

// dataVolumePtr returns a *string for a non-empty DataVolume, nil otherwise.
func dataVolumePtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// requiresBlockProductionCheck returns true for steps after which we should
// verify the network is still producing blocks.
func requiresBlockProductionCheck(step parser.Step) bool {
	switch step.Function {
	case parser.FuncStartNode:
		return !step.Failing
	case parser.FuncRunApp, parser.FuncUpdateRules, parser.FuncAdvanceEpoch:
		return true
	default:
		// stopNode, undelegate, waitFor, waitForEpoch, stopApp, checks — skip
		return false
	}
}

// waitForBlockProduction waits until the network produces a new block,
// confirming it is actively processing transactions after an epoch transition.
func waitForBlockProduction(ctx context.Context, net driver.Network) error {
	client, err := net.DialRandomRpc()
	if err != nil {
		// If we can't connect, log and proceed — the next step will fail
		// with a more descriptive error if the network is truly down.
		slog.Warn("skipping block production wait", "error", err)
		return nil
	}
	defer client.Close()

	baseline, err := client.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("failed to get block number: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for block production: %w", ctx.Err())
		default:
		}
		block, err := client.BlockNumber(ctx)
		if err != nil {
			return fmt.Errorf("failed to get block number: %w", err)
		}
		if block > baseline {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}
