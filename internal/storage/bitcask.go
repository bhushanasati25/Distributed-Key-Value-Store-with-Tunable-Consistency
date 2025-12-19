package storage

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

const (
	// File format: CRC32(4) + Timestamp(8) + KeyLen(4) + ValueLen(4) + Deleted(1) + Key + Value
	headerSize    = 4 + 8 + 4 + 4 + 1 // 21 bytes
	dataFileName  = "data.db"
	hintFileName  = "hint.db"
)

// Bitcask implements the Bitcask storage model
// - All writes are appended to a single file
// - In-memory hash map stores key -> file offset
// - Reads are O(1) lookup + single disk seek
type Bitcask struct {
	mu        sync.RWMutex
	dataDir   string
	dataFile  *os.File
	writer    *bufio.Writer
	index     *Index
	position  int64
	closed    bool
	syncWrite bool

	// Statistics
	totalReads  uint64
	totalWrites uint64
}

// NewBitcask creates a new Bitcask storage engine
func NewBitcask(dataDir string, syncWrite bool) (*Bitcask, error) {
	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	dataPath := filepath.Join(dataDir, dataFileName)
	
	// Open or create the data file
	dataFile, err := os.OpenFile(dataPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open data file: %w", err)
	}

	bc := &Bitcask{
		dataDir:   dataDir,
		dataFile:  dataFile,
		writer:    bufio.NewWriterSize(dataFile, 64*1024), // 64KB buffer
		index:     NewIndex(),
		syncWrite: syncWrite,
	}

	// Get current file position
	pos, err := dataFile.Seek(0, io.SeekEnd)
	if err != nil {
		dataFile.Close()
		return nil, fmt.Errorf("failed to seek to end: %w", err)
	}
	bc.position = pos

	// Rebuild index from existing data
	if pos > 0 {
		if err := bc.rebuildIndex(); err != nil {
			dataFile.Close()
			return nil, fmt.Errorf("failed to rebuild index: %w", err)
		}
	}

	return bc, nil
}

// rebuildIndex reads the data file and rebuilds the in-memory index
func (bc *Bitcask) rebuildIndex() error {
	file, err := os.Open(filepath.Join(bc.dataDir, dataFileName))
	if err != nil {
		return err
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 64*1024)
	var offset int64

	for {
		entry, bytesRead, err := bc.readEntry(reader, offset)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error at offset %d: %w", offset, err)
		}

		if entry.IsDeleted {
			bc.index.Delete(entry.Key, entry.Timestamp)
		} else {
			bc.index.Put(entry.Key, offset, entry.Size, entry.Timestamp)
		}

		offset += int64(bytesRead)
	}

	return nil
}

// readEntry reads a single entry from the data file
func (bc *Bitcask) readEntry(reader io.Reader, offset int64) (*Entry, int, error) {
	header := make([]byte, headerSize)
	n, err := io.ReadFull(reader, header)
	if err != nil {
		return nil, n, err
	}

	storedCRC := binary.BigEndian.Uint32(header[0:4])
	timestamp := int64(binary.BigEndian.Uint64(header[4:12]))
	keyLen := binary.BigEndian.Uint32(header[12:16])
	valueLen := binary.BigEndian.Uint32(header[16:20])
	isDeleted := header[20] == 1

	// Read key
	key := make([]byte, keyLen)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, n, fmt.Errorf("failed to read key: %w", err)
	}

	// Read value
	value := make([]byte, valueLen)
	if _, err := io.ReadFull(reader, value); err != nil {
		return nil, n, fmt.Errorf("failed to read value: %w", err)
	}

	// Verify CRC
	data := append(header[4:], key...)
	data = append(data, value...)
	calculatedCRC := crc32.ChecksumIEEE(data)
	if storedCRC != calculatedCRC {
		return nil, n, ErrCorruptData
	}

	totalBytes := headerSize + int(keyLen) + int(valueLen)

	return &Entry{
		Key:       string(key),
		Value:     value,
		Timestamp: timestamp,
		IsDeleted: isDeleted,
		Offset:    offset,
		Size:      int32(valueLen),
	}, totalBytes, nil
}

