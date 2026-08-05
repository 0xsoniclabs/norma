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

package node

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/docker"
)

// newNodeInState builds a node with no container, usable for exercising the
// state guards of the action methods. Every action checks its precondition
// before touching the container, so the guards can be tested in isolation.
func newNodeInState(t *testing.T, state NodeState) *OperaNode {
	t.Helper()
	return &OperaNode{
		config: &OperaNodeConfig{Label: t.Name()},
		state:  state,
	}
}

// Each action must reject every state it does not accept as a starting
// point, so a scenario cannot, say, heal a running node or start a killed one.
func TestOperaNode_Actions_RejectUnexpectedStates(t *testing.T) {
	actions := map[string]struct {
		accepts []NodeState
		invoke  func(*OperaNode) error
	}{
		"Initialize": {
			accepts: []NodeState{NodeStateUninitialized},
			invoke:  func(n *OperaNode) error { return n.Initialize(t.Context()) },
		},
		"StartSonicd": {
			accepts: []NodeState{NodeStateReady},
			invoke:  func(n *OperaNode) error { return n.StartSonicd(t.Context()) },
		},
		"StartSonicdAsObserver": {
			accepts: []NodeState{NodeStateReady},
			invoke: func(n *OperaNode) error {
				return n.StartSonicdAsObserver(t.Context())
			},
		},
		"WaitForSync": {
			accepts: []NodeState{NodeStateSyncing},
			invoke:  func(n *OperaNode) error { return n.WaitForSync(t.Context()) },
		},
		"StopSonicd": {
			accepts: []NodeState{NodeStateRunning},
			invoke:  func(n *OperaNode) error { return n.StopSonicd(t.Context()) },
		},
		// Also accepted from Stopping, to escalate a graceful stop that
		// did not complete.
		"ForceStopSonicd": {
			accepts: []NodeState{NodeStateRunning, NodeStateStopping},
			invoke:  func(n *OperaNode) error { return n.ForceStopSonicd(t.Context()) },
		},
		"HealSonicd": {
			accepts: []NodeState{NodeStateKilled},
			invoke:  func(n *OperaNode) error { return n.HealSonicd(t.Context()) },
		},
	}

	for name, action := range actions {
		for _, state := range allNodeStates {
			if slices.Contains(action.accepts, state) {
				continue
			}
			t.Run(name+"/from-"+state.String(), func(t *testing.T) {
				node := newNodeInState(t, state)
				if err := action.invoke(node); err == nil {
					t.Fatalf("%s must fail in state %s", name, state)
				}
				if got := node.GetState(); got != state {
					t.Errorf("rejected %s must not change state, got %s from %s",
						name, got, state)
				}
			})
		}
	}
}

// A graceful stop that never completes must not be a dead end: escalating
// to a kill is the documented way out of Stopping.
func TestOperaNode_ForceStopSonicd_EscalatesFromStopping(t *testing.T) {
	node := newNodeInState(t, NodeStateStopping)

	// Fails for lack of a container, but only after the transition is
	// accepted, which is what this asserts.
	_ = node.ForceStopSonicd(t.Context())

	if got := node.GetState(); got != NodeStateKilled {
		t.Errorf("unexpected state, got %s, want %s", got, NodeStateKilled)
	}
}

func TestOperaNode_SignalSonicd_FailsWithoutContainer(t *testing.T) {
	node := newNodeInState(t, NodeStateRunning)

	_, err := node.signalSonicd(t.Context(), "INT")
	if err == nil {
		t.Fatalf("expected an error when no container exists")
	}
	if !strings.Contains(err.Error(), "container") {
		t.Errorf("unexpected error: %v", err)
	}
}

// StopSonicd must return the node to Running when it cannot signal the
// process, otherwise the node would be stranded in the transitional state.
func TestOperaNode_StopSonicd_RestoresRunningStateOnFailure(t *testing.T) {
	node := newNodeInState(t, NodeStateRunning)

	if err := node.StopSonicd(t.Context()); err == nil {
		t.Fatalf("expected StopSonicd to fail without a container")
	}
	if got := node.GetState(); got != NodeStateRunning {
		t.Errorf("unexpected state after failed stop, got %s, want %s",
			got, NodeStateRunning)
	}
}

