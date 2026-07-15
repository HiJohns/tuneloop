ALTER TABLE orders DROP COLUMN IF EXISTS payment_deadline;
ALTER TABLE merchant_settlement_configs DROP COLUMN IF EXISTS payment_timeout;
