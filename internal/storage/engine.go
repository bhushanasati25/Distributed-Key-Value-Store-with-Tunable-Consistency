package storage

import "errors"

// Common errors
var (
	ErrKeyNotFound  = errors.New("key not found")
	ErrKeyDeleted   = errors.New("key has been deleted")
	ErrStorageFull  = errors.New("storage is full")
	ErrCorruptData  = errors.New("data corruption detected")
	ErrStorageClosed = errors.New("storage engine is closed")
)

// Engine defines the interface for the storage backend
type Engine interface {
	// Get retrieves a value by key
	// Returns ErrKeyNotFound if the key doesn't exist
	Get(key string) ([]byte, int64, error)

	// Put stores a key-value pair with a timestamp
	// Returns the offset where the data was written
	Put(key string, value []byte, timestamp int64) error

	// Delete marks a key as deleted (tombstone)
	Delete(key string, timestamp int64) error

	// Has checks if a key exists and is not deleted
	Has(key string) bool

	// Keys returns all active keys
	Keys() []string

	// Count returns the number of active keys
	Count() int64

	// Close closes the storage engine
	Close() error

	// Sync forces a sync of all pending writes to disk
	Sync() error

	// Compact performs compaction to reclaim space
	Compact() error

	// Stats returns storage statistics
	Stats() Stats
}

// Stats contains storage engine statistics
type Stats struct {
	ActiveKeys    int64  `json:"active_keys"`
	DeletedKeys   int64  `json:"deleted_keys"`
	DataFileSize  int64  `json:"data_file_size"`
	IndexSize     int64  `json:"index_size"`
	TotalReads    uint64 `json:"total_reads"`
	TotalWrites   uint64 `json:"total_writes"`
}

// Entry represents a single entry in the storage
type Entry struct {
	Key       string
	Value     []byte
	Timestamp int64
	IsDeleted bool
	Offset    int64
	Size      int32
}
