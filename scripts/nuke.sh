#!/bin/bash
# Copyright (c) 2025 Northbound System
# Author: Nicholas Skitch
#
# Nuke script - Aggressively kills all processes and cleans config files
# WARNING: This will kill all hive-server and drone-client processes
#
# Usage: source ./scripts/nuke.sh
# Note: Must be sourced (not executed) to unset environment variables

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$PROJECT_ROOT"

echo "=========================================="
echo "🧹 NUKING THE HIVE SYSTEM"
echo "=========================================="
echo ""

# Kill all hive-server processes
echo "Killing hive-server processes..."
pkill -9 hive-server || true
sleep 1
if pgrep -f hive-server > /dev/null; then
    echo "⚠️  Warning: Some hive-server processes may still be running"
else
    echo "✓ All hive-server processes killed"
fi

# Kill all drone-client processes
echo "Killing drone-client processes..."
pkill -9 drone-client || true
sleep 1
if pgrep -f drone-client > /dev/null; then
    echo "⚠️  Warning: Some drone-client processes may still be running"
else
    echo "✓ All drone-client processes killed"
fi

# Unset environment variables (only works if script is sourced)
if [ "${BASH_SOURCE[0]}" != "${0}" ]; then
    echo "Unsetting environment variables..."
    unset OPENAI_API_KEY
    unset EMBEDDER_TYPE
    unset QDRANT_URL
    unset REDIS_URL
    unset INSTALL_DATE
    unset LICENSE_KEY
    echo "✓ Environment variables unset"
else
    echo "⚠️  Warning: Script not sourced. Environment variables not unset."
    echo "   To unset variables, run: source ./scripts/nuke.sh"
fi

# Remove config files
echo "Removing config files..."
rm -f .env
rm -f config.yaml
rm -f .env.bak
echo "✓ Config files removed"

# Clean up log files (optional - comment out if you want to keep logs)
# echo "Cleaning log files..."
# rm -f hive-server.log
# rm -f drone-client.log
# echo "✓ Log files removed"

echo ""
echo "=========================================="
echo "✅ System Nuked!"
echo "=========================================="
echo ""
echo "Restarting system..."
echo ""

# Restart
if [ -f "$SCRIPT_DIR/reload.sh" ]; then
    "$SCRIPT_DIR/reload.sh"
else
    echo "⚠️  reload.sh not found. Please run manually:"
    echo "   ./scripts/reload.sh"
fi

