-- #1802 T1: 续费天数独立持久化 — order_payment_records 新增 days 列
-- 背景：续费天数（AdditionalDays）原存 RawResponse，但微信支付回调（processPaymentCallback）
-- 会覆盖 RawResponse 为回调结果 → 真实回调后续费天数丢失（潜在既有 bug）。
ALTER TABLE order_payment_records ADD COLUMN IF NOT EXISTS days integer;
