-- #2041 待提交订单缓存（未实名下单被拦截 → 缓存 → 实名通过后回跳继续提交）
CREATE TABLE IF NOT EXISTS pending_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    payload JSONB NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_pending_orders_user ON pending_orders (user_id, status);
