-- Migration: Add TenantID to clients table and create tenants table

-- Add TenantID column to clients table
ALTER TABLE clients 
ADD COLUMN IF NOT EXISTS tenant_id UUID DEFAULT '00000000-0000-0000-0000-000000000000';

-- Create index on tenant_id
CREATE INDEX IF NOT EXISTS idx_clients_tenant ON clients(tenant_id);

-- Make tenant_id NOT NULL after adding default
ALTER TABLE clients ALTER COLUMN tenant_id SET NOT NULL;

-- Create tenants table
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    status VARCHAR(20) DEFAULT 'active',
    description TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Create index on status
CREATE INDEX IF NOT EXISTS idx_tenants_status ON tenants(status);
