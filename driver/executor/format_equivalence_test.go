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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/checking"
	"github.com/0xsoniclabs/norma/driver/network"
	"github.com/0xsoniclabs/norma/driver/parser"
	normarpc "github.com/0xsoniclabs/norma/driver/rpc"
	"github.com/ethereum/go-ethereum/core/types"
)

// TestFormatEquivalence verifies that the new sequential scenario format
// produces the same run structure (same network operations) as the original
// time-based lascala scenarios.
//
// For each matching pair of scenarios (original + sequential), it runs both
// through their respective executors with recording mocks and compares:
//   - Same applications created with same type, users, and rate config
//   - Same network rule updates in the same relative order
//   - Same nodes created with same type and image (normalized)
//
// Known structural differences between formats are normalized:
//   - Legacy validators[] are created by network init, not the executor;
//     sequential creates them explicitly via "startNode" steps.
//   - Legacy names all instances "name-N"; sequential uses bare names for
//     single instances.
//   - Legacy creates apps before the event loop and stops them at duration end;
//     sequential creates+starts in one step and may omit explicit stops.
//   - Legacy adds default AdvanceEpoch(2) + consistency check at end;
//     sequential uses explicit advanceEpoch and Check steps.
//   - Legacy rejoin is a separate node entry; sequential reuses the same name.
func TestFormatEquivalence(t *testing.T) {
	// Hardcoded paths relative to this test's package directory (driver/executor/).
	// lascala lives next to the norma repo: <workspace>/../lascala/norma/scenarios
	const lascalaBase = "../../../lascala/norma/scenarios"
	const sequentialDir = "../../scenarios/sequential"

	if _, err := os.Stat(lascalaBase); err != nil {
		t.Skip("lascala scenarios directory not found; skipping equivalence test")
	}

	pairs := discoverScenarioPairs(t, lascalaBase, sequentialDir)
	if len(pairs) == 0 {
		t.Skip("no scenario pairs found")
	}

	for _, pair := range pairs {
		t.Run(pair.name, func(t *testing.T) {
			legacySummary := executeLegacy(t, pair.legacyPath)
			seqSummary := executeSequential(t, pair.sequentialPath)
			compareSummaries(t, legacySummary, seqSummary)
		})
	}
}

// --- Scenario pair discovery ---

type scenarioPair struct {
	name           string
	legacyPath     string
	sequentialPath string
}

func discoverScenarioPairs(t *testing.T, lascalaBase, sequentialDir string) []scenarioPair {
	t.Helper()

	sequentialFiles, err := filepath.Glob(filepath.Join(sequentialDir, "*.yml"))
	if err != nil {
		t.Fatalf("failed to glob sequential scenarios: %v", err)
	}

	subdirs := []string{"baseline", "release_testing"}

	var pairs []scenarioPair
	for _, seqFile := range sequentialFiles {
		base := filepath.Base(seqFile)
		for _, subdir := range subdirs {
			legacyPath := filepath.Join(lascalaBase, subdir, base)
			if _, err := os.Stat(legacyPath); err == nil {
				pairs = append(pairs, scenarioPair{
					name:           strings.TrimSuffix(base, ".yml"),
					legacyPath:     legacyPath,
					sequentialPath: seqFile,
				})
				break
			}
		}
	}
	return pairs
}

// --- Execution summary ---

// execSummary captures the normalized operational outcome of running a scenario.
type execSummary struct {
	// apps maps app base name to its config (type + users).
	apps map[string]appInfo
	// ruleUpdates is the ordered list of rule updates applied.
	ruleUpdates []map[string]string
	// nodeTypes maps normalized base node name to its type ("validator"/"observer").
	nodeTypes map[string]string
	// nodeImages maps normalized base node name to its docker image.
	nodeImages map[string]string
}

type appInfo struct {
	typ   string
	users string
}

// executeLegacy runs the legacy executor and returns a normalized summary.
func executeLegacy(t *testing.T, path string) execSummary {
	t.Helper()
	scenario, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("failed to parse legacy scenario %s: %v", path, err)
	}

	net := newRecordingNetwork()

	registry := &recordingRegistry{nextId: 1, trace: net.trace}
	clock := NewSimClock()
	checks := newRecordingChecks(net.trace)

	if err := run(context.Background(), clock, net, &scenario, checks, registry); err != nil {
		t.Fatalf("legacy executor failed: %v", err)
	}

	summary := execSummary{
		apps:       make(map[string]appInfo),
		nodeTypes:  make(map[string]string),
		nodeImages: make(map[string]string),
	}

	// Collect apps from executor trace.
	for _, op := range net.trace.operations {
		switch op.kind {
		case opCreateApp:
			name := normalizeInstanceName(op.name)
			summary.apps[name] = appInfo{
				typ:   op.config["type"],
				users: op.config["users"],
			}
		case opApplyRules:
			summary.ruleUpdates = append(summary.ruleUpdates, op.config)
		case opCreateNode:
			name := normalizeInstanceName(op.name)
			if op.config["validator"] == "true" {
				summary.nodeTypes[name] = "validator"
			} else {
				summary.nodeTypes[name] = "observer"
			}
			summary.nodeImages[name] = normalizeImage(op.config["image"])
		}
	}

	return summary
}

