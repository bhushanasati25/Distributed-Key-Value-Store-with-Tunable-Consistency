package versioning

import (
	"github.com/mini-dynamo/mini-dynamo/pkg/types"
)

// Resolver handles conflict resolution between versions
type Resolver struct {
	strategy ResolveStrategy
}

// ResolveStrategy defines how to resolve conflicts
type ResolveStrategy int

const (
	// LastWriteWins uses timestamps to resolve conflicts
	LastWriteWins ResolveStrategy = iota
	// VectorClockBased uses vector clocks for causality
	VectorClockBased
)

// NewResolver creates a new conflict resolver
func NewResolver(strategy ResolveStrategy) *Resolver {
	return &Resolver{strategy: strategy}
}

// Resolve picks the winning version from multiple conflicting entries
func (r *Resolver) Resolve(entries []types.KeyValueEntry) types.KeyValueEntry {
	if len(entries) == 0 {
		return types.KeyValueEntry{}
	}
	if len(entries) == 1 {
		return entries[0]
	}

	switch r.strategy {
	case LastWriteWins:
		return r.resolveByTimestamp(entries)
	case VectorClockBased:
		return r.resolveByVectorClock(entries)
	default:
		return r.resolveByTimestamp(entries)
	}
}

// resolveByTimestamp picks the entry with the highest timestamp
func (r *Resolver) resolveByTimestamp(entries []types.KeyValueEntry) types.KeyValueEntry {
	winner := entries[0]
	for _, e := range entries[1:] {
		if e.Timestamp > winner.Timestamp {
			winner = e
		}
	}
	return winner
}

// resolveByVectorClock picks the entry based on vector clock comparison
func (r *Resolver) resolveByVectorClock(entries []types.KeyValueEntry) types.KeyValueEntry {
	// Find entries that are not dominated by any other
	candidates := make([]types.KeyValueEntry, 0)

	for i, e1 := range entries {
		dominated := false
		for j, e2 := range entries {
			if i != j && e1.Version.Compare(e2.Version) < 0 {
				// e1 < e2, so e1 is dominated
				dominated = true
				break
			}
		}
		if !dominated {
			candidates = append(candidates, e1)
		}
	}

	// If only one candidate, it's the winner
	if len(candidates) == 1 {
		return candidates[0]
	}

	// Multiple concurrent versions - use timestamp as tiebreaker
	return r.resolveByTimestamp(candidates)
}

// IsConcurrent checks if two entries are concurrent (neither dominates)
func (r *Resolver) IsConcurrent(a, b types.KeyValueEntry) bool {
	cmp := a.Version.Compare(b.Version)
	return cmp == 0 && a.Timestamp != b.Timestamp
}

// Merge combines multiple concurrent versions
func (r *Resolver) Merge(entries []types.KeyValueEntry, nodeID string) types.KeyValueEntry {
	if len(entries) == 0 {
		return types.KeyValueEntry{}
	}

	// Take the winner
	winner := r.Resolve(entries)

	// Merge all vector clocks
	mergedClock := make(types.VectorClock)
	for _, e := range entries {
		mergedClock = mergedClock.Merge(e.Version)
	}

	// Increment for this node
	mergedClock.Increment(nodeID)

	winner.Version = mergedClock
	return winner
}
