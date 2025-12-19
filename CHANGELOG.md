# Changelog

All notable changes to Distributed Key-Value Store with Tunable Consistency will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Planned
- gRPC support for inter-node communication
- Merkle tree anti-entropy
- Prometheus metrics endpoint
- Docker Compose for multi-node deployment

---

## [1.0.0] - 2024-12-18

### Added

#### Core Features
- **Distributed Key-Value Store** - Full implementation of Dynamo-style architecture
- **Consistent Hashing** - 64-bit hash ring with MurmurHash3
- **Virtual Nodes** - 150 vnodes per physical node for even distribution
- **Replication** - Configurable N-way replication (default N=3)
- **Quorum Operations** - Tunable R/W quorum (default R=2, W=2)

#### Storage Engine
- **Bitcask Model** - Append-only log with in-memory index
- **CRC Checksums** - Data integrity verification on reads
- **Compaction** - Background space reclamation
- **Crash Recovery** - Automatic index rebuild on startup

#### Networking
- **RESTful API** - HTTP/JSON interface for all operations
- **Gossip Protocol** - UDP-based failure detection
- **Hinted Handoff** - Temporary storage for failed nodes

#### Fault Tolerance
- **Failure Detection** - Alive → Suspect → Dead state machine
- **Read Repair** - Automatic consistency healing
- **Vector Clocks** - Causality tracking
- **LWW Resolution** - Timestamp-based conflict resolution

#### Operations
- `PUT /kv/{key}` - Store key-value pair
- `GET /kv/{key}` - Retrieve value
- `DELETE /kv/{key}` - Delete key
- `GET /admin/status` - Cluster status
- `GET /admin/ring` - Ring information
- `GET /admin/keys` - List all keys
- `GET /health` - Health check

#### Tooling
- **Makefile** - Build automation
- **Load Tester** - Built-in benchmarking tool
- **Cluster Scripts** - Easy multi-node startup

#### Documentation
- Comprehensive README with architecture diagrams
- API reference with examples
- Technical deep dive sections

### Dependencies
- `github.com/gorilla/mux` v1.8.1 - HTTP routing
- `github.com/spaolacci/murmur3` v1.1.0 - Hashing

---

## Version History

| Version | Date | Highlights |
|---------|------|------------|
| 1.0.0 | 2024-12-18 | Initial release with full Dynamo implementation |

---

## Migration Guide

### Upgrading to 1.0.0

This is the initial release. No migration needed.

### Future Versions

Breaking changes will be documented here with migration instructions.

---

## Links

- [GitHub Repository](https://github.com/distributed-kvstore/distributed-kvstore)
- [Issue Tracker](https://github.com/distributed-kvstore/distributed-kvstore/issues)
- [Documentation](https://github.com/distributed-kvstore/distributed-kvstore#readme)
