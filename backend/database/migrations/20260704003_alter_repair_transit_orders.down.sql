-- Down migration: Remove v3 fields from repair_transit_orders
ALTER TABLE repair_transit_orders
    DROP COLUMN IF EXISTS direction,
    DROP COLUMN IF EXISTS transit_service_fee,
    DROP COLUMN IF EXISTS transit_logistics_fee,
    DROP COLUMN IF EXISTS note;
ALTER TABLE repair_transit_orders ALTER COLUMN status SET DEFAULT 'inbound';
