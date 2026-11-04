-- #2308: 支付回调 side effect 状态与错误留痕（避免 paid 被回滚 + 无限重试）
ALTER TABLE order_payment_records
    ADD COLUMN IF NOT EXISTS side_effect_status varchar(20) NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS side_effect_error text,
    ADD COLUMN IF NOT EXISTS side_effect_attempts integer NOT NULL DEFAULT 0;
