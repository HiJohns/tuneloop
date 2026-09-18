-- #1965 业务订单号（YYMMDD 当日序号）：YL<YYYYMMDD>-<NNN>
ALTER TABLE orders ADD COLUMN IF NOT EXISTS order_no VARCHAR(24);
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_order_no ON orders (order_no) WHERE order_no IS NOT NULL;
-- 存量回填：按创建日分组、组内按创建时间序 → YL<YYYYMMDD>-<NNN>
WITH numbered AS (
    SELECT id,
           TO_CHAR(created_at, 'YYYYMMDD') AS d,
           ROW_NUMBER() OVER (PARTITION BY (created_at AT TIME ZONE 'Asia/Shanghai')::date ORDER BY created_at ASC) AS seq
    FROM orders
    WHERE order_no IS NULL
)
UPDATE orders o
SET order_no = 'YL' || n.d || '-' || LPAD(n.seq::text, 3, '0')
FROM numbered n
WHERE o.id = n.id;
