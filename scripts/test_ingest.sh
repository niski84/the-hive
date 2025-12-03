#!/bin/bash
# Copyright (c) 2025 Northbound System
# Author: Nicholas Skitch
#
# Test script to verify UUID generation fix
# This script clears logs, starts watchdog, copies a test file, and waits for results

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=========================================="
echo "🧪 Testing UUID Generation Fix"
echo "=========================================="
echo ""

# Check if hive-server is running
if ! pgrep -f hive-server > /dev/null; then
    echo "⚠️  Warning: hive-server is not running"
    echo "   Starting server..."
    ./scripts/reload.sh
    sleep 3
fi

# Clear log file
echo "Clearing hive-server.log..."
> hive-server.log
echo "✓ Log cleared"

# Build watchdog if needed
if [ ! -f "bin/watchdog" ]; then
    echo "Building watchdog..."
    cd "$PROJECT_ROOT"
    export PATH=$PATH:/usr/local/go/bin
    export CGO_ENABLED=1
    go build -o bin/watchdog ./cmd/watchdog
    echo "✓ Watchdog built"
fi

# Start watchdog in background
echo "Starting watchdog..."
./bin/watchdog &
WATCHDOG_PID=$!
echo "✓ Watchdog started (PID: $WATCHDOG_PID)"

# Wait a moment for watchdog to initialize
sleep 1

# Find a watched directory (check drone-client config or use default)
WATCHED_DIR="${HOME}/Documents"
if [ -d "$WATCHED_DIR" ]; then
    echo "Using watched directory: $WATCHED_DIR"
else
    echo "⚠️  Warning: Default watched directory not found: $WATCHED_DIR"
    echo "   Please ensure drone-client is watching a directory"
    WATCHED_DIR=""
fi

# Create a test file if we have a watched directory
if [ -n "$WATCHED_DIR" ]; then
    TEST_FILE="$WATCHED_DIR/test_ingest_$(date +%s).txt"
    echo "Creating test file: $TEST_FILE"
    cat > "$TEST_FILE" << 'EOF'
This is a test document for UUID generation verification.
It contains some sample text to trigger ingestion.
The system should generate a valid UUID for this document.
EOF
    echo "✓ Test file created"
    echo ""
    echo "Waiting for watchdog to detect result..."
    echo ""
    
    # Wait for watchdog to exit (it will exit on success or failure)
    wait $WATCHDOG_PID
    EXIT_CODE=$?
    
    # Cleanup test file
    if [ -f "$TEST_FILE" ]; then
        rm -f "$TEST_FILE"
        echo "✓ Test file cleaned up"
    fi
    
    if [ $EXIT_CODE -eq 0 ]; then
        echo ""
        echo "=========================================="
        echo "✅ TEST PASSED: UUID generation working"
        echo "=========================================="
        exit 0
    else
        echo ""
        echo "=========================================="
        echo "❌ TEST FAILED: UUID error detected"
        echo "=========================================="
        echo ""
        echo "Last 20 lines of log:"
        tail -20 hive-server.log
        exit 1
    fi
else
    echo "⚠️  Cannot create test file - no watched directory found"
    echo "   Please configure drone-client to watch a directory"
    kill $WATCHDOG_PID 2>/dev/null || true
    exit 1
fi

