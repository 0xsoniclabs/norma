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

package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"go.uber.org/mock/gomock"
)

// transactionWithNonce builds a distinguishable transaction.
func transactionWithNonce(nonce uint64) *types.Transaction {
	to := common.Address{1}
	return types.NewTx(&types.DynamicFeeTx{Nonce: nonce, Gas: 100_000, To: &to})
}

// verifyQueued runs the verifications the checker has queued, the way its verifier
// goroutines do during a run.
func verifyQueued(c *checker) {
	for {
		select {
		case v := <-c.verifications:
			c.verify(v)
		default:
			return
		}
	}
}

func newTestChecker(t *testing.T) (*checker, *app.MockAppContext, *rpc.MockClient) {
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)
	ctxt := app.NewMockAppContext(ctrl)
	ctxt.EXPECT().GetClient().Return(client).AnyTimes()
	network := driver.NewMockNetwork(ctrl)
	network.EXPECT().GetActiveNodes().Return(nil).AnyTimes()
	c := newChecker(ctxt, network)
	// The pool of every node is consulted through the network; the tests drive it
	// directly instead of standing up nodes.
	c.heldByAnyNode = func(hash common.Hash) (bool, error) {
		status, err := client.GetTransactionPoolStatus(hash)
		return status != rpc.TxPoolStatusNotPresent, err
	}
	return c, ctxt, client
}

func TestChecker_ConfirmsTransactionsFoundInABlock(t *testing.T) {
	c, ctxt, client := newTestChecker(t)
	ctrl := gomock.NewController(t)
	generator := app.NewMockGenerator(ctrl)

	tx := transactionWithNonce(1)
	call := app.Call{Tx: tx, Outcome: app.Success}
	receipt := &types.Receipt{TxHash: tx.Hash(), Status: types.ReceiptStatusSuccessful}

	generator.EXPECT().Check(ctxt, call, receipt).Return(nil)

	c.submit("counter", generator, call)
	if got := c.numPending(); got != 1 {
		t.Fatalf("expected 1 pending transaction, got %d", got)
	}

	// One request per block covers every transaction it carries.
	client.EXPECT().BlockNumber(gomock.Any()).Return(uint64(4), nil)
	client.EXPECT().GetBlockReceipts(uint64(4)).Return([]*types.Receipt{receipt}, nil)

	if next := c.scanBlocks(context.Background(), 4); next != 5 {
		t.Errorf("expected the scan to advance to block 5, got %d", next)
	}
	verifyQueued(c)
	if got := c.numPending(); got != 0 {
		t.Errorf("expected no pending transactions, got %d", got)
	}
	if got := c.getConfirmedTransactions(); got != 1 {
		t.Errorf("expected 1 confirmed transaction, got %d", got)
	}
	if err := c.result(); err != nil {
		t.Errorf("expected no failures, got %v", err)
	}
}

func TestChecker_IgnoresTransactionsOfOtherLoads(t *testing.T) {
	c, _, client := newTestChecker(t)

	foreign := &types.Receipt{TxHash: transactionWithNonce(9).Hash()}
	client.EXPECT().BlockNumber(gomock.Any()).Return(uint64(1), nil)
	client.EXPECT().GetBlockReceipts(uint64(1)).Return([]*types.Receipt{foreign}, nil)

	c.scanBlocks(context.Background(), 1)
	verifyQueued(c)
	if got := c.getConfirmedTransactions(); got != 0 {
		t.Errorf("expected no confirmed transactions, got %d", got)
	}
}

func TestChecker_ReportsDivergingOutcomes(t *testing.T) {
	c, ctxt, client := newTestChecker(t)
	ctrl := gomock.NewController(t)
	generator := app.NewMockGenerator(ctrl)

	tx := transactionWithNonce(2)
	call := app.Call{Tx: tx, Outcome: app.Success}
	receipt := &types.Receipt{TxHash: tx.Hash(), Status: types.ReceiptStatusFailed}

	generator.EXPECT().Check(ctxt, call, receipt).Return(errors.New("was aborted"))

	c.submit("counter", generator, call)
	client.EXPECT().BlockNumber(gomock.Any()).Return(uint64(1), nil)
	client.EXPECT().GetBlockReceipts(uint64(1)).Return([]*types.Receipt{receipt}, nil)
	c.scanBlocks(context.Background(), 1)
	verifyQueued(c)

	err := c.result()
	if err == nil {
		t.Fatal("expected the failure to be reported")
	}
	// The report names the generator that produced the transaction.
	if !strings.Contains(err.Error(), "counter") || !strings.Contains(err.Error(), "was aborted") {
		t.Errorf("expected the report to name the generator and the cause, got %v", err)
	}
	if got := c.getConfirmedTransactions(); got != 0 {
		t.Errorf("expected no confirmed transactions, got %d", got)
	}
}

