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
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/0xsoniclabs/norma/driver/docker"
	"github.com/0xsoniclabs/norma/driver/network"
)

// Paths inside the container. Stage 2 of the Dockerfile copies the
// binaries into the root directory, and no WORKDIR is set. All paths are
// absolute so that nothing depends on the working directory an exec
// happens to inherit.
const (
	// sonicdBinaryPath identifies the client binary. It doubles as the
	// identity used to find the running process via /proc/<pid>/exe, which
	// avoids substring collisions with siblings such as sonictool.
	sonicdBinaryPath = "/sonicd"
	// sonicToolBinaryPath is the maintenance tool used for genesis import
	// and database healing.
	sonicToolBinaryPath = "/sonictool"
	// configFilePath holds the generated client configuration.
	configFilePath = dataDir + "/config.toml"
	// passwordFilePath holds the validator keystore password.
	passwordFilePath = dataDir + "/password.txt"
)

// clientScanScript is a POSIX shell script template that walks /proc and
// runs the snippet substituted for %s against every process whose
// executable is the client binary. Matching on the exe symlink (rather
// than a substring of cmdline) prevents accidental hits on sonictool or
// any other sibling process.
//
// The snippet increments n for each process it accounts for, and the count
// is reported on the last line as "matched=<n>". The script always exits 0,
// because finding no process is a legitimate outcome that the caller must
// be able to distinguish from a failure to look.
const clientScanScript = `n=0;` +
	` for d in /proc/[0-9]*; do` +
	` exe=$(readlink "$d/exe" 2>/dev/null) || continue;` +
	` [ "$exe" = "` + sonicdBinaryPath + `" ] || continue;` +
	` %s;` +
	` done;` +
	` echo "matched=$n"`

// clientScanCountAction only counts matching processes.
const clientScanCountAction = `n=$((n+1))`

// clientScanKillAction signals each matching process and counts the ones
// that were actually signalled, so the caller learns whether a client was
// there to receive it.
const clientScanKillAction = `kill -%s "${d##*/}" 2>/dev/null && n=$((n+1))`

// clientScanMarker prefixes the line reporting how many processes matched.
const clientScanMarker = "matched="

// Initialize prepares the OperaNode for operation performing:
//   - Create the data directory and initialize the genesis state.
//   - Write the password file for validator keystore decryption.
//   - Configure network latency simulation via tc netem.
//
// The client configuration is not written here but before each client
// start, because its content depends on the state of the network at that
// moment.
//
// Requires the node to be in NodeStateUninitialized and transitions
// it to NodeStateReady. On failure the node is returned to
// NodeStateUninitialized so the caller may retry.
func (n *OperaNode) Initialize(ctx context.Context) (err error) {
	// Claiming the transitional state under the state lock is what makes
	// this exclusive: a second concurrent caller fails the transition
	// rather than racing through the steps below.
	if err := n.transition(NodeStateUninitialized, NodeStateInitializing); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			n.forceSetState(NodeStateUninitialized)
		}
	}()

	// Skip initialization only when re-using a populated mount directory.
	needsInit := n.config.MountDataDir == nil || isDirEmpty(*n.config.MountDataDir)
	if needsInit {
		mkdirCmd := []string{"mkdir", "-m", "755", "-p", dataDir}
		if output, err := n.container.Exec(ctx, mkdirCmd); err != nil {
			return fmt.Errorf("failed to create datadir: %w - output: %s", err, output)
		}

		sonicToolCmd := []string{
			sonicToolBinaryPath,
			"--datadir", dataDir,
			"--statedb.livecache", "1",
			"genesis", "json", "--experimental", "/genesis.json",
		}
		output, err := n.container.ExecWithOptions(ctx, sonicToolCmd,
			docker.ExecOptions{LogName: "sonictool-genesis"})
		if err != nil {
			return fmt.Errorf("sonictool genesis init failed: %w - output: %s", err, output)
		}
	}

	// Write password file for validator keystore decryption. The
	// password is intentionally the fixed string "password" because
	// norma only spins up fake, throwaway validators for testing; it
	// is never used to protect real keys.
	if err := n.writeContainerFile(ctx, passwordFilePath, "password\n"); err != nil {
		return fmt.Errorf("failed to write password file: %w", err)
	}

	// Network latency simulation via tc netem.
	latency := n.config.NetworkConfig.RoundTripTime / 2
	if latency > 0 {
		tcCmd := fmt.Sprintf(
			"tc qdisc replace dev eth0 root netem delay %v"+
				" && (ip link show eth1 2>/dev/null"+
				" && tc qdisc replace dev eth1 root netem delay %v || true)",
			latency, latency)
		cmd := []string{"sh", "-c", tcCmd}
		if _, err := n.container.ExecWithOptions(ctx, cmd,
			docker.ExecOptions{LogName: "tc-setup"}); err != nil {
			return fmt.Errorf("failed to configure network latency: %w", err)
		}
	}

	return n.transition(NodeStateInitializing, NodeStateReady)
}

