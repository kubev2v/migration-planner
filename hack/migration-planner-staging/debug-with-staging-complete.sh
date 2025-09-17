#!/bin/bash
# debug-with-staging-complete.sh

set -e

# Set script directory as working directory (works from any location)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
echo "📁 Using script directory at $SCRIPT_DIR..."

# Ensure we're in the script directory
cd "$SCRIPT_DIR"

# Create staging-data directory if it doesn't exist
mkdir -p "$SCRIPT_DIR/staging-data"

# Load environment variables
source .env-debug

# Validate repository paths exist
if [ ! -d "$MIGRATION_PLANNER_REPO" ]; then
    echo "❌ Migration Planner repository not found at: $MIGRATION_PLANNER_REPO"
    echo "💡 Please update MIGRATION_PLANNER_REPO in .env-debug file"
    echo "   Copy .env-debug.example to .env-debug and edit the repository paths"
    exit 1
fi

if [ ! -d "$MIGRATION_PLANNER_UI_REPO" ]; then
    echo "❌ Migration Planner UI repository not found at: $MIGRATION_PLANNER_UI_REPO"
    echo "💡 Please update MIGRATION_PLANNER_UI_REPO in .env-debug file"
    echo "   Copy .env-debug.example to .env-debug and edit the repository paths"
    exit 1
fi

echo "🚀 Setting up debug environment with staging data..."

# Step 0: Choose authentication mode
echo "🔐 Choose authentication mode:"
echo "  1) none   - No authentication (single user as 'internal' org)"
echo "  2) local  - Local JWT authentication (multi-user simulation)"
echo ""
read -p "Select authentication mode (1 or 2): " auth_choice

case $auth_choice in
    1)
        AUTH_MODE="none"
        echo "✅ Selected: No authentication mode"
        ;;
    2)
        AUTH_MODE="local"
        echo "✅ Selected: Local JWT authentication mode"
        ;;
    *)
        echo "❌ Invalid choice. Defaulting to 'none' authentication."
        AUTH_MODE="none"
        ;;
esac

# Step 1: Validate GABI token
echo "🔐 Step 0: Validating GABI token..."
if ! ./test-gabi-response.sh >/dev/null 2>&1; then
    echo "❌ GABI token validation failed"
    echo "💡 Please check your GABI_TOKEN environment variable"
    echo "   The token may be expired or invalid"
    exit 1
fi
echo "✅ GABI token is valid"

# Generate environment and compose configuration based on auth mode
echo "📝 Generating configuration for $AUTH_MODE authentication..."

if [ "$AUTH_MODE" = "none" ]; then
    # Use auth=none configuration (no authentication)
    COMPOSE_FILE="auth_none/debug-compose.yaml"
    ENV_FILE="auth_none/.env-debug"
    UI_IMAGE="localhost/stagedb_planner-ui:latest"
else
    # Use local authentication mode with file mounting
    COMPOSE_FILE="auth_local/debug-compose-local-file.yaml"
    ENV_FILE="auth_local/.env-debug-local"
    UI_IMAGE="localhost/stagedb_planner-ui:latest-local"
    
    echo "🔑 Generating private key for local authentication..."
    cd "$MIGRATION_PLANNER_REPO"
    PRIVATE_KEY=$(./bin/planner sso private-key)
    cd "$SCRIPT_DIR"
    
    # Save private key for both API container and token generation
    echo "$PRIVATE_KEY" > auth_local/private-key.txt
    
    # Using committed env file; no runtime generation needed
    
    echo "✅ Local authentication configured with generated private key"
    echo "📁 Private key saved to: $(pwd)/private-key.txt"
fi

# Step 1: Extract fresh staging data
echo "📥 Step 1: Extracting staging data via GABI..."
if [ "$AUTH_MODE" = "local" ]; then
    echo "   🏢 Preserving original org_ids for multi-tenant debugging"
    PRESERVE_ORGS=true ./sync-staging-via-gabi.sh
