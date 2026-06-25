// Copyright 2024 Fantom Foundation
// This file is part of Norma System Testing Infrastructure for Sonic.
//
// Norma is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Norma is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with Norma. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"os"

	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/urfave/cli/v2"
)

// Run with `go run ./driver/norma scenario-help`

var scenarioHelpCommand = cli.Command{
	Action: scenarioHelp,
	Name:   "scenario-help",
	Usage:  "prints available step functions and parameters for sequential scenarios",
}

func scenarioHelp(_ *cli.Context) error {
	return parser.PrintSequentialHelp(os.Stdout)
}
