-- Migration: Add organization_code column to sites table
-- Issue #427: Bulk import needs business code field separate from IAM UUID
ALTER TABLE sites ADD COLUMN IF NOT EXISTS organization_code VARCHAR(255);
CREATE INDEX IF NOT EXISTS idx_sites_org_code ON sites(organization_code);