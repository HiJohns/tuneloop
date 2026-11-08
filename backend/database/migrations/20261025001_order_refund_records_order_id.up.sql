ALTER TABLE order_refund_records ADD COLUMN IF NOT EXISTS order_id uuid;
CREATE INDEX IF NOT EXISTS idx_order_refund_records_order ON order_refund_records(order_id);
-- 幂等回填：由支付流水映射订单（存量行 order_id 为 NULL）
UPDATE order_refund_records r
SET order_id = p.order_id
FROM order_payment_records p
WHERE p.id = r.payment_record_id AND r.order_id IS NULL;
