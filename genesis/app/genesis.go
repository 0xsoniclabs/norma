package main

import (
	"fmt"
	"github.com/0xsoniclabs/norma/genesistools/genesis"
	"github.com/urfave/cli/v2"
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
	return genesis.GenerateJsonGenesis(filePath)
}
