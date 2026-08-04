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
	"fmt"
	"slices"
	"strings"
)

type generatorFactory func(feederId, appId uint32) Generator

type registryEntry struct {
	name string
	// weight is the share of the load this generator produces within the mix.
	weight int
	create generatorFactory
}

// generators lists every kind of load Norma can produce. Unless a scenario asks
// for a single kind, all of them are deployed and each transaction is drawn from
// one of them at random, weighted by the weights below. Generators the network
// rules do not support are skipped.
var generators = []registryEntry{
	{"erc20", 10, newErc20Generator},
	{"counter", 10, newCounterGenerator},
	{"store", 10, newStoreGenerator},
	{"uniswap", 5, newUniswapGenerator},
	{"ecdsa", 2, newEcdsaGenerator},
	{"bls12add", 2, newBls12AddGenerator},
	{"transient", 1, newTransientGenerator},
	{"smartaccount", 1, newSmartAccountGenerator},
	{"subsidies", 1, newSubsidiesGenerator},
	{"selfdestructoldcontract", 1, newSelfDestructOldContractGenerator},
	{"selfdestructnewcontract", 1, newSelfDestructNewContractGenerator},
	{"largecontract", 1, newLargeContractGenerator},
	{"allofbundle", 3, newAllOfBundleGenerator},
	{"oneofbundle", 3, newOneOfBundleGenerator},
	{"subsidizedbundle", 3, newSubsidizedBundleGenerator},
	{"failingbundle", 1, newFailingBundleGenerator},
	{"duplicatedbundle", 1, newDuplicatedBundleGenerator},
}

// GeneratorInstance is a generator together with the name and weight it was
// registered under and the app id namespacing its accounts.
type GeneratorInstance struct {
	Name      string
	Weight    int
	AppId     uint32
	Generator Generator
}

// NewGenerators creates the generators for the given load type. An empty type
// yields the full mix of every registered generator; any other value yields the
// single generator registered under that name.
//
// Accounts are namespaced per generator so that concurrently running generators
// never share an account: the app id of a generator is derived from the app id of
// the load it belongs to and its position in the registry.
func NewGenerators(loadType string, feederId, appId uint32) ([]GeneratorInstance, error) {
	entries := generators
	if loadType != "" {
		index := indexOfGenerator(loadType)
		if index < 0 {
			return nil, fmt.Errorf("unknown load generator type '%s'", loadType)
		}
		entries = generators[index : index+1]
	}

	instances := make([]GeneratorInstance, 0, len(entries))
	for _, entry := range entries {
		index := indexOfGenerator(entry.name)
		generatorAppId := appId*uint32(len(generators)) + uint32(index)
		instances = append(instances, GeneratorInstance{
			Name:      entry.name,
			Weight:    entry.weight,
			AppId:     generatorAppId,
			Generator: entry.create(feederId, generatorAppId),
		})
	}
	return instances, nil
}

// IsSupportedLoadType reports whether the given load type names a registered
// generator. An empty type stands for the full mix and is always supported.
func IsSupportedLoadType(loadType string) bool {
	return loadType == "" || indexOfGenerator(loadType) >= 0
}

// LoadTypes lists the names of all registered generators.
func LoadTypes() []string {
	names := make([]string, 0, len(generators))
	for _, entry := range generators {
		names = append(names, entry.name)
	}
	return names
}

func indexOfGenerator(name string) int {
	name = strings.ToLower(name)
	return slices.IndexFunc(generators, func(entry registryEntry) bool {
		return entry.name == name
	})
}
