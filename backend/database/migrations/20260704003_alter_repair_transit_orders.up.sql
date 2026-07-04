-- Up migration: Add v3 fields to repair_transit_orders (direction, fees, note, new status)
ALTER TABLE repair_transit_orders
    ADD COLUMN IF NOT EXISTS direction VARCHAR(10),
    ADD COLUMN IF NOT EXISTS transit_service_fee DECIMAL(10,2),
    ADD COLUMN IF NOT EXISTS transit_logistics_fee DECIMAL(10,2),
    ADD COLUMN IF NOT EXISTS note TEXT;
-- Update existing records: set direction=in for legacy inbound/transiting, direction=out for outbound/sent_back
UPDATE repair_transit_orders SET direction = 'in' WHERE direction IS NULL AND status IN ('inbound', 'transiting');
UPDATE repair_transit_orders SET direction = 'out' WHERE direction IS NULL AND status IN ('outbound', 'sent_back');
-- Set remaining unset direction to 'in' as default
UPDATE repair_transit_orders SET direction = 'in' WHERE direction IS NULL;
ALTER TABLE repair_transit_orders ALTER COLUMN direction SET NOT NULL;
-- Change default status to pending_activation
ALTER TABLE repair_transit_orders ALTER COLUMN status SET DEFAULT 'pending_activation';
