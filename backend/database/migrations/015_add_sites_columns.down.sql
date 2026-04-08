-- Migration 015: Down migration for sites table columns
-- Issue #217: Rollback sites columns addition

-- Remove type column
ALTER TABLE sites DROP COLUMN IF EXISTS type;

-- Remove manager_id column
ALTER TABLE sites DROP COLUMN IF EXISTS manager_id;

-- Remove parent_id column
ALTER TABLE sites DROP COLUMN IF EXISTS parent_id;