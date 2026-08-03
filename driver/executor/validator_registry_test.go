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
	"math/big"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/sonic/gossip/contract/sfc100"
	"github.com/ethereum/go-ethereum"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestEnsureValidatorsActive_DoesNothing_WhenNoValidatorIsGiven(t *testing.T) {
	ctrl := gomock.NewController(t)
	// Any call on the network would be an unexpected call on this mock.
	net := driver.NewMockNetwork(ctrl)

	registry := netBasedValidatorRegistry{net: net}
	require.NoError(t, registry.ensureValidatorsActive(context.Background(), nil))
}

func TestEnsureValidatorsActive_Fails_WhenNoNodeIsReachable(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	// Without a node to ask, there is nothing to seal an epoch for either.
	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes"))

	registry := netBasedValidatorRegistry{net: net}
	err := registry.ensureValidatorsActive(context.Background(), []int{2})
	require.ErrorContains(t, err, "no nodes")
}

// The case of a rejoining node, whose validator never left the set: reading it
// is enough, and sealing an epoch for it would cost the scenario an epoch it
// did not ask for. An AdvanceEpoch call would be unexpected on this mock.
func TestEnsureValidatorsActive_SealsNothing_WhenValidatorsAreAlreadyInTheSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	set := []int64{1, 2, 3}
	client := sfcClientReporting(t, ctrl, 7, &set)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(client, nil)

	registry := netBasedValidatorRegistry{net: net}
	require.NoError(t,
		registry.ensureValidatorsActive(context.Background(), []int{2, 3}))
}

func TestEnsureValidatorsActive_SealsAnEpoch_WhenAValidatorIsMissing(t *testing.T) {
	ctrl := gomock.NewController(t)
	// Validator 3 has been registered but not yet admitted to the set.
	set := []int64{1, 2}
	client := sfcClientReporting(t, ctrl, 7, &set)
	client.EXPECT().BlockNumber(gomock.Any()).
		DoAndReturn(growingBlockHeight()).AnyTimes()
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(client, nil).AnyTimes()
	net.EXPECT().AdvanceEpoch(gomock.Any(), 1).
		DoAndReturn(func(context.Context, int) error {
			set = append(set, 3)
			return nil
		})

	registry := netBasedValidatorRegistry{net: net}
	require.NoError(t,
		registry.ensureValidatorsActive(context.Background(), []int{2, 3}))
}

// Sealing an epoch means landing a transaction, so a network that is not
// producing blocks is reported as such instead of as a missing receipt.
func TestEnsureValidatorsActive_Fails_WhenNoBlockIsProduced(t *testing.T) {
	ctrl := gomock.NewController(t)
	set := []int64{1}
	client := sfcClientReporting(t, ctrl, 7, &set)
	client.EXPECT().BlockNumber(gomock.Any()).
		Return(uint64(0), fmt.Errorf("network is halted"))
	net := driver.NewMockNetwork(ctrl)
	// No AdvanceEpoch: the seal is never attempted.
	net.EXPECT().DialRandomRpc().Return(client, nil).AnyTimes()

	registry := netBasedValidatorRegistry{net: net}
	err := registry.ensureValidatorsActive(context.Background(), []int{2})
	require.ErrorContains(t, err, "no block produced to seal an epoch on")
	require.ErrorContains(t, err, "[2]")
	require.ErrorContains(t, err, "network is halted")
}

func TestEnsureValidatorsActive_Fails_WhenEpochCannotBeSealed(t *testing.T) {
	ctrl := gomock.NewController(t)
	set := []int64{1}
	client := sfcClientReporting(t, ctrl, 7, &set)
	client.EXPECT().BlockNumber(gomock.Any()).
		DoAndReturn(growingBlockHeight()).AnyTimes()
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(client, nil).AnyTimes()
	net.EXPECT().AdvanceEpoch(gomock.Any(), 1).
		Return(fmt.Errorf("epoch seal rejected"))

	registry := netBasedValidatorRegistry{net: net}
	err := registry.ensureValidatorsActive(context.Background(), []int{2, 3})
	require.ErrorContains(t, err, "[2 3]")
	require.ErrorContains(t, err, "epoch seal rejected")
}

