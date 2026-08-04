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

// killSonicdScript is a POSIX shell script template that sends the
// given signal to every process in the container whose executable is
// the sonicd binary. Matching on the exe symlink (rather than a
// substring of cmdline) prevents accidental hits on sonictool or any
// other sibling process. The trailing `true` ensures the exec exits 0
// even when no process matches, which is a valid outcome (e.g., sonicd
// already exited).
const killSonicdScript = `for d in /proc/[0-9]*; do` +
	` exe=$(readlink "$d/exe" 2>/dev/null) || continue;` +
	` [ "$exe" = "` + sonicdBinaryPath + `" ] || continue;` +
	` kill -%s "${d##*/}" 2>/dev/null;` +
	` done; true`

// Initialize prepares the OperaNode for operation performing:
//   - Create the data directory and initialize the genesis state.
//   - Write the password file for validator keystore decryption.
//   - Write the config file with emitter intervals.
//   - Configure network latency simulation via tc netem.
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

	numValidators := n.config.NetworkConfig.Validators.GetNumValidators()
	dsProtection := "5000000000"
	if numValidators == 1 && n.config.ValidatorId != nil && *n.config.ValidatorId == 1 {
		dsProtection = "0"
	}
	configToml := fmt.Sprintf(
		"[Emitter.EmitIntervals]\nDoublesignProtection = %s\n", dsProtection)
	if err := n.writeContainerFile(ctx, configFilePath, configToml); err != nil {
		return fmt.Errorf("failed to write %s: %w", configFilePath, err)
	}

	// Network latency simulation via tc netem.
	latency := n.config.NetworkConfig.RoundTripTime / 2
	if latency > 0 {
		tcCmd := fmt.Sprintf(
			"tc qdisc add dev eth0 root netem delay %v"+
				" && (ip link show eth1 2>/dev/null"+
				" && tc qdisc add dev eth1 root netem delay %v || true)",
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
	if err := n.transition(NodeStateReady, NodeStateSyncing); err != nil {
		return err
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
	n.sonicd = handle
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
	if n.sonicd == nil {
		return fmt.Errorf("client process was not started")
	}
	select {
	case <-n.sonicd.Done:
		if err := n.sonicd.Err(); err != nil {
			return fmt.Errorf("client process terminated: %w", err)
		}
		return fmt.Errorf("client process exited unexpectedly")
	default:
		return nil
	}
}

// StopSonicd stops the OperaNode's sonicd process gracefully. It
// requires NodeStateRunning, passes through NodeStateStopping while
// the shutdown is in flight, and ends in NodeStateReady. If signalling
// fails the node is returned to NodeStateRunning so the caller can retry
// or escalate to ForceStopSonicd.
func (n *OperaNode) StopSonicd(ctx context.Context) error {
	if err := n.transition(NodeStateRunning, NodeStateStopping); err != nil {
		return err
	}
	slog.Info("Stopping sonicd", "node", n.config.Label)

	if err := n.signalSonicd(ctx, "INT"); err != nil {
		n.forceSetState(NodeStateRunning)
		return err
	}
	n.waitForSonicdExit(ctx)
	return n.transition(NodeStateStopping, NodeStateReady)
}

// ForceStopSonicd sends SIGKILL to sonicd, giving it no chance to flush
// the database. Intended for db-heal testing. Requires NodeStateRunning
// and transitions to NodeStateKilled.
func (n *OperaNode) ForceStopSonicd(ctx context.Context) error {
	if err := n.transition(NodeStateRunning, NodeStateKilled); err != nil {
		return err
	}
	slog.Info("Force stopping sonicd", "node", n.config.Label)

	if err := n.signalSonicd(ctx, "KILL"); err != nil {
		// The state stays Killed: after a failed SIGKILL the process may or
		// may not be gone, and only heal-then-restart is safe from here.
		return err
	}
	n.waitForSonicdExit(ctx)
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
	slog.Info("Healing database", "node", n.config.Label)

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
// prefix, e.g. "INT" or "KILL") to every sonicd process running inside
// the container. Uses a /proc/<pid>/exe symlink lookup for precise
// matching that ignores sibling binaries such as sonictool.
func (n *OperaNode) signalSonicd(ctx context.Context, sig string) error {
	if n.container == nil {
		return fmt.Errorf("node %q has no container", n.GetLabel())
	}
	if n.sonicd == nil {
		return fmt.Errorf("node %q has no client process to signal", n.GetLabel())
	}
	script := fmt.Sprintf(killSonicdScript, sig)
	output, err := n.container.Exec(ctx, []string{"sh", "-c", script})
	if err != nil {
		return fmt.Errorf(
			"failed to send SIG%s to sonicd: %w - output: %s",
			sig, err, output,
		)
	}
	return nil
}

// waitForSonicdExit blocks until the background sonicd exec's
// streaming goroutine completes (which happens after the process exits
// and the log file has been flushed) or the context is cancelled.
func (n *OperaNode) waitForSonicdExit(ctx context.Context) {
	if n.sonicd == nil {
		return
	}
	select {
	case <-n.sonicd.Done:
	case <-ctx.Done():
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
