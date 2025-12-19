package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Config holds all configuration for a Distributed Key-Value Store node
type Config struct {
	// Node identity
	NodeID   string `json:"node_id"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	GRPCPort int    `json:"grpc_port,omitempty"`

	// Cluster configuration
	SeedNodes []string `json:"seed_nodes"` // Initial nodes to contact for joining

	// Storage configuration
	DataDir         string `json:"data_dir"`
	MaxFileSize     int64  `json:"max_file_size"`      // Max size before compaction (bytes)
	SyncWrites      bool   `json:"sync_writes"`        // Sync to disk on every write
	CompactInterval int    `json:"compact_interval"`   // Compaction check interval (seconds)

	// Replication configuration
	ReplicationFactor int `json:"replication_factor"` // N - number of replicas
	ReadQuorum        int `json:"read_quorum"`        // R - reads required for success
	WriteQuorum       int `json:"write_quorum"`       // W - writes required for success

	// Consistent hashing
	VirtualNodes int `json:"virtual_nodes"` // Number of virtual nodes per physical node

	// Gossip protocol
	GossipInterval   time.Duration `json:"gossip_interval"`   // How often to gossip
	GossipPort       int           `json:"gossip_port"`       // UDP port for gossip
	SuspectTimeout   time.Duration `json:"suspect_timeout"`   // Time before marking suspect
	DeadTimeout      time.Duration `json:"dead_timeout"`      // Time before marking dead
	
	// Timeouts
	RequestTimeout   time.Duration `json:"request_timeout"`   // Timeout for inter-node requests
	HandoffTimeout   time.Duration `json:"handoff_timeout"`   // How long to keep hinted handoffs
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	hostname, _ := os.Hostname()
	return &Config{
		NodeID:            hostname,
		Address:           "127.0.0.1",
		Port:              8080,
		GRPCPort:          9090,
		SeedNodes:         []string{},
		DataDir:           "./data",
		MaxFileSize:       100 * 1024 * 1024, // 100MB
		SyncWrites:        false,
		CompactInterval:   300, // 5 minutes
		ReplicationFactor: 3,
		ReadQuorum:        2,
		WriteQuorum:       2,
		VirtualNodes:      150,
		GossipInterval:    time.Second,
		GossipPort:        7946,
		SuspectTimeout:    5 * time.Second,
		DeadTimeout:       30 * time.Second,
		RequestTimeout:    5 * time.Second,
		HandoffTimeout:    24 * time.Hour,
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.NodeID == "" {
		return fmt.Errorf("node_id is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	if c.DataDir == "" {
		return fmt.Errorf("data_dir is required")
	}
	if c.ReplicationFactor < 1 {
		return fmt.Errorf("replication_factor must be at least 1")
	}
	if c.ReadQuorum < 1 || c.ReadQuorum > c.ReplicationFactor {
		return fmt.Errorf("read_quorum must be between 1 and replication_factor")
	}
	if c.WriteQuorum < 1 || c.WriteQuorum > c.ReplicationFactor {
		return fmt.Errorf("write_quorum must be between 1 and replication_factor")
	}
	// Quorum intersection check (W + R > N for strong consistency)
	if c.WriteQuorum+c.ReadQuorum <= c.ReplicationFactor {
		// This is a warning, not an error - allows eventual consistency
		fmt.Printf("Warning: W(%d) + R(%d) <= N(%d), eventual consistency mode\n",
			c.WriteQuorum, c.ReadQuorum, c.ReplicationFactor)
	}
	if c.VirtualNodes < 1 {
		return fmt.Errorf("virtual_nodes must be at least 1")
	}
	return nil
}

// LoadFromFile loads configuration from a JSON file
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// SaveToFile saves the configuration to a JSON file
func (c *Config) SaveToFile(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// FullAddress returns the complete HTTP address
func (c *Config) FullAddress() string {
	return fmt.Sprintf("%s:%d", c.Address, c.Port)
}

// GossipAddress returns the complete gossip UDP address
func (c *Config) GossipAddress() string {
	return fmt.Sprintf("%s:%d", c.Address, c.GossipPort)
}
