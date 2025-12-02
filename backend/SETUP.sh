#!/bin/bash
set -e  # Exit immediately if any command fails

echo "Configuring local environment..."

# Detect OS and install Dependencies
if [ "$(uname)" == "Darwin" ]; then
    echo "Detected macOS. Installing dependencies..."
    # Installs protobuf and mupdf (needed for go-fitz)
    brew install protobuf mupdf
elif [ -f /etc/debian_version ]; then
    echo "Detected Debian/Ubuntu. Installing dependencies..."
    sudo apt update
    # Added build-essential and libmupdf-dev which are REQUIRED for go-fitz/CGO
    sudo apt install -y protobuf-compiler build-essential libmupdf-dev
fi

# Install Go Plugins for Protoc
echo "Installing Go protocol buffer plugins..."
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Add Go bin to PATH for this script execution
export PATH=$PATH:$(go env GOPATH)/bin

# Enable CGO (Required for go-fitz)
export CGO_ENABLED=1

echo "--------------------------------------------------"
echo "✅ Local environment ready!"
echo "NOTE: If you still see errors, run this to update your shell:"
echo "      export PATH=\$PATH:\$(go env GOPATH)/bin"
echo "--------------------------------------------------"