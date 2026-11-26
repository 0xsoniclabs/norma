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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"
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

func TestSendTransaction_CountsTheSubmission_WhenTheNodeGaveNoAnswer(t *testing.T) {
	tests := map[string]struct {
		answer     error
		unanswered int64
	}{
		"accepted":           {answer: nil},
		"refused":            {answer: rpcError("txpool is full")},
		"connection refused": {answer: &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}},
		"timed out":          {answer: fmt.Errorf("post failed: %w", context.DeadlineExceeded), unanswered: 1},
		"connection lost":    {answer: io.ErrUnexpectedEOF, unanswered: 1},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pool := NewRpcWorkerPool(t.Context())
			go func() {
				(<-pool.txs).result <- test.answer
			}()

			var tx types.Transaction
			err := pool.SendTransaction(t.Context(), &tx, driver.TransactionSource{App: "test"})
			require.Equal(t, test.answer, err)
			require.Equal(t, test.unanswered, pool.Unanswered())
		})
	}
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

func TestRunRpcSenderLoop_AnswersWithTheOutcome_WhenItsNodeAnswersLateOrNotAtAll(t *testing.T) {
	tests := map[string]struct {
		answer  string // the error the node answers with; none if empty
		drop    bool   // the node drops the connection instead of answering
		timeout time.Duration
		close   bool
		check   func(t *testing.T, err error)
	}{
		"node refuses it": {
			answer:  "txpool is full",
			timeout: time.Hour,
			check: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "txpool is full")
				require.False(t, noAnswer(err))
			},
		},
		"node drops the connection": {
			drop:    true,
			timeout: time.Hour,
			check:   func(t *testing.T, err error) { require.True(t, noAnswer(err), "%v", err) },
		},
		"node has it already": {
			answer:  "already known",
			timeout: time.Hour,
			check:   func(t *testing.T, err error) { require.NoError(t, err) },
		},
		"submission times out": {
			timeout: 100 * time.Millisecond,
			check:   func(t *testing.T, err error) { require.ErrorIs(t, err, context.DeadlineExceeded) },
		},
		"worker is closed while it waits": {
			timeout: 100 * time.Millisecond,
			close:   true,
			check:   func(t *testing.T, err error) { require.ErrorIs(t, err, context.DeadlineExceeded) },
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			defer func(old time.Duration) { submitTimeout = old }(submitTimeout)
			submitTimeout = test.timeout

			received, release := make(chan struct{}, 1), make(chan struct{})
			node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- struct{}{}
				if test.drop {
					conn, _, _ := w.(http.Hijacker).Hijack()
					_ = conn.Close()
					return
				}
				if test.answer == "" {
					<-release
					return
				}
				var request struct{ ID json.RawMessage }
				_ = json.NewDecoder(r.Body).Decode(&request)
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32000,"message":%q}}`,
					request.ID, test.answer)
			}))
			defer node.Close()
			defer close(release)

			observed := make(chan error, 1)
			observer := driver.NewMockTransactionObserver(gomock.NewController(t))
			observer.EXPECT().OnTransactionSubmitted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
				func(_ driver.TransactionSource, _ *types.Transaction, _ time.Time, err error) { observed <- err })

			txs := make(chan transactionWithSource)
			w := newWorker("test", driver.URL(node.URL), txs, nil, &observers{list: []driver.TransactionObserver{observer}})
			defer w.close()
			tx := transactionWithSource{tx: types.NewTx(&types.LegacyTx{}), result: make(chan error, 1)}
			txs <- tx
			<-received
			if test.close {
				go w.close()
			}
			select {
			case err := <-tx.result:
				test.check(t, err)
				test.check(t, <-observed)
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

// rpcError is an error a node answers a JSON-RPC request with.
type rpcError string

func (e rpcError) Error() string  { return string(e) }
func (e rpcError) ErrorCode() int { return -32000 }