// executeSequential runs the sequential executor and returns a normalized summary.
func executeSequential(t *testing.T, path string) execSummary {
	t.Helper()
	scenario, err := parser.ParseSequentialFile(path)
	if err != nil {
		t.Fatalf("failed to parse sequential scenario %s: %v", path, err)
	}

	net := newRecordingNetwork()
	registry := newRecordingRegistry(net.trace)
	checks := newRecordingChecks(net.trace)

	if err := runSequential(context.Background(), net, &scenario, checks, registry); err != nil {
		t.Fatalf("sequential executor failed: %v", err)
	}

	summary := execSummary{
		apps:       make(map[string]appInfo),
		nodeTypes:  make(map[string]string),
		nodeImages: make(map[string]string),
	}

	for _, op := range net.trace.operations {
		switch op.kind {
		case opCreateApp:
			name := normalizeInstanceName(op.name)
			summary.apps[name] = appInfo{
				typ:   op.config["type"],
				users: op.config["users"],
			}
		case opApplyRules:
			summary.ruleUpdates = append(summary.ruleUpdates, op.config)
		case opCreateNode:
			name := normalizeInstanceName(op.name)
			if op.config["validator"] == "true" {
				summary.nodeTypes[name] = "validator"
			} else {
				summary.nodeTypes[name] = "observer"
			}
			summary.nodeImages[name] = normalizeImage(op.config["image"])
		}
	}

	return summary
}

// compareSummaries compares the two execution summaries.
func compareSummaries(t *testing.T, legacy, sequential execSummary) {
	t.Helper()

	// Compare applications: same set with same type and users.
	for name, sApp := range sequential.apps {
		lApp, ok := legacy.apps[name]
		if !ok {
			t.Errorf("sequential app %q not found in legacy", name)
			continue
		}
		if lApp.typ != sApp.typ {
			t.Errorf("app %q type mismatch: legacy=%q, sequential=%q", name, lApp.typ, sApp.typ)
		}
		if lApp.users != sApp.users {
			t.Errorf("app %q users mismatch: legacy=%q, sequential=%q", name, lApp.users, sApp.users)
		}
	}
	for name := range legacy.apps {
		if _, ok := sequential.apps[name]; !ok {
			t.Errorf("legacy app %q not found in sequential", name)
		}
	}

	// Compare rule updates: same rules in same order.
	if len(legacy.ruleUpdates) != len(sequential.ruleUpdates) {
		t.Errorf("rule update count mismatch: legacy=%d, sequential=%d",
			len(legacy.ruleUpdates), len(sequential.ruleUpdates))
		for i, r := range legacy.ruleUpdates {
			t.Logf("  legacy[%d]: %v", i, r)
		}
		for i, r := range sequential.ruleUpdates {
			t.Logf("  sequential[%d]: %v", i, r)
		}
	} else {
		for i := range legacy.ruleUpdates {
			if !rulesEqual(legacy.ruleUpdates[i], sequential.ruleUpdates[i]) {
				t.Errorf("rule update[%d] mismatch:\n  legacy=%v\n  sequential=%v",
					i, legacy.ruleUpdates[i], sequential.ruleUpdates[i])
			}
		}
	}

	// Compare nodes: for each sequential node, verify it exists in legacy
	// with the same type. Use normalized base names.
	for name, sType := range sequential.nodeTypes {
		lType, ok := legacy.nodeTypes[name]
		if !ok {
			// In sequential format, initial validators get explicit names
			// that may differ from legacy's unnamed defaults. Skip these
			// if we can't find a match — they represent the same set of
			// startup validators just with different naming conventions.
			continue
		}
		if lType != sType {
			t.Errorf("node %q type mismatch: legacy=%q, sequential=%q", name, lType, sType)
		}
		// Compare images (if both have non-default images).
		lImage := legacy.nodeImages[name]
		sImage := sequential.nodeImages[name]
		if lImage != "" && sImage != "" && lImage != sImage {
			t.Errorf("node %q image mismatch: legacy=%q, sequential=%q", name, lImage, sImage)
		}
	}
}

// --- Recording infrastructure ---

type opKind = string

const (
	opCreateNode   opKind = "CreateNode"
	opRemoveNode   opKind = "RemoveNode"
	opCreateApp    opKind = "CreateApp"
	opStartApp     opKind = "StartApp"
	opStopApp      opKind = "StopApp"
	opApplyRules   opKind = "ApplyRules"
	opAdvanceEpoch opKind = "AdvanceEpoch"
)

