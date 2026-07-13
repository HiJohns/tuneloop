CREATE TABLE IF NOT EXISTS order_payment_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    org_id UUID,
    user_id UUID NOT NULL,
    order_id UUID,
    order_type VARCHAR(20) NOT NULL,
    out_trade_no VARCHAR(32) UNIQUE,
    transaction_id VARCHAR(64),
    amount DECIMAL(10,2) NOT NULL,
    type VARCHAR(20) NOT NULL DEFAULT 'payment',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    method VARCHAR(20),
    prepay_id VARCHAR(64),
    code_url TEXT,
    fail_reason TEXT,
    raw_response JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS order_refund_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    payment_record_id UUID NOT NULL REFERENCES order_payment_records(id) ON DELETE CASCADE,
    out_refund_no VARCHAR(32) UNIQUE,
    refund_id VARCHAR(64),
    amount DECIMAL(10,2) NOT NULL,
    reason VARCHAR(200),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    fail_reason TEXT,
    raw_response JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payment_records_tenant ON order_payment_records(tenant_id);
CREATE INDEX IF NOT EXISTS idx_payment_records_order ON order_payment_records(order_id);
CREATE INDEX IF NOT EXISTS idx_payment_records_user ON order_payment_records(user_id);
CREATE INDEX IF NOT EXISTS idx_payment_records_out_trade ON order_payment_records(out_trade_no);
CREATE INDEX IF NOT EXISTS idx_refund_records_payment ON order_refund_records(payment_record_id);