func TestChecker_LeavesTransactionsWaitingInThePoolAlone(t *testing.T) {
	c, _, client := newTestChecker(t)
	ctrl := gomock.NewController(t)
	generator := app.NewMockGenerator(ctrl)

	tx := transactionWithNonce(3)
	c.submit("counter", generator, app.Call{Tx: tx, Outcome: app.Success})

	// Backdate the transaction so that it counts as overdue.
	c.mutex.Lock()
	pending := c.pending[tx.Hash()]
	pending.waitingSince = pending.waitingSince.Add(-2 * settleWindow)
	c.pending[tx.Hash()] = pending
	c.mutex.Unlock()

	client.EXPECT().GetTransactionPoolStatus(tx.Hash()).Return(rpc.TxPoolStatusPending, nil)

	c.retireOverdue(context.Background())
	verifyQueued(c)
	if got := c.numPending(); got != 1 {
		t.Errorf("expected the transaction to keep waiting, got %d pending", got)
	}
	if err := c.result(); err != nil {
		t.Errorf("expected no failures, got %v", err)
	}
}

func TestChecker_ResolvesTransactionsTheNetworkDropped(t *testing.T) {
	c, ctxt, client := newTestChecker(t)
	ctrl := gomock.NewController(t)
	generator := app.NewMockGenerator(ctrl)

	tx := transactionWithNonce(4)
	call := app.Call{Tx: tx, Outcome: app.Rejected}
	c.submit("subsidies", generator, call)

	c.mutex.Lock()
	pending := c.pending[tx.Hash()]
	pending.waitingSince = pending.waitingSince.Add(-2 * settleWindow)
	c.pending[tx.Hash()] = pending
	c.mutex.Unlock()

	// Neither in the pool nor in a block: the network refused it, which is what a
	// rejected transaction is checked against.
	client.EXPECT().GetTransactionPoolStatus(tx.Hash()).Return(rpc.TxPoolStatusNotPresent, nil)
	client.EXPECT().GetTransactionReceipt(tx.Hash()).Return(nil, nil)
	generator.EXPECT().Check(ctxt, call, nil).Return(nil)

	c.retireOverdue(context.Background())
	verifyQueued(c)
	if got := c.numPending(); got != 0 {
		t.Errorf("expected the transaction to be resolved, got %d pending", got)
	}
	if got := c.getConfirmedTransactions(); got != 1 {
		t.Errorf("expected 1 confirmed transaction, got %d", got)
	}
}

func TestChecker_PrefersALateReceiptOverThePoolStatus(t *testing.T) {
	c, ctxt, client := newTestChecker(t)
	ctrl := gomock.NewController(t)
	generator := app.NewMockGenerator(ctrl)

	tx := transactionWithNonce(5)
	call := app.Call{Tx: tx, Outcome: app.Success}
	c.submit("counter", generator, call)

	c.mutex.Lock()
	pending := c.pending[tx.Hash()]
	pending.waitingSince = pending.waitingSince.Add(-2 * settleWindow)
	c.pending[tx.Hash()] = pending
	c.mutex.Unlock()

	// The transaction left the pool because it was included in a block the scan
	// has not reached yet, so the receipt has to be asked for directly.
	receipt := &types.Receipt{TxHash: tx.Hash(), Status: types.ReceiptStatusSuccessful}
	client.EXPECT().GetTransactionPoolStatus(tx.Hash()).Return(rpc.TxPoolStatusNotPresent, nil)
	client.EXPECT().GetTransactionReceipt(tx.Hash()).Return(receipt, nil)
	generator.EXPECT().Check(ctxt, call, receipt).Return(nil)

	c.retireOverdue(context.Background())
	verifyQueued(c)
	if got := c.getConfirmedTransactions(); got != 1 {
		t.Errorf("expected 1 confirmed transaction, got %d", got)
	}
}

func TestChecker_ConfirmsARejectedTransactionAsSoonAsANodeRefusesIt(t *testing.T) {
	c, ctxt, _ := newTestChecker(t)
	ctrl := gomock.NewController(t)
	generator := app.NewMockGenerator(ctrl)

	tx := transactionWithNonce(6)
	call := app.Call{Tx: tx, Outcome: app.Rejected}
	generator.EXPECT().Check(ctxt, call, nil).Return(nil)

	c.submit("counter", generator, call)
	// A node refusing the transaction settles it immediately - there is nothing
	// left to wait for.
	c.reportSent(tx.Hash(), errors.New("exceeds block gas limit"))
	verifyQueued(c)

	if got := c.numPending(); got != 0 {
		t.Errorf("expected the transaction to be resolved, got %d pending", got)
	}
	if got := c.getConfirmedTransactions(); got != 1 {
		t.Errorf("expected 1 confirmed transaction, got %d", got)
	}
	if err := c.result(); err != nil {
		t.Errorf("expected no failures, got %v", err)
	}
}

