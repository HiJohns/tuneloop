DROP TABLE IF EXISTS transit_shipping_fees;

ALTER TABLE forwarding_sessions
    DROP COLUMN IF EXISTS tracking_company,
    DROP COLUMN IF EXISTS tracking_number,
    DROP COLUMN IF EXISTS photos,
    DROP COLUMN IF EXISTS logistics_fee_cents;