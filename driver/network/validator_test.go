package network

import (
	"fmt"
	"github.com/0xsoniclabs/sonic/gossip/contract/sfc100"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/types"
	"go.uber.org/mock/gomock"
	"math/big"
	"testing"
)

func TestRegisterValidatorNode_Success(t *testing.T) {
	mockBackendForCreateValidator(t, func(backend *MockContractBackend) {
		backend.EXPECT().WaitTransactionReceipt(gomock.Any()).Return(&types.Receipt{Status: types.ReceiptStatusSuccessful}, nil)

		valId, err := RegisterValidatorNode(backend, nil)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if valId <= 0 {
			t.Errorf("expected valid validator ID, got %d", valId)
		}
	})
}

func TestRegisterValidatorNode_Failure(t *testing.T) {
	mockBackendForCreateValidator(t, func(backend *MockContractBackend) {
		backend.EXPECT().WaitTransactionReceipt(gomock.Any()).Return(nil, fmt.Errorf("failed to get receipt")).AnyTimes()

		if _, err := RegisterValidatorNode(backend, nil); err == nil {
			t.Errorf("expected error, got %v", err)
		}
	})
}

func TestRegisterValidatorNode_Failure_TransactionReverted(t *testing.T) {
	mockBackendForCreateValidator(t, func(backend *MockContractBackend) {
		backend.EXPECT().WaitTransactionReceipt(gomock.Any()).Return(&types.Receipt{Status: types.ReceiptStatusFailed}, nil).AnyTimes()

		if _, err := RegisterValidatorNode(backend, nil); err == nil {
			t.Errorf("expected error, got %v", err)
		}
	})
}

func mockBackendForCreateValidator(t *testing.T, test func(backend *MockContractBackend)) {
	t.Parallel()

	bytecode, err := convertContractBytecode(sfc100.ContractMetaData.Bin)
	if err != nil {
		t.Fatalf("failed to decode contract bytecode: %v", err)
	}

	uint256Type, err := abi.NewType("uint256", "", nil)
	if err != nil {
		t.Fatalf("failed to create uint256 type: %v", err)
	}

	// Create an Arguments object with a single uint256 argument
	args := abi.Arguments{
		{
			Type: uint256Type,
		},
	}

	// Pack the value to return from the contract
	value := big.NewInt(12)
	lastValIdPacked, err := args.Pack(value)
	if err != nil {
		t.Fatalf("failed to pack value: %v", err)
	}

	baseFee := big.Int{}
	baseFee.SetInt64(123)
	header := types.Header{BaseFee: &baseFee}

	ctrl := gomock.NewController(t)
	backend := NewMockContractBackend(ctrl)

	backend.EXPECT().CallContract(gomock.Any(), gomock.Any(), gomock.Any()).Return(lastValIdPacked, nil)
	backend.EXPECT().HeaderByNumber(gomock.Any(), gomock.Any()).Return(&header, nil)
	backend.EXPECT().SuggestGasTipCap(gomock.Any()).Return(&baseFee, nil)
	backend.EXPECT().PendingCodeAt(gomock.Any(), gomock.Any()).Return(bytecode, nil)
	backend.EXPECT().EstimateGas(gomock.Any(), gomock.Any()).Return(uint64(123), nil)
	backend.EXPECT().PendingNonceAt(gomock.Any(), gomock.Any()).Return(uint64(0), nil)
	backend.EXPECT().SendTransaction(gomock.Any(), gomock.Any()).Return(nil)

	test(backend)
}
