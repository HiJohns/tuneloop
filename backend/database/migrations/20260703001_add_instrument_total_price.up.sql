-- Add total_price to instruments and seed default pricing template

ALTER TABLE instruments ADD COLUMN IF NOT EXISTS total_price DECIMAL(12,2);

-- Seed system default pricing template (if not exists)
INSERT INTO pricing_templates (id, code, name, config_schema, is_active, is_system_default, created_at)
SELECT
  gen_random_uuid(),
  'system_default',
  '系统默认定价',
  '{"tiers":[{"days_max":30,"discount_percent":0},{"days_max":180,"discount_percent":5},{"days_max":-1,"discount_percent":10}],"deposit_mode":"ratio","deposit_ratio":0.3,"overdue_multiplier":2.0}',
  true,
  true,
  NOW()
WHERE NOT EXISTS (SELECT 1 FROM pricing_templates WHERE is_system_default = true);
