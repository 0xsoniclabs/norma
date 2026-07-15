package node

// NodeState represents the state of a node in the network.
type NodeState int

const (
	// NodeStateUnknown represents an unknown state for a node.
	NodeStateOffline NodeState = iota
	// NodeStateOnline represents an online state for a node.
	NodeStateUninitialized
	// NodeStateInitialized represents an initialized state for a node.
	NodeStateInitialized
	// NodeStateSyncing represents a syncing state for a node.
	NodeStateSyncing
	// NodeStateRunning represents a running state for a node.
	NodeStateRunning
	// NodeStateStopped represents a stopped state for a node.
	NodeStateStopped
	// NodeStateKilled represents a killed state for a node.
	NodeStateKilled
	// NodeStateHealing represents a healing state for a node.
	NodeStateHealing
)