// writeContainerFile writes content to the given absolute path inside the
// container. The content is passed via an environment variable rather than
// interpolated into the shell command, so quotes and newlines in it cannot
// alter the command.
func (n *OperaNode) writeContainerFile(
	ctx context.Context,
	path string,
	content string,
) error {
	cmd := []string{"sh", "-c", `printf '%s' "$NORMA_FILE_CONTENT" > "$NORMA_FILE_PATH"`}
	output, err := n.container.ExecWithOptions(ctx, cmd, docker.ExecOptions{
		Env: []string{
			"NORMA_FILE_CONTENT=" + content,
			"NORMA_FILE_PATH=" + path,
		},
	})
	if err != nil {
		return fmt.Errorf("%w - output: %s", err, output)
	}
	return nil
}

// doublesignProtection is the emitter's doublesign protection window used
// for every client start that is not bootstrapping a network on its own.
const doublesignProtection = 5 * time.Second

// clientConfigToml renders the client configuration for a client start, where
// bootstrap reports whether that start brings up a brand-new network.
//
// A lone genesis validator bootstrapping a network must emit without
// protection: it has no peer to sync with, and the protection heuristic
// refuses to emit while there are no connections, so no block would ever be
// produced. Every other start keeps the protection, including that same
// validator rejoining a network that is already running, where emitting
// before its own earlier events have been replayed forks the validator
// against itself and stops it for good.
func clientConfigToml(config *OperaNodeConfig, bootstrap bool) string {
	protection := doublesignProtection
	if bootstrap && isLoneGenesisValidator(config) {
		protection = 0
	}
	return fmt.Sprintf("[Emitter.EmitIntervals]\nDoublesignProtection = %d\n",
		protection.Nanoseconds())
}

// isLoneGenesisValidator reports whether the configured node is the only
// validator its network was created with.
func isLoneGenesisValidator(config *OperaNodeConfig) bool {
	return config.NetworkConfig.Validators.GetNumValidators() == 1 &&
		config.ValidatorId != nil && *config.ValidatorId == 1
}

// bootstrapsNetwork reports whether the client start being prepared brings up
// a brand-new network. Only the first client of the bootstrapping node does;
// every later start of it rejoins a network that has been running since.
func (n *OperaNode) bootstrapsNetwork() bool {
	return n.config.NetworkBootstrap && n.clientHandle() == nil
}

// StartSonicd starts the OperaNode's sonicd process in the background.
// It requires the node to be in NodeStateReady and transitions it to
// NodeStateSyncing. Returns an error if sonicd fails to start.
func (n *OperaNode) StartSonicd(ctx context.Context) error {
	return n.startSonicd(ctx, false)
}

// StartSonicdAsObserver starts sonicd without validator flags, so the node
// follows the chain without emitting events. Note that a validator started
// this way stays a non-validating observer until it is restarted with
// StartSonicd.
func (n *OperaNode) StartSonicdAsObserver(ctx context.Context) error {
	return n.startSonicd(ctx, true)
}

