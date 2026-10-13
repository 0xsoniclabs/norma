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

package nodelifetime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/steps"
	"golang.org/x/sync/errgroup"
)

// StartNode creates a node, or several numbered instances of it. A name that
// was started and stopped before rejoins under its preserved validator id
// instead of registering a new one, and a node still tracked, as after
// killSonic and healDb, is restarted in its existing container.
type StartNode struct {
	steps.Node
	Type           string  `yaml:"type" help:"Node type: \"validator\", \"observer\" (the default) or \"rpc\"."`
	ImageName      string  `yaml:"imageName" help:"Docker image name to use for the node."`
	DataVolume     string  `yaml:"dataVolume" help:"Docker volume name to mount as the node data directory."`
	Stake          *uint64 `yaml:"stake" help:"Validator stake in S (uint64). Defaults to 5,000,000 S."`
	Instances      *int    `yaml:"instances" help:"Number of node instances to start."`
	Failing        bool    `yaml:"failing" help:"When true, the node is expected to diverge from the network: the hash, height and rules checks expect it to fall behind or disagree, and the step does not wait for it to sync or join the validator set."`
	ExtraArguments string  `yaml:"extraArguments" help:"Extra command line arguments for sonicd."`
}

func (*StartNode) Name() string { return "startNode" }

func (*StartNode) Help() string {
	return "Start a new network node (validator, observer, or rpc)."
}

// AwaitsBlockProduction leaves out nodes expected to fail, which may leave
// the network stalled.
func (s *StartNode) AwaitsBlockProduction() bool { return !s.Failing }

// IsValidator reports whether the node joins the validator set.
func (s *StartNode) IsValidator() bool { return s.Type == "validator" }

// InstanceCount is the number of nodes the step starts.
func (s *StartNode) InstanceCount() int {
	if s.Instances == nil {
		return 1
	}
	return *s.Instances
}

// InstanceName is the label of the given instance: the step's name for a
// single instance, the name with a numeric suffix otherwise.
func (s *StartNode) InstanceName(instance int) string {
	if s.InstanceCount() > 1 {
		return steps.InstanceLabel(s.Node.Node, instance)
	}
	return s.Node.Node
}

func (s *StartNode) Validate() error {
	errs := []error{s.Node.Validate()}

	nodeType := s.Type
	if nodeType == "" {
		nodeType = "observer"
	}
	switch nodeType {
	case "validator", "rpc", "observer":
	default:
		errs = append(errs, fmt.Errorf("type of node must be observer, rpc or validator, was set to %s", nodeType))
	}

	if s.Instances != nil && *s.Instances < 1 {
		errs = append(errs, fmt.Errorf("number of instances must be >= 1, got %d", *s.Instances))
	}

	return errors.Join(errs...)
}

func (s *StartNode) Run(ctx context.Context, env *steps.Env) error {
	name := s.Node.Node

	if existing, ok := env.Nodes[name]; ok {
		return s.restartInPlace(ctx, env, existing)
	}

	isRejoin := env.NodeHistory[name]
	instances := s.InstanceCount()

	image := driver.DefaultClientDockerImageName
	if s.ImageName != "" {
		image = s.ImageName
	}

	// The new nodes have to reach the network's current height before the
	// step completes.
	targetBlock, err := steps.NetworkBlockHeight(ctx, env.Net)
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

	// Registration involves on-chain reads (LastValidatorID) that are not
	// safe for concurrent use, so it runs before the parallel fan-out.
	instanceNames := make([]string, instances)
	validatorIds := make([]*int, instances)
	// Validators this step has to see in the validator set before it completes:
	// the ones it creates in the SFC contract, which only join once the epoch is
	// sealed, and the ones a rejoining node brings back, which are expected to
	// be in it already. Genesis validators are left out — they are in the set
	// from the first block on.
	mustBeActive := make([]int, 0, instances)
	for instance := range instances {
		instanceName := s.InstanceName(instance)
		instanceNames[instance] = instanceName

		if !s.IsValidator() {
			continue
		}
		var stakeAmount uint64
		if s.Stake != nil {
			stakeAmount = *s.Stake
		}
		if id, ok := env.ValidatorIds[instanceName]; ok {
			// Reuse existing ID (genesis validator or rejoin).
			validatorIds[instance] = &id
			if isRejoin {
				mustBeActive = append(mustBeActive, id)
			}
		} else if isRejoin {
			return fmt.Errorf("validator %s is rejoining but has no preserved validator ID",
				instanceName,
			)
		} else {
			id, err := env.Validators.RegisterNewValidator(ctx, stakeAmount)
			if err != nil {
				return fmt.Errorf("failed to register validator %s: %w",
					instanceName, err,
				)
			}
			validatorIds[instance] = &id
			mustBeActive = append(mustBeActive, id)
		}
	}

	g, _ := errgroup.WithContext(ctx)
	for instance := range instances {
		g.Go(func() error {
			node, err := env.Net.CreateNode(&driver.NodeConfig{
				Name:           instanceNames[instance],
				Failing:        s.Failing,
				Image:          image,
				Validator:      s.IsValidator(),
				ValidatorId:    validatorIds[instance],
				DataVolume:     dataVolumePtr(s.DataVolume),
				ExtraArguments: s.ExtraArguments,
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
		env.Nodes[r.instanceName] = r.node
		env.NodeHistory[r.instanceName] = true
		if r.validatorId != nil {
			env.ValidatorIds[r.instanceName] = *r.validatorId
		}
		newNodes = append(newNodes, r.node)
	}

	// Also mark the base name in history for single-instance nodes.
	env.NodeHistory[name] = true

	// Skip sync wait for nodes expected to fail (they may never reach the target).
	if !s.Failing {
		for _, node := range newNodes {
			slog.Info("waiting for node to sync", "node", node.GetLabel(), "target_block", targetBlock)
			if err := steps.WaitForNodeSync(ctx, node, targetBlock+1); err != nil {
				slog.Warn("node sync wait failed", "node", node.GetLabel(), "error", err)
			}
		}
	}

	// Activation happens only now that the nodes are up and synced: a validator
	// gains stake weight the moment it joins the set, so admitting it earlier
	// would deprive the network of that share of its online stake.
	//
	// Nodes expected to fail are left out, as they are for the sync wait above
	// and the between-step block production wait: a node that may never come
	// up must not be the subject of an assertion. They join at the next seal
	// the scenario performs, as they did before activation existed.
	if len(mustBeActive) > 0 && !s.Failing {
		if err := env.Validators.EnsureValidatorsActive(ctx, mustBeActive); err != nil {
			return fmt.Errorf("failed to activate validators for %s: %w", name, err)
		}
	}

	return nil
}

// dataVolumePtr returns a *string for a non-empty DataVolume, nil otherwise.
func dataVolumePtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
