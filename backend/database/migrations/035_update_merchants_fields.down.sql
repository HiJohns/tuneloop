-- Migration 035 rollback: revert merchants changes

ALTER TABLE merchants DROP COLUMN IF EXISTS phone;
ALTER TABLE merchants DROP COLUMN IF EXISTS address;
ALTER TABLE merchants ALTER COLUMN code SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_merchants_tenant_code ON merchants(tenant_id, code);
