-- Migration: Add TenantID to clients table and create tenants table

-- First create clients table if it doesn't exist
CREATE TABLE IF NOT EXISTS clients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID DEFAULT '00000000-0000-0000-0000-000000000000',
    client_id VARCHAR(100) UNIQUE NOT NULL,
    client_secret VARCHAR(255),
    name VARCHAR(100),
    redirect_uris TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Create index on tenant_id
CREATE INDEX IF NOT EXISTS idx_clients_tenant ON clients(tenant_id);

-- Make tenant_id NOT NULL after adding default
ALTER TABLE clients ALTER COLUMN tenant_id SET NOT NULL;

-- Create tenants table
CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    code VARCHAR(50) UNIQUE,
    status VARCHAR(20) DEFAULT 'active',
    description TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Create index on status
CREATE INDEX IF NOT EXISTS idx_tenants_status ON tenants(status);

-- Create index on code
CREATE INDEX IF NOT EXISTS idx_tenants_code ON tenants(code);
