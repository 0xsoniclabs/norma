package rpc

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"go.uber.org/mock/gomock"
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
	mock.EXPECT().
		Call(gomock.Any(), "txpool_content", gomock.Any()).
		MinTimes(1)

	if _, err := client.WaitTransactionReceipt(common.Hash{}); err == nil || err.Error() != "failed to get transaction receipt: timeout, transaction pool status: not present" {
		t.Fatalf("expected timeout error, got %v", err)
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

func TestRpcClientImpl_WaitTransactionReceipt_TransientRetry(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mock := NewMockrpcClient(ctrl)
	client := Impl{
		rpcClient:        mock,
		txReceiptTimeout: 30 * time.Second,
	}

	injectedResult := map[string]any{
		"cumulativeGasUsed": "0x0",
		"logsBloom":         "0x" + strings.Repeat("00", 256),
		"logs":              []map[string]any{},
		"transactionHash":   "0x" + strings.Repeat("00", 32),
		"gasUsed":           "0x0",
	}

	call := 0
	mock.EXPECT().
		Call(gomock.Any(), "eth_getTransactionReceipt", gomock.Any()).
		DoAndReturn(func(result interface{}, method string, args ...interface{}) error {
			call++
			if call <= 2 {
				return errors.New("Post \"http://0.0.0.0:41851\": dial tcp 0.0.0.0:41851: connect: connection refused")
			}
			resultPtr, ok := result.(*map[string]any)
			if !ok {
				t.Fatalf("result type is not *map[string]any")
			}
			*resultPtr = injectedResult
			return nil
		}).
		Times(3)

	receipt, err := client.WaitTransactionReceipt(common.Hash{})
	if err != nil {
		t.Fatalf("expected no error after transient retries, got %v", err)
	}
	if receipt == nil {
		t.Fatal("expected non-nil receipt")
	}
}
