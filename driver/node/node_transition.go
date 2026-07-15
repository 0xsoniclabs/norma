package node

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/0xsoniclabs/norma/driver/network"
)

// Initialize prepares the OperaNode for operation performing:
// - Create the data directory and initialize the genesis state.
// - Write the password file for validator keystore decryption.
// - Write the config.toml file with emitter intervals.
// - Configure network latency simulation via tc netem.
// Returns an error if any of the steps fail or if the node is not in the
// NodeStateUninitialized state.
func Initialize(ctx context.Context, node *OperaNode) error {

	if err := node.requireState(NodeStateUninitialized); err != nil {
		return err
	}

	// Skip initialization only when re-using a populated mount directory.
	needsInit := node.config.MountDataDir == nil || isDirEmpty(*node.config.MountDataDir)
	if needsInit {
		mkdirCmd := []string{"mkdir", "-m", "755", "-p", dataDir}
		if output, err := node.host.ExecWithEnv(ctx, mkdirCmd, nil, ""); err != nil {
			return fmt.Errorf("failed to create datadir: %w - output: %s", err, output)
		}

		sonicToolCmd := []string{
			"./sonictool",
			"--datadir", dataDir,
			"--statedb.livecache", "1",
			"genesis", "json", "--experimental", "/genesis.json",
		}
		output, err := node.host.ExecWithEnv(ctx, sonicToolCmd, nil, "sonictool")
		if err != nil {
			return fmt.Errorf("sonictool genesis init failed: %w - output: %s", err, output)
		}
	}

	// 2. Write password file for validator keystore decryption.
	passwordCmd := []string{"sh", "-c", "echo password > password.txt"}
	if _, err := node.host.ExecWithEnv(ctx, passwordCmd, nil, ""); err != nil {
		return fmt.Errorf("failed to write password file: %w", err)
	}

	// 3. Write config.toml (emitter intervals).
	numValidators := node.config.NetworkConfig.Validators.GetNumValidators()
	dsProtection := "5000000000"
	if numValidators == 1 && node.config.ValidatorId != nil && *node.config.ValidatorId == 1 {
		dsProtection = "0"
	}
	configToml := fmt.Sprintf("[Emitter.EmitIntervals]\nDoublesignProtection = %s\n", dsProtection)
	configCmd := []string{"sh", "-c",
		fmt.Sprintf("printf '%%s' '%s' > config.toml", configToml)}
	if _, err := node.host.ExecWithEnv(ctx, configCmd, nil, ""); err != nil {
		return fmt.Errorf("failed to write config.toml: %w", err)
	}

	// 4. Network latency simulation via tc netem.
	latency := node.config.NetworkConfig.RoundTripTime / 2
	if latency > 0 {
		tcCmd := fmt.Sprintf(
			"tc qdisc add dev eth0 root netem delay %v"+
				" && (ip link show eth1 2>/dev/null"+
				" && tc qdisc add dev eth1 root netem delay %v || true)",
			latency, latency)
		cmd := []string{"sh", "-c", tcCmd}
		if _, err := node.host.ExecWithEnv(ctx, cmd, nil, "tc_setup"); err != nil {
			return fmt.Errorf("failed to configure network latency: %w", err)
		}
	}

	node.setState(NodeStateReady)

	return nil
}

// StartSonicd starts the OperaNode's sonicd process in the background.
// It requires the node to be in the NodeStateReady state and transitions it to
// NodeStateSyncing. Returns an error if sonicd fails to start.
func StartSonicd(ctx context.Context, node *OperaNode) error {

	if err := node.requireState(NodeStateReady); err != nil {
		return err
	}

	// 5. Build sonicd command line.
	sonicdCmd := buildSonicdCmd(
		node.config.ValidatorId,
		node.config.PubKey,
		node.config.Address,
		node.container.IP(),
		node.config.ExtraArguments)

	// 6. Start sonicd in the background.
	slog.Info("Starting sonicd", "node", node.config.Label)
	var err error
	node.sonicd, err = node.container.ExecBackground(
		ctx,
		sonicdCmd,
		[]string{"GOMEMLIMIT=1GiB"},
		"sonicd",
	)
	if err != nil {
		return fmt.Errorf("failed to start sonicd: %w", err)
	}
	slog.Info("Sonicd started")
	node.setState(NodeStateSyncing)

	return nil
}

// WaitForSync waits for the OperaNode to finish syncing and become ready.
// It requires the node to be in the NodeStateSyncing state and transitions it to
// NodeStateRunning. Returns an error if the node fails to sync.
func WaitForSync(ctx context.Context, node *OperaNode) error {

	if err := node.requireState(NodeStateSyncing); err != nil {
		return err
	}

	slog.Info("Waiting for node to sync", "node", node.config.Label)

	// Wait until the OperaNode inside the Container is ready.
	err := network.Retry(ctx, network.DefaultRetryAttempts, 1*time.Second,
		func(ctx context.Context) error {
			if err := node.host.CheckRunning(ctx); err != nil {
				return fmt.Errorf("%w: %w", err, network.ErrPermanent)
			}
			if err := connectivityCheck(ctx, node); err != nil {
				return err
			}
			_, err := node.GetNodeID()
			return err
		})

	if err == nil {
		node.setState(NodeStateRunning)
		return nil
	}
	return fmt.Errorf("node failed to sync")
}

// StopSonicd stops the OperaNode's sonicd process gracefully.
// It requires the node to be in the NodeStateRunning state and transitions it to
// NodeStateReady. Returns an error if sonicd fails to stop.
func StopSonicd(ctx context.Context, node *OperaNode) error {

	if err := node.requireState(NodeStateRunning); err != nil {
		return err
	}

	slog.Info("Stopping sonicd", "node", node.config.Label)

	// Send SIGINT to sonicd so it can shut down gracefully (close DBs, etc.).
	// The container's PID 1 is "sleep infinity" which doesn't forward signals,
	// so we must explicitly find and signal the sonicd process.
	if node.container != nil && node.sonicd != nil {
		log, err := node.container.Exec(ctx, []string{
			"sh", "-c",
			`for d in /proc/[0-9]*; do` +
				` grep -ql sonicd "$d/cmdline" 2>/dev/null &&` +
				` kill -INT "${d##*/}" 2>/dev/null; done`,
		})
		if err != nil {
			return fmt.Errorf("failed to send SIGINT to sonicd: %w\n\nlog: %s", err, log)
		}
		// Wait for sonicd to exit gracefully before stopping the container.
		select {
		case <-node.sonicd.Done:
		case <-ctx.Done():
		}
	}

	node.setState(NodeStateReady)

	return nil
}
