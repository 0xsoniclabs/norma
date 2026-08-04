// Copyright 2026 Fantom Foundation
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

package app

import (
	"math/big"
	"testing"

	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/mock/gomock"
)

// newDeployableGenerator builds a generator whose deployment needs nothing but the
// accounts of its users.
func newDeployableGenerator() *txGenerator {
	target := common.Address{1}
	g := &txGenerator{accountFactory: NewAccountFactory(0, 0)}
	g.onSuccess = func(int, *Account) (txPayload, error) {
		return txPayload{to: &target, data: []byte{1, 2, 3, 4}, gasLimit: 50_000}, nil
	}
	return g
}

func deploymentContext(t *testing.T, numUsers int) (AppContext, *rpc.MockClient) {
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)
	client.EXPECT().ChainID(gomock.Any()).Return(big.NewInt(0xFA), nil).AnyTimes()
	client.EXPECT().PendingNonceAt(gomock.Any(), gomock.Any()).Return(uint64(0), nil).AnyTimes()

	ctxt := NewMockAppContext(ctrl)
	ctxt.EXPECT().GetClient().Return(client).AnyTimes()
	ctxt.EXPECT().GetRules().Return(opera.FakeNetRules(opera.GetSonicUpgrades()), nil).AnyTimes()
	ctxt.EXPECT().FundAccounts(gomock.Len(numUsers), gomock.Any()).Return(nil).AnyTimes()
	return ctxt, client
}

func TestTxGenerator_DeployWithoutUsersProducesNoOutcomeNeedingOne(t *testing.T) {
	// An application may be created without users; the generator must not try to
	// measure the cost of a call it has no account to send from.
	g := newDeployableGenerator()
	ctxt, _ := deploymentContext(t, 0)

	if err := g.Deploy(ctxt, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, outcome := range g.outcomes {
		if outcome == Failed {
			t.Error("a generator without users must not offer the failed outcome")
		}
	}
}

func TestTxGenerator_OffersFailedOnlyWhenTheCallCanRunOutOfGas(t *testing.T) {
	tests := map[string]struct {
		estimate uint64
		want     bool
	}{
		"call costs more than its own minimum": {estimate: 50_000, want: true},
		"call is paid for by its minimum":      {estimate: 21_000, want: false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			g := newDeployableGenerator()
			ctxt, client := deploymentContext(t, 1)
			client.EXPECT().EstimateGas(gomock.Any(), gomock.Any()).Return(test.estimate, nil)

			if err := g.Deploy(ctxt, 1); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			offered := false
			for _, outcome := range g.outcomes {
				if outcome == Failed {
					offered = true
				}
			}
			if offered != test.want {
				t.Errorf("expected the failed outcome to be offered=%v, got %v", test.want, offered)
			}
		})
	}
}
