-- #2010 S1：收货地址并入 orders（唯一独有字段迁移，来自 lease_sessions）
-- 列类型 TEXT：GORM 直读纯文本，避免 jsonb 带引号/需 #>> 解包
--（响应字段 delivery_address 形状不变，前端零改动，#2012 切换到本列读取）
ALTER TABLE orders ADD COLUMN IF NOT EXISTS delivery_address TEXT;

-- 回填：以 lease_sessions 为源（当前唯一读源），CASE 解包与 GetOrder 原逻辑保真一致：
--   字符串型 → #>> '{}' 解包；对象型 → ::text（JSON 文本）
UPDATE orders o
   SET delivery_address = CASE
         WHEN jsonb_typeof(ls.delivery_address) = 'string' THEN ls.delivery_address #>> '{}'
         ELSE ls.delivery_address::text END
  FROM lease_sessions ls
 WHERE ls.order_id = o.id
   AND ls.delivery_address IS NOT NULL;
