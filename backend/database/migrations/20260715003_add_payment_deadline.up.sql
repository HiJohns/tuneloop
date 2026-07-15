ALTER TABLE orders ADD COLUMN IF NOT EXISTS payment_deadline timestamp;
ALTER TABLE merchant_settlement_configs ADD COLUMN IF NOT EXISTS payment_timeout integer;
