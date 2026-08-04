// Copyright 2026 Fantom Foundation
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

package app

import (
	"github.com/0xsoniclabs/sonic/opera"
	"github.com/ethereum/go-ethereum/common"
)

// bls12G1AddAddress is the address of the BLS12-381 G1 addition precompile,
// introduced by Prague and available in Sonic starting with Allegro.
var bls12G1AddAddress = common.HexToAddress("0x0b")

// bls12G1AddInput is a valid pair of G1 points for the addition precompile.
var bls12G1AddInput = common.FromHex("0x0000000000000000000000000000000017f1d3a73197d7942695638c4fa9ac0fc3688c4f9774b905a14e3a3f171bac586c55e83ff97a1aeffb3af00adb22c6bb0000000000000000000000000000000008b3f481e3aaa0f1a09e30ed741d8ae4fcf5e095d5d00af600db18cb2c04b3edd03cc744a2888ae40caa232946c5e7e100000000000000000000000000000000112b98340eee2777cc3c14163dea3ec97977ac3dc5c70da32e6e87578f44912e902ccef9efe28d4a78b8999dfbca942600000000000000000000000000000000186b28d92356c4dfec4b5201ad099dbdede3781f8998ddf929b4cd7756192185ca7b8f4ef7088f813270ac3d48868a21")

// newBls12AddGenerator creates a generator adding two points on the BLS12-381
// curve through the precompile at address 0x0b. The precompile has no revert path:
// an input it does not accept aborts the call and consumes all its gas, which the
// generator covers with its failing transactions.
func newBls12AddGenerator(feederId, appId uint32) Generator {
	g := &bls12AddGenerator{txGenerator: newTxGenerator(feederId, appId)}
	g.supports = func(rules opera.Rules) bool { return rules.Upgrades.Allegro }
	g.onSuccess = g.addPoints
	return g
}

type bls12AddGenerator struct {
	txGenerator
}

func (g *bls12AddGenerator) addPoints(int, *Account) (txPayload, error) {
	return txPayload{
		to:   &bls12G1AddAddress,
		data: bls12G1AddInput,
		// The limit leaves room for the data floor gas on top of the precompile.
		gasLimit: 45_000,
	}, nil
}
