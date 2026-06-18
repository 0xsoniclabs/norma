package network

import (
	"math/big"
	"testing"

	"github.com/0xsoniclabs/norma/genesis"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/ethereum/go-ethereum/core/types"
	"go.uber.org/mock/gomock"
)

func TestApplyNetworkRules_Success(t *testing.T) {
	t.Parallel()

	baseFee := big.Int{}
	baseFee.SetInt64(123)
	header := types.Header{BaseFee: &baseFee}

	ctrl := gomock.NewController(t)
	backend := NewMockContractBackend(ctrl)
	backend.EXPECT().HeaderByNumber(gomock.Any(), gomock.Any()).Return(&header, nil)
	backend.EXPECT().PendingNonceAt(gomock.Any(), gomock.Any()).Return(uint64(0), nil)
	backend.EXPECT().SendTransaction(gomock.Any(), gomock.Any()).Return(nil)
	backend.EXPECT().WaitTransactionReceipt(gomock.Any()).Return(&types.Receipt{Status: types.ReceiptStatusSuccessful}, nil)
	backend.EXPECT().GetNetworkRules("latest").Return(opera.FakeNetRules(opera.GetSonicUpgrades()), nil)

	fee := genesis.BigIntValue(*big.NewInt(456))
	rules := genesis.NetworkRulesPatch{
		Economy: &genesis.EconomyPatch{
			MinBaseFee: &fee,
		},
	}

	if err := ApplyNetworkRules(backend, rules); err != nil {
		t.Errorf("failed to apply network rules: %v", err)
	}
}