// writeEntry writes an entry to the data file
func (bc *Bitcask) writeEntry(key string, value []byte, timestamp int64, isDeleted bool) (int64, error) {
	keyBytes := []byte(key)
	keyLen := uint32(len(keyBytes))
	valueLen := uint32(len(value))

	// Build header
	header := make([]byte, headerSize)
	binary.BigEndian.PutUint64(header[4:12], uint64(timestamp))
	binary.BigEndian.PutUint32(header[12:16], keyLen)
	binary.BigEndian.PutUint32(header[16:20], valueLen)
	if isDeleted {
		header[20] = 1
	}

	// Calculate CRC
	data := append(header[4:], keyBytes...)
	data = append(data, value...)
	crc := crc32.ChecksumIEEE(data)
	binary.BigEndian.PutUint32(header[0:4], crc)

	// Write to buffer
	offset := bc.position
	totalSize := len(header) + len(keyBytes) + len(value)

	if _, err := bc.writer.Write(header); err != nil {
		return 0, fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := bc.writer.Write(keyBytes); err != nil {
		return 0, fmt.Errorf("failed to write key: %w", err)
	}
	if _, err := bc.writer.Write(value); err != nil {
		return 0, fmt.Errorf("failed to write value: %w", err)
	}

	bc.position += int64(totalSize)

	// Sync if configured
	if bc.syncWrite {
		if err := bc.writer.Flush(); err != nil {
			return 0, fmt.Errorf("failed to flush: %w", err)
		}
		if err := bc.dataFile.Sync(); err != nil {
			return 0, fmt.Errorf("failed to sync: %w", err)
		}
	}

	return offset, nil
}

// Get retrieves a value by key
func (bc *Bitcask) Get(key string) ([]byte, int64, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if bc.closed {
		return nil, 0, ErrStorageClosed
	}

	atomic.AddUint64(&bc.totalReads, 1)

	entry, exists := bc.index.Get(key)
	if !exists {
		return nil, 0, ErrKeyNotFound
	}
	if entry.IsDeleted {
		return nil, 0, ErrKeyDeleted
	}

	// Flush any pending writes first
	if err := bc.writer.Flush(); err != nil {
		return nil, 0, fmt.Errorf("failed to flush: %w", err)
	}

	// Seek to the entry position
	if _, err := bc.dataFile.Seek(entry.Offset, io.SeekStart); err != nil {
		return nil, 0, fmt.Errorf("failed to seek: %w", err)
	}

	// Read the entry
	reader := bufio.NewReader(bc.dataFile)
	readEntry, _, err := bc.readEntry(reader, entry.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read entry: %w", err)
	}

	return readEntry.Value, readEntry.Timestamp, nil
}

// Put stores a key-value pair
func (bc *Bitcask) Put(key string, value []byte, timestamp int64) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.closed {
		return ErrStorageClosed
	}

	atomic.AddUint64(&bc.totalWrites, 1)

	offset, err := bc.writeEntry(key, value, timestamp, false)
	if err != nil {
		return err
	}

	bc.index.Put(key, offset, int32(len(value)), timestamp)
	return nil
}

// Delete marks a key as deleted
func (bc *Bitcask) Delete(key string, timestamp int64) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.closed {
		return ErrStorageClosed
	}

	atomic.AddUint64(&bc.totalWrites, 1)

	// Write a tombstone entry
	_, err := bc.writeEntry(key, nil, timestamp, true)
	if err != nil {
		return err
	}

	bc.index.Delete(key, timestamp)
	return nil
}

// Has checks if a key exists and is not deleted
func (bc *Bitcask) Has(key string) bool {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if bc.closed {
		return false
	}

	return bc.index.Has(key)
}

