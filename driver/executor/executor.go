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

package executor

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/0xsoniclabs/norma/driver/checking"
	"github.com/0xsoniclabs/norma/driver/network"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/parser"
	pq "github.com/jupp0r/go-priority-queue"
)

//go:generate mockgen -source executor.go -destination executor_mock.go -package executor

// Run executes the given scenario on the given network using the provided clock
// as a time source. Execution will fail (fast) if the scenario is not valid (see
// Scenario's Check() function).
func Run(clock Clock, network driver.Network, scenario *parser.Scenario, checks checking.Checks) error {
	return run(clock, network, scenario, checks, &netBasedValidatorRegistry{
		net: network,
	})
}

// run is the internal implementation of the Run function, allowing to
// inject a validatorRegistry for testing purposes.
func run(
	clock Clock,
	network driver.Network,
	scenario *parser.Scenario,
	checks checking.Checks,
	registry validatorRegistry,
) error {
	if err := scenario.Check(); err != nil {
		return err
	}

	queue := newEventQueue()

	// Schedule end of simulation as a dummy event.
	endTime := Seconds(scenario.Duration)
	queue.add(toSingleEvent(endTime, "shutdown", func() error {
		return nil
	}))

	// schedule network consistency just before the end of simulation
	if checks != nil {
		queue.add(toSingleEvent(endTime-1, "consistency check", func() error {
			log.Printf("Checking network consistency ...\n")
			return checks.Check()
		}))
	} else {
		fmt.Printf("Network checks skipped\n")
	}

	// Schedule all operations listed in the scenario.
	scheduleValidatorEvents(scenario.Validators, queue, network, registry)
	for _, node := range scenario.Nodes {
		scheduleNodeEvents(&node, queue, network, endTime, registry)
	}
	for _, app := range scenario.Applications {
		if err := scheduleApplicationEvents(&app, queue, network, endTime); err != nil {
			return err
		}
	}
	for _, cheat := range scenario.Cheats {
		scheduleCheatEvents(&cheat, queue, network, endTime)
	}
	for _, rule := range scenario.NetworkRules.Updates {
		scheduleNetworkRulesEvents(rule, queue, network)
	}
	for _, adv := range scenario.AdvanceEpoch {
		epochs := 1
		if adv.Epochs != nil {
			epochs = *adv.Epochs
		}
		scheduleAdvanceEpochEvents(adv.Time, epochs, queue, network)
	}

	for _, c := range scenario.Checks {
		checker := checks.GetCheckerByName(c.Check)
		if checker == nil {
			return fmt.Errorf("check '%s' not found", c.Check)
		}

		configured, err := checker.Configure(c.Config)
		if err != nil {
			return fmt.Errorf("error configuring checks; %v", err)
		}

		scheduleCheckEvents(c.Time, c.Check, configured, queue, network)
	}

	// Register a handler for Ctrl+C events.
	abort := make(chan os.Signal, 1)
	signal.Notify(abort, os.Interrupt)
	defer signal.Stop(abort)

	// restart clock as network initialization could time considerable amount of time.
	clock.Restart()
	// Run all events.
	for !queue.empty() {
		event := queue.getNext()
		if event == nil {
			break
		}

		// Wait until the event is going to occur ...
		select {
		case <-clock.NotifyAt(event.time()):
			// continue processing
		case <-abort:
			// abort processing
			slog.Warn("received user abort, ending execution")
			return fmt.Errorf("aborted by user")
		}

		delay := clock.Delay(event.time())
		// display delay if it exceeds over 1 second
		if delay > time.Second {
			slog.Warn("starting processing event with delay",
				"time", clock.Now(),
				"name", event.name(),
				"event_time", event.time(),
				"delay", delay.Round(time.Second/10).Seconds(),
			)
		} else {
			slog.Info("starting processing event",
				"time", clock.Now(),
				"name", event.name(),
				"event_time", event.time(),
			)
		}

		// Execute the event and schedule successors.
		start := time.Now()
		successors, err := event.run()
		if err != nil {
			slog.Error("event execution failed",
				"time", clock.Now(),
				"name", event.name(),
				"event_time", event.time(),
				"error", err,
				"duration", time.Since(start).Round(time.Millisecond),
			)
			return err
		}

		duration := time.Since(start)

		level := slog.LevelInfo
		msg := "processing of event completed"
		if duration > 5*time.Second {
			level = slog.LevelWarn
			msg += " (slow execution)"
		}
		slog.Log(context.Background(), level, msg,
			"time", clock.Now(),
			"name", event.name(),
			"event_time", event.time(),
			"duration", duration,
		)

		queue.addAll(successors)
	}

	return nil
}

