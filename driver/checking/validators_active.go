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
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/monitoring"
	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/0xsoniclabs/norma/driver/rpc"
)

// defaultValidatorsActiveTimeout bounds how long Check waits for the queried
// node to report a validator set holding every running validator. The set is
// written at the epoch seal and read per node, so a node that has just started,
// or one that briefly lags the node which observed the seal, is behind for a
// moment without anything being wrong.
const defaultValidatorsActiveTimeout = 30 * time.Second

// validatorsActivePollInterval is the delay between convergence polls. Var so
// tests can shorten it.
var validatorsActivePollInterval = 500 * time.Millisecond

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
		timeout:             defaultValidatorsActiveTimeout,
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
//
// The requirement runs in one direction only — every running validator node
// must be in the set, not the reverse — but it does hold for all of them, so a
// scenario that leaves a validator running after taking it out of the set fails
// this check by design. An undelegated node that has not been stopped yet is
// the usual case; nodes expected to fail are the one exemption.
type validatorsActiveChecker struct {
	net driver.Network
	// getActiveValidators reads the current epoch's validator set.
	// Overridable for tests.
	getActiveValidators func(rpc.Client) ([]int, error)
	// timeout bounds convergence polling. Zero means a single attempt.
	timeout time.Duration
}

func (c *validatorsActiveChecker) Configure(CheckerConfig) Checker {
	return &validatorsActiveChecker{
		net:                 c.net,
		getActiveValidators: c.getActiveValidators,
		timeout:             c.timeout,
	}
}

func (c *validatorsActiveChecker) Check(ctx context.Context) error {
	// Which nodes are running is the driver's own bookkeeping rather than
	// network state, so these two failures are not something the network can
	// converge out of and are reported without polling.
	nodes := c.net.GetActiveNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no active nodes")
	}
	expected := expectedValidatorLabels(nodes)
	if len(expected) == 0 {
		return fmt.Errorf("no active validator nodes")
	}

	// The membership of a node in the set is on-chain state, so a disagreement
	// is either permanent or a lag of at most a moment. Poll until it is gone,
	// and only report what the deadline still finds.
	deadline := time.Now().Add(c.timeout)
	ticker := time.NewTicker(validatorsActivePollInterval)
	defer ticker.Stop()

	for {
		active, err := c.readActiveValidators(ctx, nodes)
		if err == nil {
			err = verifyAllActive(expected, active)
			if err == nil {
				slog.Info("validator set",
					"active_validators", active,
					"running_validators", expected,
				)
				return nil
			}
		}
		// A non-positive timeout leaves the deadline in the past, which makes
		// this the single attempt such a checker is configured for.
		if !time.Now().Before(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// readActiveValidators reads the validator set of the epoch currently being
// built from the first node that answers.
func (c *validatorsActiveChecker) readActiveValidators(
	ctx context.Context, nodes []driver.Node,
) ([]int, error) {
	client, err := dialFirstReachable(ctx, nodes)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return c.getActiveValidators(client)
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
