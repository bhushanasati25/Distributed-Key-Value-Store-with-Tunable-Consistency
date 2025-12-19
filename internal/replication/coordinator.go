package replication

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/mini-dynamo/mini-dynamo/internal/config"
	"github.com/mini-dynamo/mini-dynamo/internal/ring"
	"github.com/mini-dynamo/mini-dynamo/internal/storage"
	"github.com/mini-dynamo/mini-dynamo/pkg/types"
)

// Coordinator handles distributed read/write operations
type Coordinator struct {
	config     *config.Config
	ring       *ring.HashRing
	storage    storage.Engine
	httpClient *http.Client
	nodes      map[string]*types.Node
	nodesMu    sync.RWMutex
}

// NewCoordinator creates a new coordinator
func NewCoordinator(cfg *config.Config, hashRing *ring.HashRing, store storage.Engine) *Coordinator {
	return &Coordinator{
		config:  cfg,
		ring:    hashRing,
		storage: store,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
		nodes: make(map[string]*types.Node),
	}
}

// RegisterNode adds a node to the coordinator's knowledge
func (c *Coordinator) RegisterNode(node *types.Node) {
	c.nodesMu.Lock()
	defer c.nodesMu.Unlock()

	c.nodes[node.ID] = node
	c.ring.AddNode(node.ID)
}

// UnregisterNode removes a node from the coordinator
func (c *Coordinator) UnregisterNode(nodeID string) {
	c.nodesMu.Lock()
	defer c.nodesMu.Unlock()

	delete(c.nodes, nodeID)
	c.ring.RemoveNode(nodeID)
}

// GetClusterNodes returns all known nodes
func (c *Coordinator) GetClusterNodes() []*types.Node {
	c.nodesMu.RLock()
	defer c.nodesMu.RUnlock()

	nodes := make([]*types.Node, 0, len(c.nodes))
	for _, node := range c.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// GetRingTokens returns the hash ring tokens
func (c *Coordinator) GetRingTokens() []ring.VNode {
	return c.ring.GetRingTokens()
}

// Put stores a key-value pair with quorum writes
func (c *Coordinator) Put(ctx context.Context, key string, value []byte, consistency types.ConsistencyLevel) error {
	timestamp := time.Now().UnixNano()

	// Get preference list (N nodes for this key)
	preferenceList, err := c.ring.GetNodes(key, c.config.ReplicationFactor)
	if err != nil {
		return fmt.Errorf("failed to get preference list: %w", err)
	}

	// Determine required acks based on consistency level
	requiredAcks := c.getWriteQuorum(consistency)

	entry := types.KeyValueEntry{
		Key:       key,
		Value:     value,
		Timestamp: timestamp,
	}

	// Write to all nodes in parallel
	results := c.replicateToNodes(ctx, preferenceList, entry) 

	// Count successes
	successCount := 0
	for _, success := range results {
		if success {
			successCount++
		}
	}

	if successCount < requiredAcks {
		return fmt.Errorf("quorum not met: got %d acks, needed %d", successCount, requiredAcks)
	}

	return nil
}

// Get retrieves a value with quorum reads
func (c *Coordinator) Get(ctx context.Context, key string, consistency types.ConsistencyLevel) ([]byte, int64, error) {
	// Get preference list
	preferenceList, err := c.ring.GetNodes(key, c.config.ReplicationFactor)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get preference list: %w", err)
	}

	// Determine required reads based on consistency level
	requiredReads := c.getReadQuorum(consistency)

	// Read from nodes in parallel
	responses := c.readFromNodes(ctx, preferenceList, key)

	// Collect successful responses
	successfulResponses := make([]types.KeyValueEntry, 0)
	for _, resp := range responses {
		if resp != nil {
			successfulResponses = append(successfulResponses, *resp)
		}
	}

	if len(successfulResponses) < requiredReads {
		return nil, 0, fmt.Errorf("key not found")
	}

	// Find the most recent value (Last Write Wins)
	sort.Slice(successfulResponses, func(i, j int) bool {
		return successfulResponses[i].Timestamp > successfulResponses[j].Timestamp
	})

	latest := successfulResponses[0]

	// Perform read repair if there are stale copies
	go c.readRepair(context.Background(), preferenceList, latest)

	return latest.Value, latest.Timestamp, nil
}

