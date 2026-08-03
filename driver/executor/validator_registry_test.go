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

func TestActivateValidators_DoesNothing_WhenNothingWasRegistered(t *testing.T) {
	ctrl := gomock.NewController(t)
	// Any call on the network would be an unexpected call on this mock.
	net := driver.NewMockNetwork(ctrl)

	registry := netBasedValidatorRegistry{net: net}
	require.NoError(t, registry.activateValidators(context.Background(), nil))
}

func TestActivateValidators_Fails_WhenEpochCannotBeSealed(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().AdvanceEpoch(gomock.Any(), 1).
		Return(fmt.Errorf("network is halted"))

	registry := netBasedValidatorRegistry{net: net}
	err := registry.activateValidators(context.Background(), []int{2, 3})
	require.ErrorContains(t, err, "[2 3]")
	require.ErrorContains(t, err, "network is halted")
}

func TestActivateValidators_Fails_WhenNoNodeIsReachable(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().AdvanceEpoch(gomock.Any(), 1).Return(nil)
	net.EXPECT().DialRandomRpc().Return(nil, fmt.Errorf("no nodes"))

	registry := netBasedValidatorRegistry{net: net}
	err := registry.activateValidators(context.Background(), []int{2})
	require.ErrorContains(t, err, "no nodes")
}

func TestActivateValidators_Succeeds_WhenValidatorsAreInTheActiveSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	client := sfcClientReporting(t, ctrl, 7, []int64{1, 2, 3})
	net.EXPECT().AdvanceEpoch(gomock.Any(), 1).Return(nil)
	net.EXPECT().DialRandomRpc().Return(client, nil)
	client.EXPECT().Close()

	registry := netBasedValidatorRegistry{net: net}
	require.NoError(t,
		registry.activateValidators(context.Background(), []int{2, 3}))
}

func TestActivateValidators_Fails_WhenAValidatorNeverJoinsTheSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	// Validator 3 is missing from the sealed epoch's validator set.
	client := sfcClientReporting(t, ctrl, 7, []int64{1, 2})
	net.EXPECT().AdvanceEpoch(gomock.Any(), 1).Return(nil)
	net.EXPECT().DialRandomRpc().Return(client, nil)
	client.EXPECT().Close()

	// Negative, so the deadline is unambiguously in the past after the first
	// poll and the test does not depend on the clock advancing.
	original := validatorActivationTimeout
	validatorActivationTimeout = -1
	defer func() { validatorActivationTimeout = original }()

	registry := netBasedValidatorRegistry{net: net}
	err := registry.activateValidators(context.Background(), []int{2, 3})
	require.ErrorContains(t, err, "validators [3] did not join")
	require.ErrorContains(t, err, "active validators are [1 2]")
}

// sfcClientReporting returns an RPC client mock that answers SFC
// currentEpoch() and getEpochValidatorIDs() calls with the given values, so
// that network.GetActiveValidatorIDs sees the described validator set.
func sfcClientReporting(
	t *testing.T, ctrl *gomock.Controller, epoch int64, validators []int64,
) *rpc.MockClient {
	t.Helper()

	sfcAbi, err := sfc100.ContractMetaData.GetAbi()
	require.NoError(t, err)

	currentEpoch := sfcAbi.Methods["currentEpoch"]
	epochValidators := sfcAbi.Methods["getEpochValidatorIDs"]

	epochOut, err := currentEpoch.Outputs.Pack(big.NewInt(epoch))
	require.NoError(t, err)

	ids := make([]*big.Int, 0, len(validators))
	for _, v := range validators {
		ids = append(ids, big.NewInt(v))
	}
	validatorsOut, err := epochValidators.Outputs.Pack(ids)
	require.NoError(t, err)

	client := rpc.NewMockClient(ctrl)
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
				return validatorsOut, nil
			}
			return nil, fmt.Errorf("unexpected contract call")
		}).AnyTimes()
	return client
}
