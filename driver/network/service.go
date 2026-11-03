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

package network

import (
	"fmt"
)

// ServiceDescription is
type ServiceDescription struct {
	Name     string
	Port     Port
	Protocol string
}

type ServiceGroup map[Port]*ServiceDescription

// RegisterService installs a supported service in the registry.
func (s *ServiceGroup) RegisterService(service *ServiceDescription) error {
	if _, exists := (*s)[service.Port]; exists {
		return fmt.Errorf("port %d already assigned - it is not supported to bind the same port many times", service.Port)
	}

	(*s)[service.Port] = service
	return nil
}

// Services returns all registered services from the registry.
func (s *ServiceGroup) Services() []*ServiceDescription {
	res := make([]*ServiceDescription, 0, len(*s))
	for _, v := range *s {
		res = append(res, v)
	}

	return res
}

// Port provides an alias type for a TCP port.
type Port uint16
