package app

import (
	"fmt"
	"testing"

	"github.com/0xsoniclabs/norma/driver/rpc"
	"go.uber.org/mock/gomock"
)

func TestNewContext_DoesNotDeployHelperContract(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRpc := rpc.NewMockClient(ctrl)
	factory := NewMockRpcClientFactory(ctrl)
	factory.EXPECT().DialRandomRpc().Return(mockRpc, nil)

	mockRpc.EXPECT().Close()

	// NewContext should succeed without any contract deployment calls.
	// If it tried to deploy, it would call GetTransactOptions which needs
	// ChainID, SuggestGasPrice, PendingNonceAt — none of which are mocked.
	ctx, err := NewContext(factory, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer ctx.Close()

	// Verify the internal helper is nil (lazy).
	ac := ctx.(*appContext)
	if ac.helper != nil {
		t.Fatal("expected helper to be nil after NewContext (lazy deployment)")
	}
}

func TestFundAccounts_AttemptsToDeployHelperOnFirstCall(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRpc := rpc.NewMockClient(ctrl)
	factory := NewMockRpcClientFactory(ctrl)
	factory.EXPECT().DialRandomRpc().Return(mockRpc, nil)

	ctx, err := NewContext(factory, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer ctx.Close()

	mockRpc.EXPECT().Close()

	ac := ctx.(*appContext)

	// Mock the RPC calls that GetTransactOptions needs for deploying the helper.
	mockRpc.EXPECT().ChainID(gomock.Any()).Return(nil, fmt.Errorf("simulated chain ID failure"))

	// FundAccounts will try to deploy the helper, which calls GetTransactOptions,
	// which calls ChainID — our mock returns an error so we can verify the lazy
	// deployment path is taken without needing a full chain.
	err = ac.FundAccounts(nil, nil)
	if err == nil {
		t.Fatal("expected error from FundAccounts")
	}

	// The error should come from the deploy path (transaction options).
	if ac.helper != nil {
		t.Fatal("helper should remain nil when deployment fails")
	}
}
