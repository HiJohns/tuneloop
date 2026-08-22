-- #1749 down: 移除催缴幂等标记列
ALTER TABLE order_payment_records DROP COLUMN IF EXISTS reminded_at;
