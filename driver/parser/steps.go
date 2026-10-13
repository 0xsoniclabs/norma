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

package parser

import (
	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/0xsoniclabs/norma/driver/steps/checks"
	"github.com/0xsoniclabs/norma/driver/steps/staking"
	"github.com/0xsoniclabs/norma/driver/steps/startnode"
)

// stepKinds lists every step a scenario may use.
var stepKinds = []func() steps.Step{
	func() steps.Step { return &startnode.StartNode{} },
	func() steps.Step { return &steps.StopNode{} },
	func() steps.Step { return &staking.Delegate{} },
	func() steps.Step { return &staking.Undelegate{} },
	func() steps.Step { return &staking.VerifyStakes{} },
	func() steps.Step { return &steps.UpdateRules{} },
	func() steps.Step { return &steps.AdvanceEpoch{} },
	func() steps.Step { return &steps.WaitForEpoch{} },
	func() steps.Step { return &steps.RunApp{} },
	func() steps.Step { return &steps.StopApp{} },
	func() steps.Step { return &checks.Checks{} },
	func() steps.Step { return &steps.WaitFor{} },
	func() steps.Step { return &steps.WaitForBlocks{} },
	func() steps.Step { return &steps.KillSonic{} },
	func() steps.Step { return &steps.HealDb{} },
	func() steps.Step { return &steps.StopSonic{} },
	func() steps.Step { return &steps.WaitForSonicExit{} },
	func() steps.Step { return &steps.ExportGenesis{} },
	func() steps.Step { return &steps.ImportGenesis{} },
	func() steps.Step { return &steps.ExportEvents{} },
	func() steps.Step { return &steps.ImportEvents{} },
	func() steps.Step { return &steps.CheckDb{} },
}

// StepNames returns the name of every step a scenario may use.
func StepNames() []string {
	names := make([]string, 0, len(stepKinds))
	for _, kind := range stepKinds {
		names = append(names, kind().Name())
	}
	return names
}
