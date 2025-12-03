# Copyright (c) 2025 Northbound System
# Author: Nicholas Skitch
#!/bin/bash
set -e

echo "=========================================="
echo "Building and Running The Hive Server"
echo "=========================================="
echo ""

# Check if Go is available
if ! command -v go &> /dev/null; then
    echo "❌ Go is not in PATH. Trying to find it..."
    if [ -f "/usr/local/go/bin/go" ]; then
        echo "✓ Found Go at /usr/local/go/bin/go"
        export PATH=$PATH:/usr/local/go/bin
    else
        echo "❌ Go not found. Please install Go or add it to PATH"
        echo "   Install: sudo apt install golang-go"
        echo "   Or: sudo snap install go"
        exit 1
    fi
fi

echo "✓ Go version: $(go version)"
echo ""

# Check if protoc is available
if ! command -v protoc &> /dev/null; then
    echo "⚠️  protoc not found. Installing protobuf plugins may fail."
    echo "   Install: sudo apt install protobuf-compiler"
fi

# Add Go bin to PATH
export PATH=$PATH:$(go env GOPATH)/bin
export CGO_ENABLED=1

# Change to project directory
cd "$(dirname "$0")"

echo "Step 1: Installing protobuf plugins..."
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest || echo "⚠️  Failed to install protoc-gen-go"
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest || echo "⚠️  Failed to install protoc-gen-go-grpc"
echo ""

echo "Step 2: Generating protobuf code..."
make proto || {
    echo "❌ Failed to generate protobuf code"
    echo "   Make sure protoc and plugins are installed"
    exit 1
}
echo ""

echo "Step 3: Building Hive server..."
make build-hive || {
    echo "❌ Failed to build server"
    echo "   Make sure CGO is enabled and dependencies are installed"
    exit 1
}
echo ""

echo "=========================================="
echo "✅ Build complete!"
echo "=========================================="
echo ""
echo "Starting server on port 8081..."
echo "Open http://localhost:8081 in your browser"
echo ""
echo "Press Ctrl+C to stop the server"
echo ""

# Run the server
./bin/hive-server