func TestChecker_ReportsTheRefusalOfATransactionMeantToGoThrough(t *testing.T) {
	c, ctxt, _ := newTestChecker(t)
	ctrl := gomock.NewController(t)
	generator := app.NewMockGenerator(ctrl)

	tx := transactionWithNonce(7)
	call := app.Call{Tx: tx, Outcome: app.Success}
	generator.EXPECT().Check(ctxt, call, nil).Return(errors.New("never reached a block"))

	c.submit("counter", generator, call)
	c.reportSent(tx.Hash(), errors.New("insufficient funds"))
	verifyQueued(c)

	err := c.result()
	if err == nil {
		t.Fatal("expected the failure to be reported")
	}
	// The report names why no node took the transaction, not just that it is gone.
	if !strings.Contains(err.Error(), "insufficient funds") {
		t.Errorf("expected the report to name the refusal, got %v", err)
	}
}

func TestChecker_IgnoresASuccessfulSend(t *testing.T) {
	c, _, _ := newTestChecker(t)
	ctrl := gomock.NewController(t)
	generator := app.NewMockGenerator(ctrl)

	tx := transactionWithNonce(8)
	c.submit("counter", generator, app.Call{Tx: tx, Outcome: app.Success})
	c.reportSent(tx.Hash(), nil)

	if got := c.numPending(); got != 1 {
		t.Errorf("expected the transaction to stay pending, got %d pending", got)
	}
}

func TestHeldByAnyNode_AsksEveryNode(t *testing.T) {
	// A transaction waits in the pool of the node it was sent to and is invisible
	// to every other node, so one node reporting it absent means nothing.
	tests := map[string]struct {
		statuses []rpc.TxPoolStatus
		want     bool
	}{
		"held by the last node":  {statuses: []rpc.TxPoolStatus{rpc.TxPoolStatusNotPresent, rpc.TxPoolStatusNotPresent, rpc.TxPoolStatusPending}, want: true},
		"held by the first node": {statuses: []rpc.TxPoolStatus{rpc.TxPoolStatusQueued, rpc.TxPoolStatusNotPresent}, want: true},
		"held by no node":        {statuses: []rpc.TxPoolStatus{rpc.TxPoolStatusNotPresent, rpc.TxPoolStatusNotPresent}, want: false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			hash := transactionWithNonce(1).Hash()

			nodes := make([]driver.Node, 0, len(test.statuses))
			for _, status := range test.statuses {
				client := rpc.NewMockClient(ctrl)
				client.EXPECT().GetTransactionPoolStatus(hash).Return(status, nil).MaxTimes(1)
				client.EXPECT().Close().MaxTimes(1)
				node := driver.NewMockNode(ctrl)
				node.EXPECT().DialRpc(gomock.Any()).Return(client, nil).MaxTimes(1)
				nodes = append(nodes, node)
			}
			network := driver.NewMockNetwork(ctrl)
			network.EXPECT().GetActiveNodes().Return(nodes).AnyTimes()

			got, err := heldByAnyNode(network, hash)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.want {
				t.Errorf("expected held=%v, got %v", test.want, got)
			}
		})
	}
}

func TestHeldByAnyNode_ReportsAnErrorOnlyWhenNoNodeAnswers(t *testing.T) {
	ctrl := gomock.NewController(t)
	hash := transactionWithNonce(1).Hash()

	unreachable := driver.NewMockNode(ctrl)
	unreachable.EXPECT().DialRpc(gomock.Any()).Return(nil, errors.New("no route to host")).AnyTimes()

	network := driver.NewMockNetwork(ctrl)
	network.EXPECT().GetActiveNodes().Return([]driver.Node{unreachable}).AnyTimes()

	if _, err := heldByAnyNode(network, hash); err == nil {
		t.Error("expected an error when no node could be asked")
	}

	// One node answering is enough to know the transaction is not waiting there.
	client := rpc.NewMockClient(ctrl)
	client.EXPECT().GetTransactionPoolStatus(hash).Return(rpc.TxPoolStatusNotPresent, nil)
	client.EXPECT().Close()
	reachable := driver.NewMockNode(ctrl)
	reachable.EXPECT().DialRpc(gomock.Any()).Return(client, nil)

	mixed := driver.NewMockNetwork(ctrl)
	mixed.EXPECT().GetActiveNodes().Return([]driver.Node{unreachable, reachable}).AnyTimes()

	held, err := heldByAnyNode(mixed, hash)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if held {
		t.Error("expected the transaction to be reported as not held")
	}
}

func TestChecker_ResultSummarizesManyFailures(t *testing.T) {
	c, _, _ := newTestChecker(t)
	for i := 0; i < maxReportedFailures+5; i++ {
		c.recordFailure(errors.New("diverged"))
	}

	err := c.result()
	if err == nil {
		t.Fatal("expected the failures to be reported")
	}
	if !strings.Contains(err.Error(), "25 transaction(s)") {
		t.Errorf("expected the report to count all failures, got %v", err)
	}
	if !strings.Contains(err.Error(), "the first 20 of them") {
		t.Errorf("expected the report to say how many are listed, got %v", err)
	}
}
