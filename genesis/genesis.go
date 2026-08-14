package genesis

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"regexp"
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
	"github.com/0xsoniclabs/sonic/utils"
	"github.com/Fantom-foundation/lachesis-base/inter/idx"
	"github.com/holiman/uint256"
)

// GenerateJsonGenesis generates a genesis json file with the given number of validators
// and network rules configurations.
// The file is written to the given path.
//
// Every node of a network imports the same file, so the parts of it that depend
// on the client version - currently the subsidies registry - are chosen to suit
// the oldest client of the network. The versions are the ones the clients report
// for themselves; see docker.ClientVersions.
func GenerateJsonGenesis(
	jsonFile string,
	validatorStakes []uint64,
	rules *opera.Rules,
	clientVersions []string,
) error {
	registryCode, err := subsidiesRegistryCode(clientVersions)
	if err != nil {
		return err
	}

	validatorsCount := len(validatorStakes)
	jsonGenesis := makefakegenesis.GenesisJson{
		Rules:         *rules,
		BlockZeroTime: time.Unix(100, 0), // Genesis files must have the same timestamp across all nodes.
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
			Code:    registryCode,
			Nonce:   1,
		},
	}

	// Create validator accounts and distribute initial supply.
	const maxValidators = 100
	totalSupply := utils.ToFtm(1_000_000_000_000_000)
	validators := makefakegenesis.GetFakeValidators(idx.Validator(maxValidators))
	supplyEach := new(big.Int).Div(totalSupply, big.NewInt(int64(len(validators))))
	for _, validator := range validators {
		jsonGenesis.Accounts = append(jsonGenesis.Accounts, makefakegenesis.Account{
			Name:    fmt.Sprintf("validator_%d", validator.ID),
			Address: validator.Address,
			Balance: uint256.MustFromBig(supplyEach),
		})
	}

	// Configure genesis validators only for the configured number of validators.
	validators = validators[0:validatorsCount]
	delegations := make([]drivercall.Delegation, 0, validatorsCount)
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

	// Create genesis transactions.
	genesisTxs := makefakegenesis.GetGenesisTxs(0, validators, totalSupply, delegations, validators[0].Address)
	for i, tx := range genesisTxs {
		jsonGenesis.Txs = append(jsonGenesis.Txs, makefakegenesis.Transaction{
			Name: fmt.Sprintf("tx_%d", i),
			To:   *tx.To(),
			Data: tx.Data(),
		})
	}

	encoded, err := json.MarshalIndent(jsonGenesis, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode genesis json: %w", err)
	}

	if err = os.WriteFile(jsonFile, encodeBalancesAsNumbers(encoded), 0644); err != nil {
		return fmt.Errorf("failed to write genesis.json file: %w", err)
	}

	return nil
}

// quotedBalance matches the encoded account balances of a genesis file.
var quotedBalance = regexp.MustCompile(`("Balance": )"(\d+)"`)

// encodeBalancesAsNumbers rewrites quoted account balances into JSON numbers.
//
// Every node of a network imports the same genesis file with the sonictool of
// its own image, so the file has to be readable by every client version a
// scenario starts. Balances are encoded as strings by the Sonic version this
// module builds genesis files with, while clients up to v2.1.6 decode them into
// a big.Int, which rejects a string. A JSON number of arbitrary length is
// accepted by both.
func encodeBalancesAsNumbers(genesis []byte) []byte {
	return quotedBalance.ReplaceAll(genesis, []byte("${1}${2}"))
}
