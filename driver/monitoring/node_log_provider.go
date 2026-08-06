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

package monitoring

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/0xsoniclabs/norma/driver"
)

//go:generate mockgen -source node_log_provider.go -destination node_log_provider_mock.go -package monitoring

// LogListener gets data of a new block every time it is occurred for a certain node.
// All listeners are executed in sequence, i.e. each processing of a block should be fast
// not to block the loop.
type LogListener interface {

	// OnBlock is triggered every time a new block is found.
	OnBlock(node Node, block Block)
}

// NodeRestartListener is an optional interface a LogListener may implement
// to be told when a node's client goes offline and when a known node comes
// back online; a returning client replays blocks it has already reported.
type NodeRestartListener interface {
	OnNodeRestart(node Node)
}

// NodeLogProvider is an interface for registering listeners that will be notified about incoming blocks.
type NodeLogProvider interface {

	// RegisterLogListener registers the input listener to receive new blocks.
	RegisterLogListener(listener LogListener)

	// UnregisterLogListener removes the input listener from receiving new events
	UnregisterLogListener(listener LogListener)
}

// NodeLogDispatcher listens and maintains nodes of the network.
// Every time a node is added to the network, the internal list is extended.
// Log streams of all the nodes maintained in this registry are read and parsed,
// while the parsed blocks from the logs are distributed to all registered listeners.
// Furthermore, all collected logs are written to a configurable output directory.
type NodeLogDispatcher struct {
	listeners     map[LogListener]bool
	listenersLock sync.Mutex

	network driver.Network
	logDir  string
	wg      sync.WaitGroup

	nodes     map[Node]*nodeLogState
	nodesLock sync.Mutex
}

// nodeLogState records which node object's log stream is being consumed;
// source is nil while none is.
type nodeLogState struct {
	source driver.Node
}

// NewNodeLogDispatcher creates a new instance of this registry, which is filled
// by already running nodes, and further listens to newly added nodes.
func NewNodeLogDispatcher(network driver.Network, outputDir string) (*NodeLogDispatcher, error) {
	logDir := outputDir + "/node_logs"
	err := os.MkdirAll(logDir, 0700)
	if err != nil {
		return nil, fmt.Errorf("failed to create output directory: %v", err)
	}

	res := &NodeLogDispatcher{
		network:   network,
		listeners: make(map[LogListener]bool, 50),
		logDir:    logDir,
		nodes:     make(map[Node]*nodeLogState, 50),
	}

	// listen for new Nodes
	network.RegisterListener(res)

	// get nodes that have been started before this instance creation
	for _, node := range res.network.GetActiveNodes() {
		res.AfterNodeCreation(node)
	}

	return res, nil
}

// WaitForLogsToBeConsumed blocks until all goroutines that are currently
// active in consuming logs have completed. It is intended for synchronizing
// consumers in unit tests.
func (n *NodeLogDispatcher) WaitForLogsToBeConsumed() {
	n.wg.Wait()
}

func (n *NodeLogDispatcher) RegisterLogListener(listener LogListener) {
	n.listenersLock.Lock()
	defer n.listenersLock.Unlock()
	n.listeners[listener] = true
}

func (n *NodeLogDispatcher) UnregisterLogListener(listener LogListener) {
	n.listenersLock.Lock()
	defer n.listenersLock.Unlock()
	delete(n.listeners, listener)
}

func (n *NodeLogDispatcher) AfterNodeCreation(node driver.Node) {
	nodeId := Node(node.GetLabel())

	n.nodesLock.Lock()
	state, known := n.nodes[nodeId]
	if !known {
		state = &nodeLogState{}
		n.nodes[nodeId] = state
	}
	consuming := state.source == node
	n.nodesLock.Unlock()

	if known {
		n.notifyNodeRestart(nodeId)
	}

	// The stream of a client restarted in place stays open and keeps
	// delivering the new client's blocks; a second reader would report
	// every block twice.
	if consuming {
		return
	}

	logStream, err := node.StreamLog(context.Background())
	if err != nil {
		slog.Error(
			"failed to obtain logs of node, will not be able to track blocks",
			"error", err)
		return
	}
	n.nodesLock.Lock()
	state.source = node
	n.nodesLock.Unlock()

	n.wg.Add(1)
	go n.runLogCollector(node)

	n.wg.Add(1)
	n.startDispatcher(nodeId, node, logStream, state)
}

// BeforeNodeRemoval prepares listeners for replayed blocks before the
// stopped client comes back and starts speaking.
func (n *NodeLogDispatcher) BeforeNodeRemoval(node driver.Node) {
	n.notifyNodeRestart(Node(node.GetLabel()))
}

// notifyNodeRestart informs every listener that opted into restart
// notifications by implementing NodeRestartListener.
func (n *NodeLogDispatcher) notifyNodeRestart(node Node) {
	n.listenersLock.Lock()
	defer n.listenersLock.Unlock()
	for listener := range n.listeners {
		if l, ok := listener.(NodeRestartListener); ok {
			l.OnNodeRestart(node)
		}
	}
}

func (n *NodeLogDispatcher) AfterApplicationCreation(driver.Application) {
	// ignored
}

func (n *NodeLogDispatcher) startDispatcher(nodeId Node, source driver.Node, reader io.ReadCloser, state *nodeLogState) {
	go func() {
		defer n.wg.Done()
		defer func() {
			n.nodesLock.Lock()
			if state.source == source {
				state.source = nil
			}
			n.nodesLock.Unlock()
		}()
		defer func() {
			_ = reader.Close()
		}()
		ch := NewLogReader(reader)
		for b := range ch {
			n.listenersLock.Lock()
			for k := range n.listeners {
				k.OnBlock(nodeId, b)
			}
			n.listenersLock.Unlock()
		}
	}()
}

func (n *NodeLogDispatcher) runLogCollector(node driver.Node) {
	defer n.wg.Done()
	label := node.GetLabel()
	in, err := node.StreamLog(context.Background())
	if err != nil {
		slog.Error("failed to obtain logs of node, log is not captured",
			"node", label,
			"error", err)
		return
	}
	defer func() {
		if err := in.Close(); err != nil {
			slog.Error("failed to close log stream for node",
				"node", label,
				"error", err)
		}
	}()
	file := n.logDir + "/" + label + ".log"
	out, err := os.Create(file)
	if err != nil {
		slog.Error("failed to create log file for node, log is not captured",
			"file", file,
			"node", label,
			"error", err)
		return
	}
	defer func() {
		if err := out.Close(); err != nil {
			slog.Error("failed to close log file for node", "node", label, "error", err)
		}
	}()
	_, err = io.Copy(out, in)
	if err != nil {
		slog.Error("failed to capture log for node", "node", label, "error", err)
	}
}
