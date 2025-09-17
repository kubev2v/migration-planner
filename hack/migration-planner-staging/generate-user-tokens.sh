#!/bin/bash
# generate-user-tokens.sh - Generate authentication tokens for different org users

set -e

# Check if private key file exists (created during local auth setup)
if [ ! -f "auth_local/private-key.txt" ]; then
    echo "❌ Private key file not found!"
    echo "💡 This script only works with local authentication mode."
    echo "   Run debug-with-staging-complete.sh and select local auth first."
    exit 1
fi

# Load environment variables (including repository paths)
source .env-debug

PRIVATE_KEY=$(cat auth_local/private-key.txt)
API_REPO="$MIGRATION_PLANNER_REPO"

echo "🔑 Generating authentication tokens for different organizations..."
echo ""

# Discover organizations from staging data
echo "🔍 Discovering organizations from staging data..."

# Check if database container is running
if ! podman ps --format "{{.Names}}" | grep -q "^planner-db-staging$"; then
    echo "❌ Database container 'planner-db-staging' is not running!"
    echo "💡 Please start the debug environment first:"
    echo "   ./debug-with-staging-complete.sh"
    exit 1
fi

# Query database for unique organizations
echo "📊 Querying database for organizations..."
# Get domains based on migration logic:
# - redhat.com domain groups specific org_ids: 11009103, 13872092, 19194072, 18692352, 19006254, 19009423, 19010322, 19012400
# - All other org_ids get individual tokens
DOMAINS=$(podman exec planner-db-staging psql -U demouser -d planner -t -c "
    SELECT DISTINCT 
        CASE 
            WHEN org_id IN ('11009103', '13872092', '19194072', '18692352', '19006254', '19009423', '19010322', '19012400') THEN 'redhat.com'
            ELSE org_id 
        END as domain
    FROM sources 
    WHERE org_id IS NOT NULL AND org_id != '' 
    ORDER BY domain;
" 2>/dev/null | xargs)

if [ -z "$DOMAINS" ]; then
    echo "⚠️  No organizations found in staging data!"
    echo "💡 This might mean:"
    echo "   - Staging data hasn't been loaded yet"
    echo "   - The sources table is empty"
    echo "   - Database connection failed"
    exit 1
fi

DOMAIN_COUNT=$(echo $DOMAINS | wc -w)
echo "✅ Found $DOMAIN_COUNT domains in staging data"
echo ""

# Generate tokens for discovered domains
echo "=== Domain Tokens ==="
echo ""

# Array to store generated tokens for later use
declare -A TOKENS

for domain in $DOMAINS; do
    echo "📄 DOMAIN: $domain (discovered from staging data)"
    
    # Determine which org_id to use for token generation
    if [ "$domain" = "redhat.com" ]; then
        # For redhat.com domain, pick the first org_id from the grouped list
        ORG_ID=$(podman exec planner-db-staging psql -U demouser -d planner -t -c "
            SELECT org_id FROM sources 
            WHERE org_id IN ('11009103', '13872092', '19194072', '18692352', '19006254', '19009423', '19010322', '19012400')
            LIMIT 1;
        " 2>/dev/null | xargs)
    else
        # For individual domains, the domain name IS the org_id
        ORG_ID="$domain"
    fi
    
    if [ -z "$ORG_ID" ]; then
        echo "❌ Could not find org_id for domain: $domain"
        continue
    fi
    
    # Generate token using the org_id
    TOKEN=$(cd "$API_REPO" && ./bin/planner sso token --private-key "$PRIVATE_KEY" --username testuser --org "$ORG_ID" 2>/dev/null)
    
    if [ $? -ne 0 ] || [ -z "$TOKEN" ]; then
        echo "❌ Failed to generate token for domain: $domain (org: $ORG_ID)"
        continue
    fi
    
    # Create valid bash variable name by sanitizing domain name
    VAR_NAME="TOKEN_$(echo "$domain" | tr '.-' '_' | tr '[:lower:]' '[:upper:]')"
    
    # Store for later use
    TOKENS["$VAR_NAME"]="$TOKEN"
    
    echo "export $VAR_NAME='$TOKEN'"
    echo ""
done

echo "=== Usage Examples ==="
echo ""
echo "# Set tokens in your environment:"
for var_name in "${!TOKENS[@]}"; do
    echo "export $var_name='${TOKENS[$var_name]}'"
done
echo ""
echo "# Use tokens in API calls:"
# Show examples using first few discovered organizations
declare -A examples=(
    ["sources"]="sources"
    ["assessments"]="assessments"
)

example_count=0
for var_name in "${!TOKENS[@]}"; do
    if [ $example_count -eq 0 ]; then
        echo "curl -H \"X-Authorization: Bearer \$$var_name\" http://localhost:3443/api/v1/sources"
    elif [ $example_count -eq 1 ]; then
        echo "curl -H \"X-Authorization: Bearer \$$var_name\" http://localhost:3443/api/v1/assessments"
    elif [ $example_count -eq 2 ]; then
        echo "curl -H \"X-Authorization: Bearer \$$var_name\" http://localhost:3443/api/v1/sources"
        break
    fi
    ((example_count++))
done
echo ""
echo "# Test different organizations to see org-specific data filtering!"
echo ""
echo "💡 Each token represents a user from a different organization."
echo "   You should only see data belonging to that organization."