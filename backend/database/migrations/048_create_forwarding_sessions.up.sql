CREATE TABLE IF NOT EXISTS forwarding_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    org_id UUID,
    lease_session_id UUID NOT NULL,
    order_id UUID,
    merchant_id UUID,
    forwarding_site_id UUID,
    direction VARCHAR(20) NOT NULL,
    status VARCHAR(20) DEFAULT 'pending',
    session_code VARCHAR(6) UNIQUE NOT NULL,
    instrument_id UUID,
    tracking_numbers JSONB,
    notes TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_fs_tenant ON forwarding_sessions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_fs_lease ON forwarding_sessions(lease_session_id);
CREATE INDEX IF NOT EXISTS idx_fs_code ON forwarding_sessions(session_code);
CREATE INDEX IF NOT EXISTS idx_fs_status ON forwarding_sessions(status);
CREATE INDEX IF NOT EXISTS idx_fs_order ON forwarding_sessions(order_id);
CREATE INDEX IF NOT EXISTS idx_fs_instrument ON forwarding_sessions(instrument_id);
