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
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"go.uber.org/mock/gomock"
)

func TestLogsParsersImplements(t *testing.T) {
	var inst NodeLogDispatcher
	var _ NodeLogProvider = &inst
}

func TestRegisterLogParser(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)

	node1 := driver.NewMockNode(ctrl)
	node2 := driver.NewMockNode(ctrl)
	node3 := driver.NewMockNode(ctrl)

	node1.EXPECT().GetLabel().AnyTimes().Return(string(Node1TestId))
	node2.EXPECT().GetLabel().AnyTimes().Return(string(Node2TestId))
	node3.EXPECT().GetLabel().AnyTimes().Return(string(Node3TestId))

	node1.EXPECT().StreamLog(gomock.Any()).AnyTimes().DoAndReturn(func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(Node1TestLog)), nil
	})
	node2.EXPECT().StreamLog(gomock.Any()).AnyTimes().DoAndReturn(func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(Node2TestLog)), nil
	})
	node3.EXPECT().StreamLog(gomock.Any()).AnyTimes().DoAndReturn(func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(Node3TestLog)), nil
	})

	// simulate existing nodes
	net.EXPECT().RegisterListener(gomock.Any())
	net.EXPECT().GetActiveNodes().AnyTimes().Return([]driver.Node{})

	dir := t.TempDir()
	reg, err := NewNodeLogDispatcher(net, dir)
	if err != nil {
		t.Fatalf("failed to create log dispatcher: %v", err)
	}
	ch := make(chan Node, 10)
	listener := &testBlockNodeListener{data: map[Node][]Block{}, ch: ch}
	reg.RegisterLogListener(listener)

	// simulate added node
	reg.AfterNodeCreation(node1)
	reg.AfterNodeCreation(node2)
	reg.AfterNodeCreation(node3)

	reg.WaitForLogsToBeConsumed()

	// drain 3 nodes from the channel
	for _, node := range []Node{<-ch, <-ch, <-ch} {
		got := listener.getBlocks(node)
		want := NodeBlockTestData[node]
		blockEqual(t, node, got, want)
	}

	// Check that log got copied to output files.
	logs := []struct {
		path, content string
	}{
		{dir + "/node_logs/A.log", Node1TestLog},
		{dir + "/node_logs/B.log", Node2TestLog},
		{dir + "/node_logs/C.log", Node3TestLog},
	}
	for _, log := range logs {
		content, err := os.ReadFile(log.path)
		if err != nil {
			t.Errorf("failed to read log file: %v", err)
			continue
		}
		if got, want := log.content, string(content); got != want {
			t.Errorf("invalid log, wanted:\n%s\ngot:\n%s", want, got)
		}
	}
}

type testBlockNodeListener struct {
	data     map[Node][]Block
	dataLock sync.Mutex
	ch       chan Node
}

func blockEqual(t *testing.T, node Node, got, want []Block) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("wrong blocks collected for Node %v: %v != %v", node, got, want)
	}

	for i, b := range got {
		if want[i].Height != b.Height || want[i].Txs != b.Txs || want[i].GasUsed != b.GasUsed {
			t.Errorf("wrong blocks collected for Node %v: %v != %v", node, want[i], b)
		}
	}
}

func (l *testBlockNodeListener) OnBlock(node Node, b Block) {
	l.dataLock.Lock()
	defer l.dataLock.Unlock()

	// send uniq nodes
	if _, exists := l.data[node]; !exists {
		l.ch <- node
	}

	// count in only non-empty blocks
	if b.Height > 0 {
		l.data[node] = append(l.data[node], b)
	}
}

func (l *testBlockNodeListener) getBlocks(node Node) []Block {
	l.dataLock.Lock()
	defer l.dataLock.Unlock()

	return l.data[node]
}

