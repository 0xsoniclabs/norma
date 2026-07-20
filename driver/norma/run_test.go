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

package main

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"go.uber.org/mock/gomock"
)

// ctxBlockingReader mimics a docker follow log stream on a running container:
// Read never returns EOF and only unblocks when the context is cancelled.
type ctxBlockingReader struct{ ctx context.Context }

func (r *ctxBlockingReader) Read([]byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func (r *ctxBlockingReader) Close() error { return nil }

// TestDumpNodeLogs_BoundedForRunningNode is a regression test: dumpNodeLogs must
// return even when a node's log stream never reaches EOF.
func TestDumpNodeLogs_BoundedForRunningNode(t *testing.T) {
	prev := logDumpTimeout
	logDumpTimeout = 100 * time.Millisecond
	defer func() { logDumpTimeout = prev }()

	ctrl := gomock.NewController(t)
	node := driver.NewMockNode(ctrl)
	node.EXPECT().GetLabel().Return("validator-0").AnyTimes()
	node.EXPECT().StreamLog(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (io.ReadCloser, error) {
			return &ctxBlockingReader{ctx: ctx}, nil
		})

	net := driver.NewMockNetwork(ctrl)
	net.EXPECT().GetActiveNodes().Return([]driver.Node{node})

	done := make(chan struct{})
	go func() {
		dumpNodeLogs(context.Background(), net)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("dumpNodeLogs blocked on a running node's follow stream")
	}
}
