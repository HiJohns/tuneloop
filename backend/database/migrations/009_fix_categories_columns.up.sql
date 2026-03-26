-- Migration: Add missing columns to categories table
-- Adds Level, Sort, Visible, and TenantID columns

ALTER TABLE categories 
ADD COLUMN IF NOT EXISTS level INT DEFAULT 1,
ADD COLUMN IF NOT EXISTS sort INT DEFAULT 0,
ADD COLUMN IF NOT EXISTS visible BOOLEAN DEFAULT true,
ADD COLUMN IF NOT EXISTS tenant_id UUID;

-- Create index on tenant_id for performance
CREATE INDEX IF NOT EXISTS idx_categories_tenant ON categories(tenant_id);

-- Create index on visible for filtering
CREATE INDEX IF NOT EXISTS idx_categories_visible ON categories(visible);

-- Update existing rows to have default tenant_id (you may need to adjust this)
UPDATE categories SET tenant_id = '00000000-0000-0000-0000-000000000000' WHERE tenant_id IS NULL;

-- Make tenant_id NOT NULL after populating
ALTER TABLE categories ALTER COLUMN tenant_id SET NOT NULL;
