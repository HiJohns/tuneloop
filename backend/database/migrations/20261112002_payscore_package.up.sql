-- #2327 S3b: 支付分拉起凭证（package）与过期时间
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS credit_package text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS credit_package_expire_at timestamptz NULL;

COMMENT ON COLUMN orders.credit_package IS '#2327 微信支付分拉起凭证（package），前端 openBusinessView 使用；mock 模式为空串';
COMMENT ON COLUMN orders.credit_package_expire_at IS '#2327 package 过期时间（服务端按 PAYSCORE_PACKAGE_TTL_MINUTES 估算）';