// Keys returns all active keys
func (bc *Bitcask) Keys() []string {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if bc.closed {
		return nil
	}

	return bc.index.Keys()
}

// Count returns the number of active keys
func (bc *Bitcask) Count() int64 {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.index.Count()
}

// Close closes the storage engine
func (bc *Bitcask) Close() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.closed {
		return nil
	}

	bc.closed = true

	if err := bc.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush: %w", err)
	}

	if err := bc.dataFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync: %w", err)
	}

	return bc.dataFile.Close()
}

// Sync forces a sync of all pending writes to disk
func (bc *Bitcask) Sync() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.closed {
		return ErrStorageClosed
	}

	if err := bc.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush: %w", err)
	}

	return bc.dataFile.Sync()
}

// Compact performs compaction to reclaim space from deleted entries
func (bc *Bitcask) Compact() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.closed {
		return ErrStorageClosed
	}

	// Flush current writes
	if err := bc.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush: %w", err)
	}

	// Create a new temporary file
	tempPath := filepath.Join(bc.dataDir, "data.db.tmp")
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	tempWriter := bufio.NewWriterSize(tempFile, 64*1024)
	newIndex := NewIndex()
	var newPosition int64

	// Copy only active entries
	for key, entry := range bc.index.All() {
		if entry.IsDeleted {
			continue
		}

		// Read the original value
		if _, err := bc.dataFile.Seek(entry.Offset, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek: %w", err)
		}

		reader := bufio.NewReader(bc.dataFile)
		readEntry, _, err := bc.readEntry(reader, entry.Offset)
		if err != nil {
			return fmt.Errorf("failed to read entry: %w", err)
		}

		// Write to new file
		keyBytes := []byte(key)
		keyLen := uint32(len(keyBytes))
		valueLen := uint32(len(readEntry.Value))

		header := make([]byte, headerSize)
		binary.BigEndian.PutUint64(header[4:12], uint64(readEntry.Timestamp))
		binary.BigEndian.PutUint32(header[12:16], keyLen)
		binary.BigEndian.PutUint32(header[16:20], valueLen)

		data := append(header[4:], keyBytes...)
		data = append(data, readEntry.Value...)
		crc := crc32.ChecksumIEEE(data)
		binary.BigEndian.PutUint32(header[0:4], crc)

		offset := newPosition
		totalSize := len(header) + len(keyBytes) + len(readEntry.Value)

		if _, err := tempWriter.Write(header); err != nil {
			return err
		}
		if _, err := tempWriter.Write(keyBytes); err != nil {
			return err
		}
		if _, err := tempWriter.Write(readEntry.Value); err != nil {
			return err
		}

		newIndex.Put(key, offset, int32(len(readEntry.Value)), readEntry.Timestamp)
		newPosition += int64(totalSize)
	}

	if err := tempWriter.Flush(); err != nil {
		return err
	}
	if err := tempFile.Sync(); err != nil {
		return err
	}
	tempFile.Close()

	// Replace old file with new one
	bc.dataFile.Close()

	oldPath := filepath.Join(bc.dataDir, dataFileName)
	if err := os.Rename(tempPath, oldPath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	// Reopen the data file
	bc.dataFile, err = os.OpenFile(oldPath, os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to reopen data file: %w", err)
	}

	bc.writer = bufio.NewWriterSize(bc.dataFile, 64*1024)
	bc.index = newIndex
	bc.position = newPosition

	return nil
}

// Stats returns storage statistics
func (bc *Bitcask) Stats() Stats {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	var dataSize int64
	if info, err := bc.dataFile.Stat(); err == nil {
		dataSize = info.Size()
	}

	return Stats{
		ActiveKeys:   bc.index.Count(),
		DeletedKeys:  bc.index.DeletedCount(),
		DataFileSize: dataSize,
		IndexSize:    int64(bc.index.Size()),
		TotalReads:   atomic.LoadUint64(&bc.totalReads),
		TotalWrites:  atomic.LoadUint64(&bc.totalWrites),
	}
}
