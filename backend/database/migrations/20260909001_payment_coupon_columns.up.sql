-- #1853: 优惠折扣逐笔入库 — 每笔支付独立记录优惠码与折扣金额（分）
-- orders.coupon_code/coupon_discount 为最近一笔覆盖式快照（#1744），
-- 多笔优惠单无法按笔还原；此列补齐 payment 级明细。
ALTER TABLE order_payment_records
  ADD COLUMN coupon_code VARCHAR(32),
  ADD COLUMN coupon_discount BIGINT NOT NULL DEFAULT 0;
