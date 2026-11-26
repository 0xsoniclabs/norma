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
	"log/slog"
	"math/rand/v2"
	"strings"
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
	ctx       context.Context
	cancel    context.CancelFunc
	observers *observers

	mu sync.Mutex
	// nodes holds the workers of each node, by label.
	nodes map[string]*nodeWorkers
	// appNodes holds the labels of the nodes an application is restricted to.
	appNodes map[string][]string
}

// nodeWorkers are the workers of one node. Besides the transactions any node
// may submit, they take those only this node may submit from txs.
type nodeWorkers struct {
	group     workerGroup
	txs       chan transactionWithSource
	closed    chan struct{}
	closeOnce sync.Once
}

var errNoNode = errors.New("none of the nodes the application is restricted to is active")

// submitTimeout bounds the wait for a node to answer a submission. A sender
// waits for the answer even after its application stopped, so a node that
// never answers must not hold it up forever. The node may still have taken a
// transaction it did not answer in time; only a resubmission tells.
var submitTimeout = 10 * time.Second

func NewRpcWorkerPool(ctx context.Context) *RpcWorkerPool {
	ctx, cancel := context.WithCancel(ctx)

	return &RpcWorkerPool{
		txs:       make(chan transactionWithSource),
		ctx:       ctx,
		cancel:    cancel,
		observers: &observers{},
		nodes:     make(map[string]*nodeWorkers, 10),
		appNodes:  map[string][]string{},
	}
}

// RestrictApp makes the pool submit the transactions of the given application
// only through the nodes with the given labels, or through any node when there
// are none.
func (p *RpcWorkerPool) RestrictApp(app string, nodes []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(nodes) == 0 {
		delete(p.appNodes, app)
	} else {
		p.appNodes[app] = nodes
	}
}

// SendTransaction hands the transaction to a worker of a node its application
// may submit through and waits for the node's answer. When the chosen node is
// removed first, it tries another. Once a worker took the transaction, it waits
// for the answer even if the context ends, since the node may accept it.
func (p *RpcWorkerPool) SendTransaction(
	ctx context.Context,
	tx *types.Transaction,
	source driver.TransactionSource,
) error {
	t := transactionWithSource{tx: tx, source: source, result: make(chan error, 1)}
	for {
		queue, closed := p.txs, (chan struct{})(nil)
		targets, restricted := p.targets(source.App)
		if restricted {
			if len(targets) == 0 {
				p.observers.notify(source, tx, time.Now(), errNoNode)
				return errNoNode
			}
			// ponytail: random node, not the least busy one; balance on load if a slow node starts holding up the app.
			target := targets[rand.IntN(len(targets))]
			queue, closed = target.txs, target.closed
		}
		select {
		case queue <- t:
			select {
			case err := <-t.result:
				return err
			case <-p.ctx.Done():
				return p.ctx.Err()
			}
		case <-closed:
		case <-ctx.Done():
			return ctx.Err()
		case <-p.ctx.Done():
			return p.ctx.Err()
		}
	}
}

// targets returns the active nodes the given application is restricted to,
// and whether it is restricted at all.
func (p *RpcWorkerPool) targets(app string) ([]*nodeWorkers, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	labels, restricted := p.appNodes[app]
	var targets []*nodeWorkers
	for _, label := range labels {
		if n, ok := p.nodes[label]; ok {
			targets = append(targets, n)
		}
	}
	return targets, restricted
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

// AfterNodeCreation starts workers submitting through the new node, unless it
// is a pure validator, which is never sent transactions.
func (p *RpcWorkerPool) AfterNodeCreation(newNode driver.Node) {
	if p.ctx.Err() == context.Canceled || newNode.IsPure() {
		return
	}

	rpcUrl, err := newNode.GetServiceUrl(&node.OperaWsService)
	if err != nil {
		slog.Error("failed to get RPC service URL", "error", err, "node", newNode.GetLabel())
		return
	}

	n := &nodeWorkers{
		txs:    make(chan transactionWithSource),
		closed: make(chan struct{}),
	}
	for range 150 {
		n.group.add(newNode.GetLabel(), *rpcUrl, p.txs, n.txs, p.observers)
	}
	p.mu.Lock()
	p.nodes[newNode.GetLabel()] = n
	p.mu.Unlock()
	go p.retireWhenWorkersExit(newNode.GetLabel(), n)
}

// retireWhenWorkersExit stops offering the node to restricted applications once
// all its workers have exited, which they also do after failing to connect.
// Transactions waiting for one of them then move on to another node.
func (p *RpcWorkerPool) retireWhenWorkersExit(label string, n *nodeWorkers) {
	for _, w := range n.group {
		<-w.done
	}
	p.mu.Lock()
	if p.nodes[label] == n {
		delete(p.nodes, label)
		slog.Warn("all RPC workers of node exited, no longer submitting through it", "node", label)
	}
	p.mu.Unlock()
	n.signalClosed()
}

func (p *RpcWorkerPool) BeforeNodeRemoval(node driver.Node) {
	p.mu.Lock()
	n, ok := p.nodes[node.GetLabel()]
	delete(p.nodes, node.GetLabel())
	p.mu.Unlock()
	if ok {
		n.close()
	}
}

func (n *nodeWorkers) close() {
	n.signalClosed()
	n.group.close()
}

func (n *nodeWorkers) signalClosed() {
	n.closeOnce.Do(func() { close(n.closed) })
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
	p.mu.Lock()
	nodes := p.nodes
	p.nodes = map[string]*nodeWorkers{}
	p.mu.Unlock()
	for _, n := range nodes {
		n.close()
	}
	slog.Info("worker pool has closed")
	return nil
}

// workerGroup is a slice used to hold the workers.
// The workers can be added in this slice and this workerGroup
// can be closed, which closes all stored workers.
// When the group is closed, it should not be re-used and should be forgotten.
type workerGroup []*worker

func (wg *workerGroup) add(nodeName string, rpcUrl driver.URL, txs, own chan transactionWithSource, observers *observers) {
	w := newWorker(nodeName, rpcUrl, txs, own, observers)
	*wg = append(*wg, w)
}

func (wg *workerGroup) close() {
	var done sync.WaitGroup
	for _, w := range *wg {
		done.Go(func() {
			w.close()
		})
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
	nodeName  string
	rpcUrl    driver.URL
	done      chan bool
	txs       chan transactionWithSource
	own       chan transactionWithSource
	ctx       context.Context
	cancel    context.CancelFunc
	observers *observers
}

// newWorker starts a worker submitting the transactions it takes from txs, which
// any node may submit, and from own, which only its node may.
func newWorker(nodeName string, rpcUrl driver.URL, txs, own chan transactionWithSource, observers *observers) *worker {
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
	for {
		var tx transactionWithSource
		select {
		case tx = <-p.txs:
		case tx = <-p.own:
		case <-p.ctx.Done():
			return nil
		}
		ctx, cancel := context.WithTimeout(p.ctx, submitTimeout)
		err := rpcClient.SendTransaction(ctx, tx.tx)
		cancel()
		if err != nil && strings.Contains(err.Error(), "already known") {
			err = nil // the node has it from an earlier submission
		}
		// The submission is reported right away, keeping the measured moment
		// as close to the actual submission as possible.
		p.observers.notify(tx.source, tx.tx, time.Now(), err)
		tx.result <- err
	}
}

// transactionWithSource is a transaction on its way to a worker, with the load
// generator it is attributed to and where the worker answers with the node's
// response.
type transactionWithSource struct {
	tx     *types.Transaction
	source driver.TransactionSource
	result chan error
}
