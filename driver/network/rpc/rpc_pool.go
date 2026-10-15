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

func NewRpcWorkerPool(ctx context.Context) *RpcWorkerPool {
	ctx, cancel := context.WithCancel(ctx)

	return &RpcWorkerPool{
		txs:       make(chan transactionWithSource, 100),
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
// may submit through. When the chosen node is removed first, it tries another.
func (p *RpcWorkerPool) SendTransaction(tx *types.Transaction, source driver.TransactionSource) {
	t := transactionWithSource{tx: tx, source: source}
	for {
		targets, restricted := p.targets(source.App)
		if !restricted {
			p.txs <- t
			return
		}
		if len(targets) == 0 {
			p.observers.notify(source, tx, time.Now(), errNoNode)
			return
		}
		// ponytail: random node, not the least busy one; balance on load if a slow node starts holding up the app.
		target := targets[rand.IntN(len(targets))]
		select {
		case target.txs <- t:
			return
		case <-target.closed:
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

func (p *RpcWorkerPool) AfterNodeCreation(newNode driver.Node) {
	if p.ctx.Err() == context.Canceled {
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
	for i := 0; i < 150; i++ {
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
	close(p.txs)
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
		err := rpcClient.SendTransaction(context.Background(), tx.tx)
		// The submission is reported before any logging, keeping the
		// measured moment as close to the actual submission as possible.
		p.observers.notify(tx.source, tx.tx, time.Now(), err)
		if err != nil {
			slog.Warn("failed to send tx", "node", p.nodeName, "source", tx.source, "error", err)
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
