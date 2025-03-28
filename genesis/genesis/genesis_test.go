package genesis

import (
	"bytes"
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
	"github.com/0xsoniclabs/sonic/utils"
	futils "github.com/0xsoniclabs/sonic/utils"
	"github.com/stretchr/testify/assert"
	"math/big"
	"os"
	"path"
	"reflect"
	"testing"
)

func TestGenerateJsonGenesis(t *testing.T) {
	// configure expected variables
	const ValidatorsCount = 9

	// Create a temporary file
	tmpFile := path.Join(t.TempDir(), "genesis.json")

	// Call the GenerateJsonGenesis function
	rules := opera.FakeNetRules(opera.AllegroFeatures)
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
	expected := opera.FakeNetRules(opera.AllegroFeatures)

	if got, want := jsonGenesis.Rules, expected; !reflect.DeepEqual(got, want) {
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
	validators := makefakegenesis.GetFakeValidators(ValidatorsCount)
	totalSupply := utils.ToFtm(1000_000_000)
	supplyEach := new(big.Int).Div(totalSupply, big.NewInt(int64(len(validators))))
	for _, validator := range validators {
		expectedAccounts = append(expectedAccounts, makefakegenesis.Account{
			Name:    fmt.Sprintf("validator_%d", validator.ID),
			Address: validator.Address,
			Balance: supplyEach,
		})
	}

	for i, account := range expectedAccounts {
		if got, want := jsonGenesis.Accounts[i].Name, account.Name; got != want {
			t.Errorf("unexpected account name, wanted %v, got %v", want, got)
		}
		if got, want := jsonGenesis.Accounts[i].Address, account.Address; got != want {
			t.Errorf("unexpected account address, wanted %v, got %v", want, got)
		}
		if got, want := jsonGenesis.Accounts[i].Code, account.Code; !assert.True(t, bytes.Equal(got, want)) {
			t.Errorf("unexpected account code, wanted %v, got %v", want, got)
		}
		if got, want := jsonGenesis.Accounts[i].Nonce, account.Nonce; got != want {
			t.Errorf("unexpected account nonce, wanted %v, got %v", want, got)
		}
		if got, want := jsonGenesis.Accounts[i].Balance, account.Balance; got.Cmp(want) != 0 {
			t.Errorf("unexpected account balance, wanted %v, got %v", want, got)
		}
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

	expectedTxs := makefakegenesis.GetGenesisTxs(0, validators, totalSupply, delegations, validators[0].Address)
	for i, tx := range expectedTxs {
		if got, want := jsonGenesis.Txs[i].To, *tx.To(); got != want {
			t.Errorf("unexpected tx to, wanted %v, got %v", want, got)
		}
		if got, want := jsonGenesis.Txs[i].Data, tx.Data(); !bytes.Equal(got, want) {
			t.Errorf("unexpected tx data, wanted %v, got %v", want, got)
		}
	}
}
