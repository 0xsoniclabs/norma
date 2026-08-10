package rpc

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/sonic/opera"
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

	receipt, err := client.WaitTransactionReceipt(context.Background(), common.Hash{})
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

	if _, err := client.WaitTransactionReceipt(context.Background(), common.Hash{}); err == nil || err.Error() != "failed to get transaction receipt: timeout, transaction pool status: not present" {
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

	if _, err := client.WaitTransactionReceipt(context.Background(), common.Hash{}); !errors.Is(err, injectedError) {
		t.Fatalf("expected error %v, got %v", injectedError, err)
	}
}

func TestGetNetworkRules_ReturnsRules_WhenRulesAreAvailable(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mock := NewMockrpcClient(ctrl)
	client := Impl{rpcClient: mock}

	injectedRules := opera.Rules{Name: "test-rules"}

	mock.EXPECT().
		Call(gomock.Any(), "eth_getRules", "latest").
		DoAndReturn(func(result interface{}, method string, args ...interface{}) error {
			resultPtr, ok := result.(**opera.Rules)
			if !ok {
				t.Fatalf("result type is not **opera.Rules")
			}
			*resultPtr = &injectedRules
			return nil
		})

	rules, err := client.GetNetworkRules("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got, want := rules.Name, injectedRules.Name; got != want {
		t.Errorf("got rules name %q, want %q", got, want)
	}
}

func TestGetNetworkRules_ReturnsError_WhenResultIsNull(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mock := NewMockrpcClient(ctrl)
	client := Impl{rpcClient: mock}

	mock.EXPECT().
		Call(gomock.Any(), "eth_getRules", "latest").
		Return(nil)

	if _, err := client.GetNetworkRules("latest"); err == nil {
		t.Fatal("expected error for null rules result, got nil")
	}
}

func TestRpcClientImpl_GetNetworkRules_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mock := NewMockrpcClient(ctrl)
	client := Impl{rpcClient: mock}

	injectedError := errors.New("injected error")

	mock.EXPECT().
		Call(gomock.Any(), "eth_getRules", "latest").
		Return(injectedError)

	if _, err := client.GetNetworkRules("latest"); !errors.Is(err, injectedError) {
		t.Fatalf("expected error %v, got %v", injectedError, err)
	}
}
