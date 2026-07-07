DELETE FROM merchant_pricing_configs
WHERE config->>'deposit_multiplier' = '7'
  AND template_id = '00000000-0000-0000-0000-000000000001';
