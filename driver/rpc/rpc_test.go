package rpc

import (
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"go.uber.org/mock/gomock"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRpcClientImpl_WaitTransactionReceipt_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mock := NewMockrpcClient(ctrl)
	client := Impl{
		rpcClient:        mock,
		txReceiptTimeout: time.Hour,
	}

	injectedResult := map[string]any{
		"cumulativeGasUsed": "0x0",
		"logsBloom":         "0x" + strings.Repeat("00", 256),
		"logs":              []map[string]any{},
		"transactionHash":   "0x" + strings.Repeat("00", 32),
		"gasUsed":           "0x0",
	}
	expectedReceipt := &types.Receipt{
		CumulativeGasUsed: 0,
		Bloom:             types.BytesToBloom(make([]byte, 256)),
		Logs:              nil,
		TxHash:            common.BytesToHash(make([]byte, 32)),
		GasUsed:           0,
	}

	mock.EXPECT().
		Call(gomock.Any(), "eth_getTransactionReceipt", gomock.Any()).
		DoAndReturn(func(result interface{}, method string, args ...interface{}) error {
			resultPtr, ok := result.(*map[string]any)
			if !ok {
				t.Fatalf("result type is not *map[string]any")
			}
			*resultPtr = injectedResult
			return nil
		})

	receipt, err := client.WaitTransactionReceipt(common.Hash{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got, want := receipt, expectedReceipt; reflect.DeepEqual(got, want) {
		t.Errorf("got receipt %v, want %v", got, want)
	}
}

func TestRpcClientImpl_WaitTransactionReceipt_Timeout(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mock := NewMockrpcClient(ctrl)
	client := Impl{
		rpcClient:        mock,
		txReceiptTimeout: 10 * time.Second,
	}

	mock.EXPECT().
		Call(gomock.Any(), "eth_getTransactionReceipt", gomock.Any()).
		DoAndReturn(func(result interface{}, method string, args ...interface{}) error {
			resultPtr, ok := result.(*map[string]any)
			if !ok {
				t.Fatalf("result type is not *map[string]any")
			}
			*resultPtr = nil
			return nil
		}).
		AnyTimes()

	if _, err := client.WaitTransactionReceipt(common.Hash{}); err == nil || err.Error() != "failed to get transaction receipt: timeout" {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestRpcClientImpl_SendTxWithRetry_ReBroadcastsUntilReceipt(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	rpcMock := NewMockrpcClient(ctrl)
	ethMock := NewMockethRpcClient(ctrl)
	client := Impl{
		ethRpcClient:     ethMock,
		rpcClient:        rpcMock,
		txReceiptTimeout: time.Second,
		txResendInterval: 10 * time.Millisecond,
	}

	tx := types.NewTx(&types.LegacyTx{Nonce: 1})

	// The receipt is not found for the first few polls, then becomes available.
	polls := 0
	rpcMock.EXPECT().
		Call(gomock.Any(), "eth_getTransactionReceipt", gomock.Any()).
		DoAndReturn(func(result interface{}, method string, args ...interface{}) error {
			resultPtr, ok := result.(*map[string]any)
			if !ok {
				t.Fatalf("result type is not *map[string]any")
			}
			polls++
			if polls < 5 {
				*resultPtr = nil // ethereum.NotFound
				return nil
			}
			*resultPtr = map[string]any{
				"cumulativeGasUsed": "0x0",
				"logsBloom":         "0x" + strings.Repeat("00", 256),
				"logs":              []map[string]any{},
				"transactionHash":   "0x" + strings.Repeat("00", 32),
				"gasUsed":           "0x0",
			}
			return nil
		}).
		AnyTimes()

	// While waiting, the transaction must be (re-)broadcast more than once.
	ethMock.EXPECT().
		SendTransaction(gomock.Any(), gomock.Any()).
		Return(nil).
		MinTimes(2)

	receipt, err := client.SendTxWithRetry(tx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if receipt == nil {
		t.Fatalf("expected a receipt, got nil")
	}
}

func TestRpcClientImpl_SendTxWithRetry_InitialSendErrorIsFatal(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	rpcMock := NewMockrpcClient(ctrl)
	ethMock := NewMockethRpcClient(ctrl)
	client := Impl{
		ethRpcClient:     ethMock,
		rpcClient:        rpcMock,
		txReceiptTimeout: time.Hour, // would block for an hour if it wrongly polled
		txResendInterval: 10 * time.Millisecond,
	}

	tx := types.NewTx(&types.LegacyTx{Nonce: 1})

	injectedError := errors.New("transaction underpriced")
	ethMock.EXPECT().
		SendTransaction(gomock.Any(), gomock.Any()).
		Return(injectedError).
		Times(1)

	// The receipt must never be polled when the initial broadcast fails: no
	// Call expectation is registered, so any poll would fail the test.

	if _, err := client.SendTxWithRetry(tx); !errors.Is(err, injectedError) {
		t.Fatalf("expected the initial send error, got %v", err)
	}
}

func TestRpcClientImpl_SendTxWithRetry_IgnoresReBroadcastErrors(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	rpcMock := NewMockrpcClient(ctrl)
	ethMock := NewMockethRpcClient(ctrl)
	client := Impl{
		ethRpcClient:     ethMock,
		rpcClient:        rpcMock,
		txReceiptTimeout: time.Second,
		txResendInterval: 10 * time.Millisecond,
	}

	tx := types.NewTx(&types.LegacyTx{Nonce: 1})

	// The initial broadcast succeeds; subsequent re-broadcasts fail (e.g. the tx is
	// already known) and must be ignored.
	gomock.InOrder(
		ethMock.EXPECT().SendTransaction(gomock.Any(), gomock.Any()).Return(nil),
		ethMock.EXPECT().SendTransaction(gomock.Any(), gomock.Any()).
			Return(errors.New("already known")).AnyTimes(),
	)

	polls := 0
	rpcMock.EXPECT().
		Call(gomock.Any(), "eth_getTransactionReceipt", gomock.Any()).
		DoAndReturn(func(result interface{}, method string, args ...interface{}) error {
			resultPtr := result.(*map[string]any)
			polls++
			if polls < 4 {
				*resultPtr = nil // ethereum.NotFound
				return nil
			}
			*resultPtr = map[string]any{
				"cumulativeGasUsed": "0x0",
				"logsBloom":         "0x" + strings.Repeat("00", 256),
				"logs":              []map[string]any{},
				"transactionHash":   "0x" + strings.Repeat("00", 32),
				"gasUsed":           "0x0",
			}
			return nil
		}).
		AnyTimes()

	receipt, err := client.SendTxWithRetry(tx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if receipt == nil {
		t.Fatalf("expected a receipt, got nil")
	}
}

func TestRpcClientImpl_WaitTransactionReceipt_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mock := NewMockrpcClient(ctrl)
	client := Impl{
		rpcClient:        mock,
		txReceiptTimeout: time.Hour,
	}

	injectedError := errors.New("injectedError")

	mock.EXPECT().
		Call(gomock.Any(), "eth_getTransactionReceipt", gomock.Any()).
		Return(injectedError).
		Times(1)

	if _, err := client.WaitTransactionReceipt(common.Hash{}); !errors.Is(err, injectedError) {
		t.Fatalf("expected error %v, got %v", injectedError, err)
	}
}