// A validator that is missing and stays missing after the seal — undelegated,
// say — is reported instead of quietly running as an observer.
func TestEnsureValidatorsActive_Fails_WhenAValidatorNeverJoinsTheSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	set := []int64{1, 2}
	client := sfcClientReporting(t, ctrl, 7, &set)
	client.EXPECT().BlockNumber(gomock.Any()).
		DoAndReturn(growingBlockHeight()).AnyTimes()
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().DialRandomRpc().Return(client, nil).AnyTimes()
	net.EXPECT().AdvanceEpoch(gomock.Any(), 1).Return(nil)

	// Negative, so the deadline is unambiguously in the past after the first
	// poll and the test does not depend on the clock advancing.
	original := validatorActivationTimeout
	validatorActivationTimeout = -1
	t.Cleanup(func() { validatorActivationTimeout = original })

	registry := netBasedValidatorRegistry{net: net}
	err := registry.ensureValidatorsActive(context.Background(), []int{2, 3})
	require.ErrorContains(t, err, "validators [3] did not join")
	require.ErrorContains(t, err, "active validators are [1 2]")
}

func TestMissingValidators_ReportsWhatTheSetDoesNotHold(t *testing.T) {
	require.Empty(t, missingValidators([]int{2, 3}, []int{1, 2, 3}))
	require.Equal(t, []int{3}, missingValidators([]int{2, 3}, []int{1, 2}))
	require.Equal(t, []int{2, 3}, missingValidators([]int{2, 3}, nil))
}

// growingBlockHeight answers BlockNumber with an ever-growing height, which is
// what waitForBlockProduction waits for.
func growingBlockHeight() func(context.Context) (uint64, error) {
	height := uint64(0)
	return func(context.Context) (uint64, error) {
		height++
		return height, nil
	}
}

// sfcClientReporting returns an RPC client mock that answers SFC
// currentEpoch() and getEpochValidatorIDs() calls with the given values, so
// that network.GetActiveValidatorIDs sees the described validator set. The
// validator set is read through the pointer on every call, so a test can let an
// epoch seal change it.
func sfcClientReporting(
	t *testing.T, ctrl *gomock.Controller, epoch int64, validators *[]int64,
) *rpc.MockClient {
	t.Helper()

	sfcAbi, err := sfc100.ContractMetaData.GetAbi()
	require.NoError(t, err)

	currentEpoch := sfcAbi.Methods["currentEpoch"]
	epochValidators := sfcAbi.Methods["getEpochValidatorIDs"]

	epochOut, err := currentEpoch.Outputs.Pack(big.NewInt(epoch))
	require.NoError(t, err)

	client := rpc.NewMockClient(ctrl)
	// The set is read once per attempt, and every read closes its client.
	client.EXPECT().Close().MinTimes(1)
	client.EXPECT().CodeAt(gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]byte{1}, nil).AnyTimes()
	client.EXPECT().
		CallContract(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			_ context.Context, call ethereum.CallMsg, _ *big.Int,
		) ([]byte, error) {
			switch {
			case len(call.Data) >= 4 &&
				string(call.Data[:4]) == string(currentEpoch.ID):
				return epochOut, nil
			case len(call.Data) >= 4 &&
				string(call.Data[:4]) == string(epochValidators.ID):
				ids := make([]*big.Int, 0, len(*validators))
				for _, v := range *validators {
					ids = append(ids, big.NewInt(v))
				}
				return epochValidators.Outputs.Pack(ids)
			}
			return nil, fmt.Errorf("unexpected contract call")
		}).AnyTimes()
	return client
}
