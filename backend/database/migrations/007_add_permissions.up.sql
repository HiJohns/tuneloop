-- Permissions and Role Management Tables

CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    is_system BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name)
);

CREATE TABLE IF NOT EXISTS permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) UNIQUE NOT NULL,
    category VARCHAR(50) NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS role_permissions (
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE INDEX idx_role_permissions_role ON role_permissions(role_id);
CREATE INDEX idx_role_permissions_permission ON role_permissions(permission_id);

-- Insert default permissions
INSERT INTO permissions (name, category, description) VALUES
    ('dashboard:view', 'dashboard', 'View dashboard'),
    ('assets:view', 'assets', 'View assets'),
    ('assets:edit', 'assets', 'Edit assets'),
    ('assets:delete', 'assets', 'Delete assets'),
    ('leases:view', 'leases', 'View leases'),
    ('leases:create', 'leases', 'Create leases'),
    ('leases:edit', 'leases', 'Edit leases'),
    ('leases:delete', 'leases', 'Delete leases'),
    ('maintenance:view', 'maintenance', 'View maintenance'),
    ('maintenance:assign', 'maintenance', 'Assign maintenance'),
    ('maintenance:complete', 'maintenance', 'Complete maintenance'),
    ('finance:view', 'finance', 'View finance'),
    ('finance:config', 'finance', 'Configure finance'),
    ('users:view', 'users', 'View users'),
    ('users:manage', 'users', 'Manage users'),
    ('settings:view', 'settings', 'View settings'),
    ('settings:edit', 'settings', 'Edit settings')
ON CONFLICT (name) DO NOTHING;

-- Insert default roles
INSERT INTO roles (name, description, is_system) VALUES
    ('OWNER', 'Tenant owner with full access', true),
    ('ADMIN', 'Administrator with management access', true),
    ('TECHNICIAN', 'Maintenance technician', true),
    ('USER', 'Regular user', true)
ON CONFLICT (name) DO NOTHING;

-- Assign default permissions to roles
-- OWNER gets all permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p WHERE r.name = 'OWNER'
ON CONFLICT DO NOTHING;

-- ADMIN gets most permissions except user management
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p 
WHERE r.name = 'ADMIN' AND p.name NOT IN ('users:manage')
ON CONFLICT DO NOTHING;

-- TECHNICIAN gets dashboard and maintenance permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p 
WHERE r.name = 'TECHNICIAN' AND p.category IN ('dashboard', 'maintenance')
ON CONFLICT DO NOTHING;

-- USER gets view-only permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r, permissions p 
WHERE r.name = 'USER' AND p.name LIKE '%:view'
ON CONFLICT DO NOTHING;
