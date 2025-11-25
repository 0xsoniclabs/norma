package genesis

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"time"

	gas_subsidies_registry "github.com/0xsoniclabs/sonic/gossip/blockproc/subsidies/registry"
	"github.com/0xsoniclabs/sonic/integration/makefakegenesis"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/0xsoniclabs/sonic/opera/contracts/driver"
	"github.com/0xsoniclabs/sonic/opera/contracts/driver/drivercall"
	"github.com/0xsoniclabs/sonic/opera/contracts/driverauth"
	"github.com/0xsoniclabs/sonic/opera/contracts/evmwriter"
	"github.com/0xsoniclabs/sonic/opera/contracts/netinit"
	"github.com/0xsoniclabs/sonic/opera/contracts/sfc"
	"github.com/0xsoniclabs/sonic/scc"
	"github.com/0xsoniclabs/sonic/scc/bls"
	"github.com/0xsoniclabs/sonic/utils"
	"github.com/Fantom-foundation/lachesis-base/inter/idx"
)

// GenerateJsonGenesis generates a genesis json file with the given number of validators
// and network rules configurations.
// The file is written to the given path.
func GenerateJsonGenesis(jsonFile string, validatorStakes []uint64, rules *opera.Rules) error {
	validatorsCount := len(validatorStakes)
	jsonGenesis := makefakegenesis.GenesisJson{
		Rules:         *rules,
		BlockZeroTime: time.Unix(100, 0), // genesis files in each container must have the same timestamp
	}

	// Create infrastructure contracts.
	jsonGenesis.Accounts = []makefakegenesis.Account{
		{
			Name:    "NetworkInitializer",
			Address: netinit.ContractAddress,
			Code:    netinit.GetContractBin(),
			Nonce:   1,
		},
		{
			Name:    "NodeDriver",
			Address: driver.ContractAddress,
			Code:    driver.GetContractBin(),
			Nonce:   1,
		},
		{
			Name:    "NodeDriverAuth",
			Address: driverauth.ContractAddress,
			Code:    driverauth.GetContractBin(),
			Nonce:   1,
		},
		{
			Name:    "SFC",
			Address: sfc.ContractAddress,
			Code:    sfc.GetContractBin(),
			Nonce:   1,
		},
		{
			Name:    "ContractAddress",
			Address: evmwriter.ContractAddress,
			Code:    []byte{0},
			Nonce:   1,
		},
		{
			Name:    "SubsidiesRegistry",
			Address: gas_subsidies_registry.GetAddress(),
			Code:    gas_subsidies_registry.GetCode(),
			Nonce:   1,
		},
	}

	// Create the validator account and provide tokens, pre-init a maximal limit of validators.
	const maxValidators = 100
	totalSupply := utils.ToFtm(1000_000_000)
	validators := makefakegenesis.GetFakeValidators(idx.Validator(maxValidators))
	supplyEach := new(big.Int).Div(totalSupply, big.NewInt(int64(len(validators))))
	for _, validator := range validators {
		jsonGenesis.Accounts = append(jsonGenesis.Accounts, makefakegenesis.Account{
			Name:    fmt.Sprintf("validator_%d", validator.ID),
			Address: validator.Address,
			Balance: supplyEach,
		})
	}

	// set genesis validators only for the configured number of validators
	validators = validators[0:validatorsCount]
	delegations := make([]drivercall.Delegation, 0)
	for i, stake := range validatorStakes {
		val := validators[i]
		delegations = append(delegations, drivercall.Delegation{
			Address:            val.Address,
			ValidatorID:        val.ID,
			Stake:              utils.ToFtm(stake),
			LockedStake:        new(big.Int),
			LockupFromEpoch:    0,
			LockupEndTime:      0,
			LockupDuration:     0,
			EarlyUnlockPenalty: new(big.Int),
			Rewards:            new(big.Int),
		})
	}

	// Create the genesis transactions.
	genesisTxs := makefakegenesis.GetGenesisTxs(0, validators, totalSupply, delegations, validators[0].Address)
	for i, tx := range genesisTxs {
		jsonGenesis.Txs = append(jsonGenesis.Txs, makefakegenesis.Transaction{
			Name: fmt.Sprintf("tx_%d", i),
			To:   *tx.To(),
			Data: tx.Data(),
		})
	}

	// Create the genesis SCC committee.
	key := bls.NewPrivateKeyForTests(0)
	committee := scc.NewCommittee(scc.Member{
		PublicKey:         key.PublicKey(),
		ProofOfPossession: key.GetProofOfPossession(),
		VotingPower:       1,
	})
	jsonGenesis.GenesisCommittee = &committee

	encoded, err := json.MarshalIndent(jsonGenesis, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode genesis json: %w", err)
	}

	if err = os.WriteFile(jsonFile, encoded, 0644); err != nil {
		return fmt.Errorf("failed to write genesis.json file: %w", err)
	}

	return nil
}
