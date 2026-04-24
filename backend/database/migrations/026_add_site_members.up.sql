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
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Create unique constraint (skip if already exists, will fail but that's ok)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint 
        WHERE conname = 'idx_site_members_unique'
    ) THEN
        ALTER TABLE site_members ADD CONSTRAINT idx_site_members_unique UNIQUE (tenant_id, site_id, user_id);
    END IF;
END $$;

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_site_members_site ON site_members (site_id, user_id);
CREATE INDEX IF NOT EXISTS idx_site_members_user ON site_members (user_id, site_id);
CREATE INDEX IF NOT EXISTS idx_site_members_tenant ON site_members (tenant_id);

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