// event is a single action required to happen at (approximately) a given time.
type event interface {
	// The time at which the event is to be processed.
	time() Time
	// A short name describing the event for logging.
	name() string
	// Executes the event's action, potentially triggering successor events.
	run() ([]event, error)
}

// eventQueue is a type-safe wrapper of a priority queue to organize events
// to be scheduled and executed during a scenario run.
type eventQueue struct {
	queue pq.PriorityQueue
}

func newEventQueue() *eventQueue {
	return &eventQueue{pq.New()}
}

func (q *eventQueue) empty() bool {
	return q.queue.Len() == 0
}

func (q *eventQueue) add(event event) {
	q.queue.Insert(event, float64(event.time()))
}

func (q *eventQueue) addAll(events []event) {
	for _, event := range events {
		q.add(event)
	}
}

func (q *eventQueue) getNext() event {
	res, err := q.queue.Pop()
	if err != nil {
		log.Printf("Warning: event queue error encountered: %v", err)
		return nil
	}
	return res.(event)
}

// genericEvent is an implementation of an event combining an action-defining
// lambda with a time stamp determining its execution time.
type genericEvent struct {
	eventTime Time
	eventName string
	action    func() ([]event, error)
}

func (e *genericEvent) time() Time {
	return e.eventTime
}

func (e *genericEvent) name() string {
	return e.eventName
}

func (e *genericEvent) run() ([]event, error) {
	return e.action()
}

func toEvent(time Time, name string, action func() ([]event, error)) event {
	return &genericEvent{time, name, action}
}

func toSingleEvent(time Time, name string, action func() error) event {
	return toEvent(time, name, func() ([]event, error) {
		return nil, action()
	})
}

// scheduleValidatorEvents schedules activities to be performed on the set
// of validators established during network startup.
func scheduleValidatorEvents(
	validators []parser.Validator,
	queue *eventQueue,
	net driver.Network,
	registry validatorRegistry,
) {
	getNodeByName := func(name string) (driver.Node, error) {
		for _, node := range net.GetActiveNodes() {
			if node.GetLabel() == name {
				return node, nil
			}
		}
		return nil, fmt.Errorf("validator node %s not found", name)
	}

	for _, group := range validators {
		// The only event to be scheduled is a potential shutdown event.
		if group.End == nil {
			continue
		}
		instances := 1
		if group.Instances != nil {
			instances = *group.Instances
		}
		for i := range instances {
			name := fmt.Sprintf("%s-%d", group.Name, i)
			queue.add(toSingleEvent(
				Seconds(*group.End),
				fmt.Sprintf("[%s] Stop Validator", name),
				func() error {
					// Find the validator node by name.
					node, err := getNodeByName(name)
					if err != nil {
						return err
					}

					if id := node.GetValidatorId(); id != nil {
						if err := registry.unregisterValidator(*id); err != nil {
							return fmt.Errorf("failed to unregister validator %s; %v", name, err)
						}
					}
					if err := net.RemoveNode(node); err != nil {
						return fmt.Errorf("failed to remove validator %s; %v", name, err)
					}
					if err := node.Stop(); err != nil {
						return fmt.Errorf("failed to stop validator %s; %v", name, err)
					}
					if err := node.Cleanup(); err != nil {
						return fmt.Errorf("failed to cleanup validator %s; %v", name, err)
					}
					return nil
				},
			))
		}
	}
}

// validatorRegistry abstracts how an executor registers and unregisters
// validator nodes with the network.
type validatorRegistry interface {
	registerNewValidator() (int, error)
	unregisterValidator(validatorId int) error
}

