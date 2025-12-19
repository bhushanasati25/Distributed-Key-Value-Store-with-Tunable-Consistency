package gossip

import (
	"encoding/json"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/mini-dynamo/mini-dynamo/internal/config"
	"github.com/mini-dynamo/mini-dynamo/pkg/types"
)

// Protocol implements the gossip protocol for membership dissemination
type Protocol struct {
	config     *config.Config
	membership *MembershipList
	detector   *FailureDetector
	conn       *net.UDPConn
	stopCh     chan struct{}
	wg         sync.WaitGroup
	mu         sync.RWMutex
	peers      map[string]*net.UDPAddr // nodeID -> address
}

// NewProtocol creates a new gossip protocol instance
func NewProtocol(cfg *config.Config, membership *MembershipList, detector *FailureDetector) *Protocol {
	return &Protocol{
		config:     cfg,
		membership: membership,
		detector:   detector,
		stopCh:     make(chan struct{}),
		peers:      make(map[string]*net.UDPAddr),
	}
}

// Start begins the gossip protocol
func (p *Protocol) Start() error {
	// Create UDP listener
	addr, err := net.ResolveUDPAddr("udp", p.config.GossipAddress())
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	p.conn = conn

	// Start receiver
	p.wg.Add(1)
	go p.receiveLoop()

	// Start gossip sender
	p.wg.Add(1)
	go p.gossipLoop()

	log.Printf("Gossip protocol started on %s", p.config.GossipAddress())
	return nil
}

// Stop stops the gossip protocol
func (p *Protocol) Stop() {
	close(p.stopCh)
	if p.conn != nil {
		p.conn.Close()
	}
	p.wg.Wait()
}

// AddPeer adds a peer to gossip with
func (p *Protocol) AddPeer(nodeID string, address string) error {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return err
	}

	p.mu.Lock()
	p.peers[nodeID] = addr
	p.mu.Unlock()

	return nil
}

// RemovePeer removes a peer
func (p *Protocol) RemovePeer(nodeID string) {
	p.mu.Lock()
	delete(p.peers, nodeID)
	p.mu.Unlock()
}

// receiveLoop handles incoming gossip messages
func (p *Protocol) receiveLoop() {
	defer p.wg.Done()

	buffer := make([]byte, 65535)

	for {
		select {
		case <-p.stopCh:
			return
		default:
		}

		// Set read deadline for periodic checking of stop channel
		p.conn.SetReadDeadline(time.Now().Add(time.Second))

		n, remoteAddr, err := p.conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-p.stopCh:
				return
			default:
				log.Printf("Error reading gossip: %v", err)
				continue
			}
		}

		p.handleMessage(buffer[:n], remoteAddr)
	}
}

// handleMessage processes an incoming gossip message
func (p *Protocol) handleMessage(data []byte, from *net.UDPAddr) {
	var msg types.GossipMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("Invalid gossip message from %s: %v", from, err)
		return
	}

	// Record heartbeat from sender
	p.detector.RecordHeartbeat(msg.FromNode)

	// Merge membership information
	p.membership.Merge(msg.Members)

	log.Printf("Received gossip from %s, %d members", msg.FromNode, len(msg.Members))
}

// gossipLoop periodically sends gossip messages
func (p *Protocol) gossipLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.GossipInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.gossipToRandomPeer()
		}
	}
}

// gossipToRandomPeer sends membership info to a random peer
func (p *Protocol) gossipToRandomPeer() {
	p.mu.RLock()
	peerIDs := make([]string, 0, len(p.peers))
	for id := range p.peers {
		peerIDs = append(peerIDs, id)
	}
	p.mu.RUnlock()

	if len(peerIDs) == 0 {
		return
	}

	// Pick a random peer
	targetID := peerIDs[rand.Intn(len(peerIDs))]

	p.mu.RLock()
	targetAddr := p.peers[targetID]
	p.mu.RUnlock()

	if targetAddr == nil {
		return
	}

	// Build gossip message
	msg := types.GossipMessage{
		FromNode:  p.config.NodeID,
		Members:   p.membership.ToGossipFormat(),
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal gossip: %v", err)
		return
	}

	// Send gossip
	_, err = p.conn.WriteToUDP(data, targetAddr)
	if err != nil {
		log.Printf("Failed to send gossip to %s: %v", targetID, err)
	}
}

// SendDirectMessage sends a direct message to a specific node
func (p *Protocol) SendDirectMessage(nodeID string, msg types.GossipMessage) error {
	p.mu.RLock()
	addr, exists := p.peers[nodeID]
	p.mu.RUnlock()

	if !exists {
		return nil // Silent fail for unknown nodes
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = p.conn.WriteToUDP(data, addr)
	return err
}

// BroadcastToAll sends a message to all known peers
func (p *Protocol) BroadcastToAll(msg types.GossipMessage) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	for _, addr := range p.peers {
		p.conn.WriteToUDP(data, addr)
	}
}

// GetPeerCount returns the number of known peers
func (p *Protocol) GetPeerCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.peers)
}
