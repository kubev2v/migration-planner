#!/bin/bash
set -euo pipefail

echo "Loading staging data..."
DB_HOST="${DB_HOST:-planner-db-staging}"
DB_USER="${DB_USER:-demouser}"
DB_NAME="${DB_NAME:-planner}"
DB_PASSWORD="${DB_PASSWORD:-demopass}"

# Wait for DB to be ready (max ~60s)
wait_for_db() {
  local max=60 i=0
  until PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c 'SELECT 1' >/dev/null 2>&1; do
    ((i++)); if (( i>=max )); then echo "❌ Database not ready after ${max}s at $DB_HOST"; exit 1; fi
    sleep 1
  done
}
wait_for_db

# Copy staging data to writable location (staging-data is mounted read-only)
echo "Copying staging data to writable location..."
rm -rf /tmp/staging-data
cp -r /staging-data /tmp/staging-data
cd /tmp/staging-data

# Sanitize known bad zero-UUID records that can break FK/transactions
# Only remove rows where zero-UUID appears in key columns (id, source_id, assessment_id)
# Don't remove rows where zero-UUID only appears in JSON inventory data

if [ -f "sources.sql" ]; then
  # Remove sources with zero-UUID as id (first column after VALUES)
  # Pattern matches: VALUES ('00000000...', ensuring it's the first value
  sed -i "/^INSERT INTO sources.*VALUES ('00000000-0000-0000-0000-000000000000',/d" "sources.sql"
fi

if [ -f "assessments.sql" ]; then
  # Remove assessments with zero-UUID as id (first column after VALUES)
  # Pattern matches: VALUES ('00000000...', ensuring it's the first value
  sed -i "/^INSERT INTO assessments.*VALUES ('00000000-0000-0000-0000-000000000000',/d" "assessments.sql"
fi

if [ -f "snapshots.sql" ]; then
  # For snapshots, only remove if assessment_id (4th column) is zero-UUID
  # Pattern: VALUES (id, created_at, inventory, '00000000-0000-0000-0000-000000000000'
  # We need to match the 4th value being the zero-UUID
  # This is complex, so we'll use a more targeted approach in the cleanup SQL below
  echo "Snapshots will be cleaned up via SQL after load..."
fi

# Load in FK-safe order so parent tables exist before children
files=(
  "sources.sql"       # parents for many tables
  "assessments.sql"   # FK -> sources
  "snapshots.sql"     # FK -> assessments
  "agents.sql"        # FK -> sources
  "image_infras.sql"  # FK -> sources
  "labels.sql"        # FK -> sources
  "keys.sql"          # independent
)

for f in "${files[@]}"; do
  if [ -f "$f" ]; then
    echo "Loading $f..."
    # Stop on first SQL error and run each file in a single transaction
    PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 -1 -f "$f"
    echo "$f loaded"
  else
    echo "Skipping $f (not found)"
  fi
done

# Apply org_id consolidation for auth=local mode
# JWT tokens use consolidated org_ids, so database must match
# Consolidate specific org_ids to 'redhat.com' to match token generation logic
echo "Applying org_id consolidation for auth=local compatibility..."

PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" <<'EOSQL'
-- Append org_id to ALL source/assessment names being consolidated to guarantee uniqueness
-- This prevents unique constraint violations on (name, org_id) when consolidating to redhat.com
UPDATE sources 
SET name = name || '-org' || org_id
WHERE org_id IN ('11009103', '13872092', '19194072', '18692352', '19006254', '19009423', '19010322', '19012400');

UPDATE assessments 
SET name = name || '-org' || org_id
WHERE org_id IN ('11009103', '13872092', '19194072', '18692352', '19006254', '19009423', '19010322', '19012400');

-- Now consolidate org_id in sources table
UPDATE sources 
SET org_id = 'redhat.com'
WHERE org_id IN ('11009103', '13872092', '19194072', '18692352', '19006254', '19009423', '19010322', '19012400');

-- Consolidate org_id in assessments table  
UPDATE assessments 
SET org_id = 'redhat.com'
WHERE org_id IN ('11009103', '13872092', '19194072', '18692352', '19006254', '19009423', '19010322', '19012400');

-- Also set email_domain to redhat.com for consistency
UPDATE sources 
SET email_domain = 'redhat.com'
WHERE org_id = 'redhat.com';
EOSQL

echo "✅ Org consolidation complete"

# Remove zero-UUID records and orphaned FKs for data consistency
echo "Cleaning up data integrity issues..."

PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 <<'EOSQL'
-- Remove the all-zero UUID source (legacy placeholder) if present
DELETE FROM sources WHERE id = '00000000-0000-0000-0000-000000000000';

-- Remove any assessments whose source no longer exists (preserve FK consistency)
DELETE FROM assessments a WHERE source_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM sources s WHERE s.id = a.source_id);

-- Remove any snapshots whose assessment no longer exists
DELETE FROM snapshots s WHERE NOT EXISTS (SELECT 1 FROM assessments a WHERE a.id = s.assessment_id);

-- Reset snapshots sequence (explicit IDs were loaded)
SELECT setval(pg_get_serial_sequence('snapshots','id'), COALESCE(MAX(id),0)) FROM snapshots;
EOSQL

echo "✅ Data cleanup complete"
echo "All staging data loaded!"