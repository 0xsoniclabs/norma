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
	txs     chan transactionWithSource
	workers map[driver.Node]*workerGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewRpcWorkerPool() *RpcWorkerPool {
	ctx, cancel := context.WithCancel(context.Background())

	return &RpcWorkerPool{
		txs:     make(chan transactionWithSource, 100),
		workers: make(map[driver.Node]*workerGroup, 10),
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (p *RpcWorkerPool) SendTransaction(tx *types.Transaction, source string) {
	p.txs <- transactionWithSource{tx: tx, source: source}
}

func (p *RpcWorkerPool) AfterNodeCreation(newNode driver.Node) {
	if p.ctx.Err() == context.Canceled {
		return
	}

	rpcUrl := newNode.GetServiceUrl(&node.OperaWsService)
	if rpcUrl == nil {
		return
	}
	wg := workerGroup{}
	p.workers[newNode] = &wg
	for i := 0; i < 150; i++ {
		wg.add(newNode.GetLabel(), *rpcUrl, p.txs)
	}
}

func (p *RpcWorkerPool) BeforeNodeRemoval(node driver.Node) {
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

func (wg *workerGroup) add(nodeName string, rpcUrl driver.URL, txs chan transactionWithSource) {
	w := newWorker(nodeName, rpcUrl, txs)
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
	ctx      context.Context
	cancel   context.CancelFunc
}

func newWorker(nodeName string, rpcUrl driver.URL, txs chan transactionWithSource) *worker {
	ctx, cancel := context.WithCancel(context.Background())

	w := &worker{
		nodeName: nodeName,
		rpcUrl:   rpcUrl,
		done:     make(chan bool),
		txs:      txs,
		ctx:      ctx,
		cancel:   cancel,
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
		func() (*ethclient.Client, error) {
			return ethclient.Dial(string(p.rpcUrl))
		})

	if rpcClient == nil || err != nil {
		return err
	}

	defer rpcClient.Close()
	for {
		select {
		case tx := <-p.txs:
			err := rpcClient.SendTransaction(context.Background(), tx.tx)
			if err != nil {
				slog.Warn("failed to send tx", "node", p.nodeName, "source", tx.source, "error", err)
			}
		case <-p.ctx.Done():
			return nil
		}
	}
}

// transactionWithSource is a struct that holds a transaction and its source.
// It is used to provide feedback about the origin of the transaction in case
// of an error when sending it to the RPC client.
type transactionWithSource struct {
	tx *types.Transaction
	// source is a string describing the origin of the transaction,
	// e.g. the load generator that created it.
	source string
}
