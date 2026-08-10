// Copyright 2024 Fantom Foundation
// This file is part of Norma System Testing Infrastructure for Sonic.
//
// Norma is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Norma is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with Norma. If not, see <http://www.gnu.org/licenses/>.

package network

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/sonic/gossip/contract/sfc100"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	gomock "go.uber.org/mock/gomock"
)

// insufficientFundsError is the rejection a Sonic node returns when an account
// cannot cover value + gas * feeCap of the transaction it submits.
const insufficientFundsError = "insufficient funds for gas * price + value"

func TestDelegateToValidator_ZeroStakeIsRejectedWithoutTouchingTheChain(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)
	// No client interaction is expected at all.

	err := DelegateToValidator(t.Context(), client, 1, 0, testKey(t))
	if err == nil {
		t.Fatal("expected an error for a zero stake, got nil")
	}
	if !strings.Contains(err.Error(), "greater than zero") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDelegateToValidator_SubmitsStakeAsTransactionValue(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)

	var submitted *types.Transaction
	expectTransactionSubmission(client, &submitted, nil)
	client.EXPECT().WaitTransactionReceipt(gomock.Any(), gomock.Any()).
		Return(&types.Receipt{Status: types.ReceiptStatusSuccessful}, nil)

	if err := DelegateToValidator(t.Context(), client, 3, 1_000, testKey(t)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := stakeToWei(1_000)
	if got := submitted.Value(); got.Cmp(want) != 0 {
		t.Errorf("expected a transaction value of %v wei, got %v", want, got)
	}
	if got := submitted.Gas(); got != delegationGasLimit {
		t.Errorf("expected the fixed delegation gas limit %d, got %d", delegationGasLimit, got)
	}
	if got := decodeUint256Arg(t, "delegate", submitted.Data(), 0); got.Int64() != 3 {
		t.Errorf("expected validator id 3 in the call data, got %v", got)
	}
}

// A delegator account that cannot cover stake + gas is rejected by the node at
// submission time; the rejection must not be mistaken for a successful
// delegation. This is the only layer at which under-funding is observable:
// the executor always funds the account before delegating, so a scenario
// cannot express this case.
func TestDelegateToValidator_InsufficientFundsIsReportedAsSubmissionFailure(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)

	var submitted *types.Transaction
	expectTransactionSubmission(client, &submitted, fmt.Errorf("%s", insufficientFundsError))
	// The receipt must never be awaited for a transaction that was refused.

	err := DelegateToValidator(t.Context(), client, 1, 1_000, testKey(t))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), insufficientFundsError) {
		t.Errorf("expected the node rejection to be propagated, got: %v", err)
	}
}

func TestDelegateToValidator_RevertedTransactionIsReported(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)

	var submitted *types.Transaction
	expectTransactionSubmission(client, &submitted, nil)
	client.EXPECT().WaitTransactionReceipt(gomock.Any(), gomock.Any()).
		Return(&types.Receipt{Status: types.ReceiptStatusFailed}, nil)

	err := DelegateToValidator(t.Context(), client, 1, 1_000, testKey(t))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "reverted") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Undelegating without an explicit amount from a validator the delegator never
// staked on must fail before a transaction is submitted: the SFC contract would
// revert on a zero-amount undelegation, and a silent no-op would let a scenario
// believe stake was moved.
func TestUndelegateFromValidator_WithoutStakeIsRejectedBeforeSubmission(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)

	expectStakeQuery(t, client, big.NewInt(0))
	// No transaction submission is expected.

	err := UndelegateFromValidator(t.Context(), client, 7, 0, testKey(t))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no stake on validator 7") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUndelegateFromValidator_WithoutExplicitAmountUndelegatesTheQueriedStake(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)

	onChain := stakeToWei(2_500)
	expectStakeQuery(t, client, onChain)
	var submitted *types.Transaction
	expectTransactionSubmission(client, &submitted, nil)
	client.EXPECT().WaitTransactionReceipt(gomock.Any(), gomock.Any()).
		Return(&types.Receipt{Status: types.ReceiptStatusSuccessful}, nil)

	if err := UndelegateFromValidator(t.Context(), client, 7, 0, testKey(t)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// undelegate(toValidatorID, wrID, amount)
	if got := decodeUint256Arg(t, "undelegate", submitted.Data(), 2); got.Cmp(onChain) != 0 {
		t.Errorf("expected the full on-chain stake %v to be undelegated, got %v", onChain, got)
	}
}

func TestUndelegateFromValidator_StakeQueryFailureIsReported(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)

	client.EXPECT().CallContract(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("connection reset"))

	err := UndelegateFromValidator(t.Context(), client, 7, 0, testKey(t))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to query stake") {
		t.Errorf("unexpected error: %v", err)
	}
}

// An explicit amount is taken at face value: the on-chain stake is not queried,
// so over-undelegating is only caught by the contract itself.
func TestUndelegateFromValidator_ExplicitAmountSkipsTheStakeQuery(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)

	var submitted *types.Transaction
	expectTransactionSubmission(client, &submitted, nil)
	client.EXPECT().WaitTransactionReceipt(gomock.Any(), gomock.Any()).
		Return(&types.Receipt{Status: types.ReceiptStatusFailed}, nil)

	err := UndelegateFromValidator(t.Context(), client, 7, 9_000, testKey(t))
	if err == nil {
		t.Fatal("expected an error for a reverted over-undelegation, got nil")
	}
	if got := decodeUint256Arg(t, "undelegate", submitted.Data(), 2); got.Cmp(stakeToWei(9_000)) != 0 {
		t.Errorf("expected the requested amount to be submitted unchanged, got %v", got)
	}
}