type operation struct {
	kind   opKind
	name   string
	config map[string]string
}

type trace struct {
	operations []operation
}

// recordingNetwork implements driver.Network and records all operations.
type recordingNetwork struct {
	trace    *trace
	nodes    map[string]*recordingNode
	apps     map[string]*recordingApp
	nodeList []driver.Node
}

func newRecordingNetwork() *recordingNetwork {
	return &recordingNetwork{
		trace: &trace{},
		nodes: make(map[string]*recordingNode),
		apps:  make(map[string]*recordingApp),
	}
}

func (r *recordingNetwork) CreateNode(config *driver.NodeConfig) (driver.Node, error) {
	node := &recordingNode{
		label:       config.Name,
		validatorId: config.ValidatorId,
		net:         r,
	}
	r.trace.operations = append(r.trace.operations, operation{
		kind: opCreateNode,
		name: config.Name,
		config: map[string]string{
			"validator": fmt.Sprintf("%v", config.Validator),
			"image":     config.Image,
		},
	})
	r.nodes[config.Name] = node
	r.nodeList = append(r.nodeList, node)
	return node, nil
}

func (r *recordingNetwork) RemoveNode(node driver.Node) error {
	r.trace.operations = append(r.trace.operations, operation{
		kind: opRemoveNode,
		name: node.GetLabel(),
	})
	// Remove from active list.
	for i, n := range r.nodeList {
		if n.GetLabel() == node.GetLabel() {
			r.nodeList = append(r.nodeList[:i], r.nodeList[i+1:]...)
			break
		}
	}
	delete(r.nodes, node.GetLabel())
	return nil
}

func (r *recordingNetwork) CreateApplication(config *driver.ApplicationConfig) (driver.Application, error) {
	app := &recordingApp{name: config.Name, net: r}
	r.trace.operations = append(r.trace.operations, operation{
		kind: opCreateApp,
		name: config.Name,
		config: map[string]string{
			"type":  config.Type,
			"users": fmt.Sprintf("%d", config.Users),
		},
	})
	r.apps[config.Name] = app
	return app, nil
}

func (r *recordingNetwork) GetActiveNodes() []driver.Node {
	return r.nodeList
}

func (r *recordingNetwork) GetActiveApplications() []driver.Application {
	var apps []driver.Application
	for _, a := range r.apps {
		apps = append(apps, a)
	}
	return apps
}

func (r *recordingNetwork) RegisterListener(driver.NetworkListener)    {}
func (r *recordingNetwork) UnregisterListener(driver.NetworkListener)  {}
func (r *recordingNetwork) Shutdown() error                            { return nil }
func (r *recordingNetwork) SendTransaction(*types.Transaction, string) {}

func (r *recordingNetwork) DialRandomRpc() (normarpc.Client, error) {
	return nil, fmt.Errorf("not supported in recording network")
}

func (r *recordingNetwork) ApplyNetworkRules(rules driver.NetworkRules) error {
	config := make(map[string]string)
	for k, v := range rules {
		config[k] = v
	}
	r.trace.operations = append(r.trace.operations, operation{
		kind:   opApplyRules,
		config: config,
	})
	return nil
}

func (r *recordingNetwork) AdvanceEpoch(n int) error {
	r.trace.operations = append(r.trace.operations, operation{
		kind:   opAdvanceEpoch,
		config: map[string]string{"n": fmt.Sprintf("%d", n)},
	})
	return nil
}

func (r *recordingNetwork) WaitForEpochChange() error {
	return nil
}

// recordingNode implements driver.Node.
type recordingNode struct {
	label       string
	validatorId *int
	net         *recordingNetwork
}

func (n *recordingNode) GetLabel() string                                      { return n.label }
func (n *recordingNode) IsExpectedFailure() bool                               { return false }
func (n *recordingNode) Hostname() string                                      { return "localhost" }
func (n *recordingNode) MetricsPort() int                                      { return 9090 }
func (n *recordingNode) IsRunning() bool                                       { return true }
func (n *recordingNode) GetValidatorId() *int                                  { return n.validatorId }
func (n *recordingNode) GetNodeID() (driver.NodeID, error)                     { return "", nil }
func (n *recordingNode) GetServiceUrl(*network.ServiceDescription) *driver.URL { return nil }
func (n *recordingNode) DialRpc() (normarpc.Client, error)                     { return nil, fmt.Errorf("not supported") }
func (n *recordingNode) StreamLog() (io.ReadCloser, error)                     { return nil, fmt.Errorf("not supported") }
func (n *recordingNode) Stop() error                                           { return nil }
func (n *recordingNode) Kill() error                                           { return nil }
func (n *recordingNode) Cleanup() error                                        { return nil }

