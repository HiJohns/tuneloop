-- Migration: Remove is_system_default from pricing_templates
-- Down

DROP INDEX IF EXISTS idx_pricing_templates_system_default;
ALTER TABLE pricing_templates DROP COLUMN IF EXISTS is_system_default;
