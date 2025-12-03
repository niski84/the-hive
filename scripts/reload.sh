# Copyright (c) 2025 Northbound System
# Author: Nicholas Skitch
#!/bin/bash
set -e

echo "=========================================="
echo "Reloading The Hive Server and Client"
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
cd "$(dirname "$0")/.."

# Start Docker services (Redis and Qdrant)
echo "Starting Docker services (Redis and Qdrant)..."
if command -v docker-compose &> /dev/null; then
    docker-compose -f docker-compose.yaml up -d 2>/dev/null || echo "docker-compose not available or services already running"
elif command -v docker &> /dev/null; then
    docker compose -f docker-compose.yaml up -d 2>/dev/null || echo "docker not available or services already running"
else
    echo "⚠️  Docker not found. Please start Redis and Qdrant manually."
fi
sleep 2

# Kill existing processes and free ports
echo "Stopping existing processes and freeing ports..."
pkill -f "hive-server" 2>/dev/null && sleep 1 || echo "No hive-server process found"
pkill -f "drone-client" 2>/dev/null && sleep 1 || echo "No drone-client process found"

# Forcefully kill processes on ports 50051 and 8081
echo "Freeing ports 50051 and 8081..."
if command -v fuser &> /dev/null; then
    fuser -k 50051/tcp 2>/dev/null || true
    fuser -k 8081/tcp 2>/dev/null || true
else
    # Fallback to lsof
    lsof -ti:50051 | xargs kill -9 2>/dev/null || true
    lsof -ti:8081 | xargs kill -9 2>/dev/null || true
fi
sleep 1

# Build both binaries first (fail fast if build fails)
echo ""
echo "Building binaries..."
echo "Building hive-server..."
if ! go build -o bin/hive-server ./cmd/hive-server; then
    echo "❌ Failed to build hive-server"
    exit 1
fi

echo "Building drone-client..."
if ! go build -o bin/drone-client ./cmd/drone-client; then
    echo "❌ Failed to build drone-client"
    exit 1
fi

echo ""
echo "=========================================="
echo "✅ Build complete!"
echo "=========================================="
echo ""

# Start server in background
echo "Starting Hive server..."
./bin/hive-server > hive-server.log 2>&1 &
SERVER_PID=$!
echo "Hive server started (PID: $SERVER_PID)"
echo "Logs: hive-server.log"

# Wait 2 seconds and check if process is still running
sleep 2
if ! kill -0 $SERVER_PID 2>/dev/null; then
    echo ""
    echo "❌ Hive server crashed immediately!"
    echo "=========================================="
    echo "Error log:"
    echo "=========================================="
    cat hive-server.log
    echo "=========================================="
    exit 1
fi

# Start client in background
echo "Starting Drone client..."
./bin/drone-client -web-port 9091 > drone-client.log 2>&1 &
CLIENT_PID=$!
echo "Drone client started (PID: $CLIENT_PID)"
echo "Logs: drone-client.log"

# Wait 2 seconds and check if client process is still running
sleep 2
if ! kill -0 $CLIENT_PID 2>/dev/null; then
    echo ""
    echo "❌ Drone client crashed immediately!"
    echo "=========================================="
    echo "Error log:"
    echo "=========================================="
    cat drone-client.log
    echo "=========================================="
    exit 1
fi

echo ""
echo "=========================================="
echo "✅ Both services started!"
echo "=========================================="
echo ""
echo "🌐 Hive Server: http://localhost:8081"
echo "🌐 Drone Client: http://localhost:9091"
echo ""
echo "📋 Process IDs:"
echo "   Server: $SERVER_PID"
echo "   Client: $CLIENT_PID"
echo ""
echo "📄 Log files:"
echo "   Server: hive-server.log"
echo "   Client: drone-client.log"
echo ""
echo "🛑 To stop:"
echo "   pkill -f hive-server"
echo "   pkill -f drone-client"
echo ""

