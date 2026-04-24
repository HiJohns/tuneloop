-- Migration: Add merchants table for merchant management
-- Description: Stores merchant (organization) information aligned with IAM Organization

CREATE TABLE IF NOT EXISTS merchants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    org_id UUID NOT NULL,  -- Corresponding to IAM Organization ID
    name VARCHAR(255) NOT NULL,
    code VARCHAR(100) NOT NULL,  -- Unique code for URL/data isolation
    contact_name VARCHAR(255),
    contact_email VARCHAR(255),
    contact_phone VARCHAR(50),
    admin_uid UUID,  -- Merchant admin user ID
    status VARCHAR(20) DEFAULT 'active',  -- active/inactive
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    UNIQUE (tenant_id, code)
);

-- Create indexes with proper PostgreSQL syntax
CREATE INDEX IF NOT EXISTS idx_merchants_tenant ON merchants (tenant_id);
CREATE INDEX IF NOT EXISTS idx_merchants_org ON merchants (org_id);
CREATE INDEX IF NOT EXISTS idx_merchants_admin ON merchants (admin_uid);

-- Create trigger for updated_at
CREATE OR REPLACE FUNCTION update_merchants_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER merchants_updated_at_trigger
    BEFORE UPDATE ON merchants
    FOR EACH ROW
    EXECUTE FUNCTION update_merchants_updated_at();

-- Comment on table
comment on table merchants is 'Merchant/Organization management table aligned with IAM';
comment on column merchants.code is 'Unique merchant code for URL and data isolation';
comment on column merchants.org_id is 'Corresponding IAM Organization ID';
comment on column merchants.admin_uid is 'Merchant administrator user ID';

