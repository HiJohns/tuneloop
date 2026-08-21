-- #1731 down: remove order_payment_records.openid (callback-persisted payer openid).
ALTER TABLE order_payment_records DROP COLUMN IF EXISTS openid;
