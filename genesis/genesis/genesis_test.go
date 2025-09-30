package genesis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path"
	"reflect"
	"slices"
	"testing"

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
	futils "github.com/0xsoniclabs/sonic/utils"
)

func TestGenerateJsonGenesis(t *testing.T) {
	// configure expected variables
	const ValidatorsCount = 9
	const MaxValidatorsCount = 100

	tests := map[string]opera.Rules{
		"sonic":   opera.FakeNetRules(opera.GetSonicUpgrades()),
		"allegro": opera.FakeNetRules(opera.GetAllegroUpgrades()),
	}

	for name, rules := range tests {
		t.Run(name, func(t *testing.T) {
			expectedRules := rules

			// Create a temporary file
			tmpFile := path.Join(t.TempDir(), "genesis.json")

			// Call the GenerateJsonGenesis function
			if err := GenerateJsonGenesis(tmpFile, ValidatorsCount, &rules); err != nil {
				t.Fatalf("failed to generate genesis.json: %v", err)
			}

			// Read the generated file
			data, err := os.ReadFile(tmpFile)
			if err != nil {
				t.Fatalf("failed to read genesis.json: %v", err)
			}

			// Unmarshal the JSON content
			var jsonGenesis makefakegenesis.GenesisJson
			if err := json.Unmarshal(data, &jsonGenesis); err != nil {
				t.Fatalf("failed to unmarshal genesis.json: %v", err)
			}

			// Verify the content
			if got, want := jsonGenesis.Rules, expectedRules; !reflect.DeepEqual(got, want) {
				t.Errorf("unexpected rules, wanted %v, got %v", want, got)
			}

			// Verify the initial account contracts
			expectedAccounts := []makefakegenesis.Account{
				{Name: "NetworkInitializer", Address: netinit.ContractAddress, Code: netinit.GetContractBin(), Nonce: 1},
				{Name: "NodeDriver", Address: driver.ContractAddress, Code: driver.GetContractBin(), Nonce: 1},
				{Name: "NodeDriverAuth", Address: driverauth.ContractAddress, Code: driverauth.GetContractBin(), Nonce: 1},
				{Name: "SFC", Address: sfc.ContractAddress, Code: sfc.GetContractBin(), Nonce: 1},
				{Name: "ContractAddress", Address: evmwriter.ContractAddress, Code: []byte{0}, Nonce: 1},
			}

			// add validators to expected accounts
			validators := makefakegenesis.GetFakeValidators(MaxValidatorsCount)
			totalSupply := utils.ToFtm(1000_000_000)
			supplyEach := new(big.Int).Div(totalSupply, big.NewInt(int64(len(validators))))
			for _, validator := range validators {
				expectedAccounts = append(expectedAccounts, makefakegenesis.Account{
					Name:    fmt.Sprintf("validator_%d", validator.ID),
					Address: validator.Address,
					Balance: supplyEach,
				})
			}

			for _, account := range expectedAccounts {
				index := slices.IndexFunc(jsonGenesis.Accounts, func(a makefakegenesis.Account) bool {
					return a.Name == account.Name &&
						a.Address == account.Address &&
						bytes.Equal(a.Code, account.Code) &&
						a.Nonce == account.Nonce
				})
				if index < 0 {
					t.Errorf("account %s not found in genesis accounts, but should be present", account.Name)
				}
			}

			expectedMissingAccounts := []makefakegenesis.Account{
				{Name: "SubsidiesRegistry", Address: gas_subsidies_registry.GetAddress(), Code: gas_subsidies_registry.GetCode(), Nonce: 1},
			}
			for _, account := range expectedMissingAccounts {
				index := slices.IndexFunc(jsonGenesis.Accounts, func(a makefakegenesis.Account) bool {
					return a.Name == account.Name &&
						a.Address == account.Address &&
						bytes.Equal(a.Code, account.Code) &&
						a.Nonce == account.Nonce
				})
				if index >= 0 {
					t.Errorf("account %s found in genesis accounts, but should not be present", account.Name)
				}
			}

			validators = validators[0:ValidatorsCount]
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

			expectedTxs := makefakegenesis.GetGenesisTxs(0, validators, totalSupply, delegations, validators[0].Address)
			for i, tx := range expectedTxs {
				if got, want := jsonGenesis.Txs[i].To, *tx.To(); got != want {
					t.Errorf("unexpected tx to, wanted %v, got %v", want, got)
				}
				if got, want := jsonGenesis.Txs[i].Data, tx.Data(); !bytes.Equal(got, want) {
					t.Errorf("unexpected tx data, wanted %v, got %v", want, got)
				}
			}
		})
	}
}

func TestGenerateJsonGenesis_SubsidiesContractShallBeInstalled_IfFeatureEnabled(t *testing.T) {
	tests := map[string]opera.Rules{
		"sonic":   opera.FakeNetRules(opera.GetSonicUpgrades()),
		"allegro": opera.FakeNetRules(opera.GetAllegroUpgrades()),
	}

	for name, rules := range tests {
		t.Run(name, func(t *testing.T) {

			// Call the GenerateJsonGenesis function
			rules.Upgrades.GasSubsidies = true

			tmpFile := path.Join(t.TempDir(), "genesis.json")
			if err := GenerateJsonGenesis(tmpFile, 1, &rules); err != nil {
				t.Fatalf("failed to generate genesis.json: %v", err)
			}

			// Read the generated file
			data, err := os.ReadFile(tmpFile)
			if err != nil {
				t.Fatalf("failed to read genesis.json: %v", err)
			}

			// Unmarshal the JSON content
			var jsonGenesis makefakegenesis.GenesisJson
			if err := json.Unmarshal(data, &jsonGenesis); err != nil {
				t.Fatalf("failed to unmarshal genesis.json: %v", err)
			}

			index := slices.IndexFunc(jsonGenesis.Accounts, func(a makefakegenesis.Account) bool {
				return a.Address == gas_subsidies_registry.GetAddress() &&
					bytes.Equal(a.Code, gas_subsidies_registry.GetCode())
			})
			if index < 0 {
				t.Errorf("subsidies register contract not found in genesis accounts")
			}
		})
	}
}
