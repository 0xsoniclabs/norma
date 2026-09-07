package app

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/genesis"
	contract "github.com/0xsoniclabs/norma/load/contracts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestNewContext_DoesNotDeployHelperContract(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockRpc := rpc.NewMockClient(ctrl)
	factory := NewMockRpcClientFactory(ctrl)
	factory.EXPECT().DialSystemRpc().Return(mockRpc, nil)

	mockRpc.EXPECT().Close()

	// NewContext should succeed without any contract deployment calls.
	// If it tried to deploy, it would call GetTransactOptions which needs
	// ChainID, SuggestGasPrice, PendingNonceAt — none of which are mocked.
	ctx, err := NewContext(context.Background(), factory, nil, genesis.NetworkRulesPatch{})
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
	factory.EXPECT().DialSystemRpc().Return(mockRpc, nil)

	ctx, err := NewContext(context.Background(), factory, nil, genesis.NetworkRulesPatch{})
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

// The nonce for each batch is advanced from the one just spent rather than read
// from the node again, which would report the nonce the previous batch used until
// that transaction reached the pool — letting the next batch reuse it and quietly
// replace its predecessor.
func TestAppContext_FundAccounts_AdvancesNonceWithoutRereadingIt(t *testing.T) {
	require := require.New(t)
	ctrl := gomock.NewController(t)

	const startNonce = uint64(7)
	// Two batches: FundAccounts groups receivers 32 at a time.
	const receivers = 33

	mockRpc := rpc.NewMockClient(ctrl)
	factory := NewMockRpcClientFactory(ctrl)
	factory.EXPECT().DialSystemRpc().Return(mockRpc, nil)

	// Fakenet validator 1, the account the treasury holds.
	const treasuryKey = "163f5f0f9a621d72fedd85ffca3d08d131ab4e812181e0d30ffd1c885d20aac7"
	treasury, err := NewAccount(0, treasuryKey, 1)
	require.NoError(err)

	appCtx, err := NewContext(context.Background(), factory, treasury, genesis.NetworkRulesPatch{})
	require.NoError(err)
	defer appCtx.Close()

	mockRpc.EXPECT().Close()
	mockRpc.EXPECT().ChainID(gomock.Any()).Return(big.NewInt(1), nil)
	mockRpc.EXPECT().SuggestGasPrice(gomock.Any()).Return(big.NewInt(1), nil)
	mockRpc.EXPECT().PendingCodeAt(gomock.Any(), gomock.Any()).Return([]byte{0x1}, nil).AnyTimes()
	mockRpc.EXPECT().EstimateGas(gomock.Any(), gomock.Any()).Return(uint64(1_000_000), nil).AnyTimes()

	// Exactly once: reading it a second time is the bug this guards.
	mockRpc.EXPECT().PendingNonceAt(gomock.Any(), treasury.address).Return(startNonce, nil).Times(1)

	var sent []*types.Transaction
	mockRpc.EXPECT().SendTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, tx *types.Transaction) error {
			sent = append(sent, tx)
			return nil
		}).Times(2)
	mockRpc.EXPECT().WaitTransactionReceipt(gomock.Any(), gomock.Any()).
		Return(&types.Receipt{Status: types.ReceiptStatusSuccessful}, nil).Times(2)

	ac := appCtx.(*appContext)
	// Install the helper directly; deploying it is not what is under test.
	helper, err := contract.NewHelper(common.Address{0x1}, mockRpc)
	require.NoError(err)
	ac.helper = helper

	accounts := make([]common.Address, receivers)
	for i := range accounts {
		accounts[i] = common.Address{byte(i + 1)}
	}

	require.NoError(ac.FundAccounts(accounts, big.NewInt(1000)))

	require.Len(sent, 2)
	require.Equal(startNonce, sent[0].Nonce())
	require.Equal(startNonce+1, sent[1].Nonce(), "the second batch must not reuse the first batch's nonce")
}
