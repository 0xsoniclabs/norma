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
	"github.com/0xsoniclabs/norma/driver/docker"
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

// AllSonicFlags aggregates all Sonic-related global flags.
var AllSonicFlags = []cli.Flag{
	&SonicPathFlag,
}

// SetupSonicPath applies the --sonic-path flag value (if any) to the docker
// package configuration used when building the sonic:local image.
func SetupSonicPath(ctx *cli.Context) error {
	docker.SetSonicLocalPath(ctx.String(SonicPathFlag.Name))
	return nil
}
