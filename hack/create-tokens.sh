#!/bin/bash
set -e

# Script to configure local authentication and create different users for testing

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
PLANNER_CLI="$ROOT_DIR/bin/planner"
AUTH_DIR="$ROOT_DIR/.auth"

echo "🔐 Setting up local authentication..."

# Create .auth directory if it doesn't exist
mkdir -p "$AUTH_DIR"

# Build CLI
echo "📦 Building planner CLI (without libvirt support)..."
mkdir -p "$ROOT_DIR/bin"
cd "$ROOT_DIR" && go build -buildvcs=false -o "$PLANNER_CLI" ./cmd/planner

# Generate private key if it doesn't exist
if [ ! -f "$AUTH_DIR/private-key.txt" ]; then
    echo "📝 Generating private key..."
    "$PLANNER_CLI" sso private-key > "$AUTH_DIR/private-key.txt"
    echo "✅ Private key generated: $AUTH_DIR/private-key.txt"
else
    echo "✅ Using existing private key: $AUTH_DIR/private-key.txt"
fi

# Function to generate a token
generate_token() {
    local username=$1
    local org=$2
    "$PLANNER_CLI" sso token --private-key "$(cat "$AUTH_DIR/private-key.txt")" --username="$username" --org="$org" --ttl 168
}

echo ""
echo "🎫 Generating tokens for different users..."

# 1. Regular User
REGULAR_TOKEN=$(generate_token "regular-user" "regular-org")
echo "REGULAR_TOKEN=$REGULAR_TOKEN" > "$AUTH_DIR/tokens.env"

# 2. Admin User (requires being a member of an admin group)
ADMIN_TOKEN=$(generate_token "admin-user" "admin-org")
echo "ADMIN_TOKEN=$ADMIN_TOKEN" >> "$AUTH_DIR/tokens.env"

# 3. Partner User (requires being a member of a partner group)
PARTNER_TOKEN=$(generate_token "partner-user" "partner-org")
echo "PARTNER_TOKEN=$PARTNER_TOKEN" >> "$AUTH_DIR/tokens.env"

# 4. Customer User (requires an accepted partner-customer relationship)
CUSTOMER_TOKEN=$(generate_token "customer-user" "customer-org")
echo "CUSTOMER_TOKEN=$CUSTOMER_TOKEN" >> "$AUTH_DIR/tokens.env"

echo "✅ Tokens generated and saved to: $AUTH_DIR/tokens.env"
echo ""
echo "📋 Available tokens:"
echo "   Regular User:  regular-user@regular.org"
echo "   Admin User:    admin-user@admin-org"
echo "   Partner User:  partner-user@partner-org"
echo "   Customer User: customer-user@customer-org"
echo ""
echo ""
echo "🚀 To start the API with local auth:"
echo "   export MIGRATION_PLANNER_AUTH=local"
echo "   export MIGRATION_PLANNER_PRIVATE_KEY=\"\$(cat $AUTH_DIR/private-key.txt)\""
echo "   make deploy-db build-api run"
echo ""
echo "🧪 Test authentication:"
echo "   source $AUTH_DIR/tokens.env"
echo "   curl -H \"X-Authorization: Bearer \$REGULAR_TOKEN\" http://localhost:3443/api/v1/identity"
echo ""
echo "⚠️  IMPORTANT:"
echo "   - API runs on port 3443"
echo "   - Use header: X-Authorization: Bearer <TOKEN>"
echo "   - For Partner/Admin/Customer roles, run: ./hack/create-groups.sh"
