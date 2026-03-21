-- Ownership certificates table
CREATE TABLE IF NOT EXISTS ownership_certificates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id UUID NOT NULL UNIQUE,
  user_id UUID NOT NULL,
  instrument_id UUID NOT NULL,
  transfer_date TIMESTAMP NOT NULL,
  certificate_url VARCHAR(500),
  created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_certificates_user ON ownership_certificates(user_id);
CREATE INDEX idx_certificates_instrument ON ownership_certificates(instrument_id);

-- Technicians table
CREATE TABLE IF NOT EXISTS technicians (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  site_id UUID REFERENCES sites(id),
  name VARCHAR(100) NOT NULL,
  phone VARCHAR(50),
  created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_technicians_site ON technicians(site_id);

-- Add transfer fields to orders
ALTER TABLE orders 
ADD COLUMN IF NOT EXISTS transferred_at TIMESTAMP,
ADD COLUMN IF NOT EXISTS terminated_at TIMESTAMP,
ADD COLUMN IF NOT EXISTS deposit_refunded BOOLEAN DEFAULT FALSE;

-- Add technician and progress to maintenance_tickets
ALTER TABLE maintenance_tickets 
ADD COLUMN IF NOT EXISTS technician_id UUID,
ADD COLUMN IF NOT EXISTS picked_up_at TIMESTAMP,
ADD COLUMN IF NOT EXISTS completed_at TIMESTAMP,
ADD COLUMN IF NOT EXISTS progress_notes TEXT;
