-- #1853: 回退 — 删除支付记录折扣列
ALTER TABLE order_payment_records
  DROP COLUMN IF EXISTS coupon_discount,
  DROP COLUMN IF EXISTS coupon_code;
