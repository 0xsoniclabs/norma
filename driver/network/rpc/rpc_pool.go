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
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/0xsoniclabs/norma/driver/node"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type RpcWorkerPool struct {
	txs       chan transactionWithSource
	workers   map[driver.Node]*workerGroup
	queues    *nodeQueues
	ctx       context.Context
	cancel    context.CancelFunc
	observers *observers
}

func NewRpcWorkerPool(ctx context.Context) *RpcWorkerPool {
	ctx, cancel := context.WithCancel(ctx)

	return &RpcWorkerPool{
		txs:       make(chan transactionWithSource, 100),
		workers:   make(map[driver.Node]*workerGroup, 10),
		queues:    newNodeQueues(),
		ctx:       ctx,
		cancel:    cancel,
		observers: &observers{},
	}
}

func (p *RpcWorkerPool) SendTransaction(tx *types.Transaction, source driver.TransactionSource) {
	p.txs <- transactionWithSource{tx: tx, source: source}
}

// SendTransactionTo submits the transaction through the node of the given label.
// A transaction for a label no node of the network answers to is dropped and
// reported to the observers as a failed submission, which is what it is: nothing
// took it, and a scenario pinning an application to a node it then stops should
// see that in its data rather than lose the transactions silently.
func (p *RpcWorkerPool) SendTransactionTo(
	label string,
	tx *types.Transaction,
	source driver.TransactionSource,
) {
	queue, served := p.queues.of(label)
	if !served {
		err := fmt.Errorf("no node labelled %q is serving transactions", label)
		p.observers.notify(source, tx, time.Now(), err)
		slog.Warn("failed to send tx", "node", label, "source", source, "error", err)
		return
	}
	queue <- transactionWithSource{tx: tx, source: source}
}

// nodeQueues holds the transaction queue of every node label an application has
// been pinned to, and knows which of those labels a node is currently serving. A
// queue outlives the node it belongs to, so a node that leaves the network and
// comes back keeps the transactions of its applications.
type nodeQueues struct {
	mu     sync.Mutex
	queues map[string]chan transactionWithSource
	served map[string]int
}

func newNodeQueues() *nodeQueues {
	return &nodeQueues{
		queues: map[string]chan transactionWithSource{},
		served: map[string]int{},
	}
}

// queue returns the queue of the given label, creating it if it does not exist.
func (q *nodeQueues) queue(label string) chan transactionWithSource {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.queueOf(label)
}

// of returns the queue of the given label and whether a node is serving it.
func (q *nodeQueues) of(label string) (chan transactionWithSource, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.queueOf(label), q.served[label] > 0
}

func (q *nodeQueues) queueOf(label string) chan transactionWithSource {
	queue, exists := q.queues[label]
	if !exists {
		queue = make(chan transactionWithSource, 100)
		q.queues[label] = queue
	}
	return queue
}

func (q *nodeQueues) startServing(label string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.served[label]++
}

func (q *nodeQueues) stopServing(label string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.served[label] > 0 {
		q.served[label]--
	}
}

// RegisterObserver adds an observer to be notified about every transaction this
// pool submits. Observers may be registered at any time; a transaction already
// submitted is not reported retroactively.
func (p *RpcWorkerPool) RegisterObserver(observer driver.TransactionObserver) {
	p.observers.add(observer)
}

// observers is the set of transaction observers of a pool, shared by all its
// workers and therefore safe for concurrent use.
type observers struct {
	mu   sync.Mutex
	list []driver.TransactionObserver
}

func (o *observers) add(observer driver.TransactionObserver) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.list = append(o.list, observer)
}

func (o *observers) notify(
	source driver.TransactionSource,
	tx *types.Transaction,
	at time.Time,
	err error,
) {
	o.mu.Lock()
	list := o.list
	o.mu.Unlock()
	for _, observer := range list {
		observer.OnTransactionSubmitted(source, tx, at, err)
	}
}

func (p *RpcWorkerPool) AfterNodeCreation(newNode driver.Node) {
	if p.ctx.Err() == context.Canceled {
		return
	}

	rpcUrl, err := newNode.GetServiceUrl(&node.OperaWsService)
	if err != nil {
		slog.Error("failed to get RPC service URL", "error", err, "node", newNode.GetLabel())
		return
	}

	label := newNode.GetLabel()
	wg := workerGroup{}
	p.workers[newNode] = &wg
	for i := 0; i < 150; i++ {
		wg.add(label, *rpcUrl, p.txs, p.queues.queue(label), p.observers)
	}
	p.queues.startServing(label)
}

