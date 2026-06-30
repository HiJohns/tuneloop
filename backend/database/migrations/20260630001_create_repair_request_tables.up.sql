-- Up migration: Create repair request tables (Issue #1110)
CREATE TABLE IF NOT EXISTS user_instruments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id VARCHAR(255) NOT NULL,
    sn VARCHAR(255) NOT NULL,
    instrument_type VARCHAR(100),
    brand VARCHAR(100),
    model VARCHAR(100),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_user_instruments_user_id ON user_instruments(user_id);
CREATE INDEX IF NOT EXISTS idx_user_instruments_sn ON user_instruments(sn);

CREATE TABLE IF NOT EXISTS repair_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    site_id UUID NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    user_instrument_id UUID,
    status VARCHAR(20) DEFAULT 'pending_ship',
    description TEXT,
    photos JSONB DEFAULT '[]',
    quote_amount DECIMAL(10,2),
    inspection_fee DECIMAL(10,2),
    shipping_fee DECIMAL(10,2),
    tracking_company VARCHAR(100),
    tracking_number VARCHAR(100),
    return_company VARCHAR(100),
    return_tracking_number VARCHAR(100),
    worker_id VARCHAR(255),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    closed_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_repair_requests_tenant ON repair_requests(tenant_id);
CREATE INDEX IF NOT EXISTS idx_repair_requests_user ON repair_requests(user_id);
CREATE INDEX IF NOT EXISTS idx_repair_requests_site ON repair_requests(site_id);
CREATE INDEX IF NOT EXISTS idx_repair_requests_status ON repair_requests(status);

CREATE TABLE IF NOT EXISTS repair_request_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repair_request_id UUID NOT NULL,
    worker_id VARCHAR(255),
    comment TEXT,
    photos JSONB DEFAULT '[]',
    record_type VARCHAR(20),
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_repair_req_records_request ON repair_request_records(repair_request_id);
