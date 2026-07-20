package node

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/0xsoniclabs/norma/driver/network"
)

// sonicdBinaryPath is the absolute path of the sonicd binary inside
// the container. Stage 2 of the Dockerfile copies the binaries into
// the root directory (there is no WORKDIR), so `./sonicd` at runtime
// resolves to `/sonicd`. This path is used to identify the running
// sonicd process by resolving /proc/<pid>/exe, which avoids substring
// collisions with sibling binaries such as sonictool.
const sonicdBinaryPath = "/sonicd"

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
//   - Write the config.toml file with emitter intervals.
//   - Configure network latency simulation via tc netem.
//
// Requires the node to be in NodeStateUninitialized and transitions
// it to NodeStateReady. Returns an error if any of the steps fail or
// if the node is not in the expected state.
func (n *OperaNode) Initialize(ctx context.Context) error {
	// Guard concurrent Initialize calls: we don't publish the terminal
	// state (Ready) until every step below has succeeded, but we still
	// need to make sure only one caller enters this critical section.
	if s := n.GetState(); s != NodeStateUninitialized {
		return fmt.Errorf(
			"node %q: Initialize requires state %s, got %s",
			n.GetLabel(), NodeStateUninitialized, s)
	}

	// Skip initialization only when re-using a populated mount directory.
	needsInit := n.config.MountDataDir == nil || isDirEmpty(*n.config.MountDataDir)
	if needsInit {
		mkdirCmd := []string{"mkdir", "-m", "755", "-p", dataDir}
		if output, err := n.host.ExecWithEnv(ctx, mkdirCmd, nil, ""); err != nil {
			return fmt.Errorf("failed to create datadir: %w - output: %s", err, output)
		}

		sonicToolCmd := []string{
			"./sonictool",
			"--datadir", dataDir,
			"--statedb.livecache", "1",
			"genesis", "json", "--experimental", "/genesis.json",
		}
		output, err := n.host.ExecWithEnv(ctx, sonicToolCmd, nil, "sonictool")
		if err != nil {
			return fmt.Errorf("sonictool genesis init failed: %w - output: %s", err, output)
		}
	}

	// Write password file for validator keystore decryption. The
	// password is intentionally the fixed string "password" because
	// norma only spins up fake, throwaway validators for testing; it
	// is never used to protect real keys.
	passwordCmd := []string{"sh", "-c", "echo password > password.txt"}
	if _, err := n.host.ExecWithEnv(ctx, passwordCmd, nil, ""); err != nil {
		return fmt.Errorf("failed to write password file: %w", err)
	}

	// Write config.toml (emitter intervals).
	numValidators := n.config.NetworkConfig.Validators.GetNumValidators()
	dsProtection := "5000000000"
	if numValidators == 1 && n.config.ValidatorId != nil && *n.config.ValidatorId == 1 {
		dsProtection = "0"
	}
	configToml := fmt.Sprintf("[Emitter.EmitIntervals]\nDoublesignProtection = %s\n", dsProtection)
	configCmd := []string{"sh", "-c",
		fmt.Sprintf("printf '%%s' '%s' > config.toml", configToml)}
	if _, err := n.host.ExecWithEnv(ctx, configCmd, nil, ""); err != nil {
		return fmt.Errorf("failed to write config.toml: %w", err)
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
		if _, err := n.host.ExecWithEnv(ctx, cmd, nil, "tc_setup"); err != nil {
			return fmt.Errorf("failed to configure network latency: %w", err)
		}
	}

	return n.transition(NodeStateUninitialized, NodeStateReady)
}

// StartSonicd starts the OperaNode's sonicd process in the background.
// It requires the node to be in NodeStateReady and transitions it to
// NodeStateSyncing. Returns an error if sonicd fails to start.
func (n *OperaNode) StartSonicd(ctx context.Context) error {
	return n.startSonicd(ctx, false)
}

// StartSonicdAsObserver starts sonicd without validator flags, useful
// after a database heal when the emitter state has been lost.
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
		n.config.Address,
		n.container.IP(),
		n.config.ExtraArguments)

	slog.Info("Starting sonicd", "node", n.config.Label)
	handle, err := n.container.ExecBackground(
		ctx,
		sonicdCmd,
		[]string{"GOMEMLIMIT=1GiB"},
		"sonicd",
	)
	if err != nil {
		// Roll back so the caller may retry from Ready.
		n.forceSetState(NodeStateReady)
		return fmt.Errorf("failed to start sonicd: %w", err)
	}
	n.sonicd = handle
	slog.Info("Sonicd started", "node", n.config.Label)
	return nil
}

// WaitForSync waits for the OperaNode to finish syncing and become
// ready. Requires NodeStateSyncing and transitions to NodeStateRunning
// on success.
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
			if err := n.host.CheckRunning(ctx); err != nil {
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

// StopSonicd stops the OperaNode's sonicd process gracefully. It
// requires NodeStateRunning, passes through NodeStateStopping while
// the shutdown is in flight, and ends in NodeStateReady.
func (n *OperaNode) StopSonicd(ctx context.Context) error {
	if err := n.transition(NodeStateRunning, NodeStateStopping); err != nil {
		return err
	}
	slog.Info("Stopping sonicd", "node", n.config.Label)

	if err := n.signalSonicd(ctx, "INT"); err != nil {
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
		return err
	}
	n.waitForSonicdExit(ctx)
	return nil
}

// HealSonicd runs sonictool heal to recover the database after a
// forceful kill. Requires NodeStateKilled, passes through
// NodeStateHealing, and ends in NodeStateReady.
func (n *OperaNode) HealSonicd(ctx context.Context) error {
	if err := n.transition(NodeStateKilled, NodeStateHealing); err != nil {
		return err
	}
	slog.Info("Healing database", "node", n.config.Label)

	healCmd := []string{
		"./sonictool",
		"--cache", "12522",
		"--datadir", dataDir,
		"heal",
	}
	output, err := n.container.Exec(ctx, healCmd)
	if err != nil {
		return fmt.Errorf(
			"sonictool heal failed: %w - output: %s", err, output,
		)
	}
	slog.Info("Database healed", "node", n.config.Label)
	return n.transition(NodeStateHealing, NodeStateReady)
}

// signalSonicd sends the given POSIX signal (name without the "SIG"
// prefix, e.g. "INT" or "KILL") to every sonicd process running inside
// the container. Uses a /proc/<pid>/exe symlink lookup for precise
// matching that ignores sibling binaries such as sonictool.
func (n *OperaNode) signalSonicd(ctx context.Context, sig string) error {
	if n.container == nil || n.sonicd == nil {
		return nil
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
