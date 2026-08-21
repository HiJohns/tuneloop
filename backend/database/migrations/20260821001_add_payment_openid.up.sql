-- #1731: add order_payment_records.openid — payer.openid persisted at payment
-- callback, the authoritative source for later upload_shipping_info /
-- notify_confirm_receive calls (users.wx_openid cache is only a fallback).
ALTER TABLE order_payment_records ADD COLUMN IF NOT EXISTS openid varchar(128);
