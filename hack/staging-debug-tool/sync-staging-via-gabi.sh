#!/bin/bash
# sync-staging-via-gabi.sh (complete table extraction)

set -Eeuo pipefail
trap 'echo "sync-staging-via-gabi.sh: error on line $LINENO" >&2' ERR

# Configuration
GABI_URL="https://gabi-assisted-migration-stage.apps.crcs02ue1.urby.p1.openshiftapps.com/query"
# Write outputs next to this script inside the repo (hack/staging-debug-tool/staging-data)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="$SCRIPT_DIR/staging-data"

# Check for required GABI_TOKEN environment variable
if [ -z "$GABI_TOKEN" ]; then
    echo "❌ Error: GABI_TOKEN environment variable is not set"
    echo "💡 Please set it with: export GABI_TOKEN='your_token_here'"
    exit 1
fi

mkdir -p "$OUTPUT_DIR"

echo "🔄 Extracting staging data via GABI..."
echo "📁 Output directory: $OUTPUT_DIR"

# Function to escape SQL strings
escape_sql() {
    echo "$1" | sed "s/'/''/g"
}

# Function to query GABI and save as SQL
query_and_convert() {
    table=$1
    local query=$2
    local file="$OUTPUT_DIR/${table}.sql"
    
    echo "📥 Extracting $table..."
    
    # Query GABI API and save raw response
    local json_file="/tmp/gabi_response_${table}.json"
    # Add a cache-busting SQL comment per table to avoid prepared-plan reuse across tables
    local query_with_hint="/* table: ${table} */ ${query}"
    curl -s -H "Authorization: Bearer $GABI_TOKEN" -H "Content-Type: application/json" \
        --data "$(jq -n --arg query "$query_with_hint" '{query:$query}')" \
        "$GABI_URL" > "$json_file"
    
    # Check for errors
    local error_msg=$(jq -r '.error // empty' "$json_file")
    if [ -n "$error_msg" ]; then
        # Workaround for GABI prepared statement cache error by selecting explicit columns
        if echo "$error_msg" | grep -qi "cached plan must not change result type"; then
            # Fetch explicit column list from information_schema
            local cols_json="/tmp/gabi_cols_${table}.json"
            local cols_query="/* cols: ${table} */ SELECT column_name FROM information_schema.columns WHERE table_schema = 'public' AND table_name = '${table}' ORDER BY ordinal_position;"
            curl -s -H "Authorization: Bearer $GABI_TOKEN" -H "Content-Type: application/json" \
                --data "$(jq -n --arg query "$cols_query" '{query:$query}')" \
                "$GABI_URL" > "$cols_json"
            # Build comma-separated column list
            local column_list=$(jq -r '[.result[]?[0]] // [.results[]?[0]] | select(length>0) | join(", ")' "$cols_json")
            rm -f "$cols_json"
            if [ -n "$column_list" ]; then
                # Retry with explicit column list and quoted table name
                local explicit_query="/* table: ${table} explicit cols */ SELECT ${column_list} FROM \"${table}\";"
                curl -s -H "Authorization: Bearer $GABI_TOKEN" -H "Content-Type: application/json" \
                    --data "$(jq -n --arg query "$explicit_query" '{query:$query}')" \
                    "$GABI_URL" > "$json_file"
                error_msg=$(jq -r '.error // empty' "$json_file")
            fi
        fi
        # If still failing, skip table
        if [ -n "$error_msg" ]; then
            echo "⚠️  Skipping $table: $error_msg"
            rm -f "$json_file"
            return 0
        fi
    fi
    
    # Check if we have data (try both .result and .results)
    local row_count=$(jq '.result | length // (.results | length // 0)' "$json_file")
    if [ "$row_count" -le 1 ]; then
        echo "⚠️  No data found for $table"
        echo "-- No data for table: $table" > "$file"
        rm -f "$json_file"
        return 0
    fi
    
    # Generate SQL header
    echo "-- Data for table: $table" > "$file"
    echo "TRUNCATE TABLE $table CASCADE;" >> "$file"
    echo "" >> "$file"
    
    # Get headers and index them
    local headers
    headers=$(jq -r '(.result[0] // .results[0]) | join(", ")' "$json_file")
    IFS=',' read -r -a HEADER_ARR <<<"$(echo "$headers" | tr -d ' ')"
    
    # Convert each data row to INSERT statement
    local data_rows=$((row_count - 1))
    for ((i=1; i<=data_rows; i++)); do
        # Build VALUES clause by processing each column
        local values=""
        local col_count=$(jq "(.result[0] // .results[0]) | length" "$json_file")
        
        for ((j=0; j<col_count; j++)); do
            local value
            value=$(jq -r "(.result[$i] // .results[$i])[$j]" "$json_file")
            
            # Handle different value types
            local col_name="${HEADER_ARR[$j]}"
            if [ "$value" = "null" ] || [ "$value" = "" ]; then
                values="${values}NULL"
            else
                # Normalize org_id when not preserving orgs
                if [ "$col_name" = "org_id" ] && [ "${PRESERVE_ORGS:-}" != "true" ]; then
                    values="${values}'internal'"
                else
                    local escaped_value
                    escaped_value=$(escape_sql "$value")
                    values="${values}'${escaped_value}'"
                fi
            fi
            
            # Add comma if not last column
            if [ $j -lt $((col_count - 1)) ]; then
                values="${values}, "
            fi
        done
        
        echo "INSERT INTO $table ($headers) VALUES ($values);" >> "$file"
    done
    
    echo "✅ $table exported: $data_rows records → $file"
    rm -f "$json_file"
    
    # Post-process sources table for schema compatibility
    if [ "$table" = "sources" ]; then
        echo "🔧 Applying schema compatibility fixes for sources table..."
        
        # Remove extra columns from headers and corresponding values from all INSERT statements
        sed -i.bak '
            # Remove extra columns from headers
            s/, ssh_public_key, image_token_key//g
            # Remove corresponding values from INSERT statements - remove last 3 values before closing paren
            s/, [^,)]*, [^,)]*);$/);/g
        ' "$file"
        
        # org_id normalization now happens during generation (see loop above)
        if [ "${PRESERVE_ORGS:-}" = "true" ]; then
          echo "✅ Sources table org_ids preserved for multi-tenant debugging"
        else
          echo "✅ Sources table org_ids normalized during generation"
        fi
        
        # Add unique suffixes to names only in single-tenant mode to avoid duplicates
        if [ "${PRESERVE_ORGS:-}" != "true" ]; then
            counter=1
            while IFS= read -r line; do
                if [[ $line == INSERT\ INTO\ sources* ]]; then
                    # Only add suffix to the name (5th value), not to all 'internal' strings
                    echo "$line" | sed "s/VALUES (\([^,]*\), \([^,]*\), \([^,]*\), \([^,]*\), '\([^']*\)'/VALUES (\1, \2, \3, \4, '\5-$counter'/g"
                    ((counter++))
                else
                    echo "$line"
                fi
            done < "$file" > "${file}.tmp" && mv "${file}.tmp" "$file"
        fi
        
        rm -f "${file}.bak"
        
        # Replace any zero-UUID INSERT with a safe minimal row and set org to 'default' (both auth modes)
        sed -i "/^INSERT INTO sources.*'00000000-0000-0000-0000-000000000000'/d" "$file"
        cat >> "$file" <<'EOF'
