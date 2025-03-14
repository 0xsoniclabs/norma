package network

import (
	"fmt"
	"github.com/0xsoniclabs/sonic/evmcore"
	"github.com/0xsoniclabs/sonic/gossip/contract/sfc100"
	"github.com/0xsoniclabs/sonic/inter/validatorpk"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/0xsoniclabs/sonic/opera/contracts/sfc"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
)

// RegisterValidatorNode registers a validator in the SFC contract.
func RegisterValidatorNode(backend ContractBackend) (int, error) {
	newValId := 0

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

	newValId = int(lastValId.Int64()) + 1

	privateKeyECDSA := evmcore.FakeKey(uint32(newValId))
	txOpts, err := bind.NewKeyedTransactorWithChainID(privateKeyECDSA, big.NewInt(int64(opera.FakeNetRules(opera.SonicFeatures).NetworkID)))
	if err != nil {
		return 0, fmt.Errorf("failed to create txOpts; %v", err)
	}

	txOpts.Value = big.NewInt(0).Mul(big.NewInt(5_000_000), big.NewInt(1_000_000_000_000_000_000)) // 5_000_000 FTM

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
		return 0, fmt.Errorf("failed to get receipt; %v", err)
	}

	if receipt.Status != types.ReceiptStatusSuccessful {
		return 0, fmt.Errorf("failed to deploy helper contract: transaction reverted")
	}

	return newValId, nil
}
