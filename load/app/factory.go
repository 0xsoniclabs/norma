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

package app

import (
	"fmt"
	"strings"
)

type appFactoryFunc func(context AppContext, feederId, appId uint32) (Application, error)

// NewApplication creates an application of the given type.
//
// The load parameter names the traffic of application types that do not define
// their own but carry another type's - the priority lanes, which are a property
// of a load rather than a load themselves. It must be empty for every other
// type; see AcceptsLoadParameter.
func NewApplication(appType, load string, context AppContext, feederId, appId uint32) (Application, error) {
	if load != "" {
		if !AcceptsLoadParameter(appType) {
			return nil, fmt.Errorf(
				"application type '%s' generates its own load, so it takes no load parameter", appType)
		}
		if !IsSupportedApplicationType(load) {
			return nil, fmt.Errorf("unknown load '%s' for application type '%s'", load, appType)
		}
		// A lane inside a lane would register the same accounts twice and mean
		// nothing beyond the single lane it already is.
		if AcceptsLoadParameter(load) {
			return nil, fmt.Errorf("load '%s' carries a load of its own, which cannot be nested", load)
		}
	}
	if factory := getFactory(appType, load); factory != nil {
		return factory(context, feederId, appId)
	}
	return nil, fmt.Errorf("unknown application type '%s'", appType)
}

func IsSupportedApplicationType(appType string) bool {
	return getFactory(appType, "") != nil
}

// AcceptsLoadParameter reports whether the given application type carries the
// load of another one, which the load parameter of NewApplication selects.
func AcceptsLoadParameter(appType string) bool {
	return strings.ToLower(appType) == "priority"
}

func getFactory(appType, load string) appFactoryFunc {
	switch strings.ToLower(appType) {
	case "erc20":
		return NewERC20Application
	case "counter", "":
		return NewCounterApplication
	case "store":
		return NewStoreApplication
	case "uniswap":
		return NewUniswapApplication
	case "smartaccount":
		return NewSmartAccountApplication
	case "subsidies":
		return NewSubsidiesApplication
	case "priority":
		// An empty load is the counter one, which is the cheapest traffic to
		// compare a lane against.
		if load == "" {
			load = "counter"
		}
		return NewPrioritizedApplication(load)
	case "transient":
		return NewTransientApplication
	case "selfdestructoldcontract":
		return NewSelfDestructOldContractApplication
	case "selfdestructnewcontract":
		return NewSelfDestructNewContractApplication
	case "ecdsa":
		return NewEcdsaApplication
	case "largecontract":
		return NewLargeContractApplication
	case "allofbundle":
		return NewAllOfBundleApplication
	case "oneofbundle":
		return NewOneOfBundleApplication
	case "subsidizedbundle":
		return NewSubsidizedBundleApplication
	case "failingbundle":
		return NewFailingBundleApplication
	case "duplicatedbundle":
		return NewDuplicatedBundleApplication
	case "bls12add":
		return NewBls12AddApplication
	case "mix":
		return NewMixApplication
	}
	return nil
}