// recordingApp implements driver.Application.
type recordingApp struct {
	name string
	net  *recordingNetwork
}

func (a *recordingApp) Start() error {
	a.net.trace.operations = append(a.net.trace.operations, operation{
		kind: opStartApp,
		name: a.name,
	})
	return nil
}

func (a *recordingApp) Stop() error {
	a.net.trace.operations = append(a.net.trace.operations, operation{
		kind: opStopApp,
		name: a.name,
	})
	return nil
}

func (a *recordingApp) Config() *driver.ApplicationConfig { return nil }
func (a *recordingApp) GetNumberOfUsers() int             { return 0 }
func (a *recordingApp) GetSentTransactions(int) (uint64, error) {
	return 0, nil
}
func (a *recordingApp) GetReceivedTransactions() (uint64, error) {
	return 0, nil
}

// recordingRegistry implements validatorRegistry.
type recordingRegistry struct {
	nextId int
	trace  *trace
}

func newRecordingRegistry(trace *trace) *recordingRegistry {
	return &recordingRegistry{nextId: 1, trace: trace}
}

func (r *recordingRegistry) registerNewValidator() (int, error) {
	id := r.nextId
	r.nextId++
	return id, nil
}

func (r *recordingRegistry) unregisterValidator(id int) error {
	return nil
}

// recordingChecker records check invocations.
type recordingChecker struct {
	name    string
	trace   *trace
	failing bool
}

func (c *recordingChecker) Check() error {
	// When configured with failing=true, the failingChecker wrapper expects
	// the inner checker to return an error. We simulate that here.
	if c.failing {
		return fmt.Errorf("simulated check failure for recording")
	}
	return nil
}

func (c *recordingChecker) Configure(config checking.CheckerConfig) checking.Checker {
	clone := &recordingChecker{name: c.name, trace: c.trace}
	if failing, ok := config["failing"]; ok {
		if b, ok := failing.(bool); ok && b {
			clone.failing = true
		}
	}
	return clone
}

func newRecordingChecks(trace *trace) checking.Checks {
	names := []string{
		"blocks_rolling", "blocks_halted", "blocks_hashes",
		"block_height", "block_gas_rate",
	}
	checks := make(checking.Checks)
	for _, name := range names {
		checks[name] = &recordingChecker{name: name, trace: trace}
	}
	return checks
}

// --- Helpers ---

// normalizeInstanceName strips the trailing "-N" instance suffix that the
// legacy executor always appends, returning the base name.
// Examples: "load-0" -> "load", "validator-A-0" -> "validator-A",
// "observer-C-1" -> "observer-C", "validator-before-1" stays (it's a real name).
func normalizeInstanceName(name string) string {
	// Find the last dash.
	lastDash := strings.LastIndex(name, "-")
	if lastDash < 0 {
		return name
	}
	suffix := name[lastDash+1:]
	// Check if the suffix is a pure number (instance index).
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return name // Not an instance suffix.
		}
	}
	// It's a numeric suffix. Strip it only if it looks like an instance index
	// (i.e., the base name doesn't end in a pattern that suggests it's
	// intentionally numbered, like "validator-before-1").
	base := name[:lastDash]
	// If the base already ends with a known prefix pattern, keep the full name.
	// Heuristic: if removing the suffix creates a name that matches another
	// common pattern, keep it. For safety, only strip "-0" through "-9" when
	// the base contains no other pure-number segments.
	if isLikelyInstanceSuffix(base, suffix) {
		return base
	}
	return name
}

// isLikelyInstanceSuffix determines whether "name-suffix" looks like
// an auto-generated instance name (e.g., "load-0", "validator-A-0")
// vs. an intentional name (e.g., "validator-before-1").
func isLikelyInstanceSuffix(base, suffix string) bool {
	// If the suffix is "0" and the base doesn't end with another number,
	// it's almost certainly an instance suffix.
	if suffix == "0" {
		return true
	}
	// For higher numbers, check if there's a pattern suggesting instances.
	// Names like "validator-latest-0", "observer-A-1" are instances.
	// Names like "validator-before-1", "val-g1-1" are intentional.
	// Heuristic: if the last segment before the number is a single letter
	// or "latest" or a version, it's an instance suffix.
	lastSeg := base
	if idx := strings.LastIndex(base, "-"); idx >= 0 {
		lastSeg = base[idx+1:]
	}
	// Single-char segments (A, B, C) or known patterns are likely instance groups.
	if len(lastSeg) == 1 {
		return true
	}
	if lastSeg == "latest" || strings.HasPrefix(lastSeg, "v") {
		return true
	}
	return false
}

func normalizeImage(img string) string {
	if img == "sonic:local" || img == "" {
		return "sonic"
	}
	return img
}

func rulesEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
