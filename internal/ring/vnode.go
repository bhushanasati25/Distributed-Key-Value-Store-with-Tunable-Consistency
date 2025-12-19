package ring

import (
	"fmt"
)

// TokenRange represents a range of tokens owned by a node
type TokenRange struct {
	StartToken uint64 `json:"start_token"`
	EndToken   uint64 `json:"end_token"`
	NodeID     string `json:"node_id"`
}

// VNodeManager provides higher-level virtual node operations
type VNodeManager struct {
	ring *HashRing
}

// NewVNodeManager creates a vnode manager wrapping a hash ring
func NewVNodeManager(ring *HashRing) *VNodeManager {
	return &VNodeManager{ring: ring}
}

// GetTokenRanges returns the token ranges owned by each node
func (m *VNodeManager) GetTokenRanges() []TokenRange {
	tokens := m.ring.GetRingTokens()
	if len(tokens) == 0 {
		return nil
	}

	ranges := make([]TokenRange, len(tokens))
	for i, vn := range tokens {
		var startToken uint64
		if i == 0 {
			// First node's range starts from the last node's position + 1
			startToken = tokens[len(tokens)-1].Hash + 1
		} else {
			startToken = tokens[i-1].Hash + 1
		}

		ranges[i] = TokenRange{
			StartToken: startToken,
			EndToken:   vn.Hash,
			NodeID:     vn.NodeID,
		}
	}

	return ranges
}

// GetNodeTokenRanges returns token ranges for a specific node
func (m *VNodeManager) GetNodeTokenRanges(nodeID string) []TokenRange {
	allRanges := m.GetTokenRanges()
	nodeRanges := make([]TokenRange, 0)

	for _, r := range allRanges {
		if r.NodeID == nodeID {
			nodeRanges = append(nodeRanges, r)
		}
	}

	return nodeRanges
}

// CalculateLoadDistribution returns the percentage of keyspace each node owns
func (m *VNodeManager) CalculateLoadDistribution() map[string]float64 {
	ranges := m.GetTokenRanges()
	if len(ranges) == 0 {
		return nil
	}

	nodeLoad := make(map[string]uint64)
	var totalSpace uint64

	for _, r := range ranges {
		var rangeSize uint64
		if r.EndToken >= r.StartToken {
			rangeSize = r.EndToken - r.StartToken + 1
		} else {
			// Wraps around (shouldn't happen normally but handle it)
			rangeSize = (^uint64(0) - r.StartToken) + r.EndToken + 2
		}
		nodeLoad[r.NodeID] += rangeSize
		totalSpace += rangeSize
	}

	distribution := make(map[string]float64)
	for nodeID, load := range nodeLoad {
		distribution[nodeID] = float64(load) / float64(totalSpace) * 100
	}

	return distribution
}

// PrintRingStatus prints a human-readable view of the ring
func (m *VNodeManager) PrintRingStatus() string {
	nodes := m.ring.GetAllNodes()
	if len(nodes) == 0 {
		return "Ring is empty"
	}

	result := fmt.Sprintf("Ring Status: %d physical nodes, %d virtual nodes\n",
		len(nodes), len(m.ring.GetRingTokens()))

	distribution := m.CalculateLoadDistribution()
	for nodeID, load := range distribution {
		result += fmt.Sprintf("  %s: %.2f%% of keyspace\n", nodeID, load)
	}

	return result
}

// GetSuccessorNodes returns the next N nodes after the given node on the ring
func (m *VNodeManager) GetSuccessorNodes(nodeID string, n int) []string {
	tokens := m.ring.GetRingTokens()
	if len(tokens) == 0 {
		return nil
	}

	// Find the first token of this node
	startIdx := -1
	for i, vn := range tokens {
		if vn.NodeID == nodeID {
			startIdx = i
			break
		}
	}

	if startIdx == -1 {
		return nil
	}

	// Collect next N distinct nodes
	successors := make([]string, 0, n)
	seen := make(map[string]bool)
	seen[nodeID] = true // Exclude self

	for i := 1; i <= len(tokens) && len(successors) < n; i++ {
		idx := (startIdx + i) % len(tokens)
		nextNode := tokens[idx].NodeID

		if !seen[nextNode] {
			seen[nextNode] = true
			successors = append(successors, nextNode)
		}
	}

	return successors
}

// GetPredecessorNodes returns the previous N nodes before the given node
func (m *VNodeManager) GetPredecessorNodes(nodeID string, n int) []string {
	tokens := m.ring.GetRingTokens()
	if len(tokens) == 0 {
		return nil
	}

	// Find the first token of this node
	startIdx := -1
	for i, vn := range tokens {
		if vn.NodeID == nodeID {
			startIdx = i
			break
		}
	}

	if startIdx == -1 {
		return nil
	}

	// Collect previous N distinct nodes
	predecessors := make([]string, 0, n)
	seen := make(map[string]bool)
	seen[nodeID] = true // Exclude self

	for i := 1; i <= len(tokens) && len(predecessors) < n; i++ {
		idx := (startIdx - i + len(tokens)) % len(tokens)
		prevNode := tokens[idx].NodeID

		if !seen[prevNode] {
			seen[prevNode] = true
			predecessors = append(predecessors, prevNode)
		}
	}

	return predecessors
}
