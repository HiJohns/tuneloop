-- #2327 D13: 下单信用路径快照（internal|payscore；空=历史订单）
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS deposit_credit_path varchar(16) NOT NULL DEFAULT '';

COMMENT ON COLUMN orders.deposit_credit_path IS '#2327 下单信用路径：空=历史订单|internal（#1867 内部免押）|payscore（微信支付分）';
