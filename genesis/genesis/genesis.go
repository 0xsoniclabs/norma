package genesis

import (
	"encoding/json"
	"fmt"
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
	futils "github.com/0xsoniclabs/sonic/utils"
	"github.com/Fantom-foundation/lachesis-base/inter/idx"
	"math/big"
	"os"
	"time"
)

// GenerateJsonGenesis generates a genesis json file with the given number of validators
// and network rules configurations.
// The file is written to the given path.
func GenerateJsonGenesis(jsonFile string, validatorsCount int, rules *opera.Rules) error {
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
	}

	// Create the validator account and provide some tokens.
	totalSupply := futils.ToFtm(1000_000_000)
	validators := makefakegenesis.GetFakeValidators(idx.Validator(validatorsCount))
	supplyEach := new(big.Int).Div(totalSupply, big.NewInt(int64(len(validators))))
	for _, validator := range validators {
		jsonGenesis.Accounts = append(jsonGenesis.Accounts, makefakegenesis.Account{
			Name:    fmt.Sprintf("validator_%d", validator.ID),
			Address: validator.Address,
			Balance: supplyEach,
		})
	}

	var delegations []drivercall.Delegation
	for _, val := range validators {
		delegations = append(delegations, drivercall.Delegation{
			Address:            val.Address,
			ValidatorID:        val.ID,
			Stake:              futils.ToFtm(5_000_000),
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
	if err := committee.Validate(); err != nil {
		return fmt.Errorf("failed to create valid committee: %w", err)
	}

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
