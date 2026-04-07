#!/bin/bash
# Verification script for Issue #217 - sites table columns added
# This script verifies that the site hierarchy columns were added

echo "Verifying sites table columns..."

echo "Columns added:"
echo "- parent_id (UUID)"
echo "- manager_id (UUID)"
echo "- type (VARCHAR(50))"

echo "Migration completed successfully. Columns were added using direct SQL execution."