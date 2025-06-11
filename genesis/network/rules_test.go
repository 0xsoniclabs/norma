package network

import (
	"fmt"
	"github.com/0xsoniclabs/norma/genesistools/genesis"
	"github.com/0xsoniclabs/sonic/gossip/contract/driverauth100"
	"github.com/ethereum/go-ethereum/core/types"
	"go.uber.org/mock/gomock"
	"math/big"
	"testing"
)

func TestLocalNetworkApplyNetworkRules_Success(t *testing.T) {
	t.Parallel()

	baseFee := big.Int{}
	baseFee.SetInt64(123)
	header := types.Header{BaseFee: &baseFee}

	bytecode, err := convertContractBytecode(driverauth100.ContractMetaData.Bin)
	if err != nil {
		t.Fatalf("failed to decode contract bytecode: %v", err)
	}

	ctrl := gomock.NewController(t)
	backend := NewMockContractBackend(ctrl)
	backend.EXPECT().HeaderByNumber(gomock.Any(), gomock.Any()).Return(&header, nil)
	backend.EXPECT().SuggestGasTipCap(gomock.Any()).Return(&baseFee, nil)
	backend.EXPECT().PendingCodeAt(gomock.Any(), gomock.Any()).Return(bytecode, nil)
	backend.EXPECT().EstimateGas(gomock.Any(), gomock.Any()).Return(uint64(123), nil)
	backend.EXPECT().PendingNonceAt(gomock.Any(), gomock.Any()).Return(uint64(0), nil)
	backend.EXPECT().SendTransaction(gomock.Any(), gomock.Any()).Return(nil)
	backend.EXPECT().WaitTransactionReceipt(gomock.Any()).Return(&types.Receipt{Status: types.ReceiptStatusSuccessful}, nil)

	const fee = 456
	rules := genesis.NetworkRules{}
	rules["MIN_BASE_FEE"] = fmt.Sprintf("%d", fee)

	if err := ApplyNetworkRules(backend, rules); err != nil {
		t.Errorf("failed to apply network rules: %v", err)
	}
}

func TestLocalNetworkAdvanceEpoch_Success(t *testing.T) {
	t.Parallel()

	baseFee := big.Int{}
	baseFee.SetInt64(123)
	header := types.Header{BaseFee: &baseFee}

	bytecode, err := convertContractBytecode(driverauth100.ContractMetaData.Bin)
	if err != nil {
		t.Fatalf("failed to decode contract bytecode: %v", err)
	}

	ctrl := gomock.NewController(t)
	backend := NewMockContractBackend(ctrl)
	backend.EXPECT().HeaderByNumber(gomock.Any(), gomock.Any()).Return(&header, nil)
	backend.EXPECT().SuggestGasTipCap(gomock.Any()).Return(&baseFee, nil)
	backend.EXPECT().PendingCodeAt(gomock.Any(), gomock.Any()).Return(bytecode, nil)
	backend.EXPECT().EstimateGas(gomock.Any(), gomock.Any()).Return(uint64(123), nil)
	backend.EXPECT().PendingNonceAt(gomock.Any(), gomock.Any()).Return(uint64(0), nil)
	backend.EXPECT().SendTransaction(gomock.Any(), gomock.Any()).Return(nil)
	backend.EXPECT().WaitTransactionReceipt(gomock.Any()).Return(&types.Receipt{Status: types.ReceiptStatusSuccessful}, nil)

	const epochIncrement = 4
	if err := AdvanceEpoch(backend, epochIncrement); err != nil {
		t.Errorf("failed to advance epoch: %v", err)
	}
}
