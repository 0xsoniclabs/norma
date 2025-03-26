package rpc

import (
	"errors"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"go.uber.org/mock/gomock"
	"testing"
	"time"
)

func TestRpcClientImpl_WaitTransactionReceipt_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mock := NewMockethRpcClient(ctrl)
	client := Impl{
		ethRpcClient:     mock,
		txReceiptTimeout: time.Hour,
	}

	expectedReceipt := &types.Receipt{}

	mock.EXPECT().TransactionReceipt(gomock.Any(), gomock.Any()).Return(expectedReceipt, nil)

	receipt, err := client.WaitTransactionReceipt(common.Hash{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got, want := receipt, expectedReceipt; got != want {
		t.Errorf("got receipt %v, want %v", got, want)
	}
}

func TestRpcClientImpl_WaitTransactionReceipt_Timeout(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mock := NewMockethRpcClient(ctrl)
	client := Impl{
		ethRpcClient:     mock,
		txReceiptTimeout: 10 * time.Second,
	}

	mock.EXPECT().TransactionReceipt(gomock.Any(), gomock.Any()).Return(nil, ethereum.NotFound).AnyTimes()

	if _, err := client.WaitTransactionReceipt(common.Hash{}); err == nil || err.Error() != "failed to get transaction receipt: timeout" {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestRpcClientImpl_WaitTransactionReceipt_Error(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)

	mock := NewMockethRpcClient(ctrl)
	client := Impl{
		ethRpcClient:     mock,
		txReceiptTimeout: time.Hour,
	}

	injectedError := errors.New("injectedError")

	mock.EXPECT().TransactionReceipt(gomock.Any(), gomock.Any()).Return(nil, injectedError).Times(1)

	if _, err := client.WaitTransactionReceipt(common.Hash{}); !errors.Is(err, injectedError) {
		t.Fatalf("expected error %v, got %v", injectedError, err)
	}
}
