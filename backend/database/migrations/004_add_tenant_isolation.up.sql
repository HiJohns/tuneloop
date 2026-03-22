-- Add tenant_id to all business tables for multi-tenant isolation
-- This migration enforces data isolation between tenants

-- Add tenant_id to instruments table
ALTER TABLE instruments 
ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
ADD COLUMN IF NOT EXISTS org_id UUID;

CREATE INDEX IF NOT EXISTS idx_instruments_tenant ON instruments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_instruments_org ON instruments(org_id);

-- Add tenant_id to orders table
ALTER TABLE orders 
ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
ADD COLUMN IF NOT EXISTS org_id UUID;

CREATE INDEX IF NOT EXISTS idx_orders_tenant ON orders(tenant_id);
CREATE INDEX IF NOT EXISTS idx_orders_org ON orders(org_id);

-- Add tenant_id to sites table
ALTER TABLE sites 
ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
ADD COLUMN IF NOT EXISTS org_id UUID;

CREATE INDEX IF NOT EXISTS idx_sites_tenant ON sites(tenant_id);
CREATE INDEX IF NOT EXISTS idx_sites_org ON sites(org_id);

-- Add tenant_id to maintenance_tickets table
ALTER TABLE maintenance_tickets 
ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
ADD COLUMN IF NOT EXISTS org_id UUID;

CREATE INDEX IF NOT EXISTS idx_maintenance_tenant ON maintenance_tickets(tenant_id);
CREATE INDEX IF NOT EXISTS idx_maintenance_org ON maintenance_tickets(org_id);

-- Add tenant_id to brand_configs table
ALTER TABLE brand_configs 
ADD COLUMN IF NOT EXISTS tenant_id UUID;

CREATE INDEX IF NOT EXISTS idx_brand_configs_tenant ON brand_configs(tenant_id);

-- Add tenant_id to ownership_certificates table
ALTER TABLE ownership_certificates 
ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
ADD COLUMN IF NOT EXISTS org_id UUID;

CREATE INDEX IF NOT EXISTS idx_certificates_tenant ON ownership_certificates(tenant_id);

-- Add tenant_id to technicians table
ALTER TABLE technicians 
ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
ADD COLUMN IF NOT EXISTS org_id UUID;

CREATE INDEX IF NOT EXISTS idx_technicians_tenant ON technicians(tenant_id);

-- Add tenant_id to site_images table
ALTER TABLE site_images 
ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

CREATE INDEX IF NOT EXISTS idx_site_images_tenant ON site_images(tenant_id);

-- Add tenant_id to inventory_transfers table
ALTER TABLE inventory_transfers 
ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
ADD COLUMN IF NOT EXISTS org_id UUID;

CREATE INDEX IF NOT EXISTS idx_inventory_transfers_tenant ON inventory_transfers(tenant_id);
CREATE INDEX IF NOT EXISTS idx_inventory_transfers_org ON inventory_transfers(org_id);

-- Add tenant_id to categories table (for tenant-specific categories)
ALTER TABLE categories 
ADD COLUMN IF NOT EXISTS tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

CREATE INDEX IF NOT EXISTS idx_categories_tenant ON categories(tenant_id);
