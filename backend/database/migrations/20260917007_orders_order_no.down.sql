DROP INDEX IF EXISTS idx_orders_order_no;
ALTER TABLE orders DROP COLUMN IF EXISTS order_no;
