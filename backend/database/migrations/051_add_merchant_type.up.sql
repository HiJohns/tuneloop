-- Migration 051: Add merchant_type, transit_address, transit_phone, transit_contact_name to merchants
-- Related: #706 merchant types, #707 transit address

ALTER TABLE merchants ADD COLUMN IF NOT EXISTS merchant_type VARCHAR(20) DEFAULT 'full';
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS transit_address TEXT;
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS transit_phone VARCHAR(50);
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS transit_contact_name VARCHAR(255);