func (p *RpcWorkerPool) BeforeNodeRemoval(node driver.Node) {
	p.queues.stopServing(node.GetLabel())
	p.workers[node].close()
}

func (p *RpcWorkerPool) AfterApplicationCreation(application driver.Application) {
	// ignored
}

func (p *RpcWorkerPool) Close() error {
	if p.ctx.Err() == context.Canceled {
		return nil
	}
	p.cancel()
	slog.Info("waiting for worker pool to close")
	for _, wg := range p.workers {
		wg.close()
	}
	slog.Info("worker pool has closed")
	close(p.txs)
	return nil
}

// workerGroup is a slice used to hold the workers.
// The workers can be added in this slice and this workerGroup
// can be closed, which closes all stored workers.
// When the group is closed, it should not be re-used and should be forgotten.
type workerGroup []*worker

func (wg *workerGroup) add(
	nodeName string,
	rpcUrl driver.URL,
	txs chan transactionWithSource,
	own chan transactionWithSource,
	observers *observers,
) {
	w := newWorker(nodeName, rpcUrl, txs, own, observers)
	*wg = append(*wg, w)
}

func (wg *workerGroup) close() {
	var done sync.WaitGroup
	for _, w := range *wg {
		w := w
		done.Add(1)
		go func() {
			defer done.Done()
			w.close()
		}()
	}
	done.Wait()
}

// worker maintains one worker that sends transactions to an RPC client.
// It listens to incoming transactions and sends them to the client.
// The worker can be closed, and it stops listening and sending the transactions.
// The worker is initialised (i.e. the RPC connection is established) before
// it starts dispatching asynchronously. This process can be interrupted by
// closing the worker before it starts dispatching.
type worker struct {
	nodeName string
	rpcUrl   driver.URL
	done     chan bool
	txs      chan transactionWithSource
	// own holds the transactions of the applications pinned to this worker's
	// node, which no other node's workers take.
	own       chan transactionWithSource
	ctx       context.Context
	cancel    context.CancelFunc
	observers *observers
}

func newWorker(
	nodeName string,
	rpcUrl driver.URL,
	txs chan transactionWithSource,
	own chan transactionWithSource,
	observers *observers,
) *worker {
	ctx, cancel := context.WithCancel(context.Background())

	w := &worker{
		nodeName:  nodeName,
		rpcUrl:    rpcUrl,
		done:      make(chan bool),
		txs:       txs,
		own:       own,
		ctx:       ctx,
		cancel:    cancel,
		observers: observers,
	}

	go func() {
		if err := w.runRpcSenderLoop(); err != nil {
			slog.Error("failed to open RPC connection", "error", err, "node", nodeName)
			return
		}
	}()

	return w
}

func (p *worker) close() {
	if p.ctx.Err() == context.Canceled {
		return
	}
	p.cancel()
	<-p.done
}

func (p *worker) runRpcSenderLoop() error {
	defer close(p.done)
	rpcClient, err := network.RetryReturn(
		p.ctx,
		network.DefaultRetryAttempts,
		1*time.Second,
		func(ctx context.Context) (*ethclient.Client, error) {
			return ethclient.Dial(string(p.rpcUrl))
		})

	if rpcClient == nil || err != nil {
		return err
	}

	defer rpcClient.Close()
	send := func(tx transactionWithSource) {
		err := rpcClient.SendTransaction(context.Background(), tx.tx)
		// The submission is reported before any logging, keeping the measured
		// moment as close to the actual submission as possible.
		p.observers.notify(tx.source, tx.tx, time.Now(), err)
		if err != nil {
			slog.Warn("failed to send tx", "node", p.nodeName, "source", tx.source, "error", err)
		}
	}
	for {
		// The transactions pinned to this node come first: they have nowhere else
		// to go, while the ones of the shared queue are taken by every node.
		select {
		case tx := <-p.own:
			send(tx)
			continue
		case <-p.ctx.Done():
			return nil
		default:
		}
		select {
		case tx := <-p.own:
			send(tx)
		case tx := <-p.txs:
			send(tx)
		case <-p.ctx.Done():
			return nil
		}
	}
}

// transactionWithSource is a struct that holds a transaction and its source.
// It is used to provide feedback about the origin of the transaction in case
// of an error when sending it to the RPC client, and to attribute it to its
// load generator when reporting the submission to observers.
type transactionWithSource struct {
	tx     *types.Transaction
	source driver.TransactionSource
}
