package versioning

import (
	"sync"

	"github.com/mini-dynamo/mini-dynamo/pkg/types"
)

// VectorClockManager manages vector clocks for the local node
type VectorClockManager struct {
	mu     sync.RWMutex
	nodeID string
	clocks map[string]types.VectorClock // key -> vector clock
}

// NewVectorClockManager creates a new vector clock manager
func NewVectorClockManager(nodeID string) *VectorClockManager {
	return &VectorClockManager{
		nodeID: nodeID,
		clocks: make(map[string]types.VectorClock),
	}
}

// Get returns the vector clock for a key
func (m *VectorClockManager) Get(key string) types.VectorClock {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if vc, exists := m.clocks[key]; exists {
		return vc.Copy()
	}
	return make(types.VectorClock)
}

// Increment increments the vector clock for a key and returns the new clock
func (m *VectorClockManager) Increment(key string) types.VectorClock {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.clocks[key]; !exists {
		m.clocks[key] = make(types.VectorClock)
	}

	m.clocks[key].Increment(m.nodeID)
	return m.clocks[key].Copy()
}

// Update updates the vector clock for a key by merging with another clock
func (m *VectorClockManager) Update(key string, other types.VectorClock) types.VectorClock {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.clocks[key]; !exists {
		m.clocks[key] = make(types.VectorClock)
	}

	m.clocks[key] = m.clocks[key].Merge(other)
	return m.clocks[key].Copy()
}

// Set sets the vector clock for a key
func (m *VectorClockManager) Set(key string, vc types.VectorClock) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clocks[key] = vc.Copy()
}

// Delete removes the vector clock for a key
func (m *VectorClockManager) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.clocks, key)
}

// Compare compares two vector clocks
// Returns -1 if a < b, 0 if concurrent, 1 if a > b
func Compare(a, b types.VectorClock) int {
	return a.Compare(b)
}

// HappensBefore returns true if a happens before b
func HappensBefore(a, b types.VectorClock) bool {
	return a.Compare(b) < 0
}

// HappensAfter returns true if a happens after b
func HappensAfter(a, b types.VectorClock) bool {
	return a.Compare(b) > 0
}

// AreConcurrent returns true if neither clock dominates the other
func AreConcurrent(a, b types.VectorClock) bool {
	return a.Compare(b) == 0
}
