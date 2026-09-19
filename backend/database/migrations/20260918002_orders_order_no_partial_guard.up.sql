-- #1981: orders.order_no 唯一性仅由部分索引承担，并防御空串。
--
-- 背景：模型曾带 gorm `uniqueIndex` tag，使测试库 AutoMigrate 生成全量唯一索引，
-- 直插 Order{} 的零值空串 '' 从第 2 条起冲突（SQLSTATE 23505）。
-- 生产/测试统一改为部分索引：排除 NULL 与空串，允许多条“未生成订单号”的行。
DROP INDEX IF EXISTS idx_orders_order_no;
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_order_no ON orders (order_no)
    WHERE order_no IS NOT NULL AND order_no <> '';
