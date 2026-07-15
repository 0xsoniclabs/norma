package node

// NodeState represents the state of a node in the network.
type NodeState int

const (
	// NodeStateOffline represents an offline state for a node.
	NodeStateOffline NodeState = iota
	// NodeStateUninitialized represents an uninitialized state for a node.
	NodeStateUninitialized
	// NodeStateReady represents a ready state for a node.
	NodeStateReady
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
