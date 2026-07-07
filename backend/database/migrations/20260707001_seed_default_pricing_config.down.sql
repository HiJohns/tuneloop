DELETE FROM merchant_pricing_configs
WHERE (config->>'deposit_ratio' IS NOT NULL OR config->>'deposit_multiplier' IS NOT NULL)
  AND template_id = '00000000-0000-0000-0000-000000000001';