// A kill attempt leaves the database dirty even when signalling fails, so
// the node must stay in Killed and require a heal.
func TestOperaNode_ForceStopSonicd_StaysKilledOnFailure(t *testing.T) {
	node := newNodeInState(t, NodeStateRunning)

	if err := node.ForceStopSonicd(t.Context()); err == nil {
		t.Fatalf("expected ForceStopSonicd to fail without a container")
	}
	if got := node.GetState(); got != NodeStateKilled {
		t.Errorf("unexpected state after failed kill, got %s, want %s",
			got, NodeStateKilled)
	}
}

// The point of the exit watcher: a client that dies on its own must move
// the node out of a state that claims it is up.
func TestOperaNode_RecordUnexpectedClientExit_MarksNodeKilled(t *testing.T) {
	for _, state := range []NodeState{NodeStateSyncing, NodeStateRunning} {
		t.Run("from-"+state.String(), func(t *testing.T) {
			node := newNodeInState(t, state)
			node.clientGen = 1

			if !node.recordUnexpectedClientExit(1) {
				t.Fatalf("exit from state %s must be reported as unexpected", state)
			}
			if got := node.GetState(); got != NodeStateKilled {
				t.Errorf("unexpected state, got %s, want %s", got, NodeStateKilled)
			}
			// The database is dirty either way, but a crash and a
			// deliberate kill must remain distinguishable.
			if !node.ClientExitWasUnexpected() {
				t.Errorf("crash must be recorded as an unexpected exit")
			}
		})
	}
}

// Exits from any other state were asked for, so they must not be reported
// as crashes or disturb a state machine that is mid-action.
func TestOperaNode_RecordUnexpectedClientExit_IgnoresRequestedExits(t *testing.T) {
	for _, state := range allNodeStates {
		if state == NodeStateSyncing || state == NodeStateRunning {
			continue
		}
		t.Run("from-"+state.String(), func(t *testing.T) {
			node := newNodeInState(t, state)
			node.clientGen = 1

			if node.recordUnexpectedClientExit(1) {
				t.Fatalf("exit from state %s must not be reported", state)
			}
			if got := node.GetState(); got != state {
				t.Errorf("state must not change, got %s, want %s", got, state)
			}
			if node.ClientExitWasUnexpected() {
				t.Errorf("must not be recorded as an unexpected exit")
			}
		})
	}
}

// The previous process's watcher fires after the node has been restarted;
// attributing that exit to the new client would kill a healthy node.
func TestOperaNode_RecordUnexpectedClientExit_IgnoresSupersededGeneration(t *testing.T) {
	node := newNodeInState(t, NodeStateRunning)
	node.clientGen = 2

	if node.recordUnexpectedClientExit(1) {
		t.Fatalf("exit of a superseded client must not be reported")
	}
	if got := node.GetState(); got != NodeStateRunning {
		t.Errorf("state must not change, got %s, want %s", got, NodeStateRunning)
	}
}

// beginClientStart has to retire the previous watcher, otherwise the old
// process's exit lands on the new generation.
func TestOperaNode_BeginClientStart_AdvancesGenerationAndClearsCrashFlag(t *testing.T) {
	node := newNodeInState(t, NodeStateReady)
	node.clientGen = 4
	node.clientExitWasUnexpected = true

	gen, err := node.beginClientStart()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gen != 5 {
		t.Errorf("unexpected generation, got %d, want 5", gen)
	}
	if got := node.GetState(); got != NodeStateSyncing {
		t.Errorf("unexpected state, got %s, want %s", got, NodeStateSyncing)
	}
	if node.ClientExitWasUnexpected() {
		t.Errorf("crash flag must be cleared for a new client")
	}
	if node.recordUnexpectedClientExit(4) {
		t.Errorf("the retired generation must no longer be reported")
	}
}

