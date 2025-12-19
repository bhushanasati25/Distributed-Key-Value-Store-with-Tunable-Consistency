#!/bin/bash

# Start Distributed Key-Value Store Cluster Script
# Usage: ./start_cluster.sh [num_nodes]

set -e

NUM_NODES=${1:-3}
BASE_PORT=8000
BASE_GOSSIP_PORT=7000
DATA_DIR="/tmp/distributed-kvstore"

echo "Starting Distributed Key-Value Store cluster with $NUM_NODES nodes..."
echo "=============================================="

# Create data directories
mkdir -p "$DATA_DIR"

# Build the binary first
echo "Building Distributed Key-Value Store..."
cd "$(dirname "$0")/.."
go build -o bin/dynamo ./cmd/dynamo

PIDS=()

# Start first node (no seeds)
NODE_ID="node1"
PORT=$((BASE_PORT + 1))
GOSSIP_PORT=$((BASE_GOSSIP_PORT + 1))
NODE_DATA="$DATA_DIR/$NODE_ID"

mkdir -p "$NODE_DATA"
echo "Starting $NODE_ID on port $PORT..."

./bin/dynamo \
    --node-id="$NODE_ID" \
    --address="127.0.0.1" \
    --port="$PORT" \
    --gossip-port="$GOSSIP_PORT" \
    --data-dir="$NODE_DATA" \
    > "$NODE_DATA/output.log" 2>&1 &

PIDS+=($!)
FIRST_GOSSIP="127.0.0.1:$GOSSIP_PORT"

sleep 2

# Start remaining nodes
for i in $(seq 2 $NUM_NODES); do
    NODE_ID="node$i"
    PORT=$((BASE_PORT + i))
    GOSSIP_PORT=$((BASE_GOSSIP_PORT + i))
    NODE_DATA="$DATA_DIR/$NODE_ID"
    
    mkdir -p "$NODE_DATA"
    echo "Starting $NODE_ID on port $PORT..."
    
    ./bin/dynamo \
        --node-id="$NODE_ID" \
        --address="127.0.0.1" \
        --port="$PORT" \
        --gossip-port="$GOSSIP_PORT" \
        --data-dir="$NODE_DATA" \
        --seeds="$FIRST_GOSSIP" \
        > "$NODE_DATA/output.log" 2>&1 &
    
    PIDS+=($!)
    sleep 1
done

echo ""
echo "Cluster started!"
echo "================"
echo ""
echo "Node Endpoints:"
for i in $(seq 1 $NUM_NODES); do
    echo "  Node $i: http://127.0.0.1:$((BASE_PORT + i))"
done
echo ""
echo "PIDs: ${PIDS[*]}"
echo ""
echo "Test commands:"
echo "  curl -X PUT http://localhost:8001/kv/hello -d '{\"value\":\"world\"}'"
echo "  curl http://localhost:8002/kv/hello"
echo "  curl http://localhost:8001/admin/status"
echo ""
echo "To stop: ./stop_cluster.sh"

# Save PIDs to file
echo "${PIDS[*]}" > "$DATA_DIR/pids.txt"

# Wait for all processes (optional - comment out for background)
# wait
