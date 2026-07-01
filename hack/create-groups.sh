#!/bin/bash
set -e

# Script to create test groups and members in the database

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
AUTH_DIR="$ROOT_DIR/.auth"

if [ ! -f "$AUTH_DIR/private-key.txt" ]; then
    echo "❌ Private key not found. Run ./hack/create-tokens.sh first"
    exit 1
fi

if [ ! -f "$AUTH_DIR/tokens.env" ]; then
    echo "❌ Tokens not found. Run ./hack/create-tokens.sh first"
    exit 1
fi

# Load tokens
source "$AUTH_DIR/tokens.env"

echo "🏢 Creating test groups..."

# API URL (can be overridden by environment variable)
API_URL="${PLANNER_API_URL:-http://localhost:3443}"

# Check if API is running
echo "🔍 Checking API connectivity..."
if ! curl -s -f -H "X-Authorization: Bearer $ADMIN_TOKEN" -o /dev/null "$API_URL/api/v1/identity" 2>/dev/null; then
    echo "❌ API is not responding at $API_URL"
    echo "   Make sure the API is running with:"
    echo "   export MIGRATION_PLANNER_PRIVATE_KEY=\"\$(cat .auth/private-key.txt)\""
    echo "   export MIGRATION_PLANNER_AUTH=local"
    echo "   make deploy-db build-api run"
    exit 1
fi
echo "✅ API is running"

# Function to make API requests
api_call() {
    local method=$1
    local path=$2
    local data=$3

    local response
    response=$(curl -s -w "\n%{http_code}" -X "$method" \
        -H "X-Authorization: Bearer $ADMIN_TOKEN" \
        -H "Content-Type: application/json" \
        ${data:+-d "$data"} \
        "$API_URL$path")

    local http_code=$(echo "$response" | tail -n1)
    local body=$(echo "$response" | sed '$d')

    if [ "$http_code" -ge 400 ]; then
        echo "❌ HTTP $http_code: $body" >&2
        return 1
    fi

    echo "$body"
}

# Create Partner group
echo "📝 Creating Partner group..."
PARTNER_GROUP=$(api_call POST "/api/v1/groups" '{
    "name": "Tech Solutions Inc",
    "description": "Specialized in cloud migration services with over 10 years of experience helping enterprises transition to modern infrastructure.",
    "kind": "partner",
    "company": "Tech Solutions Inc",
    "icon": "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAxMDAgMTAwIiBmaWxsPSJub25lIiBzaGFwZS1yZW5kZXJpbmc9ImF1dG8iPjxtZXRhZGF0YSB4bWxuczpyZGY9Imh0dHA6Ly93d3cudzMub3JnLzE5OTkvMDIvMjItcmRmLXN5bnRheC1ucyMiIHhtbG5zOnhzaT0iaHR0cDovL3d3dy53My5vcmcvMjAwMS9YTUxTY2hlbWEtaW5zdGFuY2UiIHhtbG5zOmRjPSJodHRwOi8vcHVybC5vcmcvZGMvZWxlbWVudHMvMS4xLyIgeG1sbnM6ZGN0ZXJtcz0iaHR0cDovL3B1cmwub3JnL2RjL3Rlcm1zLyI+PHJkZjpSREY+PHJkZjpEZXNjcmlwdGlvbj48ZGM6dGl0bGU+U2hhcGVzPC9kYzp0aXRsZT48ZGM6Y3JlYXRvcj5EaWNlQmVhcjwvZGM6Y3JlYXRvcj48ZGM6c291cmNlIHhzaTp0eXBlPSJkY3Rlcm1zOlVSSSI+aHR0cHM6Ly93d3cuZGljZWJlYXIuY29tPC9kYzpzb3VyY2U+PGRjdGVybXM6bGljZW5zZSB4c2k6dHlwZT0iZGN0ZXJtczpVUkkiPmh0dHBzOi8vY3JlYXRpdmVjb21tb25zLm9yZy9wdWJsaWNkb21haW4vemVyby8xLjAvPC9kY3Rlcm1zOmxpY2Vuc2U+PGRjOnJpZ2h0cz7igJ5TaGFwZXPigJ0gKGh0dHBzOi8vd3d3LmRpY2ViZWFyLmNvbSkgYnkg4oCeRGljZUJlYXLigJ0sIGxpY2Vuc2VkIHVuZGVyIOKAnkNDMCAxLjDigJ0gKGh0dHBzOi8vY3JlYXRpdmVjb21tb25zLm9yZy9wdWJsaWNkb21haW4vemVyby8xLjAvKTwvZGM6cmlnaHRzPjwvcmRmOkRlc2NyaXB0aW9uPjwvcmRmOlJERj48L21ldGFkYXRhPjxtYXNrIGlkPSJ2aWV3Ym94TWFzayI+PHJlY3Qgd2lkdGg9IjEwMCIgaGVpZ2h0PSIxMDAiIHJ4PSIwIiByeT0iMCIgeD0iMCIgeT0iMCIgZmlsbD0iI2ZmZiIgLz48L21hc2s+PGcgbWFzaz0idXJsKCN2aWV3Ym94TWFzaykiPjxyZWN0IGZpbGw9IiM2OWQyZTciIHdpZHRoPSIxMDAiIGhlaWdodD0iMTAwIiB4PSIwIiB5PSIwIiAvPjxnIHRyYW5zZm9ybT0ibWF0cml4KDEuMiAwIDAgMS4yIC0xMCAtMTApIj48ZyB0cmFuc2Zvcm09InRyYW5zbGF0ZSgzLCAtNSkgcm90YXRlKDQyIDUwIDUwKSI+PHBhdGggZD0iTTAgMGgxMDB2MTAwSDBWMFoiIGZpbGw9IiNmMWY0ZGMiLz48L2c+PC9nPjxnIHRyYW5zZm9ybT0ibWF0cml4KC44IDAgMCAuOCAxMCAxMCkiPjxnIHRyYW5zZm9ybT0idHJhbnNsYXRlKC0yMCwgLTQpIHJvdGF0ZSgtOTAgNTAgNTApIj48cGF0aCBmaWxsPSIjMGE1YjgzIiBkPSJNNDUtMTUwaDEwdjQwMEg0NXoiLz48L2c+PC9nPjxnIHRyYW5zZm9ybT0ibWF0cml4KC40IDAgMCAuNCAzMCAzMCkiPjxnIHRyYW5zZm9ybT0idHJhbnNsYXRlKDIwLCAtMjApIHJvdGF0ZSgxNDUgNTAgNTApIj48cGF0aCBmaWxsLXJ1bGU9ImV2ZW5vZGQiIGNsaXAtcnVsZT0iZXZlbm9kZCIgZD0iTTkwIDEwSDEwdjgwaDgwVjEwWk0wIDB2MTAwaDEwMFYwSDBaIiBmaWxsPSIjMWM3OTlmIi8+PC9nPjwvZz48L2c+PC9zdmc+"
}')
PARTNER_GROUP_ID=$(echo "$PARTNER_GROUP" | jq -r '.id // empty')

