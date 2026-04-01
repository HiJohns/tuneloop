-- Migration: Add instrument detail columns (model, sn, site fields)

ALTER TABLE instruments 
ADD COLUMN IF NOT EXISTS model VARCHAR(100),
ADD COLUMN IF NOT EXISTS sn VARCHAR(100),
ADD COLUMN IF NOT EXISTS site VARCHAR(255),
ADD COLUMN IF NOT EXISTS site_id UUID,
ADD COLUMN IF NOT EXISTS current_site_id UUID;

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_instruments_site ON instruments(site_id);
CREATE INDEX IF NOT EXISTS idx_instruments_current_site ON instruments(current_site_id);

-- Update existing rows to have empty strings instead of NULL for text fields
UPDATE instruments SET model = '' WHERE model IS NULL;
UPDATE instruments SET sn = '' WHERE sn IS NULL;
UPDATE instruments SET site = '' WHERE site IS NULL;

-- Make text columns NOT NULL after populating
ALTER TABLE instruments ALTER COLUMN model SET NOT NULL;
ALTER TABLE instruments ALTER COLUMN model SET DEFAULT '';

ALTER TABLE instruments ALTER COLUMN sn SET NOT NULL;
ALTER TABLE instruments ALTER COLUMN sn SET DEFAULT '';

ALTER TABLE instruments ALTER COLUMN site SET NOT NULL;
ALTER TABLE instruments ALTER COLUMN site set DEFAULT '';

-- Update site_id and current_site_id to have NULL as default (optional fields)
-- No changes needed as they should be nullable