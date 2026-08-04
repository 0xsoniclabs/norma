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
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/core/types"
)

//go:generate mockgen -source generator.go -destination generator_mock.go -package app

// Outcome is the result a generated transaction is expected to produce.
type Outcome int

const (
	// Success is included in a block and runs to completion.
	Success Outcome = iota
	// Reverted is included in a block and aborted by a REVERT, which refunds the
	// gas the call did not use.
	Reverted
	// Failed is included in a block and aborted without revert data, which
	// consumes all the gas the call was given.
	Failed
	// Rejected never reaches a block.
	Rejected
	// Indeterminate leaves the network free to include the transaction or to drop
	// it. A generator using it verifies the effect of the transaction by other
	// means in its own Check.
	Indeterminate
)

func (o Outcome) String() string {
	switch o {
	case Success:
		return "success"
	case Reverted:
		return "reverted"
	case Failed:
		return "failed"
	case Rejected:
		return "rejected"
	case Indeterminate:
		return "indeterminate"
	}
	return fmt.Sprintf("Outcome(%d)", int(o))
}

// Call is a transaction produced by a Generator together with the outcome it is
// expected to produce on the network it was created for.
type Call struct {
	Tx      *types.Transaction
	Outcome Outcome
	// Reason is the revert reason expected of a Reverted call. An empty Reason
	// accepts any revert reason.
	Reason string
}

// ErrUnsupported is returned by Deploy when the network rules in effect do not
// allow the kind of load a generator produces. Such a generator is skipped
// rather than failing the scenario.
var ErrUnsupported = errors.New("unsupported by the network rules")

// Generator produces one kind of load on the network. A generator is deployed
// once and then produces transactions for a fixed number of users; calls for
// distinct users may run concurrently.
type Generator interface {
	// Deploy installs the contracts this generator needs on the chain and
	// prepares the accounts of numUsers users. It returns ErrUnsupported if the
	// network rules in effect do not allow this kind of load.
	Deploy(ctxt AppContext, numUsers int) error

	// Call produces the next transaction for the given user, choosing one of the
	// outcomes the network rules allow it to produce.
	Call(user int) (Call, error)

	// Check verifies that the given call produced the outcome it was created
	// for. The receipt is the one the network produced for the call, or nil if
	// the transaction was never included in a block.
	Check(ctxt AppContext, call Call, receipt *types.Receipt) error
}