if [ -n "$PARTNER_GROUP_ID" ]; then
    echo "✅ Partner group created: $PARTNER_GROUP_ID"

    # Add partner member
    echo "📝 Adding partner-user member..."
    api_call POST "/api/v1/groups/$PARTNER_GROUP_ID/members" '{
        "username": "partner-user",
        "email": "partner-user@partner-org.com"
    }' > /dev/null
    echo "✅ partner-user member added"
else
    echo "⚠️  Partner group already exists or error during creation"
fi

# Create customer-partner relationship
if [ -n "$PARTNER_GROUP_ID" ]; then
    echo ""
    echo "📝 Setting up customer-partner relationship..."

    # Customer creates a partner request
    echo "   Creating partner request from customer..."
    REQUEST_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
        -H "X-Authorization: Bearer $CUSTOMER_TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"Customer Corp\",\"contactName\":\"Customer User\",\"contactPhone\":\"+33123456789\",\"email\":\"customer-user@customer-org.com\",\"location\":\"Paris, France\"}" \
        "$API_URL/api/v1/partners/$PARTNER_GROUP_ID/request")

    REQUEST_HTTP_CODE=$(echo "$REQUEST_RESPONSE" | tail -n1)
    REQUEST_BODY=$(echo "$REQUEST_RESPONSE" | sed '$d')

    if [ "$REQUEST_HTTP_CODE" -ge 400 ]; then
        echo "   ⚠️  Failed to create partner request: HTTP $REQUEST_HTTP_CODE"
        echo "   $REQUEST_BODY"
    else
        REQUEST_ID=$(echo "$REQUEST_BODY" | jq -r '.id // empty')

        if [ -n "$REQUEST_ID" ]; then
            echo "   ✅ Partner request created: $REQUEST_ID"

            # Partner accepts the request
            echo "   Accepting partner request..."
            ACCEPT_RESPONSE=$(curl -s -w "\n%{http_code}" -X PUT \
                -H "X-Authorization: Bearer $PARTNER_TOKEN" \
                -H "Content-Type: application/json" \
                -d '{"status":"accepted"}' \
                "$API_URL/api/v1/partners/requests/$REQUEST_ID")

            ACCEPT_HTTP_CODE=$(echo "$ACCEPT_RESPONSE" | tail -n1)
            ACCEPT_BODY=$(echo "$ACCEPT_RESPONSE" | sed '$d')

            if [ "$ACCEPT_HTTP_CODE" -ge 400 ]; then
                echo "   ⚠️  Failed to accept partner request: HTTP $ACCEPT_HTTP_CODE"
                echo "   $ACCEPT_BODY"
            else
                echo "   ✅ Partner request accepted!"
                echo "   ✅ Customer-Partner relationship established"
            fi
        else
            echo "   ⚠️  Could not extract request ID from response"
        fi
    fi
else
    echo "⚠️  Skipping customer-partner relationship (no partner group)"
fi
echo ""

echo "✅ Configuration complete!"
echo ""

