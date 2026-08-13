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

package driver

// GetClientImages returns the client image of every node the network may run:
// the genesis validators plus the nodes a scenario starts later, as listed in
// ClientImages. The genesis file is generated once for the whole network, so its
// version-dependent parts have to suit the clients in all of them; see
// docker.ClientVersions.
func (c *NetworkConfig) GetClientImages() []string {
	images := make([]string, 0, len(c.Validators)+len(c.ClientImages))
	for _, validator := range c.Validators {
		images = append(images, ResolveClientImageName(validator.ImageName))
	}
	return append(images, c.ClientImages...)
}
