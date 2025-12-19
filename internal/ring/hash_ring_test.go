package ring

import (
	"fmt"
	"testing"
)

func TestHashRingAddNode(t *testing.T) {
	ring := NewHashRing(10) // 10 vnodes for testing

	ring.AddNode("node1")
	ring.AddNode("node2")
	ring.AddNode("node3")

	if ring.Size() != 3 {
		t.Errorf("Expected 3 nodes, got %d", ring.Size())
	}

	tokens := ring.GetRingTokens()
	if len(tokens) != 30 { // 3 nodes * 10 vnodes
		t.Errorf("Expected 30 vnodes, got %d", len(tokens))
	}
}

func TestHashRingRemoveNode(t *testing.T) {
	ring := NewHashRing(10)

	ring.AddNode("node1")
	ring.AddNode("node2")
	ring.AddNode("node3")

	ring.RemoveNode("node2")

	if ring.Size() != 2 {
		t.Errorf("Expected 2 nodes, got %d", ring.Size())
	}

	tokens := ring.GetRingTokens()
	for _, token := range tokens {
		if token.NodeID == "node2" {
			t.Error("node2 should not be in the ring")
		}
	}
}

func TestHashRingGetNode(t *testing.T) {
	ring := NewHashRing(100)

	ring.AddNode("node1")
	ring.AddNode("node2")
	ring.AddNode("node3")

	// Same key should always map to same node
	node1, _ := ring.GetNode("testkey")
	node2, _ := ring.GetNode("testkey")

	if node1 != node2 {
		t.Errorf("Same key mapped to different nodes: %s vs %s", node1, node2)
	}

	// Different keys should distribute across nodes
	nodeCount := make(map[string]int)
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		node, _ := ring.GetNode(key)
		nodeCount[node]++
	}

	// All nodes should have some keys
	for _, node := range []string{"node1", "node2", "node3"} {
		if nodeCount[node] == 0 {
			t.Errorf("Node %s has no keys", node)
		}
	}
}

func TestHashRingGetNodes(t *testing.T) {
	ring := NewHashRing(100)

	ring.AddNode("node1")
	ring.AddNode("node2")
	ring.AddNode("node3")

	// Get 3 nodes for replication
	nodes, err := ring.GetNodes("testkey", 3)
	if err != nil {
		t.Fatalf("GetNodes failed: %v", err)
	}

	if len(nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(nodes))
	}

	// All nodes should be unique
	seen := make(map[string]bool)
	for _, n := range nodes {
		if seen[n] {
			t.Errorf("Duplicate node in preference list: %s", n)
		}
		seen[n] = true
	}
}

func TestHashRingConsistency(t *testing.T) {
	ring := NewHashRing(100)

	ring.AddNode("node1")
	ring.AddNode("node2")
	ring.AddNode("node3")

	// Record key mappings before adding node
	mappings := make(map[string]string)
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%d", i)
		node, _ := ring.GetNode(key)
		mappings[key] = node
	}

	// Add a new node
	ring.AddNode("node4")

	// Count how many keys moved
	movedCount := 0
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key-%d", i)
		newNode, _ := ring.GetNode(key)
		if newNode != mappings[key] {
			movedCount++
		}
	}

	// With consistent hashing, roughly 1/4 of keys should move
	// Allow some variance
	if movedCount > 50 {
		t.Errorf("Too many keys moved (%d), consistent hashing not working properly", movedCount)
	}
}

func TestHashRingEmptyRing(t *testing.T) {
	ring := NewHashRing(100)

	_, err := ring.GetNode("testkey")
	if err == nil {
		t.Error("Expected error for empty ring")
	}

	nodes, err := ring.GetNodes("testkey", 3)
	if err == nil || nodes != nil {
		t.Error("Expected error for empty ring")
	}
}

func TestHashRingHasNode(t *testing.T) {
	ring := NewHashRing(10)

	ring.AddNode("node1")

	if !ring.HasNode("node1") {
		t.Error("HasNode should return true for existing node")
	}

	if ring.HasNode("node2") {
		t.Error("HasNode should return false for non-existing node")
	}
}

func TestVNodeManager(t *testing.T) {
	ring := NewHashRing(10)
	ring.AddNode("node1")
	ring.AddNode("node2")
	ring.AddNode("node3")

	manager := NewVNodeManager(ring)

	// Test token ranges
	ranges := manager.GetTokenRanges()
	if len(ranges) != 30 {
		t.Errorf("Expected 30 ranges, got %d", len(ranges))
	}

	// Test load distribution
	distribution := manager.CalculateLoadDistribution()
	if len(distribution) != 3 {
		t.Errorf("Expected 3 nodes in distribution, got %d", len(distribution))
	}

	// Total should be close to 100%
	total := 0.0
	for _, load := range distribution {
		total += load
	}
	if total < 99.9 || total > 100.1 {
		t.Errorf("Load distribution should sum to 100%%, got %.2f%%", total)
	}

	// Test successor nodes
	successors := manager.GetSuccessorNodes("node1", 2)
	if len(successors) != 2 {
		t.Errorf("Expected 2 successors, got %d", len(successors))
	}
	for _, s := range successors {
		if s == "node1" {
			t.Error("Successor should not include self")
		}
	}
}
