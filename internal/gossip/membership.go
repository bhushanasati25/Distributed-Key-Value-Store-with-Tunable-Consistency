package gossip

import (
	"sync"
	"time"

	"github.com/mini-dynamo/mini-dynamo/pkg/types"
)

// MemberInfo contains metadata about a cluster member
type MemberInfo struct {
	Node          *types.Node
	LastHeartbeat time.Time
	Version       uint64 // Incarnation number for conflict resolution
}

// MembershipList manages the cluster membership
type MembershipList struct {
	mu       sync.RWMutex
	members  map[string]*MemberInfo
	selfID   string
	version  uint64 // Local incarnation number
}

// NewMembershipList creates a new membership list
func NewMembershipList(selfID string) *MembershipList {
	ml := &MembershipList{
		members: make(map[string]*MemberInfo),
		selfID:  selfID,
		version: 1,
	}

	// Add self as first member
	ml.members[selfID] = &MemberInfo{
		Node: &types.Node{
			ID:    selfID,
			State: types.NodeAlive,
		},
		LastHeartbeat: time.Now(),
		Version:       1,
	}

	return ml
}

// AddMember adds or updates a member in the list
func (ml *MembershipList) AddMember(node *types.Node) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	existing, exists := ml.members[node.ID]
	if exists {
		// Update existing member
		existing.Node = node
		existing.LastHeartbeat = time.Now()
	} else {
		ml.members[node.ID] = &MemberInfo{
			Node:          node,
			LastHeartbeat: time.Now(),
			Version:       1,
		}
	}
}

// RemoveMember removes a member from the list
func (ml *MembershipList) RemoveMember(nodeID string) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	if nodeID != ml.selfID {
		delete(ml.members, nodeID)
	}
}

// GetMember returns a member by ID
func (ml *MembershipList) GetMember(nodeID string) (*MemberInfo, bool) {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	member, exists := ml.members[nodeID]
	if !exists {
		return nil, false
	}

	// Return a copy
	return &MemberInfo{
		Node:          member.Node,
		LastHeartbeat: member.LastHeartbeat,
		Version:       member.Version,
	}, true
}

// GetAllMembers returns all members
func (ml *MembershipList) GetAllMembers() []*MemberInfo {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	members := make([]*MemberInfo, 0, len(ml.members))
	for _, m := range ml.members {
		members = append(members, &MemberInfo{
			Node:          m.Node,
			LastHeartbeat: m.LastHeartbeat,
			Version:       m.Version,
		})
	}
	return members
}

// GetAliveMembers returns only alive members
func (ml *MembershipList) GetAliveMembers() []*MemberInfo {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	members := make([]*MemberInfo, 0)
	for _, m := range ml.members {
		if m.Node.State == types.NodeAlive {
			members = append(members, &MemberInfo{
				Node:          m.Node,
				LastHeartbeat: m.LastHeartbeat,
				Version:       m.Version,
			})
		}
	}
	return members
}

// UpdateState updates the state of a member
func (ml *MembershipList) UpdateState(nodeID string, state types.NodeState) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	if member, exists := ml.members[nodeID]; exists {
		member.Node.State = state
		if state == types.NodeAlive {
			member.LastHeartbeat = time.Now()
		}
	}
}

// RecordHeartbeat updates the heartbeat time for a member
func (ml *MembershipList) RecordHeartbeat(nodeID string) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	if member, exists := ml.members[nodeID]; exists {
		member.LastHeartbeat = time.Now()
		if member.Node.State != types.NodeAlive {
			member.Node.State = types.NodeAlive
		}
	}
}

// GetStaleMembers returns members without recent heartbeats
func (ml *MembershipList) GetStaleMembers(threshold time.Duration) []*MemberInfo {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	now := time.Now()
	stale := make([]*MemberInfo, 0)

	for _, m := range ml.members {
		if m.Node.ID != ml.selfID && now.Sub(m.LastHeartbeat) > threshold {
			stale = append(stale, &MemberInfo{
				Node:          m.Node,
				LastHeartbeat: m.LastHeartbeat,
				Version:       m.Version,
			})
		}
	}
	return stale
}

// Merge merges another membership list into this one
func (ml *MembershipList) Merge(other map[string]types.NodeInfo) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	for nodeID, info := range other {
		if nodeID == ml.selfID {
			continue // Don't update self from gossip
		}

		existing, exists := ml.members[nodeID]
		if !exists {
			// New member
			state := types.NodeAlive
			if info.State == "suspect" {
				state = types.NodeSuspect
			} else if info.State == "dead" {
				state = types.NodeDead
			}

			ml.members[nodeID] = &MemberInfo{
				Node: &types.Node{
					ID:       nodeID,
					Address:  info.Address,
					State:    state,
					LastSeen: info.LastSeen,
				},
				LastHeartbeat: info.LastSeen,
				Version:       1,
			}
		} else {
			// Update if newer
			if info.LastSeen.After(existing.LastHeartbeat) {
				existing.LastHeartbeat = info.LastSeen
				existing.Node.Address = info.Address
			}
		}
	}
}

// ToGossipFormat converts the membership list to gossip message format
func (ml *MembershipList) ToGossipFormat() map[string]types.NodeInfo {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	result := make(map[string]types.NodeInfo)
	for nodeID, m := range ml.members {
		result[nodeID] = types.NodeInfo{
			ID:       nodeID,
			Address:  m.Node.Address,
			State:    m.Node.State.String(),
			LastSeen: m.LastHeartbeat,
		}
	}
	return result
}

// Size returns the number of members
func (ml *MembershipList) Size() int {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	return len(ml.members)
}

// IncrementVersion increments the local incarnation number
func (ml *MembershipList) IncrementVersion() {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.version++
}

// GetSelf returns info about the local node
func (ml *MembershipList) GetSelf() *MemberInfo {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	return ml.members[ml.selfID]
}