func TestGetDelegatorStake_ConvertsWeiToWholeS(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		wei  *big.Int
		want uint64
	}{
		"no stake":     {big.NewInt(0), 0},
		"whole amount": {stakeToWei(1_000), 1_000},
		"rounded down": {new(big.Int).Add(stakeToWei(1_000), big.NewInt(9e17)), 1_000},
		"below one S":  {big.NewInt(9e17), 0},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := rpc.NewMockClient(ctrl)
			expectStakeQuery(t, client, test.wei)

			got, err := GetDelegatorStake(client, common.Address{1}, 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.want {
				t.Errorf("expected %d S, got %d S", test.want, got)
			}
		})
	}
}

func TestFundAccount_TransfersTheRequestedAmount(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)

	var submitted *types.Transaction
	client.EXPECT().PendingNonceAt(gomock.Any(), gomock.Any()).Return(uint64(7), nil)
	client.EXPECT().SendTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, tx *types.Transaction) error {
			submitted = tx
			return nil
		})
	client.EXPECT().WaitTransactionReceipt(gomock.Any(), gomock.Any()).
		Return(&types.Receipt{Status: types.ReceiptStatusSuccessful}, nil)

	dest := common.Address{0xAB}
	if err := FundAccount(t.Context(), client, testKey(t), dest, 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := submitted.To(); got == nil || *got != dest {
		t.Errorf("expected a transfer to %s, got %v", dest.Hex(), got)
	}
	if got, want := submitted.Value(), stakeToWei(42); got.Cmp(want) != 0 {
		t.Errorf("expected %v wei to be transferred, got %v", want, got)
	}
	if got := submitted.Nonce(); got != 7 {
		t.Errorf("expected the pending nonce 7 to be used, got %d", got)
	}
	if got := submitted.Gas(); got != fundingGasLimit {
		t.Errorf("expected the plain-transfer gas limit %d, got %d", fundingGasLimit, got)
	}
}

// The treasury account funds every delegator; when it runs dry the failure has
// to surface here rather than as an unexplained delegation error later on.
func TestFundAccount_InsufficientTreasuryFundsIsReported(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)

	client.EXPECT().PendingNonceAt(gomock.Any(), gomock.Any()).Return(uint64(0), nil)
	client.EXPECT().SendTransaction(gomock.Any(), gomock.Any()).
		Return(fmt.Errorf("%s", insufficientFundsError))

	err := FundAccount(t.Context(), client, testKey(t), common.Address{1}, 1_000)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), insufficientFundsError) {
		t.Errorf("expected the node rejection to be propagated, got: %v", err)
	}
}

func TestFundAccount_NonceQueryFailureIsReported(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)

	client.EXPECT().PendingNonceAt(gomock.Any(), gomock.Any()).
		Return(uint64(0), fmt.Errorf("connection reset"))

	err := FundAccount(t.Context(), client, testKey(t), common.Address{1}, 1_000)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to get pending nonce") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFundAccount_RevertedTransferIsReported(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	client := rpc.NewMockClient(ctrl)

	client.EXPECT().PendingNonceAt(gomock.Any(), gomock.Any()).Return(uint64(0), nil)
	client.EXPECT().SendTransaction(gomock.Any(), gomock.Any()).Return(nil)
	client.EXPECT().WaitTransactionReceipt(gomock.Any(), gomock.Any()).
		Return(&types.Receipt{Status: types.ReceiptStatusFailed}, nil)

	err := FundAccount(t.Context(), client, testKey(t), common.Address{1}, 1_000)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "reverted") {
		t.Errorf("unexpected error: %v", err)
	}
}

func testKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to create a test key: %v", err)
	}
	return key
}

// expectTransactionSubmission sets up the calls the bind layer makes for a
// contract transaction with a fixed gas limit: base fee lookup, nonce lookup
// and the submission itself. The submitted transaction is captured in tx.
func expectTransactionSubmission(
	client *rpc.MockClient,
	tx **types.Transaction,
	sendErr error,
) {
	header := types.Header{BaseFee: big.NewInt(123)}
	client.EXPECT().HeaderByNumber(gomock.Any(), gomock.Any()).Return(&header, nil)
	client.EXPECT().PendingNonceAt(gomock.Any(), gomock.Any()).Return(uint64(0), nil)
	client.EXPECT().SendTransaction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, submitted *types.Transaction) error {
			*tx = submitted
			return sendErr
		})
}

// expectStakeQuery makes the next contract call return the given stake in wei.
func expectStakeQuery(t *testing.T, client *rpc.MockClient, stake *big.Int) {
	t.Helper()
	uint256Type, err := abi.NewType("uint256", "", nil)
	if err != nil {
		t.Fatalf("failed to create uint256 type: %v", err)
	}
	packed, err := abi.Arguments{{Type: uint256Type}}.Pack(stake)
	if err != nil {
		t.Fatalf("failed to pack the stake: %v", err)
	}
	client.EXPECT().CallContract(gomock.Any(), gomock.Any(), gomock.Any()).Return(packed, nil)
}

// decodeUint256Arg returns the i-th argument of an encoded SFC call.
func decodeUint256Arg(t *testing.T, method string, data []byte, i int) *big.Int {
	t.Helper()
	if len(data) < 4 {
		t.Fatalf("call data of %s is too short: %d bytes", method, len(data))
	}
	sfcAbi, err := sfc100.ContractMetaData.GetAbi()
	if err != nil {
		t.Fatalf("failed to load the SFC ABI: %v", err)
	}
	args, err := sfcAbi.Methods[method].Inputs.Unpack(data[4:])
	if err != nil {
		t.Fatalf("failed to unpack the %s call data: %v", method, err)
	}
	value, ok := args[i].(*big.Int)
	if !ok {
		t.Fatalf("argument %d of %s is a %T, not a uint256", i, method, args[i])
	}
	return value
}
