#!/bin/bash

# Script to fix dirty migration state
# This needs to be run manually to mark migration 013 as completed

echo "⚠️  IMPORTANT: This script will force-fix migration version 13"
echo "Make sure the migration SQL is correct before proceeding"
echo ""

# Check if psql is available
if ! command -v psql &> /dev/null; then
    echo "❌ psql command not found. Please install PostgreSQL client or use another method"
    exit 1
fi

# Get database config from environment or use defaults
DB_HOST=${POSTGRES_HOST:-localhost}
DB_USER=${POSTGRES_USER:-tuneloop}
DB_NAME=${TUNELOOP_DB:-tuneloop}

echo "📊 Checking current migration status..."
psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c "SELECT version, dirty FROM schema_migrations;"

echo ""
echo "📝 Options:"
echo "1. Mark migration 13 as completed (if SQL already applied)"
echo "2. Reset migration 13 and re-run (drop tables if exists and retry)"
echo "3. Exit"
read -p "Select option (1-3): " option

case $option in
    1)
        echo "✅ Marking migration 13 as completed..."
        psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c "UPDATE schema_migrations SET dirty = false WHERE version = 13;"
        if [ $? -eq 0 ]; then
            echo "✅ Migration 13 marked as completed successfully"
        else
            echo "❌ Failed to update migration status"
        fi
        ;;
    2)
        echo "⚠️  Resetting migration 13..."
        psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c "DELETE FROM schema_migrations WHERE version = 13;"
        if [ $? -eq 0 ]; then
            echo "✅ Migration 13 reset. You can now re-run the backend to apply it."
        else
            echo "❌ Failed to reset migration"
        fi
        ;;
    *)
        echo "Exiting without changes"
        exit 0
        ;;
esac
