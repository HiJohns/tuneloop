-- Migration 021: Rollback properties schema refactoring
-- Issue #276: Database schema rollback

-- ========================================
-- 1. Rollback property_options table
-- ========================================

-- Drop foreign key constraint
ALTER TABLE property_options DROP CONSTRAINT IF EXISTS fk_property_options_properties;

-- Add property_id column back
ALTER TABLE property_options ADD COLUMN IF NOT EXISTS property_id UUID;

-- Try to migrate data back (best effort - may not be perfect)
UPDATE property_options po
SET property_id = p.id::uuid
FROM properties p
WHERE po.property_name = p.name AND po.tenant_id = p.tenant_id AND po.property_id IS NULL;

-- Drop property_name column
ALTER TABLE property_options DROP COLUMN IF EXISTS property_name;

-- Drop index
DROP INDEX IF EXISTS idx_property_options_property_name;

-- ========================================
-- 2. Rollback instrument_properties table
-- ========================================

-- Add property_id column back
ALTER TABLE instrument_properties ADD COLUMN IF NOT EXISTS property_id UUID;

-- Try to migrate data back (best effort - may not be perfect)
UPDATE instrument_properties ip
SET property_id = p.id::uuid
FROM properties p
WHERE ip.property_name = p.name AND ip.tenant_id = p.tenant_id AND ip.property_id IS NULL;

-- Drop property_name column
ALTER TABLE instrument_properties DROP COLUMN IF EXISTS property_name;

-- Drop index
DROP INDEX IF EXISTS idx_instrument_properties_property_name;

-- ========================================
-- 3. Rollback properties table
-- ========================================

-- Remove foreign key constraint
ALTER TABLE properties DROP CONSTRAINT IF EXISTS uniq_properties_tenant_name;

-- Note: We cannot easily add back the id column as primary key in PostgreSQL
-- This is a limitation of the rollback - in production, we'd need a more sophisticated approach

-- Drop caption and status columns
ALTER TABLE properties DROP COLUMN IF EXISTS caption;
ALTER TABLE properties DROP COLUMN IF EXISTS status;

-- ========================================
-- 4. Rollback instruments table
-- ========================================

-- Drop properties JSONB column
ALTER TABLE instruments DROP COLUMN IF EXISTS properties;

