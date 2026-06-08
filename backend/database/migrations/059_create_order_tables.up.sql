CREATE TABLE IF NOT EXISTS lease_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    org_id UUID,
    order_id UUID NOT NULL,
    user_id UUID NOT NULL,
    instrument_id UUID NOT NULL,
    start_date DATE,
    end_date DATE,
    actual_end_date DATE,
    status VARCHAR(20) DEFAULT 'active',
    delivery_address JSONB,
    return_method VARCHAR(20) DEFAULT '',
    return_tracking VARCHAR(100) DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lease_sessions_tenant_id ON lease_sessions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_lease_sessions_order_id ON lease_sessions(order_id);
CREATE INDEX IF NOT EXISTS idx_lease_sessions_user_id ON lease_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_lease_sessions_instrument_id ON lease_sessions(instrument_id);
CREATE INDEX IF NOT EXISTS idx_lease_sessions_status ON lease_sessions(status);

CREATE TABLE IF NOT EXISTS electronic_contracts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    org_id UUID,
    order_id UUID NOT NULL,
    user_id UUID NOT NULL,
    instrument_id UUID NOT NULL,
    contract_url VARCHAR(500) NOT NULL DEFAULT '',
    contract_number VARCHAR(50) UNIQUE,
    generated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_electronic_contracts_tenant_id ON electronic_contracts(tenant_id);
CREATE INDEX IF NOT EXISTS idx_electronic_contracts_order_id ON electronic_contracts(order_id);
CREATE INDEX IF NOT EXISTS idx_electronic_contracts_user_id ON electronic_contracts(user_id);
