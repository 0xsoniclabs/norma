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
	"testing/synctest"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/ethereum/go-ethereum/core/types"
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

func TestCloseWorkerStartStop(t *testing.T) {
	txs := make(chan transactionWithSource)
	w := newWorker("test", "wrong", txs, nil, &observers{})
	w.close()
}

func TestCloseWorkerGroupStartStop(t *testing.T) {
	txs := make(chan transactionWithSource)
	wg := workerGroup{}
	for i := 0; i < 150; i++ {
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
	pool.SendTransaction(&tx, driver.TransactionSource{App: "restricted"})
	pool.SendTransaction(&tx, driver.TransactionSource{App: "free"})

	if got := (<-a.txs).source.App; got != "restricted" {
		t.Errorf("node a got a transaction of %q", got)
	}
	if got := (<-pool.txs).source.App; got != "free" {
		t.Errorf("shared queue got a transaction of %q", got)
	}
	if len(b.txs) != 0 {
		t.Errorf("node b got a transaction")
	}
}

func TestSendTransaction_ReportsARestrictedAppWithoutActiveNodes(t *testing.T) {
	observer := driver.NewMockTransactionObserver(gomock.NewController(t))
	pool := NewRpcWorkerPool(t.Context())
	pool.RegisterObserver(observer)
	pool.RestrictApp("restricted", []string{"gone"})

	var tx types.Transaction
	source := driver.TransactionSource{App: "restricted"}
	observer.EXPECT().OnTransactionSubmitted(source, &tx, gomock.Any(), errNoNode)
	pool.SendTransaction(&tx, source)
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
			pool.SendTransaction(&tx, driver.TransactionSource{App: "restricted"})
		}()
		synctest.Wait() // the sender waits for a worker of node a

		pool.mu.Lock()
		pool.nodes["b"] = b
		pool.mu.Unlock()
		node := driver.NewMockNode(gomock.NewController(t))
		node.EXPECT().GetLabel().Return("a").AnyTimes()
		pool.BeforeNodeRemoval(node)

		if got := (<-b.txs).source.App; got != "restricted" {
			t.Errorf("node b got a transaction of %q", got)
		}
	})
}

func TestSendTransaction_ReportsAFailure_WhenTheWorkersOfItsOnlyNodeCannotConnect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctrl := gomock.NewController(t)
		node := driver.NewMockNode(ctrl)
		node.EXPECT().GetLabel().Return("a").AnyTimes()
		url := driver.URL("wrong")
		node.EXPECT().GetServiceUrl(gomock.Any()).Return(&url, nil)
		observer := driver.NewMockTransactionObserver(ctrl)

		pool := NewRpcWorkerPool(t.Context())
		pool.RegisterObserver(observer)
		pool.AfterNodeCreation(node)
		pool.RestrictApp("restricted", []string{"a"})

		var tx types.Transaction
		source := driver.TransactionSource{App: "restricted"}
		observer.EXPECT().OnTransactionSubmitted(source, &tx, gomock.Any(), errNoNode)
		pool.SendTransaction(&tx, source)
	})
}
