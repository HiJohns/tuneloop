DROP INDEX IF EXISTS idx_order_refund_records_order;
ALTER TABLE order_refund_records DROP COLUMN IF EXISTS order_id;
