package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/0xsoniclabs/sonic/integration/makefakegenesis"
	"github.com/urfave/cli/v2"
	"os"
	"path"
	"strings"
	"testing"
)

func TestExportGenesis(t *testing.T) {
	const ValidatorCount = 9
	const MaxValidatorsCount = 100
	const MaxBlockGas = 2000000

	// Create a temporary file
	tmpFile := path.Join(t.TempDir(), "genesis.json")

	if err := os.Setenv("VALIDATORS_COUNT", fmt.Sprintf("%d", ValidatorCount)); err != nil {
		t.Fatalf("failed to set VALIDATORS_COUNT: %v", err)
	}
	if err := os.Setenv("MAX_BLOCK_GAS", fmt.Sprintf("%d", MaxBlockGas)); err != nil {
		t.Fatalf("failed to set MAX_BLOCK_GAS: %v", err)
	}

	defer func() {
		if err := os.Unsetenv("VALIDATORS_COUNT"); err != nil {
			t.Errorf("failed to unset VALIDATORS_COUNT: %v", err)
		}

		if err := os.Unsetenv("MAX_BLOCK_GAS"); err != nil {
			t.Errorf("failed to unset MAX_BLOCK_GAS: %v", err)
		}
	}()

	// Create a new CLI context with the file path argument
	app := cli.NewApp()
	set := flag.NewFlagSet("test", 0)
	if err := set.Parse([]string{tmpFile}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}
	ctx := cli.NewContext(app, set, nil)

	// Call the exportGenesis function
	if err := exportGenesis(ctx); err != nil {
		t.Fatalf("failed to export genesis: %v", err)
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

	// Verify network rules were updated
	if got, want := jsonGenesis.Rules.Blocks.MaxBlockGas, uint64(MaxBlockGas); got != want {
		t.Errorf("unexpected max block gas, wanted %v, got %v", want, got)
	}

	// Verify the number of validators
	var validators int
	for _, account := range jsonGenesis.Accounts {
		if strings.HasPrefix(account.Name, "validator_") {
			validators++
		}
	}

	if got, want := validators, MaxValidatorsCount; got != want {
		t.Errorf("unexpected number of validators, wanted %v, got %v", want, got)
	}
}
