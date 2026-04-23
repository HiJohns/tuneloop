-- Migration: Add site_members table for many-to-many user-site relationship
-- Description: Supports users belonging to multiple sites with different roles

CREATE TABLE IF NOT EXISTS site_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    site_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role VARCHAR(20) DEFAULT 'Staff',  -- Manager or Staff
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    FOREIGN KEY (site_id) REFERENCES sites(id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    
    -- Constraint: One user can only have one record per site
    UNIQUE (tenant_id, site_id, user_id),
    
    INDEX idx_site_members_site (site_id, user_id),
    INDEX idx_site_members_user (user_id, site_id),
    INDEX idx_site_members_tenant (tenant_id)
);

-- Create trigger for updated_at
CREATE OR REPLACE FUNCTION update_site_members_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER site_members_updated_at_trigger
    BEFORE UPDATE ON site_members
    FOR EACH ROW
    EXECUTE FUNCTION update_site_members_updated_at();

-- Comment on table
comment on table site_members is 'Many-to-many relationship between users and sites';
comment on column site_members.role is 'Member role: Manager or Staff';
comment on column site_members.site_id is 'Site ID';
comment on column site_members.user_id is 'User ID';

-- Insert sample data for testing (commented out)
/*
INSERT INTO site_members (tenant_id, site_id, user_id, role) VALUES
    ('tenant-uuid-1', 'site-uuid-1', 'user-uuid-1', 'Manager'),
    ('tenant-uuid-1', 'site-uuid-1', 'user-uuid-2', 'Staff');
*/

