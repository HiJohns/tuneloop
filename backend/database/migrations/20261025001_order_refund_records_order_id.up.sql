ALTER TABLE order_refund_records ADD COLUMN IF NOT EXISTS order_id uuid;
CREATE INDEX IF NOT EXISTS idx_order_refund_records_order ON order_refund_records(order_id);
