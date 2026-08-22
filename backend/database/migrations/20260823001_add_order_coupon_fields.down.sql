-- #1744 down: 移除优惠码订单快照字段
ALTER TABLE orders DROP COLUMN IF EXISTS coupon_code;
ALTER TABLE orders DROP COLUMN IF EXISTS coupon_discount;
