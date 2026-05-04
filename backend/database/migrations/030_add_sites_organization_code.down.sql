-- Migration: Remove organization_code column from sites table
-- Issue #427: Rollback
ALTER TABLE sites DROP COLUMN IF EXISTS organization_code;
DROP INDEX IF EXISTS idx_sites_org_code;