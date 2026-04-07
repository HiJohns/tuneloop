-- Add properties table if not exists (in case it wasn't in initial schema)
CREATE TABLE IF NOT EXISTS properties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name VARCHAR(100) NOT NULL,
    property_type VARCHAR(20) NOT NULL,
    is_required BOOLEAN DEFAULT false,
    unit VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_properties_tenant ON properties(tenant_id);
CREATE INDEX IF NOT EXISTS idx_properties_name ON properties(name);

-- Get or create default tenant ID
create or replace function get_default_tenant()
returns uuid as $$
declare
    tenant_id uuid;
begin
    SELECT id INTO tenant_id FROM tenants WHERE code = 'default' LIMIT 1;
    IF tenant_id IS NULL THEN
        tenant_id := gen_random_uuid();
        INSERT INTO tenants (id, name, code, created_at, updated_at)
        VALUES (tenant_id, 'Default', 'default', NOW(), NOW());
    END IF;
    RETURN tenant_id;
end;
$$ language plpgsql;

-- Add Brand property for instruments
INSERT INTO properties (id, tenant_id, name, property_type, is_required, unit, created_at, updated_at)
VALUES (
    gen_random_uuid(),
    get_default_tenant(),
    'Brand',
    'text',
    true,
    NULL,
    NOW(),
    NOW()
) ON CONFLICT DO NOTHING;

-- Add Model property for instruments
INSERT INTO properties (id, tenant_id, name, property_type, is_required, unit, created_at, updated_at)
VALUES (
    gen_random_uuid(),
    get_default_tenant(),
    'Model',
    'text',
    true,
    NULL,
    NOW(),
    NOW()
) ON CONFLICT DO NOTHING;

-- Drop the helper function
DROP FUNCTION get_default_tenant();
