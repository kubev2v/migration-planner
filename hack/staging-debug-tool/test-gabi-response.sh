#!/bin/bash

set -e

# Configuration
GABI_URL="https://gabi-assisted-migration-stage.apps.crcs02ue1.urby.p1.openshiftapps.com/query"

# Check for required GABI_TOKEN environment variable
if [ -z "$GABI_TOKEN" ]; then
    echo "❌ Error: GABI_TOKEN environment variable is not set"
    echo "💡 Please set it with: export GABI_TOKEN='your_token_here'"
    exit 1
fi

echo "🔍 Debugging GABI response..."

# Test query with detailed output
echo "📥 Testing sources query..."
json_result=$(curl -s -H "Authorization: Bearer $GABI_TOKEN" \
    "$GABI_URL" \
    -d '{"query": "SELECT * FROM sources LIMIT 5;"}')

echo "📊 Pretty-printed JSON:"
echo "$json_result" | jq '.'

