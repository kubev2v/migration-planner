#!/bin/bash
echo "Loading staging data..."
sleep 3
cd /staging-data
for f in *.sql; do
  if [ -f "$f" ]; then
    echo "Loading $f..."
    PGPASSWORD=demopass psql -h planner-db-staging -U demouser -d planner -f "$f"
    echo "$f loaded"
  fi
done
echo "All staging data loaded!"