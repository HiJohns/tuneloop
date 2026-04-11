#!/bin/bash

# Load environment variables from main .env file
if [ -f ~/tuneloop/.env ]; then
  source ~/tuneloop/.env
fi

# Set defaults for missing variables
POSTGRES_HOST=${POSTGRES_HOST:-localhost}
POSTGRES_PORT=${POSTGRES_PORT:-5432}
POSTGRES_USER=${POSTGRES_USER:-tuneloop_user}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-tune_secret_2026}
TUNELOOP_DB=${TUNELOOP_DB:-tuneloop_db}

echo "Connecting to PostgreSQL..."
echo "Host: $POSTGRES_HOST:$POSTGRES_PORT"
echo "Database: $TUNELOOP_DB"
echo "User: $POSTGRES_USER"

# Check if psql is available
if ! command -v psql &> /dev/null; then
  echo "Error: psql command not found. Please install PostgreSQL client."
  exit 1
fi

# Connect and get count
COUNT=$(psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$TUNELOOP_DB" -t -c "SELECT COUNT(*) FROM instruments;" 2>&1)

if [[ $? -ne 0 ]]; then
  echo "Error connecting to database: $COUNT"
  exit 1
fi

COUNT=$(echo $COUNT | tr -d ' ')

if [ "$COUNT" = "0" ]; then
  echo "ℹ Instrument table is already empty"
  exit 0
fi

echo "⚠ Found $COUNT instruments in the table"
read -p "Are you sure you want to delete ALL instruments? (type 'YES' to confirm): " response

if [ "$response" != "YES" ]; then
  echo "✓ Operation cancelled"
  exit 0
fi

# Execute TRUNCATE
RESULT=$(psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$TUNELOOP_DB" -c "TRUNCATE TABLE instruments RESTART IDENTITY CASCADE;" 2>&1)

if [[ $? -ne 0 ]]; then
  echo "Error truncating table: $RESULT"
  exit 1
fi

echo "✓ Successfully cleaned instrument table"

# Verify
COUNT=$(psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$TUNELOOP_DB" -t -c "SELECT COUNT(*) FROM instruments;" 2>&1)
COUNT=$(echo $COUNT | tr -d ' ')
echo "ℹ Instrument table now contains $COUNT records"