else
    echo "   🔄 Transforming org_ids to 'internal' for single-user mode"
    ./sync-staging-via-gabi.sh
fi

# Step 2: Build containers (API and UI)
echo "🔨 Step 2: Building containers..."
cd "$MIGRATION_PLANNER_REPO"
DEBUG_MODE=true MIGRATION_PLANNER_API_IMAGE=localhost/migration-planner-api make migration-planner-api-container

# Copy UI modification files to UI repo temporarily
cp "$SCRIPT_DIR/webpack-proxy.patch" "$MIGRATION_PLANNER_UI_REPO/"

if [ "$AUTH_MODE" = "local" ]; then
    # For auth=local, copy the authFetch.ts, token page, and use local dockerfile
    cp "$TOOLS_DIR/auth_local/set-ui-token.html" "$MIGRATION_PLANNER_UI_REPO/"
    cp "$TOOLS_DIR/auth_local/authFetch.ts" "$MIGRATION_PLANNER_UI_REPO/"
    cd "$MIGRATION_PLANNER_UI_REPO"
    podman build -f "$SCRIPT_DIR/auth_local/Dockerfile.ui-local" -t "$UI_IMAGE" .
    rm -f authFetch.ts set-ui-token.html
else
    # For auth=none, use standard dockerfile without authFetch
    cd "$MIGRATION_PLANNER_UI_REPO"
    podman build -f "$SCRIPT_DIR/auth_none/Dockerfile.ui" -t "$UI_IMAGE" .
fi

# Clean up temporary files
rm -f webpack-proxy.patch
cd "$SCRIPT_DIR"

# Step 3: Start debug environment with staging data
echo "🚀 Step 3: Starting debug environment..."
echo "   🔄 Cleaning up any existing containers..."
podman-compose -f "$COMPOSE_FILE" down 2>/dev/null || true
echo "   🏗️  Starting fresh environment..."
podman-compose -f "$COMPOSE_FILE" up -d

echo "⏳ Waiting for environment to be ready..."
sleep 20

echo "🎉 Debug environment ready with staging data!"
echo ""
echo "🔗 Connections:"
echo "  🖥️  UI: http://localhost:3000"
echo "  🌐 API: http://localhost:3443"
echo "  💡 Debugger: localhost:40000"
echo "  🗄️  Database: localhost:5432"
echo ""
echo "📊 Staging data loaded (org_ids transformed to 'internal'):"
echo "  ✅ All tables loaded successfully - see extraction summary above for record counts"
echo ""
echo "🎯 Complete Migration Planner stack ready with real staging data!"
echo "💡 Access the UI at http://localhost:3000 to interact with the loaded staging data"
echo ""
echo "📁 All staging artifacts are now organized in: $SCRIPT_DIR/"
echo "   📂 Scripts and compose files are in the main directory"
echo "   📂 staging-data/: Extracted staging data"
echo ""
if [ "$AUTH_MODE" = "local" ]; then
    echo "🔑 Multi-tenant debugging enabled with local authentication:"
    echo "   📋 Run './generate-user-tokens.sh' to get authentication tokens"
    echo "   🔄 Use different tokens to simulate different organization users"
    echo "   🏢 Original org_ids preserved: 11009103, example, redhat.com, etc."
    echo "   💡 Each token will show org-specific data only"
    echo ""
    echo "🌐 UI Authentication Setup:"
    echo "   1. Open: http://localhost:3000/set-ui-token.html"
    echo "   2. Generate tokens: ./generate-user-tokens.sh"
    echo "   3. Copy a token and paste it in the UI auth page"
    echo "   4. Refresh the Migration Planner UI to use the token"
    echo ""
    echo "🔧 API Testing:"
    echo "   curl -H 'X-Authorization: Bearer \$TOKEN_11009103' http://localhost:3443/api/v1/sources"
else
    echo "👤 Single-user debugging with no authentication:"
    echo "   🏢 All data appears under 'internal' organization"
    echo "   💡 No token required for API calls"
fi