func (n *OperaNode) startSonicd(ctx context.Context, observer bool) error {
	gen, err := n.beginClientStart()
	if err != nil {
		return err
	}

	// Two clients on one data directory corrupt it, so confirm none is
	// running rather than inferring it from the state we just left.
	if err := n.requireNoClientRunning(ctx, "start the client"); err != nil {
		n.forceSetState(NodeStateReady)
		return err
	}

	configToml := clientConfigToml(n.config, n.bootstrapsNetwork())
	if err := n.writeContainerFile(ctx, configFilePath, configToml); err != nil {
		n.forceSetState(NodeStateReady)
		return fmt.Errorf("failed to write %s: %w", configFilePath, err)
	}

	validatorId := n.config.ValidatorId
	if observer {
		validatorId = nil
	}
	sonicdCmd := buildSonicdCmd(
		validatorId,
		n.config.PubKey,
		n.container.IP(),
		n.config.ExtraArguments)

	slog.Info("Starting sonicd", "node", n.config.Label, "observer", observer)
	handle, err := n.container.ExecBackground(ctx, sonicdCmd,
		docker.ExecOptions{LogName: "sonicd"})
	if err != nil {
		// Roll back so the caller may retry from Ready.
		n.forceSetState(NodeStateReady)
		return fmt.Errorf("failed to start sonicd: %w", err)
	}
	n.attachClient(gen, handle)

	// Watch for the process dying on its own, so the recorded state stops
	// claiming the node is up the moment it is not.
	go n.watchClientExit(gen, handle)

	slog.Info("Sonicd started", "node", n.config.Label, "log", handle.LogPath)
	return nil
}

// WaitForSync waits for the OperaNode to finish syncing and become
// ready. Requires NodeStateSyncing and transitions to NodeStateRunning
// on success. The node is left in NodeStateSyncing on failure, since the
// client process is still running and its state is unchanged.
func (n *OperaNode) WaitForSync(ctx context.Context) error {
	if s := n.GetState(); s != NodeStateSyncing {
		return fmt.Errorf(
			"node %q: WaitForSync requires state %s, got %s",
			n.GetLabel(), NodeStateSyncing, s,
		)
	}
	slog.Info("Waiting for node to sync", "node", n.config.Label)

	err := network.Retry(ctx, network.DefaultRetryAttempts, 1*time.Second,
		func(ctx context.Context) error {
			if err := n.container.CheckRunning(ctx); err != nil {
				return fmt.Errorf("%w: %w", err, network.ErrPermanent)
			}
			// A client that already exited will never answer, so fail fast
			// with the reason it died instead of retrying until timeout.
			if err := n.clientExitError(); err != nil {
				return fmt.Errorf("%w: %w", err, network.ErrPermanent)
			}
			if err := connectivityCheck(ctx, n); err != nil {
				return err
			}
			_, err := n.GetNodeID()
			return err
		})
	if err != nil {
		return fmt.Errorf("node failed to sync: %w", err)
	}
	return n.transition(NodeStateSyncing, NodeStateRunning)
}

// clientExitError reports the failure of the client process if it has
// already terminated, and nil while it is still running.
func (n *OperaNode) clientExitError() error {
	handle := n.clientHandle()
	if handle == nil {
		return fmt.Errorf("client process was not started")
	}
	select {
	case <-handle.Done:
		if err := handle.Err(); err != nil {
			return fmt.Errorf("client process terminated: %w", err)
		}
		return fmt.Errorf("client process exited unexpectedly")
	default:
		return nil
	}
}

// watchClientExit records the death of a client process that was not asked
// to stop. Without it the recorded state would keep claiming the node is
// syncing or running long after the client crashed, and the discrepancy
// would only surface when some later action happened to touch it.
//
// gen identifies the start this watcher belongs to, so a process that dies
// after the node has already been restarted cannot be mistaken for the
// current one.
func (n *OperaNode) watchClientExit(gen uint64, handle *docker.ExecHandle) {
	<-handle.Done
	if !n.recordUnexpectedClientExit(gen) {
		return
	}
	slog.Error("client process exited unexpectedly; node marked as killed",
		"node", n.GetLabel(),
		"exit_code", handle.ExitCode(),
		"error", handle.Err(),
		"log", handle.LogPath)
}

