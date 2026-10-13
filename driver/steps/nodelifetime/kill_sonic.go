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

// KillSonic sends SIGKILL to the sonicd process inside a running node,
// leaving the database in a dirty state. The container stays alive and the
// node stays tracked, so it can be healed and restarted later.
type KillSonic struct {
	steps.Node
}

func (*KillSonic) Name() string { return "killSonic" }

func (*KillSonic) Help() string {
	return "Kill the sonicd process with SIGKILL, leaving the database dirty."
}

func (s *KillSonic) Run(ctx context.Context, env *steps.Env) error {
	opera, err := s.Opera(env)
	if err != nil {
		return err
	}
	env.Net.SuspendNode(opera)
	return opera.ForceStopSonicd(ctx)
}
