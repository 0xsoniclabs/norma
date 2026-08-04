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
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
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

// Each action must reject every state other than the one it starts from,
// so that a scenario cannot, say, heal a running node or start a killed one.
func TestOperaNode_Actions_RejectUnexpectedStates(t *testing.T) {
	actions := map[string]struct {
		requires NodeState
		invoke   func(*OperaNode) error
	}{
		"Initialize": {
			requires: NodeStateUninitialized,
			invoke:   func(n *OperaNode) error { return n.Initialize(t.Context()) },
		},
		"StartSonicd": {
			requires: NodeStateReady,
			invoke:   func(n *OperaNode) error { return n.StartSonicd(t.Context()) },
		},
		"StartSonicdAsObserver": {
			requires: NodeStateReady,
			invoke: func(n *OperaNode) error {
				return n.StartSonicdAsObserver(t.Context())
			},
		},
		"WaitForSync": {
			requires: NodeStateSyncing,
			invoke:   func(n *OperaNode) error { return n.WaitForSync(t.Context()) },
		},
		"StopSonicd": {
			requires: NodeStateRunning,
			invoke:   func(n *OperaNode) error { return n.StopSonicd(t.Context()) },
		},
		"ForceStopSonicd": {
			requires: NodeStateRunning,
			invoke:   func(n *OperaNode) error { return n.ForceStopSonicd(t.Context()) },
		},
		"HealSonicd": {
			requires: NodeStateKilled,
			invoke:   func(n *OperaNode) error { return n.HealSonicd(t.Context()) },
		},
	}

	for name, action := range actions {
		for _, state := range allNodeStates {
			if state == action.requires {
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

// A node whose client process is gone must not be reported as startable:
// signalSonicd is the step that would otherwise silently do nothing.
func TestOperaNode_SignalSonicd_FailsWithoutClientProcess(t *testing.T) {
	node := newNodeInState(t, NodeStateRunning)

	err := node.signalSonicd(t.Context(), "INT")
	if err == nil {
		t.Fatalf("expected an error when no client process exists")
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
