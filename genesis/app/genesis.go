package main

import (
	"fmt"
	"github.com/0xsoniclabs/norma/genesistools/genesis"
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/urfave/cli/v2"
	"os"
	"strconv"
)

// genesisExportCommand is the command for exporting genesis file.
var genesisExportCommand = cli.Command{
	Name:  "genesis",
	Usage: "Genesis manipulation commands",
	Subcommands: []*cli.Command{
		{
			Name:   "export",
			Usage:  "exports genesis file",
			Action: exportGenesis,
		},
	},
}

// exportGenesis exports genesis file.
// File path must be provided as the first program argument.
func exportGenesis(ctx *cli.Context) error {
	if ctx.Args().Len() == 0 {
		return fmt.Errorf("no file path provided")
	}

	filePath := ctx.Args().Get(0)

	rules := opera.FakeNetRules()

	// apply the rules configuration
	if err := genesis.ConfigureNetworkRulesEnv(&rules); err != nil {
		return fmt.Errorf("failed to configure network rules: %w", err)
	}

	// configuration is read from environment variables and defaults
	validatorsCount := os.Getenv("VALIDATORS_COUNT")
	validatorsCountInt, err := strconv.ParseInt(validatorsCount, 10, 32)
	if err != nil {
		return fmt.Errorf("failed to parse validators count: %w", err)
	}

	if validatorsCountInt < 1 {
		return fmt.Errorf("invalid validators count: %d, must be greater than 0", validatorsCountInt)
	}

	return genesis.GenerateJsonGenesis(filePath, int(validatorsCountInt), &rules)
}