// replicateToNodes sends write requests to multiple nodes
func (c *Coordinator) replicateToNodes(ctx context.Context, nodes []string, entry types.KeyValueEntry) map[string]bool {
	results := make(map[string]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, nodeID := range nodes {
		wg.Add(1)
		go func(nodeID string) {
			defer wg.Done()

			success := false

			// Check if it's the local node
			if nodeID == c.config.NodeID {
				err := c.storage.Put(entry.Key, entry.Value, entry.Timestamp)
				success = err == nil
			} else {
				success = c.sendReplication(ctx, nodeID, entry)
			}

			mu.Lock()
			results[nodeID] = success
			mu.Unlock()
		}(nodeID)
	}

	wg.Wait()
	return results
}

// readFromNodes reads from multiple nodes in parallel
func (c *Coordinator) readFromNodes(ctx context.Context, nodes []string, key string) []*types.KeyValueEntry {
	responses := make([]*types.KeyValueEntry, len(nodes))
	var wg sync.WaitGroup

	for i, nodeID := range nodes {
		wg.Add(1)
		go func(idx int, nodeID string) {
			defer wg.Done()

			var entry *types.KeyValueEntry

			// Check if it's the local node
			if nodeID == c.config.NodeID {
				value, timestamp, err := c.storage.Get(key)
				if err == nil {
					entry = &types.KeyValueEntry{
						Key:       key,
						Value:     value,
						Timestamp: timestamp,
					}
				}
			} else {
				entry = c.fetchFromNode(ctx, nodeID, key)
			}

			responses[idx] = entry
		}(i, nodeID)
	}

	wg.Wait()
	return responses
}

// sendReplication sends a replication request to a remote node
func (c *Coordinator) sendReplication(ctx context.Context, nodeID string, entry types.KeyValueEntry) bool {
	c.nodesMu.RLock()
	node, exists := c.nodes[nodeID]
	c.nodesMu.RUnlock()

	if !exists {
		log.Printf("Node %s not found for replication", nodeID)
		return false
	}

	url := fmt.Sprintf("http://%s:%d/internal/replicate", node.Address, node.Port)

	req := types.ReplicationRequest{
		Entry:    entry,
		FromNode: c.config.NodeID,
	}

	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return false
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		log.Printf("Failed to replicate to %s: %v", nodeID, err)
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// fetchFromNode fetches a key from a remote node
func (c *Coordinator) fetchFromNode(ctx context.Context, nodeID string, key string) *types.KeyValueEntry {
	c.nodesMu.RLock()
	node, exists := c.nodes[nodeID]
	c.nodesMu.RUnlock()

	if !exists {
		return nil
	}

	url := fmt.Sprintf("http://%s:%d/internal/read?key=%s", node.Address, node.Port, key)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var entry types.KeyValueEntry
	if err := json.Unmarshal(body, &entry); err != nil {
		return nil
	}

	return &entry
}

// readRepair updates stale nodes with the latest value
func (c *Coordinator) readRepair(ctx context.Context, nodes []string, latest types.KeyValueEntry) {
	for _, nodeID := range nodes {
		if nodeID == c.config.NodeID {
			// Update local storage if needed
			value, timestamp, err := c.storage.Get(latest.Key)
			if err != nil || timestamp < latest.Timestamp {
				c.storage.Put(latest.Key, latest.Value, latest.Timestamp)
			} else if timestamp == latest.Timestamp && !bytes.Equal(value, latest.Value) {
				c.storage.Put(latest.Key, latest.Value, latest.Timestamp)
			}
		} else {
			// Send repair to remote node
			c.sendReplication(ctx, nodeID, latest)
		}
	}
}

// getWriteQuorum returns the number of write acks needed
func (c *Coordinator) getWriteQuorum(consistency types.ConsistencyLevel) int {
	switch consistency {
	case types.ConsistencyOne:
		return 1
	case types.ConsistencyAll:
		return c.config.ReplicationFactor
	default: // quorum
		return c.config.WriteQuorum
	}
}

// getReadQuorum returns the number of read responses needed
func (c *Coordinator) getReadQuorum(consistency types.ConsistencyLevel) int {
	switch consistency {
	case types.ConsistencyOne:
		return 1
	case types.ConsistencyAll:
		return c.config.ReplicationFactor
	default: // quorum
		return c.config.ReadQuorum
	}
}
