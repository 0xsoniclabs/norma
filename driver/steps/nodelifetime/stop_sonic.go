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

package nodelifetime

import (
	"context"

	"github.com/0xsoniclabs/norma/driver/steps"
)

// StopSonic stops the client gracefully, keeping the container and the
// tracked node so a later startNode restarts it in place.
type StopSonic struct {
	steps.Node
}

func (*StopSonic) Name() string { return "stopSonic" }

func (*StopSonic) Help() string {
	return "Stop the sonicd process gracefully, keeping the node's container and data directory."
}

func (s *StopSonic) Run(ctx context.Context, env *steps.Env) error {
	opera, err := s.Opera(env)
	if err != nil {
		return err
	}
	env.Net.SuspendNode(opera)
	return opera.StopSonicd(ctx)
}
