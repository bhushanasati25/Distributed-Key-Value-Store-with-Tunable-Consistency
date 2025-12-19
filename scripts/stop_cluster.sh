#!/bin/bash

# Stop Distributed Key-Value Store Cluster Script

DATA_DIR="/tmp/distributed-kvstore"
PIDS_FILE="$DATA_DIR/pids.txt"

echo "Stopping Distributed Key-Value Store cluster..."

if [ -f "$PIDS_FILE" ]; then
    PIDS=$(cat "$PIDS_FILE")
    for PID in $PIDS; do
        if kill -0 "$PID" 2>/dev/null; then
            echo "Stopping process $PID..."
            kill -TERM "$PID" 2>/dev/null
        fi
    done
    rm "$PIDS_FILE"
else
    echo "No PID file found. Trying to find processes..."
    # Find and kill any dynamo processes
    pkill -f "bin/dynamo" 2>/dev/null || true
fi

# Also kill by port (backup method)
for PORT in 8001 8002 8003 8004 8005; do
    PID=$(lsof -t -i:$PORT 2>/dev/null)
    if [ -n "$PID" ]; then
        echo "Killing process on port $PORT (PID: $PID)"
        kill -TERM "$PID" 2>/dev/null || true
    fi
done

echo "Cluster stopped."

# Optional: Clean up data
read -p "Remove data directory? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm -rf "$DATA_DIR"
    echo "Data directory removed."
fi
