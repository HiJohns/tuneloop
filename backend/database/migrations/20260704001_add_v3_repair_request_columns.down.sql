-- Down migration: Remove v3 fields from repair_requests
DROP INDEX IF EXISTS idx_repair_requests_accepted_quote_id;
DROP INDEX IF EXISTS idx_repair_requests_controlled_site_id;
DROP INDEX IF EXISTS idx_repair_requests_transit_site_id;
ALTER TABLE repair_requests
    DROP COLUMN IF EXISTS merchant_type,
    DROP COLUMN IF EXISTS transit_site_id,
    DROP COLUMN IF EXISTS controlled_site_id,
    DROP COLUMN IF EXISTS accepted_quote_id,
    DROP COLUMN IF EXISTS check_fee_snapshot,
    DROP COLUMN IF EXISTS paid_amount,
    DROP COLUMN IF EXISTS expire_at,
    DROP COLUMN IF EXISTS reminder_sent;
