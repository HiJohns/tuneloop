-- Leases table for tracking lease agreements
CREATE TABLE IF NOT EXISTS leases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    instrument_id UUID NOT NULL,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    monthly_rent DECIMAL(10,2) NOT NULL,
    deposit_amount DECIMAL(10,2) NOT NULL,
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_leases_tenant ON leases(tenant_id);
CREATE INDEX idx_leases_user ON leases(user_id);
CREATE INDEX idx_leases_instrument ON leases(instrument_id);
CREATE INDEX idx_leases_status ON leases(status);
CREATE INDEX idx_leases_dates ON leases(start_date, end_date);

-- Deposits table for tracking deposit transactions
CREATE TABLE IF NOT EXISTS deposits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    lease_id UUID NOT NULL,
    user_id UUID NOT NULL,
    amount DECIMAL(10,2) NOT NULL,
    type VARCHAR(20) NOT NULL,
    status VARCHAR(20) DEFAULT 'pending',
    transaction_date DATE NOT NULL,
    notes TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_deposits_lease FOREIGN KEY (lease_id) REFERENCES leases(id) ON DELETE CASCADE
);

CREATE INDEX idx_deposits_tenant ON deposits(tenant_id);
CREATE INDEX idx_deposits_lease ON deposits(lease_id);
CREATE INDEX idx_deposits_user ON deposits(user_id);
CREATE INDEX idx_deposits_type ON deposits(type);
CREATE INDEX idx_deposits_status ON deposits(status);