func TestOperaNode_AttachClient_IgnoresSupersededGeneration(t *testing.T) {
	node := newNodeInState(t, NodeStateSyncing)
	node.clientGen = 3
	handle := &docker.ExecHandle{ExecID: "stale"}

	node.attachClient(2, handle)

	if got := node.clientHandle(); got != nil {
		t.Errorf("superseded handle must not be attached, got %v", got)
	}
}

func TestOperaNode_WaitForSonicdExit(t *testing.T) {
	t.Run("returns once the process has exited", func(t *testing.T) {
		done := make(chan struct{})
		close(done)
		node := newNodeInState(t, NodeStateStopping)
		node.sonicd = &docker.ExecHandle{Done: done}

		if err := node.waitForSonicdExit(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	// Reporting success here would let the caller declare the node stopped
	// while the client still holds the data directory.
	t.Run("fails when the context ends first", func(t *testing.T) {
		node := newNodeInState(t, NodeStateStopping)
		node.sonicd = &docker.ExecHandle{Done: make(chan struct{})}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		if err := node.waitForSonicdExit(ctx); err == nil {
			t.Errorf("expected an error when the process has not exited")
		}
	})

	t.Run("succeeds when no client was started", func(t *testing.T) {
		node := newNodeInState(t, NodeStateReady)

		if err := node.waitForSonicdExit(t.Context()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

// End-to-end for the watcher goroutine: while the client is alive the node
// keeps its state, and the moment the process ends the node is marked as
// killed without anyone having to ask.
func TestOperaNode_WatchClientExit_MarksNodeKilledWhenClientDies(t *testing.T) {
	done := make(chan struct{})
	handle := &docker.ExecHandle{Done: done}

	node := newNodeInState(t, NodeStateRunning)
	node.clientGen = 1
	node.sonicd = handle

	watcherReturned := make(chan struct{})
	go func() {
		defer close(watcherReturned)
		node.watchClientExit(1, handle)
	}()

	// The watcher is blocked on the handle, so the node stays as it was.
	if got := node.GetState(); got != NodeStateRunning {
		t.Fatalf("state changed while the client was alive, got %s", got)
	}

	close(done) // the client process exits

	select {
	case <-watcherReturned:
	case <-time.After(10 * time.Second):
		t.Fatalf("watcher did not react to the client exiting")
	}

	if got := node.GetState(); got != NodeStateKilled {
		t.Errorf("unexpected state, got %s, want %s", got, NodeStateKilled)
	}
	if !node.ClientExitWasUnexpected() {
		t.Errorf("exit must be recorded as unexpected")
	}
}

func TestParseClientScanCount(t *testing.T) {
	tests := map[string]struct {
		output  string
		want    int
		wantErr bool
	}{
		"zero matches":         {output: "matched=0\n", want: 0},
		"several matches":      {output: "matched=3\n", want: 3},
		"ignores prior output": {output: "noise\nmore noise\nmatched=1\n", want: 1},
		"tolerates no newline": {output: "matched=2", want: 2},
		"missing result":       {output: "nothing here\n", wantErr: true},
		"empty output":         {output: "", wantErr: true},
		"malformed count":      {output: "matched=abc\n", wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := parseClientScanCount(test.output)
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q", test.output)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.want {
				t.Errorf("unexpected count, got %d, want %d", got, test.want)
			}
		})
	}
}

// The scan script must be a single valid shell command for both actions,
// and must always report a count so the caller can tell "none running"
// from "could not look".
func TestClientScanScript_IsWellFormedForBothActions(t *testing.T) {
	actions := map[string]string{
		"count": clientScanCountAction,
		"kill":  fmt.Sprintf(clientScanKillAction, "INT"),
	}

	for name, action := range actions {
		t.Run(name, func(t *testing.T) {
			script := fmt.Sprintf(clientScanScript, action)
			if strings.Contains(script, "%!") {
				t.Fatalf("script has unconsumed format directives: %s", script)
			}
			if !strings.Contains(script, sonicdBinaryPath) {
				t.Errorf("script does not match on the client binary: %s", script)
			}
			if !strings.Contains(script, `echo "`+clientScanMarker+`$n"`) {
				t.Errorf("script does not report a count: %s", script)
			}
		})
	}
}

func TestOperaNode_StreamExecLog_FailsWithoutClientProcess(t *testing.T) {
	node := newNodeInState(t, NodeStateReady)

	if _, err := node.StreamExecLog(); err == nil {
		t.Fatalf("expected an error when no client process exists")
	}
}

func TestOperaNode_ExecDone_IsClosedWithoutClientProcess(t *testing.T) {
	node := newNodeInState(t, NodeStateReady)

	select {
	case <-node.ExecDone():
	default:
		t.Fatalf("ExecDone must be closed when no client process exists")
	}
}

// Doublesign protection may only be dropped for a lone genesis validator
// bringing up a brand-new network. Dropping it when rejoining a running
// network lets the validator emit before it has replayed its own history,
// which forks it against itself.
func TestClientConfigToml_DropsDoublesignProtectionOnlyWhenBootstrappingAlone(t *testing.T) {
	validatorOne, validatorTwo := 1, 2
	tests := map[string]struct {
		numValidators int
		validatorId   *int
		bootstrap     bool
		wantDropped   bool
	}{
		"lone genesis validator bootstrapping": {
			numValidators: 1, validatorId: &validatorOne,
			bootstrap: true, wantDropped: true,
		},
		"lone genesis validator rejoining": {
			numValidators: 1, validatorId: &validatorOne,
			bootstrap: false, wantDropped: false,
		},
		"one of many validators bootstrapping": {
			numValidators: 4, validatorId: &validatorOne,
			bootstrap: true, wantDropped: false,
		},
		"other validator in a single-validator genesis": {
			numValidators: 1, validatorId: &validatorTwo,
			bootstrap: true, wantDropped: false,
		},
		"observer bootstrapping": {
			numValidators: 1, validatorId: nil,
			bootstrap: true, wantDropped: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config := &OperaNodeConfig{
				ValidatorId: test.validatorId,
				NetworkConfig: &driver.NetworkConfig{
					Validators: driver.NewDefaultValidators(test.numValidators),
				},
			}

			got := clientConfigToml(config, test.bootstrap)

			want := fmt.Sprintf("DoublesignProtection = %d\n",
				doublesignProtection.Nanoseconds())
			if test.wantDropped {
				want = "DoublesignProtection = 0\n"
			}
			if !strings.HasSuffix(got, want) {
				t.Errorf("unexpected config %q, want it to end in %q", got, want)
			}
			if !strings.HasPrefix(got, "[Emitter.EmitIntervals]\n") {
				t.Errorf("config %q does not declare the emitter section", got)
			}
		})
	}
}

// The bootstrap exception applies to the first client of the bootstrapping
// node only: once it has run, any later start rejoins a running network.
func TestOperaNode_BootstrapsNetwork_OnlyUntilTheFirstClientRan(t *testing.T) {
	node := newNodeInState(t, NodeStateReady)
	node.config.NetworkBootstrap = true

	if !node.bootstrapsNetwork() {
		t.Errorf("the first client of a bootstrapping node starts the network")
	}

	node.attachClient(node.clientGen, &docker.ExecHandle{})

	if node.bootstrapsNetwork() {
		t.Errorf("a restart must not be mistaken for bootstrapping the network")
	}
}

func TestOperaNode_BootstrapsNetwork_FalseForNodesJoiningARunningNetwork(t *testing.T) {
	node := newNodeInState(t, NodeStateReady)

	if node.bootstrapsNetwork() {
		t.Errorf("a node created after bootstrap must not claim to bootstrap")
	}
}

func TestBuildSonicdCmd_UsesExecFormWithoutShell(t *testing.T) {
	cmd := buildSonicdCmd(nil, "", "1.2.3.4", "")

	if len(cmd) == 0 {
		t.Fatalf("command must not be empty")
	}
	if cmd[0] != sonicdBinaryPath {
		t.Errorf("command must invoke %s directly, got %q",
			sonicdBinaryPath, cmd[0])
	}
	// A shell wrapper would reintroduce word-splitting of the arguments and
	// hide the client's exit status behind the shell's.
	for _, arg := range cmd {
		if arg == "sh" || arg == "-c" || strings.Contains(arg, "|") {
			t.Errorf("command must not be shell-wrapped, found %q in %v", arg, cmd)
		}
	}
}

func TestBuildSonicdCmd_UsesAbsolutePathsForGeneratedFiles(t *testing.T) {
	validatorId := 3
	cmd := buildSonicdCmd(&validatorId, "pubkey", "1.2.3.4", "")

	for _, want := range []string{configFilePath, passwordFilePath} {
		if !slices.Contains(cmd, want) {
			t.Errorf("command does not reference %q: %v", want, cmd)
		}
		if !filepath.IsAbs(want) {
			t.Errorf("%q must be absolute so it does not depend on the working directory", want)
		}
	}
}

func TestBuildSonicdCmd_AddsValidatorFlagsOnlyForValidators(t *testing.T) {
	validatorId := 7
	tests := map[string]struct {
		id            *int
		wantValidator bool
	}{
		"observer":         {id: nil, wantValidator: false},
		"validator":        {id: &validatorId, wantValidator: true},
		"zero id observer": {id: new(int), wantValidator: false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cmd := buildSonicdCmd(test.id, "pubkey", "1.2.3.4", "")
			got := slices.Contains(cmd, "--validator.id")
			if got != test.wantValidator {
				t.Errorf("--validator.id present = %t, want %t in %v",
					got, test.wantValidator, cmd)
			}
		})
	}
}

func TestBuildSonicdCmd_AppendsExtraArgumentsAsSeparateEntries(t *testing.T) {
	cmd := buildSonicdCmd(nil, "", "1.2.3.4", "--statedb.checkpointinterval 1")

	i := slices.Index(cmd, "--statedb.checkpointinterval")
	if i < 0 {
		t.Fatalf("extra argument missing from %v", cmd)
	}
	if i+1 >= len(cmd) || cmd[i+1] != "1" {
		t.Errorf("extra argument value must be a separate entry, got %v", cmd)
	}
}

func TestIsDirEmpty(t *testing.T) {
	t.Run("true for empty directory", func(t *testing.T) {
		if !isDirEmpty(t.TempDir()) {
			t.Errorf("empty directory reported as non-empty")
		}
	})

	// The keystore is mounted before initialization, so its presence alone
	// must not be mistaken for an already initialized data directory.
	t.Run("true when only the keystore is present", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "keystore"), 0755); err != nil {
			t.Fatalf("failed to create keystore dir: %v", err)
		}
		if !isDirEmpty(dir) {
			t.Errorf("directory holding only a keystore reported as non-empty")
		}
	})

	t.Run("false when other entries are present", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "chaindata"),
			[]byte("x"), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
		if isDirEmpty(dir) {
			t.Errorf("non-empty directory reported as empty")
		}
	})

	t.Run("true for a missing directory", func(t *testing.T) {
		if !isDirEmpty(filepath.Join(t.TempDir(), "absent")) {
			t.Errorf("missing directory reported as non-empty")
		}
	})
}

// Logs placed in the configured output directory must outlive the run, so
// they must not be reported as temporary (which would have them deleted).
func TestResolveLogsDir_UsesConfiguredDirectoryAndKeepsIt(t *testing.T) {
	outputDir := t.TempDir()

	dir, temporary, err := resolveLogsDir(&OperaNodeConfig{LogsDir: outputDir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if temporary {
		t.Errorf("configured logs dir must not be treated as temporary")
	}
	if !strings.HasPrefix(dir, outputDir) {
		t.Errorf("logs dir %q is not inside the configured directory %q",
			dir, outputDir)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("logs dir was not created: %v", err)
	}
}

func TestResolveLogsDir_FallsBackToTemporaryDirectory(t *testing.T) {
	dir, temporary, err := resolveLogsDir(&OperaNodeConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	if !temporary {
		t.Errorf("fallback logs dir must be reported as temporary so it is cleaned up")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("fallback logs dir was not created: %v", err)
	}
}
