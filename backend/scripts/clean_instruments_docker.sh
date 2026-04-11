#!/bin/bash

# Load environment variables
source ~/tuneloop/.env

# Set defaults
POSTGRES_HOST=${POSTGRES_HOST:-localhost}
POSTGRES_PORT=${POSTGRES_PORT:-5432}
CONTAINER_NAME="jobmaster-postgres"

echo "Connecting to PostgreSQL in Docker container: $CONTAINER_NAME"
echo "Database: $TUNELOOP_DB"
echo "User: $POSTGRES_USER"
echo ""

# Count instruments (remove leading/trailing whitespace)
COUNT=$(docker exec -i $CONTAINER_NAME psql -U "$POSTGRES_USER" -d "$TUNELOOP_DB" -t -c "SELECT COUNT(*) FROM instruments;" 2>&1 | grep -E '[0-9]+' | tr -d '[:space:]')

if [ "$COUNT" = "0" ]; then
  echo "ℹ Instrument table is already empty"
  exit 0
fi

echo "⚠ Found $COUNT instruments in the table"
read -p "Are you sure you want to delete ALL instruments? (type 'YES' to confirm): " response

if [ "$response" = "YES" ]; then
  echo "Cleaning instrument table..."
  docker exec -i $CONTAINER_NAME psql -U "$POSTGRES_USER" -d "$TUNELOOP_DB" -c "TRUNCATE TABLE instruments RESTART IDENTITY CASCADE;" 2>&1
  
  # Verify
  NEW_COUNT=$(docker exec -i $CONTAINER_NAME psql -U "$POSTGRES_USER" -d "$TUNELOOP_DB" -t -c "SELECT COUNT(*) FROM instruments;" 2>&1 | grep -E '[0-9]+' | tr -d '[:space:]')
  echo ""
  echo "✓ Instrument table cleaned successfully"
  echo "ℹ Instrument table now contains $NEW_COUNT records"
else
  echo "✓ Operation cancelled"
  exit 0
fi
