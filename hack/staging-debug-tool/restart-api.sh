#!/bin/bash
# restart-api.sh - Restart just the API container after debugging

set -e

echo "🔄 Restarting API container..."

# Stop the API container
echo "   ⏹️  Stopping API container..."
podman stop planner-api-debug 2>/dev/null || true

# Start the API container (dependencies are already completed)
echo "   ▶️  Starting API container..."
podman start planner-api-debug

echo "✅ API container restarted successfully"
echo "🌐 API available at: http://localhost:3443"
echo "💡 Debugger port: localhost:40000"