INSERT INTO sources (id, created_at, updated_at, deleted_at, name, v_center_id, username, org_id, inventory, on_premises, email_domain) VALUES ('00000000-0000-0000-0000-000000000000', now(), now(), NULL, 'Example-22', NULL, NULL, 'default', NULL, 'false', NULL);
EOF
    fi
    
    # Post-process assessments table for org_id compatibility
    if [ "$table" = "assessments" ]; then
        echo "🔧 Applying schema compatibility fixes for assessments table..."
        
        # org_id normalization now happens during generation (see loop above)
        if [ "${PRESERVE_ORGS:-}" = "true" ]; then
            # Consolidate RH numeric orgs to redhat.com for parity with sources
            sed -i "s/'\(11009103\|13872092\|19194072\|18692352\|19006254\|19009423\|19010322\|19012400\)'/'redhat.com'/g" "$file"
            echo "✅ Assessments table org_ids consolidated to 'redhat.com' where applicable"
        else
            echo "✅ Assessments table org_ids normalized during generation"
            
            # Add unique suffixes to names to avoid (org_id, name) uniqueness collisions
            counter=1
            while IFS= read -r line; do
                if [[ $line == INSERT\ INTO\ assessments* ]]; then
                    # Only add suffix to the name (4th value)
                    echo "$line" | sed "s/VALUES (\([^,]*\), \([^,]*\), \([^,]*\), '\([^']*\)'/VALUES (\1, \2, \3, '\4-${counter}'/g"
                    ((counter++))
                else
                    echo "$line"
                fi
            done < "$file" > "${file}.tmp" && mv "${file}.tmp" "$file"
        fi
    fi
}

# Extract all relevant tables
echo ""
echo "🗃️  Extracting all staging database tables..."

# Core application tables
query_and_convert "sources" "SELECT * FROM sources;"
query_and_convert "assessments" "SELECT * FROM assessments;"
query_and_convert "agents" "SELECT * FROM agents;"
query_and_convert "keys" "SELECT * FROM keys;"
query_and_convert "labels" "SELECT * FROM labels;"
query_and_convert "snapshots" "SELECT * FROM snapshots;"
query_and_convert "image_infras" "SELECT * FROM image_infras;"

# Skip goose_db_version (just migration tracking)

echo ""
echo "✅ All staging data extracted to $OUTPUT_DIR/"
echo "📋 Files created:"
ls -la "$OUTPUT_DIR"/*.sql 2>/dev/null || echo "No SQL files found"

# Show summary
echo ""
echo "📊 Data extraction summary:"
for file in "$OUTPUT_DIR"/*.sql; do
    if [ -f "$file" ]; then
        count=$(grep -c "^INSERT INTO" "$file" 2>/dev/null || echo "0")
        table=$(basename "$file" .sql)
        echo "  📄 $table: $count records"
    fi
done
