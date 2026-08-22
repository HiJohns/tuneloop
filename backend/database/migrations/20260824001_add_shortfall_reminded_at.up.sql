-- #1749 L-04D：补缴超时催缴幂等标记 — 同一订单仅报警一次
ALTER TABLE order_payment_records ADD COLUMN IF NOT EXISTS reminded_at TIMESTAMP;
