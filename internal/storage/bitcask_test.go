package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBitcaskBasicOperations(t *testing.T) {
	// Create temp directory
	dir := t.TempDir()

	// Create storage
	bc, err := NewBitcask(dir, false)
	if err != nil {
		t.Fatalf("Failed to create Bitcask: %v", err)
	}
	defer bc.Close()

	// Test Put
	timestamp := time.Now().UnixNano()
	err = bc.Put("key1", []byte("value1"), timestamp)
	if err != nil {
		t.Fatalf("Failed to put: %v", err)
	}

	// Test Get
	value, ts, err := bc.Get("key1")
	if err != nil {
		t.Fatalf("Failed to get: %v", err)
	}
	if string(value) != "value1" {
		t.Errorf("Expected 'value1', got '%s'", string(value))
	}
	if ts != timestamp {
		t.Errorf("Timestamp mismatch")
	}

	// Test Has
	if !bc.Has("key1") {
		t.Error("Has() should return true for existing key")
	}
	if bc.Has("nonexistent") {
		t.Error("Has() should return false for non-existing key")
	}

	// Test Count
	if bc.Count() != 1 {
		t.Errorf("Expected count 1, got %d", bc.Count())
	}
}

func TestBitcaskDelete(t *testing.T) {
	dir := t.TempDir()

	bc, err := NewBitcask(dir, false)
	if err != nil {
		t.Fatalf("Failed to create Bitcask: %v", err)
	}
	defer bc.Close()

	// Put a key
	bc.Put("key1", []byte("value1"), time.Now().UnixNano())

	// Delete it
	err = bc.Delete("key1", time.Now().UnixNano())
	if err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	// Verify it's deleted
	_, _, err = bc.Get("key1")
	if err != ErrKeyDeleted && err != ErrKeyNotFound {
		t.Errorf("Expected key deleted error, got %v", err)
	}

	if bc.Has("key1") {
		t.Error("Has() should return false for deleted key")
	}
}

func TestBitcaskPersistence(t *testing.T) {
	dir := t.TempDir()

	// Create and populate storage
	bc, err := NewBitcask(dir, true) // sync writes
	if err != nil {
		t.Fatalf("Failed to create Bitcask: %v", err)
	}

	bc.Put("key1", []byte("value1"), time.Now().UnixNano())
	bc.Put("key2", []byte("value2"), time.Now().UnixNano())
	bc.Put("key3", []byte("value3"), time.Now().UnixNano())
	bc.Delete("key2", time.Now().UnixNano())
	bc.Close()

	// Reopen and verify
	bc2, err := NewBitcask(dir, false)
	if err != nil {
		t.Fatalf("Failed to reopen Bitcask: %v", err)
	}
	defer bc2.Close()

	// key1 should exist
	value, _, err := bc2.Get("key1")
	if err != nil || string(value) != "value1" {
		t.Errorf("key1 not recovered properly")
	}

	// key2 should be deleted
	if bc2.Has("key2") {
		t.Error("key2 should be deleted")
	}

	// key3 should exist
	value, _, err = bc2.Get("key3")
	if err != nil || string(value) != "value3" {
		t.Errorf("key3 not recovered properly")
	}

	// Count should be 2
	if bc2.Count() != 2 {
		t.Errorf("Expected count 2, got %d", bc2.Count())
	}
}

func TestBitcaskCompaction(t *testing.T) {
	dir := t.TempDir()

	bc, err := NewBitcask(dir, false)
	if err != nil {
		t.Fatalf("Failed to create Bitcask: %v", err)
	}
	defer bc.Close()

	// Write some data with updates
	for i := 0; i < 10; i++ {
		bc.Put("key1", []byte("value-update"), time.Now().UnixNano())
	}
	bc.Put("key2", []byte("value2"), time.Now().UnixNano())
	bc.Delete("key2", time.Now().UnixNano())
	bc.Put("key3", []byte("value3"), time.Now().UnixNano())

	// Get initial file size
	initialSize := getFileSize(filepath.Join(dir, "data.db"))

	// Run compaction
	err = bc.Compact()
	if err != nil {
		t.Fatalf("Compaction failed: %v", err)
	}

	// File should be smaller
	compactedSize := getFileSize(filepath.Join(dir, "data.db"))
	if compactedSize >= initialSize {
		t.Errorf("Compaction didn't reduce file size: %d >= %d", compactedSize, initialSize)
	}

	// Data should still be valid
	value, _, err := bc.Get("key1")
	if err != nil || string(value) != "value-update" {
		t.Error("key1 not valid after compaction")
	}

	if bc.Has("key2") {
		t.Error("key2 should still be deleted after compaction")
	}

	value, _, err = bc.Get("key3")
	if err != nil || string(value) != "value3" {
		t.Error("key3 not valid after compaction")
	}
}

func TestBitcaskKeys(t *testing.T) {
	dir := t.TempDir()

	bc, err := NewBitcask(dir, false)
	if err != nil {
		t.Fatalf("Failed to create Bitcask: %v", err)
	}
	defer bc.Close()

	bc.Put("alpha", []byte("1"), time.Now().UnixNano())
	bc.Put("beta", []byte("2"), time.Now().UnixNano())
	bc.Put("gamma", []byte("3"), time.Now().UnixNano())
	bc.Delete("beta", time.Now().UnixNano())

	keys := bc.Keys()
	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}

	// Check keys contain alpha and gamma
	hasAlpha, hasGamma := false, false
	for _, k := range keys {
		if k == "alpha" {
			hasAlpha = true
		}
		if k == "gamma" {
			hasGamma = true
		}
	}
	if !hasAlpha || !hasGamma {
		t.Error("Expected keys alpha and gamma")
	}
}

func TestBitcaskStats(t *testing.T) {
	dir := t.TempDir()

	bc, err := NewBitcask(dir, false)
	if err != nil {
		t.Fatalf("Failed to create Bitcask: %v", err)
	}
	defer bc.Close()

	bc.Put("key1", []byte("value1"), time.Now().UnixNano())
	bc.Put("key2", []byte("value2"), time.Now().UnixNano())
	bc.Get("key1")
	bc.Delete("key2", time.Now().UnixNano())

	stats := bc.Stats()
	if stats.ActiveKeys != 1 {
		t.Errorf("Expected 1 active key, got %d", stats.ActiveKeys)
	}
	if stats.DeletedKeys != 1 {
		t.Errorf("Expected 1 deleted key, got %d", stats.DeletedKeys)
	}
	if stats.TotalWrites != 3 {
		t.Errorf("Expected 3 writes, got %d", stats.TotalWrites)
	}
	if stats.TotalReads != 1 {
		t.Errorf("Expected 1 read, got %d", stats.TotalReads)
	}
}

func getFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
