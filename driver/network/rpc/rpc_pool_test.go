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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestRetryRpcReturnGracefully(t *testing.T) {
	t.Parallel()

	start := time.Now()
	txs := make(chan transactionWithSource)
	w := newWorker("test", "wrong", txs, make(chan transactionWithSource), &observers{})

	time.Sleep(6 * time.Second)
	w.close()

	if got, want := time.Since(start).Seconds(), float64(5); got < want {
		t.Errorf("RPC should be attempted around 6s, was: %f < %f", got, want)
	}
}

func TestClosePool(t *testing.T) {
	t.Parallel()

	pool := NewRpcWorkerPool(t.Context())
	counter := &atomic.Int32{}
	wg := &sync.WaitGroup{}

	go func() {
		for range pool.txs {
			counter.Add(1)
			wg.Done()
		}
		wg.Done() // will get here when the channel is closed
	}()

	var tx types.Transaction
	for i := 0; i < 10; i++ {
		wg.Add(1)
		pool.SendTransaction(&tx, driver.TransactionSource{App: "test"})
	}

	wg.Wait()
	wg.Add(1) // extra count to check the go routine ended

	if got, want := counter.Load(), int32(10); got != want {
		t.Errorf("not all data read from the channel: %d != %d", got, want)
	}

	if err := pool.Close(); err != nil {
		t.Fatalf("error: %s", err)
	}

	wg.Wait()

	if got, want := counter.Load(), int32(10); got > want {
		t.Errorf("not all data read from the channel: %d != %d", got, want)
	}
}

func TestPool_PinnedTransactionsGoToTheirNodeAlone(t *testing.T) {
	pool := NewRpcWorkerPool(t.Context())
	pool.queues.startServing("A")

	var tx types.Transaction
	pool.SendTransactionTo("A", &tx, driver.TransactionSource{App: "pinned"})

	select {
	case got := <-pool.queues.queue("A"):
		if got.source.App != "pinned" {
			t.Errorf("queue of A holds the transactions of %q", got.source.App)
		}
	default:
		t.Error("the transaction did not reach the queue of node A")
	}
	select {
	case got := <-pool.txs:
		t.Errorf("a pinned transaction reached the shared queue: %v", got.source)
	default:
	}
}

func TestPool_ReportsPinnedTransactionsNoNodeServes(t *testing.T) {
	pool := NewRpcWorkerPool(t.Context())
	observer := &recordingObserver{}
	pool.RegisterObserver(observer)

	var tx types.Transaction
	pool.SendTransactionTo("gone", &tx, driver.TransactionSource{App: "pinned"})

	if got := len(observer.errs); got != 1 {
		t.Fatalf("got %d submissions reported, wanted the dropped one", got)
	}
	if observer.errs[0] == nil {
		t.Error("the dropped transaction was reported as submitted")
	}
	select {
	case got := <-pool.queues.queue("gone"):
		t.Errorf("the transaction was queued for a node that does not exist: %v", got.source)
	default:
	}
}

// recordingObserver keeps the error of every submission reported to it.
type recordingObserver struct {
	mu   sync.Mutex
	errs []error
}

func (o *recordingObserver) OnTransactionSubmitted(
	_ driver.TransactionSource, _ *types.Transaction, _ time.Time, err error,
) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.errs = append(o.errs, err)
}

func TestCloseWorkerStartStop(t *testing.T) {
	txs := make(chan transactionWithSource)
	w := newWorker("test", "wrong", txs, make(chan transactionWithSource), &observers{})
	w.close()
}

func TestCloseWorkerGroupStartStop(t *testing.T) {
	txs := make(chan transactionWithSource)
	wg := workerGroup{}
	for i := 0; i < 150; i++ {
		wg.add("test", "wrong", txs, make(chan transactionWithSource), &observers{})
	}
	wg.close()
}
