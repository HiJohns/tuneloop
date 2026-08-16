-- Drop session_id from order_payment_records (#1664)
ALTER TABLE order_payment_records DROP COLUMN IF EXISTS session_id;
