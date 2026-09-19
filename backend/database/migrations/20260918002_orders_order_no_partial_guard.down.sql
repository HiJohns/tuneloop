-- #1981 回滚：恢复原先的部分唯一索引（仅排除 NULL）。
DROP INDEX IF EXISTS idx_orders_order_no;
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_order_no ON orders (order_no)
    WHERE order_no IS NOT NULL;
