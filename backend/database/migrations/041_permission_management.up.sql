-- Permission Management: replace legacy string-based RBAC with cus_perm bitmap model
-- Issue #660

-- Drop legacy RBAC tables (string-based)
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;

-- Create new roles table (cus_perm bitmap model, IAM cache)
CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    iam_template_id VARCHAR(100),
    name VARCHAR(100) NOT NULL,
    code VARCHAR(50) NOT NULL,
    cus_perm_codes TEXT[] DEFAULT '{}',
    is_system BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, code)
);

-- Add individual cus_perm to site_members
ALTER TABLE site_members ADD COLUMN IF NOT EXISTS cus_perm_codes TEXT[] DEFAULT '{}';

-- Seed system roles for all existing tenants
INSERT INTO roles (tenant_id, name, code, cus_perm_codes, is_system)
SELECT t.id, '商户管理员', 'merchant_admin', ARRAY['instrument:create','instrument:read','instrument:update','instrument:delete','instrument:price','instrument:maintain','order:create','order:read','order:update','order:cancel'], true
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM roles r WHERE r.tenant_id = t.id AND r.code = 'merchant_admin');

INSERT INTO roles (tenant_id, name, code, cus_perm_codes, is_system)
SELECT t.id, '网点管理员', 'site_admin', ARRAY['instrument:create','instrument:read','instrument:update','instrument:price','instrument:maintain','order:read','order:update','order:cancel'], true
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM roles r WHERE r.tenant_id = t.id AND r.code = 'site_admin');

INSERT INTO roles (tenant_id, name, code, cus_perm_codes, is_system)
SELECT t.id, '网点员工', 'site_member', ARRAY['instrument:create','instrument:read','instrument:update','instrument:maintain','order:create','order:read','order:update'], true
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM roles r WHERE r.tenant_id = t.id AND r.code = 'site_member');

INSERT INTO roles (tenant_id, name, code, cus_perm_codes, is_system)
SELECT t.id, '维修工程师', 'worker', ARRAY['instrument:read','instrument:maintain'], true
FROM tenants t
WHERE NOT EXISTS (SELECT 1 FROM roles r WHERE r.tenant_id = t.id AND r.code = 'worker');
