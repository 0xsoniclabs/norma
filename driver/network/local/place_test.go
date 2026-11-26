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

package local

import (
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/docker"
	"github.com/0xsoniclabs/norma/driver/node"
	"github.com/stretchr/testify/require"
)

func TestLocalNetwork_Place_GivesEachNodeAHostOfItsOwn_WhenHostsAreConfigured(t *testing.T) {
	type placement struct {
		label, volume string
		wantIP        string
		wantErr       string
	}
	tests := map[string][]placement{
		"nodes take the hosts in order": {
			{label: "a", wantIP: "10.0.0.1"},
			{label: "b", wantIP: "10.0.0.2"},
		},
		"a node keeps its host when it starts again": {
			{label: "a", wantIP: "10.0.0.1"},
			{label: "b", wantIP: "10.0.0.2"},
			{label: "a", wantIP: "10.0.0.1"},
		},
		"a data volume brings a later node to its host": {
			{label: "a", volume: "v", wantIP: "10.0.0.1"},
			{label: "b", wantIP: "10.0.0.2"},
			{label: "c", volume: "v", wantIP: "10.0.0.1"},
		},
		"a node without a free host is refused": {
			{label: "a", wantIP: "10.0.0.1"},
			{label: "b", wantIP: "10.0.0.2"},
			{label: "c", wantErr: "node c needs a host, but all 2 are taken"},
		},
	}
	for name, placements := range tests {
		t.Run(name, func(t *testing.T) {
			net := newPlacementTestNetwork("10.0.0.1", "10.0.0.2")
			for _, p := range placements {
				config := &node.OperaNodeConfig{Label: p.label}
				if p.volume != "" {
					config.DataVolume = &p.volume
				}

				client, err := net.place(config)

				if p.wantErr != "" {
					require.ErrorContains(t, err, p.wantErr)
					continue
				}
				require.NoError(t, err)
				require.Equal(t, p.wantIP, config.HostIP)
				require.Same(t, net.hostAt(p.wantIP).client, client)
			}
		})
	}
}

func TestLocalNetwork_Place_TakesAHostReservedForTheKindOfNode_WhenHostsAreReserved(t *testing.T) {
	type placement struct {
		label     string
		validator bool
		wantIP    string
		wantErr   string
	}
	tests := map[string][]placement{
		"each kind takes its own hosts in order": {
			{label: "rpc", wantIP: "10.0.0.2"},
			{label: "val-0", validator: true, wantIP: "10.0.0.1"},
			{label: "val-1", validator: true, wantIP: "10.0.0.3"},
		},
		"a kind without a free host is refused": {
			{label: "rpc-0", wantIP: "10.0.0.2"},
			{label: "rpc-1", wantErr: "node rpc-1 needs a host, but all 3 are taken"},
		},
	}
	for name, placements := range tests {
		t.Run(name, func(t *testing.T) {
			net := newPlacementTestNetwork("10.0.0.1", "10.0.0.2", "10.0.0.3")
			net.hosts[0].Validator, net.hosts[1].Validator, net.hosts[2].Validator = new(true), new(false), new(true)
			for _, p := range placements {
				config := &node.OperaNodeConfig{Label: p.label}
				if p.validator {
					config.ValidatorId = new(1)
				}

				_, err := net.place(config)

				if p.wantErr != "" {
					require.ErrorContains(t, err, p.wantErr)
					continue
				}
				require.NoError(t, err)
				require.Equal(t, p.wantIP, config.HostIP)
			}
		})
	}
}

func TestLocalNetwork_Place_UsesTheLocalDaemon_WhenNoHostsAreConfigured(t *testing.T) {
	local := &docker.Client{}
	net := newPlacementTestNetwork()
	net.docker = local
	volume := "v"
	config := &node.OperaNodeConfig{Label: "a", DataVolume: &volume}

	client, err := net.place(config)

	require.NoError(t, err)
	require.Same(t, local, client)
	require.Empty(t, config.HostIP)
	require.Same(t, local, net.volumes[volume])
}

func newPlacementTestNetwork(ips ...string) *LocalNetwork {
	net := &LocalNetwork{placement: map[string]*host{}, volumes: map[string]*docker.Client{}}
	for _, ip := range ips {
		net.hosts = append(net.hosts, &host{Host: driver.Host{Name: ip, IP: ip}, client: &docker.Client{}})
	}
	return net
}

func (n *LocalNetwork) hostAt(ip string) *host {
	for _, h := range n.hosts {
		if h.IP == ip {
			return h
		}
	}
	return nil
}