// scheduleNodeEvents schedules a number of events covering the life-cycle of a class of
// nodes during the scenario execution. The nature of the scheduled nodes is taken from the
// given node description, and actions are applied to the given network.
// Node Lifecycle: create -> timer sim events {start, rejoin, end, leave} -> remove
func scheduleNodeEvents(
	node *parser.Node,
	queue *eventQueue,
	net driver.Network,
	end Time,
	registry validatorRegistry,
) {
	instances := 1
	if node.Instances != nil {
		instances = *node.Instances
	}
	startTime := Time(0)
	if node.Start != nil {
		startTime = Seconds(*node.Start)
	}
	endTime := end
	if node.End != nil {
		endTime = Seconds(*node.End)
	}

	nodeIsValidator := false
	if node.Client.Type == "validator" {
		nodeIsValidator = true
	}
	nodeIsCheater := false

	image := driver.DefaultClientDockerImageName
	if node.Client.ImageName != "" {
		image = node.Client.ImageName
	}

	for i := 0; i < instances; i++ {
		name := fmt.Sprintf("%s-%d", node.Name, i)
		var instance = new(driver.Node)

		if node.Start != nil {
			queue.add(toSingleEvent(
				startTime,
				fmt.Sprintf("[%s] Creating node", name),
				func() error {
					// Validators only need to be registered if they are started
					// and not rejoining.
					//
					// If specifically assigned an id, then it is checked that
					// the ID that was determined by the network matches the
					// one that was explicitly provided.
					//
					// When generating ID for multiple instances, assume the
					// sequence starting at the assigned id, e.g. node with
					// instances = 3, client.validatorId = 10 will get 10, 11, 12.
					if nodeIsValidator {
						id, err := registry.registerNewValidator()
						if err != nil {
							return fmt.Errorf("failed to register validator node; %v", err)
						}
						// If an explicit validator ID is provided, make sure it
						// matches the one that was obtained from the network.
						if node.Client.ValidatorId != nil {
							if want, got := *node.Client.ValidatorId+i, id; want != got {
								return fmt.Errorf("validator ID mismatch: expected %d, got %d", want, got)
							}
						} else {
							node.Client.ValidatorId = new(int)
							*node.Client.ValidatorId = id
						}
					}

					newNode, err := net.CreateNode(&driver.NodeConfig{
						Name:        name,
						Failing:     node.Failing,
						Image:       image,
						Validator:   nodeIsValidator,
						ValidatorId: node.Client.ValidatorId,
						Cheater:     nodeIsCheater,
						DataVolume:  node.Client.DataVolume,
					})

					*instance = newNode
					return err
				},
			))
		}

		if node.Rejoin != nil {
			queue.add(toSingleEvent(
				Seconds(*node.Rejoin),
				fmt.Sprintf("[%s] Creating rejoining node", name),
				func() error {
					newNode, err := net.CreateNode(&driver.NodeConfig{
						Name:        name,
						Failing:     node.Failing,
						Image:       image,
						Validator:   nodeIsValidator,
						ValidatorId: node.Client.ValidatorId,
						Cheater:     nodeIsCheater,
						DataVolume:  node.Client.DataVolume,
					})

					*instance = newNode
					return err
				},
			))
		}

		if node.Leave != nil {
			queue.add(toSingleEvent(
				Seconds(*node.Leave),
				fmt.Sprintf("[%s] Node Leaving", name),
				func() error {
					if instance == nil {
						return nil
					}
					if err := net.RemoveNode(*instance); err != nil {
						return err
					}
					if err := (*instance).Stop(); err != nil {
						return err
					}
					if err := (*instance).Cleanup(); err != nil {
						return err
					}
					return nil
				},
			))
		}

		if node.End != nil {
			queue.add(toSingleEvent(
				endTime,
				fmt.Sprintf("[%s] Stop Node", name),
				func() error {
					if instance == nil {
						return nil
					}
					// Validators only need to be unregistered if they are stopped
					// before the end of the scenario. At the end of the scenario,
					// validators can no longer be unregistered since the network
					// is being shut down, losing the ability to run transactions.
					if endTime != end && nodeIsValidator {
						if id := (*instance).GetValidatorId(); id != nil {
							registry.unregisterValidator(*id)
						}
					}
					if err := net.RemoveNode(*instance); err != nil {
						return err
					}
					if err := (*instance).Stop(); err != nil {
						return err
					}
					if err := (*instance).Cleanup(); err != nil {
						return err
					}
					return nil
				},
			))
		}
	}
}

