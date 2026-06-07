-- Migration: Add is_system_default to pricing_templates
-- Up

ALTER TABLE pricing_templates ADD COLUMN IF NOT EXISTS is_system_default BOOLEAN DEFAULT false;

-- Ensure at most one system default template
CREATE UNIQUE INDEX IF NOT EXISTS idx_pricing_templates_system_default ON pricing_templates (is_system_default) WHERE is_system_default = true;

-- Mark the default tiered_discount_v1 template as system default
UPDATE pricing_templates SET is_system_default = true WHERE code = 'tiered_discount_v1';
