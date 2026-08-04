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
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/load/app"
	"github.com/0xsoniclabs/norma/load/shaper"
)

// AppController produces load on a network. The Shaper it is given controls how
// many transactions per second are emitted; the generators it is given decide what
// those transactions are.
//
// Every user runs its own loop, and each of its transactions is drawn from one of
// the generators at random, weighted by the share of the load that generator is
// meant to produce. This way a single load covers every kind of transaction the
// network supports rather than one kind at a time.
type AppController struct {
	shaper     shaper.Shaper
	generators []deployedGenerator
	network    driver.Network
	trigger    chan struct{}
	numUsers   int
	sent       []atomic.Uint64
	// checks verifies the outcome of every transaction produced. It is nil when the
	// scenario turned transaction checks off.
	checks *checker
	// weights is the cumulative weight of the generators, used to draw one.
	weights     []int
	totalWeight int
}

type deployedGenerator struct {
	name      string
	generator app.Generator
}

// NewAppController installs the given generators on the network and prepares the
// load they produce. Generators the network rules do not support are skipped.
func NewAppController(
	instances []app.GeneratorInstance,
	shaper shaper.Shaper,
	numUsers int,
	checkTransactions bool,
	context app.AppContext,
	network driver.Network,
) (*AppController, error) {
	start := time.Now()
	slog.Info("start load initialization",
		"generators", len(instances), "numUsers", numUsers)

	deployed := make([]deployedGenerator, 0, len(instances))
	weights := make([]int, 0, len(instances))
	totalWeight := 0
	for _, instance := range instances {
		generatorStart := time.Now()
		err := instance.Generator.Deploy(context, numUsers)
		if errors.Is(err, app.ErrUnsupported) {
			slog.Info("skipping load generator not supported by the network rules",
				"generator", instance.Name)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to deploy the %s load generator; %w", instance.Name, err)
		}
		slog.Info("deployed load generator",
			"generator", instance.Name, "duration", time.Since(generatorStart))

		deployed = append(deployed, deployedGenerator{name: instance.Name, generator: instance.Generator})
		totalWeight += instance.Weight
		weights = append(weights, totalWeight)
	}
	if len(deployed) == 0 {
		return nil, fmt.Errorf("none of the load generators is supported by the network rules")
	}

	slog.Info("completed load initialization",
		"generators", len(deployed), "numUsers", numUsers, "duration", time.Since(start))

	controller := &AppController{
		shaper:      shaper,
		generators:  deployed,
		network:     network,
		trigger:     make(chan struct{}, 100),
		numUsers:    numUsers,
		sent:        make([]atomic.Uint64, numUsers),
		weights:     weights,
		totalWeight: totalWeight,
	}
	if checkTransactions {
		controller.checks = newChecker(context, network)
	}
	return controller, nil
}

func (ac *AppController) Run(ctx context.Context) error {
	var users, checks sync.WaitGroup

	// The checks outlive the load: once the users stop producing transactions the
	// checker still has to resolve the ones that were in flight, which is what
	// stopping it makes it do.
	stopChecks := func() {}
	if ac.checks != nil {
		checksCtx, cancel := context.WithCancel(context.Background())
		stopChecks = cancel
		checks.Add(1)
		go func() {
			defer checks.Done()
			ac.checks.run(checksCtx)
		}()
	}

	for user := 0; user < ac.numUsers; user++ {
		users.Add(1)
		go func() {
			defer users.Done()
			ac.runUserLoop(user)
		}()
	}

	var pending float64
	lastUpdate := time.Now()
	ac.shaper.Start(lastUpdate, ac)

	for {
		// Re-plenish the number of pending messages.
		now := time.Now()
		pending += ac.shaper.GetNumMessagesInInterval(lastUpdate, now.Sub(lastUpdate))
		lastUpdate = now

		for pending > 0 {
			ac.trigger <- struct{}{}
			pending -= 1
		}

		select {
		case <-time.After(time.Millisecond):
			// just waiting for next time to send messages.
		case <-ctx.Done():
			close(ac.trigger)
			users.Wait()
			stopChecks()
			checks.Wait()
			err := ctx.Err()
			if err == context.DeadlineExceeded || err == context.Canceled {
				return nil // terminated gracefully
			}
			return err
		}
	}
}

// runUserLoop produces one transaction per trigger, drawing the generator that
// produces it anew every time.
func (ac *AppController) runUserLoop(user int) {
	for range ac.trigger {
		generator := ac.pickGenerator()
		call, err := generator.generator.Call(user)
		if err != nil {
			slog.Error("failed to generate a transaction",
				"generator", generator.name, "error", err)
			continue
		}

		var onSent func(error)
		if ac.checks != nil {
			// Registering the transaction before sending it makes sure the report of
			// a refusal always finds it.
			ac.checks.submit(generator.name, generator.generator, call)
			hash := call.Tx.Hash()
			onSent = func(err error) { ac.checks.reportSent(hash, err) }
		}
		ac.network.SendTransaction(call.Tx, generator.name, onSent)
		ac.sent[user].Add(1)
	}
}

func (ac *AppController) pickGenerator() deployedGenerator {
	if len(ac.generators) == 1 {
		return ac.generators[0]
	}
	draw := rand.Intn(ac.totalWeight)
	for i, cumulative := range ac.weights {
		if draw < cumulative {
			return ac.generators[i]
		}
	}
	return ac.generators[len(ac.generators)-1]
}

// CheckResult reports the failures the transaction checks found, or nil if every
// transaction reached the outcome it was created for. It reports nil when the
// scenario turned transaction checks off.
func (ac *AppController) CheckResult() error {
	if ac.checks == nil {
		return nil
	}
	return ac.checks.result()
}

func (ac *AppController) GetNumberOfUsers() int {
	return ac.numUsers
}

func (ac *AppController) GetTransactionsSentBy(user int) (uint64, error) {
	if user < 0 || user >= ac.numUsers {
		return 0, nil
	}
	return ac.sent[user].Load(), nil
}

func (ac *AppController) GetSentTransactions() (uint64, error) {
	sum := uint64(0)
	for i := range ac.sent {
		sum += ac.sent[i].Load()
	}
	return sum, nil
}

// GetReceivedTransactions reports how many of the produced transactions the network
// has processed, counted by the transaction checks as they confirm the outcome of
// each transaction. Without those checks the number is not observable and stays
// zero, which leaves the auto shaper without its overload signal.
func (ac *AppController) GetReceivedTransactions() (uint64, error) {
	if ac.checks == nil {
		return 0, nil
	}
	return ac.checks.getConfirmedTransactions(), nil
}
