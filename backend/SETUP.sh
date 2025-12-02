#!/bin/bash
set -e  # Exit immediately if any command fails

echo "=========================================="
echo "The Hive - Development Environment Setup"
echo "=========================================="
echo ""

# Detect OS and install Dependencies
if [ "$(uname)" == "Darwin" ]; then
    echo "Detected macOS. Installing dependencies..."
    # Check if Homebrew is installed
    if ! command -v brew &> /dev/null; then
        echo "❌ Homebrew is not installed. Please install it from https://brew.sh"
        exit 1
    fi
    # Installs protobuf and mupdf (needed for go-fitz)
    echo "Installing protobuf and mupdf..."
    brew install protobuf mupdf || echo "⚠️  Some packages may already be installed"
elif [ -f /etc/debian_version ]; then
    echo "Detected Debian/Ubuntu. Installing dependencies..."
    sudo apt update
    # Install all required dependencies
    echo "Installing build tools, protobuf, and MuPDF..."
    sudo apt install -y \
        protobuf-compiler \
        build-essential \
        libmupdf-dev \
        pkg-config \
        libc6-dev || echo "⚠️  Some packages may already be installed"
else
    echo "⚠️  Unsupported OS. Please install manually:"
    echo "   - Go 1.24+"
    echo "   - Protobuf compiler"
    echo "   - MuPDF development libraries"
    echo "   - Build tools (gcc, make, etc.)"
fi

# Check Go installation
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.24+ from https://go.dev/dl/"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "✓ Found Go version: $GO_VERSION"

# Install Go Plugins for Protoc
echo ""
echo "Installing Go protocol buffer plugins..."
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Add Go bin to PATH for this script execution
export PATH=$PATH:$(go env GOPATH)/bin

# Verify protoc plugins are installed
if [ -f "$(go env GOPATH)/bin/protoc-gen-go" ] && [ -f "$(go env GOPATH)/bin/protoc-gen-go-grpc" ]; then
    echo "✓ Protobuf plugins installed"
else
    echo "⚠️  Protobuf plugins may not be in PATH"
fi

# Enable CGO (Required for go-fitz and sqlite)
export CGO_ENABLED=1

# Download Go dependencies
echo ""
echo "Downloading Go module dependencies..."
cd "$(dirname "$0")/.." || exit 1
go mod download
go mod tidy

echo ""
echo "=========================================="
echo "✅ Local environment ready!"
echo "=========================================="
echo ""
echo "Next steps:"
echo "1. Generate protobuf code:"
echo "   make proto"
echo ""
echo "2. Build the binaries:"
echo "   make build"
echo ""
echo "3. Start Qdrant (vector database):"
echo "   docker run -d -p 6333:6333 -p 6334:6334 --name qdrant qdrant/qdrant"
echo ""
echo "4. Start the Hive server:"
echo "   ./bin/hive-server"
echo ""
echo "5. Start the Drone client (in another terminal):"
echo "   mkdir -p watch"
echo "   ./bin/drone-client -watch-dir ./watch"
echo ""
echo "NOTE: If you see 'command not found' errors, add Go bin to your PATH:"
echo "   export PATH=\$PATH:\$(go env GOPATH)/bin"
echo "   (Add this to your ~/.bashrc or ~/.zshrc to make it permanent)"
echo "=========================================="