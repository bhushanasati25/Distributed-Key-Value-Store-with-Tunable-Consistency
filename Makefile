.PHONY: all build test clean run cluster stop-cluster lint fmt

# Build settings
BINARY_NAME=dynamo
BUILD_DIR=bin
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Go settings
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

all: build

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Build the binary
build: deps
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/dynamo

# Build for multiple platforms
build-all: deps
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/dynamo
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/dynamo
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/dynamo
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/dynamo

# Run unit tests
test:
	$(GOTEST) -v -race ./internal/...

# Run integration tests (requires built binary)
test-integration: build
	$(GOTEST) -v -tags=integration ./test/integration/...

# Run all tests
test-all: test test-integration

# Run with coverage
coverage:
	$(GOTEST) -coverprofile=coverage.out ./internal/...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run benchmarks
bench:
	$(GOTEST) -bench=. -benchmem ./internal/...

# Run the load tester
load-test: build
	$(GOCMD) run ./test/load/benchmark.go -requests=1000 -concurrency=10

# Run a single node
run: build
	./$(BUILD_DIR)/$(BINARY_NAME) --port=8001 --data-dir=./data/node1

# Start a 3-node cluster
cluster: build
	chmod +x scripts/start_cluster.sh
	./scripts/start_cluster.sh 3

# Stop the cluster
stop-cluster:
	chmod +x scripts/stop_cluster.sh
	./scripts/stop_cluster.sh

# Format code
fmt:
	$(GOCMD) fmt ./...

# Run linter
lint:
	golangci-lint run ./...

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -rf coverage.out coverage.html
	rm -rf /tmp/distributed-kvstore

# Show help
help:
	@echo "Distributed Key-Value Store Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make build          - Build the binary"
	@echo "  make test           - Run unit tests"
	@echo "  make test-all       - Run unit and integration tests"
	@echo "  make coverage       - Run tests with coverage report"
	@echo "  make run            - Run a single node"
	@echo "  make cluster        - Start a 3-node cluster"
	@echo "  make stop-cluster   - Stop the cluster"
	@echo "  make load-test      - Run load testing"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make help           - Show this help"
