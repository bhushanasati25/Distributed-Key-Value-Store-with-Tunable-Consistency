package replication

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/mini-dynamo/mini-dynamo/pkg/types"
)

// HintedHandoffStore stores data for failed nodes
type HintedHandoffStore struct {
	mu      sync.RWMutex
	hints   map[string][]types.HintedHandoff // targetNodeID -> hints
	maxAge  time.Duration                    // Maximum age for hints
	maxSize int                              // Maximum hints per target
}

// NewHintedHandoffStore creates a new hinted handoff store
func NewHintedHandoffStore(maxAge time.Duration, maxSize int) *HintedHandoffStore {
	store := &HintedHandoffStore{
		hints:   make(map[string][]types.HintedHandoff),
		maxAge:  maxAge,
		maxSize: maxSize,
	}
	
	// Start cleanup goroutine
	go store.cleanupLoop()
	
	return store
}

// Store adds a hinted handoff entry
func (s *HintedHandoffStore) Store(targetNode string, entry types.KeyValueEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hint := types.HintedHandoff{
		TargetNode: targetNode,
		Entry:      entry,
		CreatedAt:  time.Now(),
		Attempts:   0,
	}

	hints := s.hints[targetNode]
	
	// Limit hints per target
	if len(hints) >= s.maxSize {
		// Remove oldest
		hints = hints[1:]
	}
	
	s.hints[targetNode] = append(hints, hint)
	log.Printf("Stored hint for node %s, key: %s", targetNode, entry.Key)
}

// GetHints returns all hints for a target node
func (s *HintedHandoffStore) GetHints(targetNode string) []types.HintedHandoff {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hints, exists := s.hints[targetNode]
	if !exists {
		return nil
	}

	// Return a copy
	result := make([]types.HintedHandoff, len(hints))
	copy(result, hints)
	return result
}

// RemoveHint removes a hint after successful delivery
func (s *HintedHandoffStore) RemoveHint(targetNode string, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hints := s.hints[targetNode]
	newHints := make([]types.HintedHandoff, 0, len(hints))
	
	for _, h := range hints {
		if h.Entry.Key != key {
			newHints = append(newHints, h)
		}
	}
	
	if len(newHints) == 0 {
		delete(s.hints, targetNode)
	} else {
		s.hints[targetNode] = newHints
	}
}

// IncrementAttempts increments the attempt counter for a hint
func (s *HintedHandoffStore) IncrementAttempts(targetNode string, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hints := s.hints[targetNode]
	for i := range hints {
		if hints[i].Entry.Key == key {
			hints[i].Attempts++
			break
		}
	}
}

// GetAllTargets returns all nodes with pending hints
func (s *HintedHandoffStore) GetAllTargets() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	targets := make([]string, 0, len(s.hints))
	for target := range s.hints {
		targets = append(targets, target)
	}
	return targets
}

// Count returns the total number of pending hints
func (s *HintedHandoffStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, hints := range s.hints {
		count += len(hints)
	}
	return count
}

// cleanupLoop removes expired hints periodically
func (s *HintedHandoffStore) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.cleanup()
	}
}

// cleanup removes expired hints
func (s *HintedHandoffStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for target, hints := range s.hints {
		newHints := make([]types.HintedHandoff, 0, len(hints))
		for _, h := range hints {
			if now.Sub(h.CreatedAt) < s.maxAge {
				newHints = append(newHints, h)
			}
		}
		if len(newHints) == 0 {
			delete(s.hints, target)
		} else {
			s.hints[target] = newHints
		}
	}
}

// HandoffManager manages the delivery of hinted handoffs
type HandoffManager struct {
	store       *HintedHandoffStore
	coordinator *Coordinator
	interval    time.Duration
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// NewHandoffManager creates a new handoff manager
func NewHandoffManager(store *HintedHandoffStore, coord *Coordinator, interval time.Duration) *HandoffManager {
	return &HandoffManager{
		store:       store,
		coordinator: coord,
		interval:    interval,
		stopCh:      make(chan struct{}),
	}
}

// Start starts the handoff delivery loop
func (m *HandoffManager) Start() {
	m.wg.Add(1)
	go m.deliveryLoop()
}

// Stop stops the handoff delivery loop
func (m *HandoffManager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// deliveryLoop periodically attempts to deliver pending handoffs
func (m *HandoffManager) deliveryLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.deliverPendingHandoffs()
		}
	}
}

// deliverPendingHandoffs attempts to deliver all pending handoffs
func (m *HandoffManager) deliverPendingHandoffs() {
	targets := m.store.GetAllTargets()
	
	for _, targetNode := range targets {
		// Check if node is alive
		nodes := m.coordinator.GetClusterNodes()
		nodeAlive := false
		for _, node := range nodes {
			if node.ID == targetNode && node.State == types.NodeAlive {
				nodeAlive = true
				break
			}
		}
		
		if !nodeAlive {
			continue
		}
		
		// Deliver hints to this node
		hints := m.store.GetHints(targetNode)
		for _, hint := range hints {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			success := m.coordinator.sendReplication(ctx, targetNode, hint.Entry)
			cancel()
			
			if success {
				m.store.RemoveHint(targetNode, hint.Entry.Key)
				log.Printf("Successfully delivered hint to %s, key: %s", targetNode, hint.Entry.Key)
			} else {
				m.store.IncrementAttempts(targetNode, hint.Entry.Key)
				if hint.Attempts > 10 {
					m.store.RemoveHint(targetNode, hint.Entry.Key)
					log.Printf("Gave up on hint delivery to %s, key: %s", targetNode, hint.Entry.Key)
				}
			}
		}
	}
}
