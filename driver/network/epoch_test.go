package network

import (
	big "math/big"
	"testing"

	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/sonic/gossip/contract/driverauth100"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	gomock "go.uber.org/mock/gomock"
)

func TestAdvanceEpoch_Success(t *testing.T) {
	t.Parallel()

	baseFee := big.Int{}
	baseFee.SetInt64(123)
	header := types.Header{BaseFee: &baseFee}

	bytecode, err := convertContractBytecode(driverauth100.ContractMetaData.Bin)
	if err != nil {
		t.Fatalf("failed to decode contract bytecode: %v", err)
	}

	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)

	// Obtaining the current epoch before advancing.
	client.EXPECT().Call(gomock.Any(), "eth_currentEpoch").DoAndReturn(
		func(result *hexutil.Uint64, _ string, _ ...any) error {
			*result = hexutil.Uint64(1)
			return nil
		},
	)

	client.EXPECT().HeaderByNumber(gomock.Any(), gomock.Any()).Return(&header, nil)
	client.EXPECT().SuggestGasTipCap(gomock.Any()).Return(&baseFee, nil)
	client.EXPECT().PendingCodeAt(gomock.Any(), gomock.Any()).Return(bytecode, nil)
	client.EXPECT().EstimateGas(gomock.Any(), gomock.Any()).Return(uint64(123), nil)
	client.EXPECT().PendingNonceAt(gomock.Any(), gomock.Any()).Return(uint64(0), nil)
	client.EXPECT().SendTransaction(gomock.Any(), gomock.Any()).Return(nil)
	client.EXPECT().WaitTransactionReceipt(gomock.Any()).Return(&types.Receipt{Status: types.ReceiptStatusSuccessful}, nil)

	// Report the updated epoch after advancing.
	client.EXPECT().Call(gomock.Any(), "eth_currentEpoch").DoAndReturn(
		func(result *hexutil.Uint64, _ string, _ ...any) error {
			*result = hexutil.Uint64(2)
			return nil
		},
	)

	// The epoch state log reporting.
	any := gomock.Any()
	client.EXPECT().CallContract(any, any, any).Return(nil, nil)
	client.EXPECT().CodeAt(any, any, any).Return(bytecode, nil)

	if err := AdvanceEpoch(client, 1); err != nil {
		t.Errorf("failed to advance epoch: %v", err)
	}
}