func TestNodeLogDispatcher_InPlaceRestartAttachesNoSecondReader(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any())
	net.EXPECT().GetActiveNodes().AnyTimes().Return([]driver.Node{})

	dispatchReader, dispatchWriter := io.Pipe()
	collectReader, collectWriter := io.Pipe()

	node := driver.NewMockNode(ctrl)
	node.EXPECT().GetLabel().AnyTimes().Return(string(Node1TestId))
	gomock.InOrder(
		node.EXPECT().StreamLog(gomock.Any()).Return(dispatchReader, nil),
		node.EXPECT().StreamLog(gomock.Any()).Return(collectReader, nil),
	)

	reg, err := NewNodeLogDispatcher(net, t.TempDir())
	if err != nil {
		t.Fatalf("failed to create log dispatcher: %v", err)
	}
	listener := newRestartRecordingListener()
	reg.RegisterLogListener(listener)

	reg.AfterNodeCreation(node)

	// In-place restart: the stream is still open, so no new readers may
	// attach (the mock rejects further StreamLog calls).
	reg.AfterNodeCreation(node)

	if _, err := dispatchWriter.Write([]byte(Node1TestLog)); err != nil {
		t.Fatalf("failed to write log: %v", err)
	}
	_ = dispatchWriter.Close()
	_ = collectWriter.Close()
	reg.WaitForLogsToBeConsumed()

	if got := listener.getRestarts(); len(got) != 1 || got[0] != Node1TestId {
		t.Errorf("expected one restart notification for %v, got %v", Node1TestId, got)
	}
	blockEqual(t, Node1TestId, listener.getBlocks(Node1TestId), NodeBlockTestData[Node1TestId])
}

func TestNodeLogDispatcher_ReattachesWhenRejoiningAfterStreamEnd(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any())
	net.EXPECT().GetActiveNodes().AnyTimes().Return([]driver.Node{})

	first := driver.NewMockNode(ctrl)
	first.EXPECT().GetLabel().AnyTimes().Return(string(Node1TestId))
	first.EXPECT().StreamLog(gomock.Any()).Times(2).DoAndReturn(func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(Node1TestLog)), nil
	})

	reg, err := NewNodeLogDispatcher(net, t.TempDir())
	if err != nil {
		t.Fatalf("failed to create log dispatcher: %v", err)
	}
	listener := newRestartRecordingListener()
	reg.RegisterLogListener(listener)

	reg.AfterNodeCreation(first)
	reg.WaitForLogsToBeConsumed()

	// Rejoin after the container was recreated: fresh readers must attach.
	second := driver.NewMockNode(ctrl)
	second.EXPECT().GetLabel().AnyTimes().Return(string(Node1TestId))
	second.EXPECT().StreamLog(gomock.Any()).Times(2).DoAndReturn(func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(Node2TestLog)), nil
	})

	reg.AfterNodeCreation(second)
	reg.WaitForLogsToBeConsumed()

	if got := listener.getRestarts(); len(got) != 1 || got[0] != Node1TestId {
		t.Errorf("expected one restart notification for %v, got %v", Node1TestId, got)
	}
	want := append(append([]Block{}, NodeBlockTestData[Node1TestId]...), NodeBlockTestData[Node2TestId]...)
	blockEqual(t, Node1TestId, listener.getBlocks(Node1TestId), want)
}

func TestNodeLogDispatcher_NodeSuspensionNotifiesRestartListeners(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().RegisterListener(gomock.Any())
	net.EXPECT().GetActiveNodes().AnyTimes().Return([]driver.Node{})

	node := driver.NewMockNode(ctrl)
	node.EXPECT().GetLabel().AnyTimes().Return(string(Node1TestId))

	reg, err := NewNodeLogDispatcher(net, t.TempDir())
	if err != nil {
		t.Fatalf("failed to create log dispatcher: %v", err)
	}
	listener := newRestartRecordingListener()
	reg.RegisterLogListener(listener)

	reg.BeforeNodeRemoval(node)

	if got := listener.getRestarts(); len(got) != 1 || got[0] != Node1TestId {
		t.Errorf("expected restart notification on suspension, got %v", got)
	}
}

type restartRecordingListener struct {
	mu       sync.Mutex
	blocks   map[Node][]Block
	restarts []Node
}

func newRestartRecordingListener() *restartRecordingListener {
	return &restartRecordingListener{blocks: map[Node][]Block{}}
}

func (l *restartRecordingListener) OnBlock(node Node, b Block) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if b.Height > 0 {
		l.blocks[node] = append(l.blocks[node], b)
	}
}

func (l *restartRecordingListener) OnNodeRestart(node Node) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.restarts = append(l.restarts, node)
}

func (l *restartRecordingListener) getBlocks(node Node) []Block {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Block{}, l.blocks[node]...)
}

func (l *restartRecordingListener) getRestarts() []Node {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Node{}, l.restarts...)
}