// recordUnexpectedClientExit moves the node to NodeStateKilled if the exit
// of client generation gen was not requested, reporting whether it did.
//
// An unexpected exit is treated exactly like a kill because the outcome is
// the same: the client went away without flushing, so the data directory
// must be assumed dirty and heal is the only way back to Ready.
func (n *OperaNode) recordUnexpectedClientExit(gen uint64) bool {
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()

	if n.clientGen != gen {
		return false // superseded by a newer client process
	}
	// Any other state means the exit was asked for, or already accounted for.
	if n.state != NodeStateSyncing && n.state != NodeStateRunning {
		return false
	}
	n.state = NodeStateKilled
	n.clientExitWasUnexpected = true
	return true
}

// ClientExitWasUnexpected reports whether the node reached
// NodeStateKilled because its client died on its own rather than because
// it was killed deliberately.
func (n *OperaNode) ClientExitWasUnexpected() bool {
	n.stateMutex.Lock()
	defer n.stateMutex.Unlock()
	return n.clientExitWasUnexpected
}

// StopSonicd stops the OperaNode's sonicd process gracefully. It requires
// NodeStateRunning, passes through NodeStateStopping while the shutdown is
// in flight, and ends in NodeStateReady.
//
// It only reports success once the process has actually exited. If
// signalling fails the node returns to NodeStateRunning so the caller can
// retry; if the process does not exit in time the node stays in
// NodeStateStopping, because it may still hold the data directory and
// starting a second client on it has to be refused. ForceStopSonicd is
// accepted from there to escalate.
func (n *OperaNode) StopSonicd(ctx context.Context) error {
	if err := n.transition(NodeStateRunning, NodeStateStopping); err != nil {
		return err
	}
	slog.Info("Stopping sonicd", "node", n.config.Label)

	signalled, err := n.signalSonicd(ctx, "INT")
	if err != nil {
		n.forceSetState(NodeStateRunning)
		return err
	}
	if signalled == 0 {
		// The node was believed to be running, so a client that is already
		// gone did not shut down cleanly and the database is suspect.
		n.forceSetState(NodeStateKilled)
		return fmt.Errorf(
			"node %q: no client process was running to stop; "+
				"it exited on its own and the database must be healed",
			n.GetLabel())
	}

	if err := n.waitForSonicdExit(ctx); err != nil {
		return fmt.Errorf("node %q: %w", n.GetLabel(), err)
	}
	return n.transition(NodeStateStopping, NodeStateReady)
}

// ForceStopSonicd sends SIGKILL to sonicd, giving it no chance to flush
// the database. Intended for db-heal testing. Requires NodeStateRunning,
// or NodeStateStopping to escalate a graceful stop that did not complete,
// and transitions to NodeStateKilled.
func (n *OperaNode) ForceStopSonicd(ctx context.Context) error {
	if err := n.transitionFromAny(NodeStateKilled,
		NodeStateRunning, NodeStateStopping); err != nil {
		return err
	}
	slog.Info("Force stopping sonicd", "node", n.config.Label)

	// The state stays Killed on every path below: once a kill has been
	// attempted the process may or may not be gone, and only
	// heal-then-restart is safe from here.
	if _, err := n.signalSonicd(ctx, "KILL"); err != nil {
		return err
	}
	if err := n.waitForSonicdExit(ctx); err != nil {
		return fmt.Errorf("node %q: %w", n.GetLabel(), err)
	}
	return nil
}

