-- #1744: 优惠码使用订单快照 — prepay 应用优惠码后回写订单，
-- 订单可审计/还原优惠事实（码 + 折扣金额）。
-- coupon_code: 优惠码（ENO/OREZ 等，创建后不可修改的码定义）
-- coupon_discount: 折扣金额（分），优惠前金额 − 折后实付（waive = 原价）
ALTER TABLE orders ADD COLUMN IF NOT EXISTS coupon_code varchar(32);
ALTER TABLE orders ADD COLUMN IF NOT EXISTS coupon_discount bigint NOT NULL DEFAULT 0;
