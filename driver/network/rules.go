package network

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/0xsoniclabs/norma/genesis"
	"github.com/0xsoniclabs/sonic/evmcore"
	"github.com/0xsoniclabs/sonic/gossip/contract/driverauth100"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/0xsoniclabs/sonic/opera/contracts/driverauth"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
)

// ApplyNetworkRules updates the network rules on the network.
func ApplyNetworkRules(ctx context.Context, backend ContractBackend, rules genesis.NetworkRulesPatch) error {
	// Bind contract to update network rules
	contract, err := driverauth100.NewContract(driverauth.ContractAddress, backend)
	if err != nil {
		return fmt.Errorf("failed to get driver auth contract representation; %v", err)
	}

	patchJSON, err := json.Marshal(rules)
	if err != nil {
		return fmt.Errorf("failed to marshal network rules patch; %v", err)
	}
	originalRules := opera.FakeNetRules(opera.GetSonicUpgrades())

	// Use Fake ID for the network
	// Driver owner is the first validator from the list i.e., index 1 (defined in genesis export in genesis.GenerateJsonGenesis)
	txOpts, err := bind.NewKeyedTransactorWithChainID(evmcore.FakeKey(1), big.NewInt(int64(originalRules.NetworkID)))
	if err != nil {
		return fmt.Errorf("failed to create txOpts; %v", err)
	}
	txOpts.Context = ctx
	txOpts.GasTipCap = systemTxGasTipCap
	txOpts.GasLimit = systemTxGasLimit

	tx, err := contract.UpdateNetworkRules(txOpts, []byte(patchJSON))
	if err != nil {
		return fmt.Errorf("failed to update network rules; %v", err)
	}

	rec, err := backend.WaitTransactionReceipt(ctx, tx.Hash())
	if err != nil {
		return fmt.Errorf("failed to get receipt; %v", err)
	}

	if rec.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("failed to update network rules: status: %v", rec.Status)
	}

	return nil
}