// HealSonicd runs sonictool heal to recover the database after a
// forceful kill. Requires NodeStateKilled, passes through
// NodeStateHealing, and ends in NodeStateReady. A failed heal returns the
// node to NodeStateKilled so healing can be retried.
func (n *OperaNode) HealSonicd(ctx context.Context) error {
	if err := n.transition(NodeStateKilled, NodeStateHealing); err != nil {
		return err
	}
	slog.Info("Healing database", "node", n.config.Label,
		"after_unexpected_exit", n.ClientExitWasUnexpected())

	// Healing a database that a live client is still writing to would
	// corrupt it, so confirm the process is gone rather than trusting that
	// the kill took effect.
	if err := n.requireNoClientRunning(ctx, "heal the database"); err != nil {
		n.forceSetState(NodeStateKilled)
		return err
	}

	healCmd := []string{
		sonicToolBinaryPath,
		"--datadir", dataDir,
		"heal",
	}
	output, err := n.container.ExecWithOptions(ctx, healCmd,
		docker.ExecOptions{LogName: "sonictool-heal"})
	if err != nil {
		n.forceSetState(NodeStateKilled)
		return fmt.Errorf("sonictool heal failed: %w - output: %s", err, output)
	}
	slog.Info("Database healed", "node", n.config.Label)
	return n.transition(NodeStateHealing, NodeStateReady)
}

// signalSonicd sends the given POSIX signal (name without the "SIG"
// prefix, e.g. "INT" or "KILL") to every sonicd process running inside the
// container, and returns how many processes were signalled. A count of
// zero means no client was running, which the caller has to distinguish
// from a successful stop.
func (n *OperaNode) signalSonicd(ctx context.Context, sig string) (int, error) {
	if n.container == nil {
		return 0, fmt.Errorf("node %q has no container", n.GetLabel())
	}
	action := fmt.Sprintf(clientScanKillAction, sig)
	count, err := n.scanClientProcesses(ctx, action)
	if err != nil {
		return 0, fmt.Errorf("failed to send SIG%s to sonicd: %w", sig, err)
	}
	return count, nil
}

// countRunningClients reports how many client processes are alive inside
// the container, as observed from /proc rather than from recorded state.
func (n *OperaNode) countRunningClients(ctx context.Context) (int, error) {
	if n.container == nil {
		return 0, fmt.Errorf("node %q has no container", n.GetLabel())
	}
	return n.scanClientProcesses(ctx, clientScanCountAction)
}

// requireNoClientRunning fails unless no client process is alive in the
// container. purpose names the operation being guarded, for the message.
func (n *OperaNode) requireNoClientRunning(ctx context.Context, purpose string) error {
	running, err := n.countRunningClients(ctx)
	if err != nil {
		return fmt.Errorf("cannot %s: %w", purpose, err)
	}
	if running > 0 {
		return fmt.Errorf(
			"cannot %s on node %q: %d client process(es) still running",
			purpose, n.GetLabel(), running)
	}
	return nil
}

// scanClientProcesses runs the /proc scan with the given per-match action
// and returns the reported count.
func (n *OperaNode) scanClientProcesses(ctx context.Context, action string) (int, error) {
	script := fmt.Sprintf(clientScanScript, action)
	output, err := n.container.Exec(ctx, []string{"sh", "-c", script})
	if err != nil {
		return 0, fmt.Errorf("%w - output: %s", err, output)
	}
	count, err := parseClientScanCount(output)
	if err != nil {
		return 0, fmt.Errorf("%w - output: %s", err, output)
	}
	return count, nil
}

// parseClientScanCount extracts the count reported by clientScanScript.
func parseClientScanCount(output string) (int, error) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, clientScanMarker) {
			continue
		}
		count, err := strconv.Atoi(strings.TrimPrefix(line, clientScanMarker))
		if err != nil {
			return 0, fmt.Errorf("malformed process scan result %q: %w", line, err)
		}
		return count, nil
	}
	return 0, fmt.Errorf("process scan did not report a result")
}

// waitForSonicdExit blocks until the background sonicd exec's streaming
// goroutine completes, which happens after the process exits and the log
// file has been flushed. It fails if the context expires first, since the
// process is then still running and the caller must not assume otherwise.
func (n *OperaNode) waitForSonicdExit(ctx context.Context) error {
	handle := n.clientHandle()
	if handle == nil {
		return nil
	}
	select {
	case <-handle.Done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf(
			"client process did not exit before the context ended: %w", ctx.Err())
	}
}

// isDirEmpty reports whether the directory at path contains no
// entries (ignoring the "keystore" directory written before init).
func isDirEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return true
	}
	for _, e := range entries {
		if e.Name() != "keystore" {
			return false
		}
	}
	return true
}
