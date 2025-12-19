package ring

import (
	"fmt"
	"sort"
	"sync"

	"github.com/spaolacci/murmur3"
)

// VNode represents a virtual node on the hash ring
type VNode struct {
	Hash     uint64 // Position on the ring
	NodeID   string // Physical node ID
	VNodeIdx int    // Virtual node index (0 to K-1)
}

// HashRing implements consistent hashing with virtual nodes
type HashRing struct {
	mu           sync.RWMutex
	vnodes       []VNode              // Sorted list of virtual nodes
	nodeVNodes   map[string][]uint64  // NodeID -> list of vnode hashes
	virtualCount int                  // Number of virtual nodes per physical node
}

// NewHashRing creates a new consistent hash ring
func NewHashRing(virtualNodes int) *HashRing {
	if virtualNodes < 1 {
		virtualNodes = 150 // Default
	}
	return &HashRing{
		vnodes:       make([]VNode, 0),
		nodeVNodes:   make(map[string][]uint64),
		virtualCount: virtualNodes,
	}
}

// hash computes the 64-bit MurmurHash3 of a string
func hash(key string) uint64 {
	h := murmur3.New64()
	h.Write([]byte(key))
	return h.Sum64()
}

// AddNode adds a physical node to the ring with virtual nodes
func (r *HashRing) AddNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if node already exists
	if _, exists := r.nodeVNodes[nodeID]; exists {
		return
	}

	vnodeHashes := make([]uint64, 0, r.virtualCount)

	for i := 0; i < r.virtualCount; i++ {
		// Create unique key for each virtual node
		vnodeKey := fmt.Sprintf("%s#vnode%d", nodeID, i)
		h := hash(vnodeKey)

		r.vnodes = append(r.vnodes, VNode{
			Hash:     h,
			NodeID:   nodeID,
			VNodeIdx: i,
		})
		vnodeHashes = append(vnodeHashes, h)
	}

	r.nodeVNodes[nodeID] = vnodeHashes

	// Keep vnodes sorted by hash
	sort.Slice(r.vnodes, func(i, j int) bool {
		return r.vnodes[i].Hash < r.vnodes[j].Hash
	})
}

// RemoveNode removes a physical node and all its virtual nodes from the ring
func (r *HashRing) RemoveNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.nodeVNodes[nodeID]; !exists {
		return
	}

	// Filter out vnodes belonging to this node
	newVNodes := make([]VNode, 0, len(r.vnodes)-r.virtualCount)
	for _, vn := range r.vnodes {
		if vn.NodeID != nodeID {
			newVNodes = append(newVNodes, vn)
		}
	}

	r.vnodes = newVNodes
	delete(r.nodeVNodes, nodeID)
}

// GetNode returns the node responsible for a given key
func (r *HashRing) GetNode(key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.vnodes) == 0 {
		return "", fmt.Errorf("no nodes in ring")
	}

	h := hash(key)

	// Binary search for the first vnode with hash >= key hash
	idx := sort.Search(len(r.vnodes), func(i int) bool {
		return r.vnodes[i].Hash >= h
	})

	// Wrap around to the first node if we're past the end
	if idx >= len(r.vnodes) {
		idx = 0
	}

	return r.vnodes[idx].NodeID, nil
}

// GetNodes returns N distinct physical nodes for a key (preference list)
// These are the nodes where the data will be replicated
func (r *HashRing) GetNodes(key string, n int) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.vnodes) == 0 {
		return nil, fmt.Errorf("no nodes in ring")
	}

	h := hash(key)

	// Binary search for starting position
	startIdx := sort.Search(len(r.vnodes), func(i int) bool {
		return r.vnodes[i].Hash >= h
	})

	if startIdx >= len(r.vnodes) {
		startIdx = 0
	}

	// Collect N distinct physical nodes
	nodes := make([]string, 0, n)
	seen := make(map[string]bool)

	for i := 0; i < len(r.vnodes) && len(nodes) < n; i++ {
		idx := (startIdx + i) % len(r.vnodes)
		nodeID := r.vnodes[idx].NodeID

		if !seen[nodeID] {
			seen[nodeID] = true
			nodes = append(nodes, nodeID)
		}
	}

	return nodes, nil
}

// GetAllNodes returns all physical nodes in the ring
func (r *HashRing) GetAllNodes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]string, 0, len(r.nodeVNodes))
	for nodeID := range r.nodeVNodes {
		nodes = append(nodes, nodeID)
	}
	return nodes
}

// Size returns the number of physical nodes in the ring
func (r *HashRing) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.nodeVNodes)
}

// IsEmpty returns true if there are no nodes in the ring
func (r *HashRing) IsEmpty() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.vnodes) == 0
}

// HasNode checks if a node exists in the ring
func (r *HashRing) HasNode(nodeID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.nodeVNodes[nodeID]
	return exists
}

// GetRingTokens returns all tokens on the ring (for visualization/debugging)
func (r *HashRing) GetRingTokens() []VNode {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tokens := make([]VNode, len(r.vnodes))
	copy(tokens, r.vnodes)
	return tokens
}

// GetKeyHash returns the hash of a key (for testing/debugging)
func GetKeyHash(key string) uint64 {
	return hash(key)
}
