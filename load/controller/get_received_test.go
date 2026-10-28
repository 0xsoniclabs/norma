package controller

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/load/app"
	"go.uber.org/mock/gomock"
)

func TestGetReceivedTransactions_DialsRpcWhenClientIsNil(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockApp := app.NewMockApplication(ctrl)
	mockNet := driver.NewMockNetwork(ctrl)
	mockRpc := rpc.NewMockClient(ctrl)

	ac := &AppController{
		application: mockApp,
		network:     mockNet,
		rpcClient:   nil, // simulate nil client (e.g. after a failed reconnect)
	}

	// Expect a dial, then a successful query.
	mockNet.EXPECT().DialRandomRpc().Return(mockRpc, nil)
	mockApp.EXPECT().GetReceivedTransactions(mockRpc).Return(uint64(42), nil)

	got, err := ac.GetReceivedTransactions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestGetReceivedTransactions_ReturnsErrorWhenDialFailsOnNilClient(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockApp := app.NewMockApplication(ctrl)
	mockNet := driver.NewMockNetwork(ctrl)

	ac := &AppController{
		application: mockApp,
		network:     mockNet,
		rpcClient:   nil,
	}

	dialErr := errors.New("connection refused")
	mockNet.EXPECT().DialRandomRpc().Return(nil, dialErr)

	_, err := ac.GetReceivedTransactions()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, dialErr) {
		t.Fatalf("expected wrapped dialErr, got: %v", err)
	}
}

func TestGetReceivedTransactions_SetsClientToNilOnReconnectFailure(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockApp := app.NewMockApplication(ctrl)
	mockNet := driver.NewMockNetwork(ctrl)
	mockRpc := rpc.NewMockClient(ctrl)

	ac := &AppController{
		application: mockApp,
		network:     mockNet,
		rpcClient:   mockRpc,
	}

	// First call fails, triggering a reconnect attempt.
	appErr := errors.New("rpc call failed")
	mockApp.EXPECT().GetReceivedTransactions(mockRpc).Return(uint64(0), appErr)
	mockRpc.EXPECT().Close()

	// Reconnect also fails.
	reconnectErr := errors.New("all nodes down")
	mockNet.EXPECT().DialRandomRpc().Return(nil, reconnectErr)

	_, err := ac.GetReceivedTransactions()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, reconnectErr) {
		t.Fatalf("expected wrapped reconnectErr, got: %v", err)
	}

	// The client must be nil so the next call re-dials instead of
	// using a closed client.
	if ac.rpcClient != nil {
		t.Fatal("expected rpcClient to be nil after reconnect failure")
	}
}

func TestGetReceivedTransactions_ConcurrentReadersShareOneClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockApp := app.NewMockApplication(ctrl)
	mockNet := driver.NewMockNetwork(ctrl)
	ac := &AppController{application: mockApp, network: mockNet}

	// Every dial hands out a new client that records when it is closed.
	var mu sync.Mutex
	dials := 0
	closed := map[rpc.Client]bool{}
	mockNet.EXPECT().DialRandomRpc().AnyTimes().DoAndReturn(func() (rpc.Client, error) {
		client := rpc.NewMockClient(ctrl)
		client.EXPECT().Close().AnyTimes().Do(func() {
			mu.Lock()
			defer mu.Unlock()
			closed[client] = true
		})
		mu.Lock()
		defer mu.Unlock()
		dials++
		return client, nil
	})

	// The first read fails, so its reader closes the client and re-dials,
	// while the other readers are in the middle of reading.
	var reads atomic.Int32
	mockApp.EXPECT().GetReceivedTransactions(gomock.Any()).AnyTimes().DoAndReturn(
		func(client rpc.Client) (uint64, error) {
			first := reads.Add(1) == 1
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			defer mu.Unlock()
			if closed[client] {
				t.Errorf("a read used a client that was closed")
			}
			if first {
				return 0, errors.New("node unreachable")
			}
			return 1, nil
		})

	start := make(chan struct{})
	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			<-start
			if _, err := ac.GetReceivedTransactions(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
	close(start)
	readers.Wait()

	if dials != 2 {
		t.Errorf("expected one dial and one re-dial, got %d dials", dials)
	}
}
