-- Migration 066: Clean up stale property records and ensure Chinese default properties
--
-- Problem: Migrations 013 and 021 created English-named properties (Brand/brand/Model/model)
-- with inconsistent casing, resulting in duplicates. Properties should use Chinese names:
-- 品牌、型号、产地, and should be created only once (not from two separate migrations).
--
-- See Issue #913, Sub-task #914 for details.

-- Step 1: Delete stale English-named property records and their options
DELETE FROM property_options WHERE property_name IN ('Brand', 'brand', 'Model', 'model');
DELETE FROM properties WHERE name IN ('Brand', 'brand', 'Model', 'model');

-- Step 2: Ensure Chinese default properties exist for platform-wide use
-- Uses ON CONFLICT DO NOTHING so it's idempotent.
-- Pick an existing tenant_id as creator metadata (first available).
DO $$
DECLARE
  first_tenant UUID;
BEGIN
  SELECT tenant_id INTO first_tenant FROM properties LIMIT 1;
  IF first_tenant IS NULL THEN
    first_tenant := '00000000-0000-0000-0000-000000000000'::UUID;
  END IF;

  INSERT INTO properties (id, tenant_id, name, property_type, is_required, caption, status, created_at, updated_at)
  VALUES (gen_random_uuid(), first_tenant, '品牌', 'string', false, '品牌', 'active', NOW(), NOW())
  ON CONFLICT (tenant_id, name) DO NOTHING;

  INSERT INTO properties (id, tenant_id, name, property_type, is_required, caption, status, created_at, updated_at)
  VALUES (gen_random_uuid(), first_tenant, '型号', 'string', false, '型号', 'active', NOW(), NOW())
  ON CONFLICT (tenant_id, name) DO NOTHING;

  INSERT INTO properties (id, tenant_id, name, property_type, is_required, caption, status, created_at, updated_at)
  VALUES (gen_random_uuid(), first_tenant, '产地', 'string', false, '产地', 'active', NOW(), NOW())
  ON CONFLICT (tenant_id, name) DO NOTHING;
END $$;
