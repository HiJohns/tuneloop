-- Migration: Add category columns to instruments table

ALTER TABLE instruments 
ADD COLUMN IF NOT EXISTS category_id UUID,
ADD COLUMN IF NOT EXISTS category_name VARCHAR(100),
ADD COLUMN IF NOT EXISTS org_id UUID,
ALTER COLUMN tenant_id DROP DEFAULT,
ALTER COLUMN tenant_id SET NOT NULL;

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_instruments_category ON instruments(category_id);
CREATE INDEX IF NOT EXISTS idx_instruments_category_name ON instruments(category_name);
CREATE INDEX IF NOT EXISTS idx_instruments_org ON instruments(org_id);

-- Update existing rows to have default org_id if needed
UPDATE instruments SET org_id = tenant_id WHERE org_id IS NULL;

-- Make org_id NOT NULL after populating
ALTER TABLE instruments ALTER COLUMN org_id SET NOT NULL;
