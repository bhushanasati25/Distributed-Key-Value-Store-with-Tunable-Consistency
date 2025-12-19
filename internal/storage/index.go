package storage

import (
	"sync"
)

// IndexEntry stores the location of a value in the data file
type IndexEntry struct {
	Offset    int64 // Position in the data file
	Size      int32 // Size of the value in bytes
	Timestamp int64 // Unix timestamp of the write
	IsDeleted bool  // Tombstone marker
}

// Index is a thread-safe in-memory hash map for key lookups
type Index struct {
	mu      sync.RWMutex
	entries map[string]*IndexEntry
	stats   struct {
		active  int64
		deleted int64
	}
}

// NewIndex creates a new in-memory index
func NewIndex() *Index {
	return &Index{
		entries: make(map[string]*IndexEntry),
	}
}

// Get retrieves an index entry by key
func (idx *Index) Get(key string) (*IndexEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	
	entry, exists := idx.entries[key]
	if !exists {
		return nil, false
	}
	
	// Return a copy to prevent external modification
	return &IndexEntry{
		Offset:    entry.Offset,
		Size:      entry.Size,
		Timestamp: entry.Timestamp,
		IsDeleted: entry.IsDeleted,
	}, true
}

// Put adds or updates an index entry
func (idx *Index) Put(key string, offset int64, size int32, timestamp int64) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	existing, exists := idx.entries[key]
	if exists && existing.IsDeleted {
		idx.stats.deleted--
	} else if !exists {
		idx.stats.active++
	}
	
	idx.entries[key] = &IndexEntry{
		Offset:    offset,
		Size:      size,
		Timestamp: timestamp,
		IsDeleted: false,
	}
}

// Delete marks a key as deleted in the index
func (idx *Index) Delete(key string, timestamp int64) bool {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	entry, exists := idx.entries[key]
	if !exists {
		// Create a tombstone entry
		idx.entries[key] = &IndexEntry{
			Timestamp: timestamp,
			IsDeleted: true,
		}
		idx.stats.deleted++
		return false
	}
	
	if !entry.IsDeleted {
		entry.IsDeleted = true
		entry.Timestamp = timestamp
		idx.stats.active--
		idx.stats.deleted++
		return true
	}
	
	return false
}

// Has checks if a key exists and is not deleted
func (idx *Index) Has(key string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	
	entry, exists := idx.entries[key]
	return exists && !entry.IsDeleted
}

// Keys returns all active (non-deleted) keys
func (idx *Index) Keys() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	
	keys := make([]string, 0, idx.stats.active)
	for key, entry := range idx.entries {
		if !entry.IsDeleted {
			keys = append(keys, key)
		}
	}
	return keys
}

// Count returns the number of active keys
func (idx *Index) Count() int64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.stats.active
}

// DeletedCount returns the number of deleted keys (tombstones)
func (idx *Index) DeletedCount() int64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.stats.deleted
}

// Clear removes all entries from the index
func (idx *Index) Clear() {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	idx.entries = make(map[string]*IndexEntry)
	idx.stats.active = 0
	idx.stats.deleted = 0
}

// Size returns the total number of entries (including deleted)
func (idx *Index) Size() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.entries)
}

// All returns all entries (for compaction purposes)
func (idx *Index) All() map[string]*IndexEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	
	result := make(map[string]*IndexEntry, len(idx.entries))
	for k, v := range idx.entries {
		result[k] = &IndexEntry{
			Offset:    v.Offset,
			Size:      v.Size,
			Timestamp: v.Timestamp,
			IsDeleted: v.IsDeleted,
		}
	}
	return result
}

// UpdateOffset updates the offset for a key (used during compaction)
func (idx *Index) UpdateOffset(key string, newOffset int64, newSize int32) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	
	if entry, exists := idx.entries[key]; exists {
		entry.Offset = newOffset
		entry.Size = newSize
	}
}
