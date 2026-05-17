-- Migration 035: Add phone, address to merchants, make code optional
-- Issue #564

ALTER TABLE merchants ADD COLUMN IF NOT EXISTS phone VARCHAR(50);
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS address TEXT;
ALTER TABLE merchants ALTER COLUMN code DROP NOT NULL;
DROP INDEX IF EXISTS idx_merchants_tenant_code;
