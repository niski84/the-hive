#!/bin/bash
# Copyright (c) 2025 Northbound System
# Author: Nicholas Skitch

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=========================================="
echo "Resetting .env File"
echo "=========================================="
echo ""

# Backup existing .env if it exists
if [ -f ".env" ]; then
    echo "Backing up existing .env to .env.bak..."
    cp .env .env.bak
    echo "✓ Backup created: .env.bak"
else
    echo "No existing .env file found"
fi

# Create fresh .env with placeholders
echo ""
echo "Creating fresh .env file with placeholders..."
cat > .env << 'EOF'
# The Hive Server Configuration
# Edit these values as needed

# AI Provider: openai, gemini, or mock
EMBEDDER_TYPE=openai

# OpenAI API Key (required if EMBEDDER_TYPE=openai)
# Format: sk-proj-...
OPENAI_API_KEY=

# Qdrant Vector Database URL
QDRANT_URL=localhost:6334

# Redis URL (optional, for job queue)
REDIS_URL=localhost:6379

# Installation Date (auto-set on first run)
# INSTALL_DATE=2025-01-01

# License Key (optional, for production)
# LICENSE_KEY=
EOF

echo "✓ Fresh .env file created"
echo ""
echo "=========================================="
echo "✅ Reset complete!"
echo "=========================================="
echo ""
echo "Next steps:"
echo "1. Edit .env and add your OPENAI_API_KEY"
echo "2. Run: ./scripts/reload.sh"
echo ""

