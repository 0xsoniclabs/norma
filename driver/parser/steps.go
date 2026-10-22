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
	"github.com/0xsoniclabs/norma/driver/steps/apps"
	"github.com/0xsoniclabs/norma/driver/steps/checks"
	"github.com/0xsoniclabs/norma/driver/steps/network"
	"github.com/0xsoniclabs/norma/driver/steps/nodelifetime"
	"github.com/0xsoniclabs/norma/driver/steps/sonictool"
	"github.com/0xsoniclabs/norma/driver/steps/wait"
)

// stepKinds lists every step a scenario may use.
var stepKinds = []func() steps.Step{
	func() steps.Step { return &nodelifetime.StartNode{} },
	func() steps.Step { return &nodelifetime.PrepareNode{} },
	func() steps.Step { return &nodelifetime.StopNode{} },
	func() steps.Step { return &nodelifetime.StopSonic{} },
	func() steps.Step { return &nodelifetime.KillSonic{} },

	func() steps.Step { return &sonictool.HealDb{} },
	func() steps.Step { return &sonictool.ExportGenesis{} },
	func() steps.Step { return &sonictool.ImportGenesis{} },
	func() steps.Step { return &sonictool.ExportEvents{} },
	func() steps.Step { return &sonictool.ImportEvents{} },

	func() steps.Step { return &apps.RunApp{} },
	func() steps.Step { return &apps.StopApp{} },

	func() steps.Step { return &network.Delegate{} },
	func() steps.Step { return &network.Undelegate{} },
	func() steps.Step { return &network.VerifyStakes{} },
	func() steps.Step { return &network.UpdateRules{} },
	func() steps.Step { return &network.AdvanceEpoch{} },

	func() steps.Step { return &wait.WaitFor{} },
	func() steps.Step { return &wait.WaitForBlocks{} },
	func() steps.Step { return &wait.WaitForEpoch{} },
	func() steps.Step { return &wait.WaitForSonicExit{} },

	func() steps.Step { return &checks.Checks{} },
	func() steps.Step { return &checks.CheckDb{} },
}

// StepNames returns the name of every step a scenario may use.
func StepNames() []string {
	names := make([]string, 0, len(stepKinds))
	for _, kind := range stepKinds {
		names = append(names, kind().Name())
	}
	return names
}
