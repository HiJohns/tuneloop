-- Rollback 051: Drop merchant_type, transit_address, transit_phone, transit_contact_name

ALTER TABLE merchants DROP COLUMN IF EXISTS merchant_type;
ALTER TABLE merchants DROP COLUMN IF EXISTS transit_address;
ALTER TABLE merchants DROP COLUMN IF EXISTS transit_phone;
ALTER TABLE merchants DROP COLUMN IF EXISTS transit_contact_name;
