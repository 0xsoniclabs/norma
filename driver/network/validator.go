package network

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/sonic/evmcore"
	"github.com/0xsoniclabs/sonic/gossip/contract/sfc100"
	"github.com/0xsoniclabs/sonic/inter/validatorpk"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/0xsoniclabs/sonic/opera/contracts/sfc"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// defaultStakePerValidator is the default stake used when no stake is specified.
const defaultStakePerValidator = uint64(5_000_000)

// RegisterValidatorNode registers a validator in the SFC contract.
// If stake is 0, the default stake of 5,000,000 S is used.
func RegisterValidatorNode(ctx context.Context, backend ContractBackend, stake uint64) (int, error) {
	// get a representation of the deployed contract
	SFCContract, err := sfc100.NewContract(sfc.ContractAddress, backend)
	if err != nil {
		return 0, fmt.Errorf("failed to get SFC contract representation; %v", err)
	}

	var lastValId *big.Int
	lastValId, err = SFCContract.LastValidatorID(nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get validator count; %v", err)
	}
	newValId := int(lastValId.Int64()) + 1

	privateKeyECDSA := evmcore.FakeKey(uint32(newValId))
	txOpts, err := bind.NewKeyedTransactorWithChainID(privateKeyECDSA, big.NewInt(int64(opera.FakeNetRules(opera.GetSonicUpgrades()).NetworkID)))
	if err != nil {
		return 0, fmt.Errorf("failed to create txOpts; %v", err)
	}
	txOpts.GasTipCap = systemTxGasTipCap
	txOpts.GasLimit = systemTxGasLimit

	if stake == 0 {
		stake = defaultStakePerValidator
	}
	txOpts.Value = stakeToWei(stake)

	validatorPubKey := validatorpk.PubKey{
		Raw:  crypto.FromECDSAPub(&privateKeyECDSA.PublicKey),
		Type: validatorpk.Types.Secp256k1,
	}

	tx, err := SFCContract.CreateValidator(txOpts, validatorPubKey.Bytes())
	if err != nil {
		return 0, fmt.Errorf("failed to create validator; %v", err)
	}

	receipt, err := backend.WaitTransactionReceipt(ctx, tx.Hash())
	if err != nil {
		return 0, fmt.Errorf("failed to create validator, receipt error: %v", err)
	}

	if receipt.Status != types.ReceiptStatusSuccessful {
		return 0, fmt.Errorf("failed to deploy helper contract: transaction reverted")
	}

	slog.Info(
		"Completed registration of new validator node",
		"validator_id", newValId,
	)

	return newValId, nil
}

// UnregisterValidatorNode undelegates a validator's self-stake from the SFC
// contract. If stake is 0, the currently staked amount is queried on-chain via
// GetSelfStake and used as the undelegate amount.
func UnregisterValidatorNode(ctx context.Context, client rpc.Client, validatorId int, stake uint64) error {
	slog.Info("start unregistering validator node", "validator_id", validatorId)

	// get a representation of the deployed contract
	sfcContract, err := sfc100.NewContract(sfc.ContractAddress, client)
	if err != nil {
		return fmt.Errorf("failed to get SFC contract representation; %v", err)
	}

	key := evmcore.FakeKey(uint32(validatorId))
	txOpts, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(int64(opera.FakeNetRules(opera.GetSonicUpgrades()).NetworkID)))
	if err != nil {
		return fmt.Errorf("failed to create txOpts; %v", err)
	}
	txOpts.GasTipCap = systemTxGasTipCap
	txOpts.GasLimit = systemTxGasLimit

	var stakeWei *big.Int
	if stake == 0 {
		// No stake specified — query the current self-stake on-chain.
		stakeWei, err = sfcContract.GetSelfStake(nil, big.NewInt(int64(validatorId)))
		if err != nil {
			return fmt.Errorf("failed to query self-stake for validator %d; %v", validatorId, err)
		}
	} else {
		stakeWei = stakeToWei(stake)
	}

	// withdraw ID must be unique, so we use the current time in nanoseconds
	withdrawId := big.NewInt(time.Now().UnixNano())
	tx, err := sfcContract.Undelegate(txOpts, big.NewInt(int64(validatorId)), withdrawId, stakeWei)
	if err != nil {
		return fmt.Errorf("failed to undelegate validator stake; %v", err)
	}

	receipt, err := client.WaitTransactionReceipt(ctx, tx.Hash())
	if err != nil {
		return fmt.Errorf("failed to unregister validator, receipt error: %v", err)
	}

	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("failed to unregister validator: transaction reverted")
	}

	slog.Info(
		"Completed unregistering validator node",
		"validator_id", validatorId,
	)

	return nil
}

// stakeToWei converts a stake value in S to wei.
func stakeToWei(stake uint64) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(stake), big.NewInt(1e18))
}
