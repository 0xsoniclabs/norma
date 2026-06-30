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
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/0xsoniclabs/norma/driver/globalflags"
	"github.com/urfave/cli/v2"
)

// Run with `go run ./driver/norma`

func main() {

	app := &cli.App{
		Name:      "Norma Network Runner",
		HelpName:  "norma",
		Usage:     "A set of tools for running network scenarios",
		Copyright: "(c) 2023 Fantom Foundation",
		Flags:     globalflags.AllGlobalFlags,
		Commands: []*cli.Command{
			&checkCommand,
			&runCommand,
			&buildCommand,
			&purgeCommand,
			&renderCommand,
			&diffCommand,
			&scenarioHelpCommand,
		},
		Before: globalflags.ProcessGlobalFlags,
	}

	stoppableCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-stoppableCtx.Done()
		slog.Info("stopping...")
		stop() // second Ctrl+C will force-kill
	}()

	if err := app.RunContext(stoppableCtx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
