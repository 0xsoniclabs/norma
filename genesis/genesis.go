package genesis

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/0xsoniclabs/sonic/integration/makefakegenesis"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/0xsoniclabs/sonic/utils"
	"github.com/Fantom-foundation/lachesis-base/inter/idx"
)

// GenerateJsonGenesis generates a genesis json file with the given number of validators
// and network rules configurations.
// The file is written to the given path.
func GenerateJsonGenesis(jsonFile string, validatorStakes []uint64, rules *opera.Rules) error {
	jsonGenesis := makefakegenesis.GenerateFakeJsonGenesis(rules.Upgrades, validatorStakes)
	jsonGenesis.Rules = *rules
	jsonGenesis.BlockZeroTime = time.Unix(100, 0) // Genesis files must have the same timestamp across all nodes.
	// When the consensus chain is enabled from genesis, run it canonically from
	// block one instead of going through the runtime hand-over.
	jsonGenesis.UseConsensusChain = rules.Upgrades.RunConsensusChain

	// Fund validator accounts beyond the initial set so that validators
	// joining the network later in a scenario can pay for their stake.
	const maxValidators = 100
	tokensPerValidator := utils.ToFtmU256(1_000_000_000)
	validators := makefakegenesis.GetFakeValidators(idx.Validator(maxValidators))
	for _, validator := range validators[len(validatorStakes):] {
		jsonGenesis.Accounts = append(jsonGenesis.Accounts, makefakegenesis.Account{
			Name:    fmt.Sprintf("validator_%d", validator.ID),
			Address: validator.Address,
			Balance: tokensPerValidator,
		})
	}

	// The rules were replaced after the genesis was generated, so the on-chain
	// network-rules contract storage must be re-seeded from them.
	if err := jsonGenesis.SeedNetworkRules(); err != nil {
		return fmt.Errorf("failed to seed network rules: %w", err)
	}

	encoded, err := encodeGenesisJson(jsonGenesis)
	if err != nil {
		return fmt.Errorf("failed to encode genesis json: %w", err)
	}

	if err = os.WriteFile(jsonFile, encoded, 0644); err != nil {
		return fmt.Errorf("failed to write genesis.json file: %w", err)
	}

	return nil
}

// encodeGenesisJson encodes the genesis with account balances as JSON numbers.
// The client's Account.Balance type (*uint256.Int) marshals into a quoted
// decimal string, which pre-restructure clients (*big.Int) cannot parse; both
// old and new clients accept a plain JSON number.
func encodeGenesisJson(jsonGenesis *makefakegenesis.GenesisJson) ([]byte, error) {
	encoded, err := json.Marshal(jsonGenesis)
	if err != nil {
		return nil, err
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &document); err != nil {
		return nil, err
	}
	if rawAccounts, ok := document["Accounts"]; ok {
		var accounts []map[string]json.RawMessage
		if err := json.Unmarshal(rawAccounts, &accounts); err != nil {
			return nil, err
		}
		for _, account := range accounts {
			var balance string
			if raw, ok := account["Balance"]; ok {
				if err := json.Unmarshal(raw, &balance); err != nil {
					return nil, fmt.Errorf("unexpected account balance encoding: %w", err)
				}
				account["Balance"] = json.RawMessage(balance)
			}
		}
		if document["Accounts"], err = json.Marshal(accounts); err != nil {
			return nil, err
		}
	}

	return json.MarshalIndent(document, "", "  ")
}
