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
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/0xsoniclabs/norma/load/shaper"
)

// AppController emits transactions to the testing network into a blockchain app to generate a load.
// The Shaper passed into the driver controls the frequency of emitting transactions,
// or, if it is shaper.Saturate, lets every user submit as fast as the network accepts.
// The users of the application construct the transactions.
// The controller reads the progress of the app through a client of its own,
// since the client of the app context is shared by all apps. The client lives
// as long as the controller: the progress of a stopped app is still read.
type AppController struct {
	name        string
	shaper      shaper.Shaper
	application app.Application
	network     driver.Network
	users       []app.User
	accepted    []atomic.Uint64 // accepted transactions, by user

	rpcClientMu sync.Mutex // guards rpcClient, used by both the shaper and the monitoring
	rpcClient   rpc.Client // dialed on first use
}

// NewAppController creates the controller of an application. The name is the
// one the scenario gave the application; it travels with every transaction so
// that monitoring can attribute it to this application rather than to the type
// of load it generates - two applications of different types can generate the
// same transactions.
func NewAppController(
	name string,
	application app.Application,
	shaper shaper.Shaper,
	numUsers int,
	context app.AppContext,
	network driver.Network,
) (*AppController, error) {
	start := time.Now()

	// create users for this application
	slog.Info("start app initialization",
		"app", reflect.TypeOf(application).Elem().Name(),
		"numUsers", numUsers)
	users, err := application.CreateUsers(context, numUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to create users for app; %v", err)
	}
	slog.Info("completed app initialization",
		"app", reflect.TypeOf(application).Elem().Name(),
		"numUsers", numUsers,
		"duration", time.Since(start))

	return &AppController{
		name:        name,
		shaper:      shaper,
		application: application,
		network:     network,
		users:       users,
		accepted:    make([]atomic.Uint64, len(users)),
	}, nil
}

func (ac *AppController) Run(ctx context.Context) error {
	var trigger chan struct{}
	if _, saturate := ac.shaper.(shaper.Saturate); !saturate {
		trigger = make(chan struct{}, len(ac.users))
	}

	var done sync.WaitGroup
	for i, user := range ac.users {
		source := driver.TransactionSource{App: ac.name, User: i}
		done.Go(func() {
			runGeneratorLoop(ctx, user, source, trigger, ac.network, &ac.accepted[i])
		})
	}
	if trigger != nil {
		ac.pace(ctx, trigger)
	}
	<-ctx.Done()
	done.Wait()
	return nil
}

// pace hands out the signals for the users to submit their next transaction at
// the rate of the shaper, until the context ends. When all users still wait for
// the network to accept their previous transaction, it holds until one is free
// rather than piling up signals, and does not make up for the time it held.
func (ac *AppController) pace(ctx context.Context, trigger chan<- struct{}) {
	var pending float64
	lastUpdate := time.Now()
	ac.shaper.Start(lastUpdate)
	var lastWarning time.Time
	for {
		now := time.Now()
		pending += ac.shaper.GetNumMessagesInInterval(lastUpdate, now.Sub(lastUpdate))
		lastUpdate = now

		for ; pending > 0; pending-- {
			select {
			case trigger <- struct{}{}:
				continue
			default:
			}
			if time.Since(lastWarning) >= 10*time.Second {
				slog.Warn("app cannot sustain its rate, the network does not accept its transactions fast enough",
					"app", ac.name)
				lastWarning = time.Now()
			}
			select {
			case trigger <- struct{}{}:
			case <-ctx.Done():
				return
			}
			lastUpdate = time.Now()
			pending = min(pending, 1)
		}

		if !wait(ctx, time.Millisecond) {
			return
		}
	}
}

func (ac *AppController) GetNumberOfUsers() int {
	return len(ac.users)
}

func (ac *AppController) GetTransactionsSentBy(user int) (uint64, error) {
	if user < 0 || user >= len(ac.users) {
		return 0, nil
	}
	return ac.accepted[user].Load(), nil
}

// GetSentTransactions returns the number of transactions the network accepted
// from all users.
func (ac *AppController) GetSentTransactions() (uint64, error) {
	sum := uint64(0)
	for i := range ac.accepted {
		sum += ac.accepted[i].Load()
	}
	return sum, nil
}

func (ac *AppController) GetReceivedTransactions() (uint64, error) {
	ac.rpcClientMu.Lock()
	defer ac.rpcClientMu.Unlock()
	for retry := 0; ; retry++ {
		if ac.rpcClient == nil {
			var err error
			ac.rpcClient, err = ac.network.DialRandomRpc()
			if err != nil {
				return 0, fmt.Errorf("failed to dial random RPC: %w", err)
			}
		}
		// fetch transaction data from the network
		res, err := ac.application.GetReceivedTransactions(ac.rpcClient)
		if err == nil {
			return res, nil
		}
		if retry >= 5 {
			return 0, err
		}

		// attempt a re-connect
		ac.rpcClient.Close()
		ac.rpcClient, err = ac.network.DialRandomRpc()
		if err != nil {
			ac.rpcClient = nil
			return 0, fmt.Errorf("failed to dial random RPC: %w", err)
		}
	}
}
