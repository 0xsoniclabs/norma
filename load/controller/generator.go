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

package controller

import (
	"context"
	"iter"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	minRetryDelay = 10 * time.Millisecond
	// maxRetryDelay is short so that a full pool, once it has room, is filled
	// again quickly by the users waiting for it.
	maxRetryDelay = 100 * time.Millisecond
	// stallWarning is how long a transaction may wait for the network to
	// accept it before the wait is reported.
	stallWarning = 30 * time.Second
)

// runGeneratorLoop submits the transactions of a user, each once it takes a
// signal from the trigger, or back to back when there is no trigger. It pulls
// the next transaction only once the network took the current one, or refused
// it for good, so the nonces of the user never have a gap. It counts the
// transactions the network took.
func runGeneratorLoop(
	ctx context.Context,
	user app.User,
	source driver.TransactionSource,
	trigger <-chan struct{},
	network driver.Network,
	accepted *atomic.Uint64,
) {
	next, stop := iter.Pull2(user.Transactions())
	defer stop()
	for {
		if trigger != nil {
			select {
			case <-trigger:
			case <-ctx.Done():
				return
			}
		}
		tx, err, ok := next()
		if !ok {
			return
		}
		if err != nil {
			slog.Error("failed to generate tx", "error", err, "source", source)
			if !wait(ctx, minRetryDelay) {
				return
			}
			continue
		}
		if submit(ctx, network, tx, source) {
			accepted.Add(1)
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// submit sends the transaction until the network takes it, waiting longer
// after each rejection, and reports whether it did. It gives up when the
// context ends or the network refuses the transaction for good.
func submit(
	ctx context.Context,
	network driver.Network,
	tx *types.Transaction,
	source driver.TransactionSource,
) bool {
	start, warned := time.Now(), false
	for delay := minRetryDelay; ; delay = min(2*delay, maxRetryDelay) {
		err := network.SendTransaction(ctx, tx, source)
		switch {
		case err == nil || hasAny(err, known):
			return true
		case hasAny(err, refused):
			return false
		}
		if !warned && time.Since(start) >= stallWarning {
			slog.Warn("network keeps rejecting tx, retrying",
				"source", source, "nonce", tx.Nonce(), "error", err)
			warned = true
		}
		if !wait(ctx, delay) {
			return false
		}
	}
}

// known are the rejections of a transaction the network already has, in its
// pool or on chain.
var known = []string{"already known", "nonce too low", "bundle has already been processed"}

// refused are the rejections no resubmission overcomes. Only bundles are
// refused like this, and bundle apps take the nonces of each bundle from the
// chain, so dropping one leaves no gap.
var refused = []string{"invalid bundle transaction", "bundle is not executable", "approves no execution plan"}

// hasAny tells whether the message of the error contains any of the given texts.
func hasAny(err error, texts []string) bool {
	return slices.ContainsFunc(texts, func(text string) bool {
		return strings.Contains(err.Error(), text)
	})
}

// wait waits for the given delay and returns false if the context ends first.
func wait(ctx context.Context, delay time.Duration) bool {
	select {
	case <-time.After(delay):
		return true
	case <-ctx.Done():
		return false
	}
}
