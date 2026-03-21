-- Site images table
CREATE TABLE IF NOT EXISTS site_images (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  site_id UUID NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
  url VARCHAR(500) NOT NULL,
  sort_order INT DEFAULT 0,
  created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_site_images_site ON site_images(site_id);

-- Inventory transfers table
CREATE TABLE IF NOT EXISTS inventory_transfers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  asset_id UUID NOT NULL,
  from_site_id UUID NOT NULL,
  to_site_id UUID NOT NULL,
  reason TEXT,
  status VARCHAR(20) DEFAULT 'pending',
  created_by UUID,
  created_at TIMESTAMP DEFAULT NOW(),
  completed_at TIMESTAMP
);

CREATE INDEX idx_transfers_asset ON inventory_transfers(asset_id);
CREATE INDEX idx_transfers_from ON inventory_transfers(from_site_id);
CREATE INDEX idx_transfers_to ON inventory_transfers(to_site_id);

-- Add current_site_id to instruments
ALTER TABLE instruments 
ADD COLUMN IF NOT EXISTS current_site_id UUID REFERENCES sites(id);

CREATE INDEX idx_instruments_site ON instruments(current_site_id);

-- Add category field to instruments for easier filtering
ALTER TABLE instruments 
ADD COLUMN IF NOT EXISTS category VARCHAR(50);

CREATE INDEX idx_instruments_category ON instruments(category);
