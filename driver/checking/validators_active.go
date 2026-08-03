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

package checking

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/0xsoniclabs/norma/driver/rpc"
)

func init() {
	RegisterNetworkCheck("validatorsActive",
		func(net driver.Network, _ *monitoring.Monitor) Checker {
			return newValidatorsActiveChecker(net)
		})
}

func newValidatorsActiveChecker(net driver.Network) *validatorsActiveChecker {
	return &validatorsActiveChecker{
		net:                 net,
		getActiveValidators: network.GetActiveValidatorIDs,
	}
}

// validatorsActiveChecker verifies that every running validator node really
// takes part in consensus, by requiring its validator id to appear in the
// validator set of the epoch currently being built.
//
// Registering a validator in the SFC contract does not make it one: validator
// sets are per-epoch, so a validator created mid-epoch carries no stake weight
// until that epoch is sealed. A node missing from the set is an observer that
// merely looks like a validator, and the remaining validators carry its share
// of the consensus load.
//
// Membership is checked rather than observed event emission, which is a flaky
// proxy: a validator that recently joined, or one whose empty events are
// suppressed by the event throttler, is a full member yet emits rarely.
type validatorsActiveChecker struct {
	net driver.Network
	// getActiveValidators reads the current epoch's validator set.
	// Overridable for tests.
	getActiveValidators func(rpc.Client) ([]int, error)
}

func (c *validatorsActiveChecker) Configure(CheckerConfig) Checker {
	return c
}

func (c *validatorsActiveChecker) Check(ctx context.Context) error {
	nodes := c.net.GetActiveNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no active nodes")
	}

	expected := expectedValidatorLabels(nodes)
	if len(expected) == 0 {
		return fmt.Errorf("no active validator nodes")
	}

	client, err := dialFirstReachable(ctx, nodes)
	if err != nil {
		return err
	}
	defer client.Close()

	active, err := c.getActiveValidators(client)
	if err != nil {
		return err
	}
	slog.Info("validator set",
		"active_validators", active,
		"running_validators", expected,
	)

	return verifyAllActive(expected, active)
}

// expectedValidatorLabels maps the validator id of every running validator
// node to its label. Nodes expected to fail are skipped, since they may
// legitimately have left the validator set already.
func expectedValidatorLabels(nodes []driver.Node) map[int]string {
	labels := make(map[int]string, len(nodes))
	for _, n := range nodes {
		if n.IsExpectedFailure() {
			continue
		}
		if id := n.GetValidatorId(); id != nil {
			labels[*id] = n.GetLabel()
		}
	}
	return labels
}

// verifyAllActive fails when a running validator node is absent from the
// current epoch's validator set.
func verifyAllActive(expected map[int]string, active []int) error {
	missing := make([]string, 0, len(expected))
	for id, label := range expected {
		if !slices.Contains(active, id) {
			missing = append(missing, fmt.Sprintf("%s (validator %d)", label, id))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	slices.Sort(missing)
	return fmt.Errorf(
		"%d of %d running validator nodes are not in the validator set and "+
			"so take no part in consensus: %s; the active set is %v",
		len(missing), len(expected), strings.Join(missing, ", "), active,
	)
}
