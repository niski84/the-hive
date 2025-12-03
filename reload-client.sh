#!/bin/bash
set -e

echo "=========================================="
echo "Reloading The Hive Drone Client"
echo "=========================================="
echo ""

# Check if Go is available
if ! command -v go &> /dev/null; then
    if [ -f "/usr/local/go/bin/go" ]; then
        export PATH=$PATH:/usr/local/go/bin
    else
        echo "❌ Go not found. Please install Go or add it to PATH"
        exit 1
    fi
fi

# Add Go bin to PATH
export PATH=$PATH:$(go env GOPATH)/bin
export CGO_ENABLED=1

# Change to project directory
cd "$(dirname "$0")"

# Kill existing drone-client if running
echo "Stopping existing drone-client..."
pkill -f "drone-client" 2>/dev/null && sleep 1 || echo "No existing process found"

# Build the client
echo "Building drone-client..."
make build-drone || {
    echo "❌ Failed to build drone-client"
    exit 1
}

# Get port from command line or use default
PORT=${1:-9091}

echo ""
echo "=========================================="
echo "✅ Build complete!"
echo "=========================================="
echo ""
echo "Starting drone-client on port $PORT..."
echo "Web dashboard: http://localhost:$PORT"
echo ""
echo "Press Ctrl+C to stop the server"
echo ""

# Run the client
./bin/drone-client -web-port "$PORT"

