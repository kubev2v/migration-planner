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
cd /staging-data

# Load in FK-safe order so parent tables exist before children
files=(
  "sources.sql"       # parents for many tables
  "agents.sql"        # FK -> sources
  "assessments.sql"   # FK -> sources (nullable, but still enforced when non-null)
  "snapshots.sql"     # FK -> assessments
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

echo "All staging data loaded!"