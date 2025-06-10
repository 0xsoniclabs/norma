package network

import (
	"fmt"
	"github.com/0xsoniclabs/norma/genesistools/genesis"
	"github.com/0xsoniclabs/sonic/evmcore"
	"github.com/0xsoniclabs/sonic/gossip/contract/driverauth100"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/0xsoniclabs/sonic/opera/contracts/driverauth"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
	"math/big"
)

// ApplyNetworkRules updates the network rules on the network.
func ApplyNetworkRules(backend ContractBackend, rules genesis.NetworkRules) error {
	// Bind contract to update network rules
	contract, err := driverauth100.NewContract(driverauth.ContractAddress, backend)
	if err != nil {
		return fmt.Errorf("failed to get driver auth contract representation; %v", err)
	}

	originalRules := opera.FakeNetRules(opera.GetSonicUpgrades())
	diff, err := genesis.GenerateJsonNetworkRulesUpdates(originalRules, rules)
	if err != nil {
		return fmt.Errorf("failed to generate network rules updates; %v", err)
	}

	// Use Fake ID for the network
	// Driver owner is the first validator from the list i.e., index 1 (defined in genesis export in genesis.GenerateJsonGenesis)
	txOpts, err := bind.NewKeyedTransactorWithChainID(evmcore.FakeKey(1), big.NewInt(int64(originalRules.NetworkID)))
	if err != nil {
		return fmt.Errorf("failed to create txOpts; %v", err)
	}

	tx, err := contract.UpdateNetworkRules(txOpts, []byte(diff))
	if err != nil {
		return fmt.Errorf("failed to update network rules; %v", err)
	}

	rec, err := backend.WaitTransactionReceipt(tx.Hash())
	if err != nil {
		return fmt.Errorf("failed to get receipt; %v", err)
	}

	if rec.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("failed to update network rules: status: %v", rec.Status)
	}

	return nil
}

// AdvanceEpoch trigger the sealing of an epoch and advance the epoch by the given number.
// The function blocks until the final epoch has been reached
func AdvanceEpoch(backend ContractBackend, epochIncrement int) error {
	contract, err := driverauth100.NewContract(driverauth.ContractAddress, backend)
	if err != nil {
		return fmt.Errorf("failed to get driver auth contract representation; %v", err)
	}

	originalRules := opera.FakeNetRules(opera.GetSonicUpgrades())

	// Use Fake ID for the network
	// Driver owner is the first validator from the list i.e., index 1 (defined in genesis export in genesis.GenerateJsonGenesis)
	txOpts, err := bind.NewKeyedTransactorWithChainID(evmcore.FakeKey(1), big.NewInt(int64(originalRules.NetworkID)))
	if err != nil {
		return fmt.Errorf("failed to create txOpts; %v", err)
	}

	tx, err := contract.AdvanceEpochs(txOpts, big.NewInt(int64(epochIncrement)))
	if err != nil {
		return fmt.Errorf("failed to advance epoch; %v", err)
	}

	rec, err := backend.WaitTransactionReceipt(tx.Hash())
	if err != nil {
		return fmt.Errorf("failed to get receipt; %v", err)
	}

	if rec.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("failed to advance epoch; receipt status: %v", rec.Status)
	}

	return nil
}
