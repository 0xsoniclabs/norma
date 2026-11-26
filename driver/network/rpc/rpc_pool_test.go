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

package rpc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRetryRpcReturnGracefully(t *testing.T) {
	t.Parallel()

	start := time.Now()
	txs := make(chan transactionWithSource)
	w := newWorker("test", "wrong", txs, nil, &observers{})

	time.Sleep(6 * time.Second)
	w.close()

	if got, want := time.Since(start).Seconds(), float64(5); got < want {
		t.Errorf("RPC should be attempted around 6s, was: %f < %f", got, want)
	}
}

func TestSendTransaction_ReturnsTheAnswerOfTheNode(t *testing.T) {
	pool := NewRpcWorkerPool(t.Context())
	rejected := errors.New("txpool is full")
	go func() {
		(<-pool.txs).result <- rejected
	}()

	var tx types.Transaction
	err := pool.SendTransaction(t.Context(), &tx, driver.TransactionSource{App: "test"})
	require.ErrorIs(t, err, rejected)
}

func TestSendTransaction_Returns_WhenThePoolIsClosed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := NewRpcWorkerPool(t.Context())
		done := make(chan error)
		go func() {
			var tx types.Transaction
			done <- pool.SendTransaction(t.Context(), &tx, driver.TransactionSource{App: "test"})
		}()
		synctest.Wait() // the sender waits for a worker

		require.NoError(t, pool.Close())
		require.ErrorIs(t, <-done, context.Canceled)
	})
}

func TestSendTransaction_ReturnsTheAnswerOfTheNode_WhenItsContextEndsAfterAWorkerTookIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := NewRpcWorkerPool(t.Context())
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error)
		go func() {
			var tx types.Transaction
			done <- pool.SendTransaction(ctx, &tx, driver.TransactionSource{App: "test"})
		}()
		taken := <-pool.txs
		cancel()
		synctest.Wait()
		taken.result <- nil
		require.NoError(t, <-done)
	})
}

func TestSendTransaction_Returns_WhenItsContextEndsBeforeAWorkerTookIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := NewRpcWorkerPool(t.Context())
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error)
		go func() {
			var tx types.Transaction
			done <- pool.SendTransaction(ctx, &tx, driver.TransactionSource{App: "test"})
		}()
		synctest.Wait() // no worker takes it
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
	})
}

func TestWorker_AnswersWithAnError_WhenItsNodeNeverAnswers(t *testing.T) {
	tests := map[string]struct {
		timeout time.Duration
		close   bool
	}{
		"submission times out": {timeout: 100 * time.Millisecond},
		"worker is closed":     {timeout: time.Hour, close: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			defer func(old time.Duration) { submitTimeout = old }(submitTimeout)
			submitTimeout = test.timeout

			received, release := make(chan struct{}, 1), make(chan struct{})
			node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- struct{}{}
				<-release
			}))
			defer node.Close()
			defer close(release)

			txs := make(chan transactionWithSource)
			w := newWorker("test", driver.URL(node.URL), txs, nil, &observers{})
			defer w.close()
			tx := transactionWithSource{tx: types.NewTx(&types.LegacyTx{}), result: make(chan error, 1)}
			txs <- tx
			<-received
			if test.close {
				go w.close()
			}
			select {
			case err := <-tx.result:
				require.Error(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("the worker kept waiting for the node")
			}
		})
	}
}

func TestCloseWorkerStartStop(t *testing.T) {
	txs := make(chan transactionWithSource)
	w := newWorker("test", "wrong", txs, nil, &observers{})
	w.close()
}

func TestCloseWorkerGroupStartStop(t *testing.T) {
	txs := make(chan transactionWithSource)
	wg := workerGroup{}
	for range 150 {
		wg.add("test", "wrong", txs, nil, &observers{})
	}
	wg.close()
}