type netBasedValidatorRegistry struct {
	net driver.Network
}

func (a netBasedValidatorRegistry) registerNewValidator() (int, error) {
	rpcClient, err := a.net.DialRandomRpc()
	if err != nil {
		return 0, fmt.Errorf("failed to connect to RPC; %v", err)
	}
	defer rpcClient.Close()
	id, err := network.RegisterValidatorNode(rpcClient)
	if err != nil {
		return 0, fmt.Errorf("failed to register validator node; %v", err)
	}

	return id, nil
}

func (a netBasedValidatorRegistry) unregisterValidator(validatorId int) error {
	rpcClient, err := a.net.DialRandomRpc()
	if err != nil {
		return fmt.Errorf("failed to connect to RPC; %v", err)
	}
	defer rpcClient.Close()
	err = network.UnregisterValidatorNode(rpcClient, validatorId)
	if err != nil {
		return fmt.Errorf("failed to unregister validator node; %v", err)
	}
	return nil
}

// scheduleApplicationEvents schedules a number of events covering the life-cycle of a class of
// applications during the scenario execution. The nature of the scheduled applications is taken from the
// given application description, and actions are applied to the given network.
func scheduleApplicationEvents(source *parser.Application, queue *eventQueue, net driver.Network, end Time) error {
	instances := 1
	if source.Instances != nil {
		instances = *source.Instances
	}
	users := 1
	if source.Users != nil {
		users = *source.Users
	}
	startTime := Time(0)
	if source.Start != nil {
		startTime = Seconds(*source.Start)
	}
	endTime := end
	if source.End != nil {
		endTime = Seconds(*source.End)
	}

	for i := 0; i < instances; i++ {
		name := fmt.Sprintf("%s-%d", source.Name, i)
		newApp, err := net.CreateApplication(&driver.ApplicationConfig{
			Name:  name,
			Type:  source.Type,
			Rate:  &source.Rate,
			Users: users,
		})
		if err != nil {
			return err
		}
		queue.add(toSingleEvent(startTime, fmt.Sprintf("starting app %s", name), func() error {
			return newApp.Start()
		}))
		queue.add(toSingleEvent(endTime, fmt.Sprintf("stopping app %s", name), func() error {
			return newApp.Stop()
		}))
	}
	return nil
}

// scheduleCheatEvents schedules a number of events covering the life-cycle of a class of
// cheats during the scenario execution. Currently, a cheat is defined a simultaneous start
// of multiple validator nodes with the same key.
func scheduleCheatEvents(cheat *parser.Cheat, queue *eventQueue, net driver.Network, end Time) {
	startTime := Time(0)
	if cheat.Start != nil {
		startTime = Seconds(*cheat.Start)
	}

	queue.add(toSingleEvent(startTime, fmt.Sprintf("Attempting Cheat %s - currently unsupported cheat, nothing happens", cheat.Name), func() error {
		return nil
	}))
}

// scheduleNetworkRulesEvents schedules an event to apply network rules at a given time.
func scheduleNetworkRulesEvents(rule parser.NetworkRulesUpdate, queue *eventQueue, network driver.Network) {
	queue.add(toSingleEvent(Seconds(rule.Time), fmt.Sprintf("Applying network rules: %v", rule.Rules), func() error {
		return network.ApplyNetworkRules(driver.NetworkRules(rule.Rules))
	}))
}

// scheduleAdvanceEpochEvents schedules an event to advance epoch
func scheduleAdvanceEpochEvents(timing float32, epochIncrement int, queue *eventQueue, network driver.Network) {
	queue.add(toSingleEvent(Seconds(timing), fmt.Sprintf("Advancing Epoch by %d", epochIncrement), func() error {
		return network.AdvanceEpoch(epochIncrement)
	}))
}

// scheduleCheckEvents schedules an event to send perform corresponding check
func scheduleCheckEvents(timing float32, name string, check checking.Checker, queue *eventQueue, network driver.Network) {
	queue.add(toSingleEvent(Seconds(timing), fmt.Sprintf("Check [%s]", name), func() error {
		return check.Check()
	}))
}
