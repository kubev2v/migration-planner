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

# Create Admin group
echo "📝 Creating Admin group..."
ADMIN_GROUP=$(api_call POST "/api/v1/groups" '{
    "name": "Red Hat",
    "description": "Red Hat is an American software company that provides open source software products to enterprises",
    "kind": "admin",
    "company": "Red Hat",
    "icon": "data:image/png;base64,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"
}')
ADMIN_GROUP_ID=$(echo "$ADMIN_GROUP" | jq -r '.id // empty')

if [ -n "$ADMIN_GROUP_ID" ]; then
    echo "✅ Admin group created: $ADMIN_GROUP_ID"

    # Add admin member
    echo "📝 Adding admin-user member..."
    api_call POST "/api/v1/groups/$ADMIN_GROUP_ID/members" '{
        "username": "admin-user",
        "email": "admin-user@admin-org.com"
    }' > /dev/null
    echo "✅ admin-user member added"
else
    echo "⚠️  Admin group already exists or error during creation"
fi

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

