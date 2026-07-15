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
	"fmt"
	"regexp"
)

const namePatternStr = `^[A-Za-z0-9-.]+$`

// NamePattern is the regular expression that all node, validator, and
// application identifiers must match.
var NamePattern = regexp.MustCompile(namePatternStr)

// isTypeValid reports whether the given node type is one of the supported
// values (observer, rpc, validator).
func isTypeValid(t string) error {
	switch t {
	case "validator", "rpc", "observer":
		return nil
	}
	return fmt.Errorf("type of node must be observer, rpc or validator, was set to %s", t)
}
