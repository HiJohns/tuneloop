INSERT INTO merchant_pricing_configs (tenant_id, template_id, config)
SELECT id, '00000000-0000-0000-0000-000000000001',
       '{"deposit_mode":"ratio","deposit_multiplier":7}'::jsonb
FROM tenants
WHERE NOT EXISTS (
  SELECT 1 FROM merchant_pricing_configs WHERE tenant_id = tenants.id
);
