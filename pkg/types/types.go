package types

import (
	"time"
)

// NodeState represents the health state of a node in the cluster
type NodeState int

const (
	NodeAlive NodeState = iota
	NodeSuspect
	NodeDead
)

func (s NodeState) String() string {
	switch s {
	case NodeAlive:
		return "alive"
	case NodeSuspect:
		return "suspect"
	case NodeDead:
		return "dead"
	default:
		return "unknown"
	}
}

// Node represents a node in the distributed cluster
type Node struct {
	ID        string    `json:"id"`
	Address   string    `json:"address"`
	Port      int       `json:"port"`
	State     NodeState `json:"state"`
	LastSeen  time.Time `json:"last_seen"`
	TokenRing []uint64  `json:"token_ring,omitempty"` // Virtual node positions
}

// FullAddress returns the complete address string (host:port)
func (n *Node) FullAddress() string {
	return n.Address + ":" + string(rune(n.Port))
}

// KeyValueEntry represents a stored key-value pair with metadata
type KeyValueEntry struct {
	Key       string       `json:"key"`
	Value     []byte       `json:"value"`
	Timestamp int64        `json:"timestamp"`
	Version   VectorClock  `json:"version,omitempty"`
	IsDeleted bool         `json:"is_deleted"` // Tombstone marker
}

// VectorClock tracks causality across distributed nodes
type VectorClock map[string]uint64

// Increment increases the clock for a specific node
func (vc VectorClock) Increment(nodeID string) {
	vc[nodeID]++
}

// Merge combines two vector clocks, taking max of each component
func (vc VectorClock) Merge(other VectorClock) VectorClock {
	result := make(VectorClock)
	for k, v := range vc {
		result[k] = v
	}
	for k, v := range other {
		if v > result[k] {
			result[k] = v
		}
	}
	return result
}

// Compare returns -1 if vc < other, 0 if concurrent, 1 if vc > other
func (vc VectorClock) Compare(other VectorClock) int {
	less, greater := false, false
	
	allKeys := make(map[string]bool)
	for k := range vc {
		allKeys[k] = true
	}
	for k := range other {
		allKeys[k] = true
	}
	
	for k := range allKeys {
		v1, v2 := vc[k], other[k]
		if v1 < v2 {
			less = true
		}
		if v1 > v2 {
			greater = true
		}
	}
	
	if less && !greater {
		return -1
	}
	if greater && !less {
		return 1
	}
	return 0 // Concurrent
}

// Copy creates a deep copy of the vector clock
func (vc VectorClock) Copy() VectorClock {
	result := make(VectorClock)
	for k, v := range vc {
		result[k] = v
	}
	return result
}

// ConsistencyLevel defines the consistency requirements for operations
type ConsistencyLevel string

const (
	ConsistencyOne    ConsistencyLevel = "one"
	ConsistencyQuorum ConsistencyLevel = "quorum"
	ConsistencyAll    ConsistencyLevel = "all"
)

// PutRequest represents a request to store a key-value pair
type PutRequest struct {
	Key         string           `json:"key"`
	Value       string           `json:"value"`
	Consistency ConsistencyLevel `json:"consistency,omitempty"`
}

// GetRequest represents a request to retrieve a value
type GetRequest struct {
	Key         string           `json:"key"`
	Consistency ConsistencyLevel `json:"consistency,omitempty"`
}

// GetResponse represents the response for a get operation
type GetResponse struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Version   int64  `json:"version"`
	Found     bool   `json:"found"`
}

// ClusterStatus represents the current state of the cluster
type ClusterStatus struct {
	NodeID      string          `json:"node_id"`
	Nodes       []NodeInfo      `json:"nodes"`
	Ring        []RingToken     `json:"ring,omitempty"`
	TotalKeys   int64           `json:"total_keys"`
	Uptime      string          `json:"uptime"`
}

// NodeInfo provides information about a node in the cluster
type NodeInfo struct {
	ID       string    `json:"id"`
	Address  string    `json:"address"`
	State    string    `json:"state"`
	LastSeen time.Time `json:"last_seen"`
}

// RingToken represents a position on the hash ring
type RingToken struct {
	Token  uint64 `json:"token"`
	NodeID string `json:"node_id"`
}

// HintedHandoff stores data temporarily for a failed node
type HintedHandoff struct {
	TargetNode string        `json:"target_node"`
	Entry      KeyValueEntry `json:"entry"`
	CreatedAt  time.Time     `json:"created_at"`
	Attempts   int           `json:"attempts"`
}

// ReplicationRequest is sent between nodes to replicate data
type ReplicationRequest struct {
	Entry     KeyValueEntry `json:"entry"`
	FromNode  string        `json:"from_node"`
	IsHandoff bool          `json:"is_handoff"`
}

// ReplicationResponse is the response to a replication request
type ReplicationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// GossipMessage is exchanged between nodes for failure detection
type GossipMessage struct {
	FromNode   string              `json:"from_node"`
	Members    map[string]NodeInfo `json:"members"`
	Timestamp  time.Time           `json:"timestamp"`
}
