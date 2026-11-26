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

package controller

import (
	"context"
	"errors"
	"iter"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestSubmit_SendsTheTransactionUntilTheNetworkDecides_WhenItIsRejected(t *testing.T) {
	full := errors.New("txpool is full")
	tests := map[string]struct {
		answers []error
		taken   bool
	}{
		"accepted at once":    {answers: []error{nil}, taken: true},
		"pool full twice":     {answers: []error{full, full, nil}, taken: true},
		"already in the pool": {answers: []error{full, errors.New("already known")}, taken: true},
		"already on chain":    {answers: []error{errors.New("nonce too low")}, taken: true},
		"bundle processed": {
			answers: []error{errors.New("bundle has already been processed recently")},
			taken:   true,
		},
		"bundle not executable": {answers: []error{full, errors.New("bundle is not executable")}},
		"bundle invalid":        {answers: []error{errors.New("invalid bundle transaction\nroot")}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				network := driver.NewMockNetwork(gomock.NewController(t))
				tx := types.NewTx(&types.LegacyTx{Nonce: 7})
				for _, answer := range test.answers {
					network.EXPECT().SendTransaction(gomock.Any(), tx, gomock.Any()).Return(answer)
				}
				require.Equal(t, test.taken, submit(t.Context(), network, tx, driver.TransactionSource{}))
			})
		})
	}
}

func TestSubmit_GivesUp_WhenTheContextEndsWhileRetrying(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		network := driver.NewMockNetwork(gomock.NewController(t))
		ctx, cancel := context.WithCancel(t.Context())
		network.EXPECT().SendTransaction(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(context.Context, *types.Transaction, driver.TransactionSource) error {
				cancel()
				return errors.New("txpool is full")
			})
		require.False(t, submit(ctx, network, types.NewTx(&types.LegacyTx{}), driver.TransactionSource{}))
	})
}

func TestRunGeneratorLoop_SubmitsNoncesWithoutGaps_WhenTheNetworkRejectsTransactions(t *testing.T) {
	tests := map[string]struct {
		trigger bool
	}{
		"saturate": {trigger: false},
		"paced":    {trigger: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				user := app.NewMockUser(ctrl)
				user.EXPECT().Transactions().Return(counting())
				ctx, cancel := context.WithCancel(t.Context())

				var calls int
				var accepted []uint64
				network := driver.NewMockNetwork(ctrl)
				network.EXPECT().SendTransaction(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().
					DoAndReturn(func(_ context.Context, tx *types.Transaction, _ driver.TransactionSource) error {
						calls++
						if calls%3 != 0 {
							return errors.New("txpool is full")
						}
						accepted = append(accepted, tx.Nonce())
						if len(accepted) == 10 {
							cancel()
						}
						return nil
					})

				var trigger chan struct{}
				if test.trigger {
					trigger = make(chan struct{})
					go func() {
						for {
							select {
							case trigger <- struct{}{}:
							case <-ctx.Done():
								return
							}
						}
					}()
				}

				var count atomic.Uint64
				runGeneratorLoop(ctx, user, driver.TransactionSource{}, trigger, network, &count)

				require.Equal(t, []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, accepted)
				require.Equal(t, uint64(10), count.Load())
			})
		})
	}
}

// counting returns transactions with increasing nonces, starting at 0.
func counting() iter.Seq2[*types.Transaction, error] {
	return func(yield func(*types.Transaction, error) bool) {
		for nonce := uint64(0); yield(types.NewTx(&types.LegacyTx{Nonce: nonce}), nil); nonce++ {
		}
	}
}
