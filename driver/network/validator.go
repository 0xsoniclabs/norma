package network

import (
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

// RegisterValidatorNode registers a validator in the SFC contract.
func RegisterValidatorNode(backend ContractBackend) (int, error) {
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

	txOpts.Value = getStakePerValidator()

	validatorPubKey := validatorpk.PubKey{
		Raw:  crypto.FromECDSAPub(&privateKeyECDSA.PublicKey),
		Type: validatorpk.Types.Secp256k1,
	}

	tx, err := SFCContract.CreateValidator(txOpts, validatorPubKey.Bytes())
	if err != nil {
		return 0, fmt.Errorf("failed to create validator; %v", err)
	}

	receipt, err := backend.WaitTransactionReceipt(tx.Hash())
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

func UnregisterValidatorNode(client rpc.Client, validatorId int) error {
	slog.Info("Start unregistering validator node", "validator_id", validatorId)

	// get a representation of the deployed contract
	sfc, err := sfc100.NewContract(sfc.ContractAddress, client)
	if err != nil {
		return fmt.Errorf("failed to get SFC contract representation; %v", err)
	}

	key := evmcore.FakeKey(uint32(validatorId))
	txOpts, err := bind.NewKeyedTransactorWithChainID(key, big.NewInt(int64(opera.FakeNetRules(opera.GetSonicUpgrades()).NetworkID)))
	if err != nil {
		return fmt.Errorf("failed to create txOpts; %v", err)
	}

	stake := getStakePerValidator()

	// withdraw ID must be unique, so we use the current time in nanoseconds
	withdrawId := big.NewInt(time.Now().UnixNano())
	tx, err := sfc.Undelegate(txOpts, big.NewInt(int64(validatorId)), withdrawId, stake)
	if err != nil {
		return fmt.Errorf("failed to undelegate validator stake; %v", err)
	}

	receipt, err := client.WaitTransactionReceipt(tx.Hash())
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

func getStakePerValidator() *big.Int {
	// 5_000_000 S
	return new(big.Int).Mul(big.NewInt(5_000_000), big.NewInt(1e18))
}
