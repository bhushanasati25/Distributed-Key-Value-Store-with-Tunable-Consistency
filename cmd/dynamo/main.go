package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mini-dynamo/mini-dynamo/internal/api"
	"github.com/mini-dynamo/mini-dynamo/internal/config"
	"github.com/mini-dynamo/mini-dynamo/internal/gossip"
	"github.com/mini-dynamo/mini-dynamo/internal/replication"
	"github.com/mini-dynamo/mini-dynamo/internal/ring"
	"github.com/mini-dynamo/mini-dynamo/internal/storage"
	"github.com/mini-dynamo/mini-dynamo/pkg/types"
)

var (
	version   = "1.0.0"
	buildTime = "unknown"
)

func main() {
	// Command line flags
	var (
		nodeID        = flag.String("node-id", "", "Unique node identifier")
		address       = flag.String("address", "127.0.0.1", "Listen address")
		port          = flag.Int("port", 8080, "HTTP port")
		gossipPort    = flag.Int("gossip-port", 7946, "Gossip UDP port")
		dataDir       = flag.String("data-dir", "./data", "Data directory")
		seedNodes     = flag.String("seeds", "", "Comma-separated seed node addresses")
		replFactor    = flag.Int("replication", 3, "Replication factor (N)")
		readQuorum    = flag.Int("read-quorum", 2, "Read quorum (R)")
		writeQuorum   = flag.Int("write-quorum", 2, "Write quorum (W)")
		virtualNodes  = flag.Int("vnodes", 150, "Virtual nodes per physical node")
		configFile    = flag.String("config", "", "Configuration file path")
		showVersion   = flag.Bool("version", false, "Show version")
	)

	flag.Parse()

	if *showVersion {
		fmt.Printf("Mini-Dynamo v%s (built: %s)\n", version, buildTime)
		os.Exit(0)
	}

	// Load or create configuration
	var cfg *config.Config
	var err error

	if *configFile != "" {
		cfg, err = config.LoadFromFile(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
	} else {
		cfg = config.DefaultConfig()
	}

	// Override with command line flags
	if *nodeID != "" {
		cfg.NodeID = *nodeID
	} else if cfg.NodeID == "" {
		hostname, _ := os.Hostname()
		cfg.NodeID = fmt.Sprintf("%s-%d", hostname, *port)
	}

	cfg.Address = *address
	cfg.Port = *port
	cfg.GossipPort = *gossipPort
	cfg.DataDir = *dataDir
	cfg.ReplicationFactor = *replFactor
	cfg.ReadQuorum = *readQuorum
	cfg.WriteQuorum = *writeQuorum
	cfg.VirtualNodes = *virtualNodes

	// Parse seed nodes
	if *seedNodes != "" {
		cfg.SeedNodes = splitAndTrim(*seedNodes, ",")
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	log.Printf("Starting Mini-Dynamo node: %s", cfg.NodeID)
	log.Printf("Address: %s:%d, Gossip: %d", cfg.Address, cfg.Port, cfg.GossipPort)
	log.Printf("Replication: N=%d, R=%d, W=%d", cfg.ReplicationFactor, cfg.ReadQuorum, cfg.WriteQuorum)

	// Initialize storage engine
	store, err := storage.NewBitcask(cfg.DataDir, cfg.SyncWrites)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	log.Printf("Storage initialized: %d keys loaded", store.Count())

	// Initialize hash ring
	hashRing := ring.NewHashRing(cfg.VirtualNodes)

	// Initialize gossip membership
	membership := gossip.NewMembershipList(cfg.NodeID)

	// Set up node state change handler
	onStateChange := func(nodeID string, oldState, newState types.NodeState) {
		log.Printf("Node %s: %s -> %s", nodeID, oldState.String(), newState.String())
		if newState == types.NodeDead {
			hashRing.RemoveNode(nodeID)
		} else if newState == types.NodeAlive && oldState == types.NodeDead {
			hashRing.AddNode(nodeID)
		}
	}

	// Initialize failure detector
	detector := gossip.NewFailureDetector(
		membership,
		cfg.SuspectTimeout,
		cfg.DeadTimeout,
		onStateChange,
	)

	// Initialize gossip protocol
	gossipProto := gossip.NewProtocol(cfg, membership, detector)

	// Initialize coordinator
	coordinator := replication.NewCoordinator(cfg, hashRing, store)

	// Register self as node
	selfNode := &types.Node{
		ID:      cfg.NodeID,
		Address: cfg.Address,
		Port:    cfg.Port,
		State:   types.NodeAlive,
	}
	coordinator.RegisterNode(selfNode)
	hashRing.AddNode(cfg.NodeID)

	// Initialize hinted handoff store
	handoffStore := replication.NewHintedHandoffStore(cfg.HandoffTimeout, 1000)
	handoffManager := replication.NewHandoffManager(handoffStore, coordinator, 30*time.Second)

	// Initialize API server
	server := api.NewServer(cfg, store, coordinator)

	// Start services
	if err := gossipProto.Start(); err != nil {
		log.Fatalf("Failed to start gossip: %v", err)
	}
	detector.Start()
	handoffManager.Start()

	// Connect to seed nodes
	for _, seedAddr := range cfg.SeedNodes {
		log.Printf("Connecting to seed node: %s", seedAddr)
		// Parse seed address and register
		gossipProto.AddPeer(seedAddr, seedAddr)
	}

	// Start HTTP server in goroutine
	go func() {
		if err := server.Start(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	log.Printf("Node %s is ready", cfg.NodeID)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	handoffManager.Stop()
	detector.Stop()
	gossipProto.Stop()

	if err := server.Stop(ctx); err != nil {
		log.Printf("Error stopping server: %v", err)
	}

	if err := store.Sync(); err != nil {
		log.Printf("Error syncing storage: %v", err)
	}

	log.Println("Shutdown complete")
}

// splitAndTrim splits a string by separator and trims whitespace
func splitAndTrim(s string, sep string) []string {
	if s == "" {
		return nil
	}
	parts := make([]string, 0)
	for _, p := range splitString(s, sep) {
		if trimmed := trimSpace(p); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

// splitString splits a string by separator (simple implementation)
func splitString(s, sep string) []string {
	result := make([]string, 0)
	current := ""
	sepLen := len(sep)

	for i := 0; i < len(s); i++ {
		if i+sepLen <= len(s) && s[i:i+sepLen] == sep {
			result = append(result, current)
			current = ""
			i += sepLen - 1
		} else {
			current += string(s[i])
		}
	}
	result = append(result, current)
	return result
}

// trimSpace trims whitespace from a string
func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}