func TestSendTransaction_RoutesARestrictedAppThroughItsNodesOnly(t *testing.T) {
	pool := NewRpcWorkerPool(t.Context())
	a := &nodeWorkers{txs: make(chan transactionWithSource, 1), closed: make(chan struct{})}
	b := &nodeWorkers{txs: make(chan transactionWithSource, 1), closed: make(chan struct{})}
	pool.nodes = map[string]*nodeWorkers{"a": a, "b": b}
	pool.RestrictApp("restricted", []string{"a"})

	var tx types.Transaction
	for _, app := range []string{"restricted", "free"} {
		go func() {
			_ = pool.SendTransaction(t.Context(), &tx, driver.TransactionSource{App: app})
		}()
	}

	require.Equal(t, "restricted", answer(<-a.txs).source.App)
	require.Equal(t, "free", answer(<-pool.txs).source.App)
	require.Empty(t, b.txs)
}

func TestSendTransaction_ReportsARestrictedAppWithoutActiveNodes(t *testing.T) {
	observer := driver.NewMockTransactionObserver(gomock.NewController(t))
	pool := NewRpcWorkerPool(t.Context())
	pool.RegisterObserver(observer)
	pool.RestrictApp("restricted", []string{"gone"})

	var tx types.Transaction
	source := driver.TransactionSource{App: "restricted"}
	observer.EXPECT().OnTransactionSubmitted(source, &tx, gomock.Any(), errNoNode)
	require.ErrorIs(t, pool.SendTransaction(t.Context(), &tx, source), errNoNode)
}

func TestSendTransaction_MovesToAnotherNode_WhenTheChosenOneIsRemoved(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := NewRpcWorkerPool(t.Context())
		a := &nodeWorkers{txs: make(chan transactionWithSource), closed: make(chan struct{})}
		b := &nodeWorkers{txs: make(chan transactionWithSource, 1), closed: make(chan struct{})}
		pool.nodes = map[string]*nodeWorkers{"a": a}
		pool.RestrictApp("restricted", []string{"a", "b"})

		go func() {
			var tx types.Transaction
			_ = pool.SendTransaction(t.Context(), &tx, driver.TransactionSource{App: "restricted"})
		}()
		synctest.Wait() // the sender waits for a worker of node a

		pool.mu.Lock()
		pool.nodes["b"] = b
		pool.mu.Unlock()
		node := driver.NewMockNode(gomock.NewController(t))
		node.EXPECT().GetLabel().Return("a").AnyTimes()
		pool.BeforeNodeRemoval(node)

		require.Equal(t, "restricted", answer(<-b.txs).source.App)
	})
}

func TestSendTransaction_ReportsAFailure_WhenTheWorkersOfItsOnlyNodeCannotConnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		node := driver.NewMockNode(ctrl)
		node.EXPECT().GetLabel().Return("a").AnyTimes()
		url := driver.URL("wrong")
		node.EXPECT().IsPure().Return(false)
		node.EXPECT().GetServiceUrl(gomock.Any()).Return(&url, nil)
		observer := driver.NewMockTransactionObserver(ctrl)

		pool := NewRpcWorkerPool(t.Context())
		pool.RegisterObserver(observer)
		pool.AfterNodeCreation(node)
		pool.RestrictApp("restricted", []string{"a"})

		var tx types.Transaction
		source := driver.TransactionSource{App: "restricted"}
		observer.EXPECT().OnTransactionSubmitted(source, &tx, gomock.Any(), errNoNode)
		require.ErrorIs(t, pool.SendTransaction(t.Context(), &tx, source), errNoNode)
	})
}

func TestAfterNodeCreation_SendsNothingThroughANode_WhenItIsPure(t *testing.T) {
	node := driver.NewMockNode(gomock.NewController(t))
	node.EXPECT().IsPure().Return(true)

	pool := NewRpcWorkerPool(t.Context())
	pool.AfterNodeCreation(node)

	require.Empty(t, pool.nodes)
}

// answer plays the worker that submitted the given transaction successfully.
func answer(t transactionWithSource) transactionWithSource {
	t.result <- nil
	return t
}
