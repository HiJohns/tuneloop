CREATE TABLE IF NOT EXISTS payment_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    type VARCHAR(20) NOT NULL DEFAULT 'payment',   -- 'payment' | 'refund' | 'auto_debit'
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- 'pending' | 'paid' | 'failed' | 'cancelled' | 'refunded'
    amount DECIMAL(10,2) NOT NULL,
    breakdown JSONB,                 -- 支付分解：{cash_amount, prepaid_used, gift_used, ...}
    wallet_snapshot JSONB,           -- 预付点/赠点余额快照
    out_trade_no VARCHAR(32) UNIQUE,
    transaction_id VARCHAR(64),
    method VARCHAR(20),              -- 'jsapi' | 'native' | 'h5' | 'mock'
    fail_reason TEXT,
    raw_response JSONB,
    refund_from_id UUID REFERENCES payment_sessions(id),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payment_sessions_tenant ON payment_sessions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_payment_sessions_user ON payment_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_payment_sessions_out_trade ON payment_sessions(out_trade_no);

CREATE TABLE IF NOT EXISTS session_order_links (
    session_id UUID NOT NULL REFERENCES payment_sessions(id) ON DELETE CASCADE,
    order_id VARCHAR(32) NOT NULL,
    PRIMARY KEY (session_id, order_id)
);

CREATE INDEX IF NOT EXISTS idx_session_order_links_session ON session_order_links(session_id);
CREATE INDEX IF NOT EXISTS idx_session_order_links_order ON session_order_links(order_id);

ALTER TABLE orders ADD COLUMN IF NOT EXISTS current_payment_session_id UUID REFERENCES payment_sessions(id);
CREATE INDEX IF NOT EXISTS idx_orders_current_session ON orders(current_payment_session_id);
