package replication

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mini-dynamo/mini-dynamo/pkg/types"
)

// QuorumResult represents the result of a quorum operation
type QuorumResult struct {
	Success      bool
	Value        []byte
	Timestamp    int64
	SuccessCount int
	ErrorCount   int
	Errors       []error
}

// QuorumManager handles quorum-based operations
type QuorumManager struct {
	replicationFactor int
	readQuorum        int
	writeQuorum       int
	timeout           time.Duration
}

// NewQuorumManager creates a new quorum manager
func NewQuorumManager(n, r, w int, timeout time.Duration) *QuorumManager {
	return &QuorumManager{
		replicationFactor: n,
		readQuorum:        r,
		writeQuorum:       w,
		timeout:           timeout,
	}
}

// WriteOperation represents a write to be performed
type WriteOperation struct {
	NodeID    string
	Execute   func(ctx context.Context) error
}

// ReadOperation represents a read to be performed
type ReadOperation struct {
	NodeID    string
	Execute   func(ctx context.Context) (*types.KeyValueEntry, error)
}

// ExecuteWrite executes a write operation with quorum
func (qm *QuorumManager) ExecuteWrite(ctx context.Context, ops []WriteOperation, consistency types.ConsistencyLevel) *QuorumResult {
	required := qm.getWriteRequirement(consistency)
	
	ctx, cancel := context.WithTimeout(ctx, qm.timeout)
	defer cancel()

	result := &QuorumResult{
		Errors: make([]error, 0),
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	done := make(chan struct{})

	for _, op := range ops {
		wg.Add(1)
		go func(operation WriteOperation) {
			defer wg.Done()

			err := operation.Execute(ctx)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.ErrorCount++
				result.Errors = append(result.Errors, fmt.Errorf("node %s: %w", operation.NodeID, err))
			} else {
				result.SuccessCount++
			}

			// Check if quorum is met
			if result.SuccessCount >= required {
				select {
				case <-done:
				default:
					close(done)
				}
			}
		}(op)
	}

	// Wait for either quorum or all operations to complete
	go func() {
		wg.Wait()
		select {
		case <-done:
		default:
			close(done)
		}
	}()

	<-done
	result.Success = result.SuccessCount >= required
	return result
}

// ExecuteRead executes a read operation with quorum
func (qm *QuorumManager) ExecuteRead(ctx context.Context, ops []ReadOperation, consistency types.ConsistencyLevel) (*QuorumResult, []types.KeyValueEntry) {
	required := qm.getReadRequirement(consistency)

	ctx, cancel := context.WithTimeout(ctx, qm.timeout)
	defer cancel()

	result := &QuorumResult{
		Errors: make([]error, 0),
	}
	entries := make([]types.KeyValueEntry, 0, len(ops))

	var mu sync.Mutex
	var wg sync.WaitGroup
	done := make(chan struct{})

	for _, op := range ops {
		wg.Add(1)
		go func(operation ReadOperation) {
			defer wg.Done()

			entry, err := operation.Execute(ctx)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.ErrorCount++
				result.Errors = append(result.Errors, fmt.Errorf("node %s: %w", operation.NodeID, err))
			} else if entry != nil {
				result.SuccessCount++
				entries = append(entries, *entry)
			}

			// Check if quorum is met
			if result.SuccessCount >= required {
				select {
				case <-done:
				default:
					close(done)
				}
			}
		}(op)
	}

	// Wait for either quorum or all operations to complete
	go func() {
		wg.Wait()
		select {
		case <-done:
		default:
			close(done)
		}
	}()

	<-done
	result.Success = result.SuccessCount >= required

	// Return the latest value
	if len(entries) > 0 {
		latest := entries[0]
		for _, e := range entries {
			if e.Timestamp > latest.Timestamp {
				latest = e
			}
		}
		result.Value = latest.Value
		result.Timestamp = latest.Timestamp
	}

	return result, entries
}

// getWriteRequirement returns the number of successful writes needed
func (qm *QuorumManager) getWriteRequirement(consistency types.ConsistencyLevel) int {
	switch consistency {
	case types.ConsistencyOne:
		return 1
	case types.ConsistencyAll:
		return qm.replicationFactor
	default:
		return qm.writeQuorum
	}
}

// getReadRequirement returns the number of successful reads needed
func (qm *QuorumManager) getReadRequirement(consistency types.ConsistencyLevel) int {
	switch consistency {
	case types.ConsistencyOne:
		return 1
	case types.ConsistencyAll:
		return qm.replicationFactor
	default:
		return qm.readQuorum
	}
}

// ValidateQuorum checks if W + R > N for strong consistency
func (qm *QuorumManager) ValidateQuorum() bool {
	return qm.writeQuorum+qm.readQuorum > qm.replicationFactor
}
