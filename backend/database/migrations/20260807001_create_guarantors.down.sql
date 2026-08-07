-- Drop guarantors + order_guarantors tables and orders.deposit_waived (#1557)
ALTER TABLE orders DROP COLUMN IF EXISTS deposit_waived;
DROP TABLE IF EXISTS order_guarantors;
DROP TABLE IF EXISTS guarantors;
