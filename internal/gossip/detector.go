package gossip

import (
	"log"
	"sync"
	"time"

	"github.com/mini-dynamo/mini-dynamo/pkg/types"
)

// FailureDetector monitors node health and detects failures
type FailureDetector struct {
	mu             sync.RWMutex
	membership     *MembershipList
	suspectTimeout time.Duration
	deadTimeout    time.Duration
	stopCh         chan struct{}
	wg             sync.WaitGroup
	onStateChange  func(nodeID string, oldState, newState types.NodeState)
}

// NewFailureDetector creates a new failure detector
func NewFailureDetector(
	membership *MembershipList,
	suspectTimeout time.Duration,
	deadTimeout time.Duration,
	onStateChange func(nodeID string, oldState, newState types.NodeState),
) *FailureDetector {
	return &FailureDetector{
		membership:     membership,
		suspectTimeout: suspectTimeout,
		deadTimeout:    deadTimeout,
		stopCh:         make(chan struct{}),
		onStateChange:  onStateChange,
	}
}

// Start begins the failure detection loop
func (fd *FailureDetector) Start() {
	fd.wg.Add(1)
	go fd.detectionLoop()
}

// Stop stops the failure detector
func (fd *FailureDetector) Stop() {
	close(fd.stopCh)
	fd.wg.Wait()
}

// detectionLoop runs periodically to check for failed nodes
func (fd *FailureDetector) detectionLoop() {
	defer fd.wg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-fd.stopCh:
			return
		case <-ticker.C:
			fd.checkMembers()
		}
	}
}

// checkMembers checks all members for state transitions
func (fd *FailureDetector) checkMembers() {
	members := fd.membership.GetAllMembers()
	now := time.Now()

	for _, member := range members {
		// Skip self
		if member.Node.ID == fd.membership.selfID {
			continue
		}

		elapsed := now.Sub(member.LastHeartbeat)
		oldState := member.Node.State

		switch member.Node.State {
		case types.NodeAlive:
			if elapsed > fd.suspectTimeout {
				fd.transitionTo(member.Node.ID, types.NodeSuspect)
				log.Printf("Node %s marked as SUSPECT (no heartbeat for %v)", member.Node.ID, elapsed)
			}

		case types.NodeSuspect:
			if elapsed > fd.deadTimeout {
				fd.transitionTo(member.Node.ID, types.NodeDead)
				log.Printf("Node %s marked as DEAD (no heartbeat for %v)", member.Node.ID, elapsed)
			}

		case types.NodeDead:
			// Dead nodes stay dead until explicitly revived
		}

		// Notify callback if state changed
		if oldState != member.Node.State && fd.onStateChange != nil {
			fd.onStateChange(member.Node.ID, oldState, member.Node.State)
		}
	}
}

// transitionTo changes a node's state
func (fd *FailureDetector) transitionTo(nodeID string, newState types.NodeState) {
	fd.membership.UpdateState(nodeID, newState)
}

// Revive marks a dead node as alive again
func (fd *FailureDetector) Revive(nodeID string) {
	member, exists := fd.membership.GetMember(nodeID)
	if !exists {
		return
	}

	oldState := member.Node.State
	if oldState == types.NodeDead || oldState == types.NodeSuspect {
		fd.membership.UpdateState(nodeID, types.NodeAlive)
		fd.membership.RecordHeartbeat(nodeID)
		log.Printf("Node %s revived from %s to ALIVE", nodeID, oldState.String())

		if fd.onStateChange != nil {
			fd.onStateChange(nodeID, oldState, types.NodeAlive)
		}
	}
}

// RecordHeartbeat records a heartbeat from a node
func (fd *FailureDetector) RecordHeartbeat(nodeID string) {
	member, exists := fd.membership.GetMember(nodeID)
	if !exists {
		return
	}

	oldState := member.Node.State
	fd.membership.RecordHeartbeat(nodeID)

	// If node was suspect or dead, revive it
	if oldState != types.NodeAlive {
		fd.Revive(nodeID)
	}
}

// GetNodeState returns the current state of a node
func (fd *FailureDetector) GetNodeState(nodeID string) types.NodeState {
	member, exists := fd.membership.GetMember(nodeID)
	if !exists {
		return types.NodeDead
	}
	return member.Node.State
}

// GetSuspectNodes returns all nodes in suspect state
func (fd *FailureDetector) GetSuspectNodes() []string {
	members := fd.membership.GetAllMembers()
	suspects := make([]string, 0)

	for _, m := range members {
		if m.Node.State == types.NodeSuspect {
			suspects = append(suspects, m.Node.ID)
		}
	}
	return suspects
}

// GetDeadNodes returns all nodes in dead state
func (fd *FailureDetector) GetDeadNodes() []string {
	members := fd.membership.GetAllMembers()
	dead := make([]string, 0)

	for _, m := range members {
		if m.Node.State == types.NodeDead {
			dead = append(dead, m.Node.ID)
		}
	}
	return dead
}
