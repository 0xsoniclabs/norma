package node

// NodeState represents the lifecycle state of an OperaNode.
//
// Legal transitions are enforced by OperaNode.transition:
//
//	Uninitialized --Initialize--> Ready
//	Ready         --StartSonicd--> Syncing
//	Syncing       --WaitForSync--> Running
//	Running       --StopSonicd--> Stopping --> Ready
//	Running       --ForceStopSonicd--> Killed
//	Killed        --HealSonicd--> Healing --> Ready
type NodeState int

const (
	// NodeStateUninitialized is the initial state: container exists but
	// nothing has been written to the data directory yet.
	NodeStateUninitialized NodeState = iota
	// NodeStateReady means the data directory is initialized and sonicd
	// can be started.
	NodeStateReady
	// NodeStateSyncing means sonicd is running but has not yet reached
	// the network head.
	NodeStateSyncing
	// NodeStateRunning means sonicd is running and synced.
	NodeStateRunning
	// NodeStateStopping is the transitional state entered while sonicd
	// is being asked to shut down gracefully.
	NodeStateStopping
	// NodeStateKilled means sonicd was terminated with SIGKILL and the
	// on-disk database is likely dirty.
	NodeStateKilled
	// NodeStateHealing is the transitional state entered while
	// sonictool heal is running against a killed node.
	NodeStateHealing
)

// String returns a human-readable name for the state, used in error
// messages produced by transition.
func (s NodeState) String() string {
	switch s {
	case NodeStateUninitialized:
		return "uninitialized"
	case NodeStateReady:
		return "ready"
	case NodeStateSyncing:
		return "syncing"
	case NodeStateRunning:
		return "running"
	case NodeStateStopping:
		return "stopping"
	case NodeStateKilled:
		return "killed"
	case NodeStateHealing:
		return "healing"
	default:
		return "unknown"
	}
}
