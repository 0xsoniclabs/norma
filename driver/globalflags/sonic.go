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

package globalflags

import (
	"fmt"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/docker"
	"github.com/0xsoniclabs/norma/driver/parser"
	"github.com/urfave/cli/v2"
)

// SonicPathFlag lets the user override the path used as the docker build
// context for the "sonic:local" image. When unset, the built-in default from
// the docker package is used (typically the "sonic" directory inside the
// Norma build root).
var SonicPathFlag = cli.StringFlag{
	Name: "sonic-path",
	Usage: "path to the sonic source tree used to build the sonic:local " +
		"image; may be absolute or relative to the norma build root",
	Value: docker.DefaultSonicLocalPath,
}

// GoVersionFlag lets the user sweep the candidate with a specific Go
// toolchain: it applies to nodes that do not pin an imageName. Nodes pinned
// to a released image stand in for shipped clients and keep the Dockerfile
// default toolchain. A scenario-level goVersion takes precedence.
var GoVersionFlag = cli.StringFlag{
	Name: "go-version",
	Usage: "Go toolchain version (e.g. 1.27.0) used to build the client " +
		"images of nodes that do not pin an imageName",
}

// AllSonicFlags aggregates all Sonic-related global flags.
var AllSonicFlags = []cli.Flag{
	&SonicPathFlag,
	&GoVersionFlag,
}

// SetupSonicPath applies the --sonic-path flag value (if any) to the docker
// package configuration used when building the sonic:local image.
func SetupSonicPath(ctx *cli.Context) error {
	docker.SetSonicLocalPath(ctx.String(SonicPathFlag.Name))
	return nil
}

// SetupGoVersion applies the --go-version flag value (if any) to the driver
// configuration used when resolving client images.
func SetupGoVersion(ctx *cli.Context) error {
	version := ctx.String(GoVersionFlag.Name)
	if version != "" && !parser.GoVersionPattern.MatchString(version) {
		return fmt.Errorf(
			"invalid --go-version %q; expected a Go toolchain version like 1.27.0",
			version)
	}
	driver.SetDefaultClientGoVersion(version)
	return nil
}
