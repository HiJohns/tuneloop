-- Two-phase registration sessions + membership discount coupons (#1663/#1664).
CREATE TABLE IF NOT EXISTS registration_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    openid VARCHAR(128),
    exchange_token VARCHAR(128),
    form_data JSONB,
    coupon_code VARCHAR(32),
    amount NUMERIC(10,2) NOT NULL DEFAULT 0,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    error TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_registration_sessions_openid ON registration_sessions(openid);

CREATE TABLE IF NOT EXISTS coupons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(32) NOT NULL UNIQUE,
    type VARCHAR(16) NOT NULL DEFAULT 'percent',
    value NUMERIC(10,4) NOT NULL DEFAULT 0,
    active BOOLEAN NOT NULL DEFAULT true,
    description TEXT
);

-- Default coupons: OREZ (full waiver) and ENO (1% of membership fee).
INSERT INTO coupons (code, type, value, active, description) VALUES
('OREZ', 'waive', 0, true, '开业优惠码：全额免会员费'),
('ENO', 'percent', 1, true, '体验码：会员费 1%')
ON CONFLICT (code) DO NOTHING;
