-- Up migration: Create repair_quotes table (v3 competitive quoting)
CREATE TABLE IF NOT EXISTS repair_quotes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repair_request_id UUID NOT NULL REFERENCES repair_requests(id),
    site_id UUID,
    worker_id VARCHAR(255) NOT NULL,
    quote_no VARCHAR(30) UNIQUE,
    material_fee DECIMAL(10,2) NOT NULL,
    service_fee DECIMAL(10,2) NOT NULL,
    logistics_fee DECIMAL(10,2),
    duration VARCHAR(100),
    comment TEXT,
    is_renegotiation BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_repair_quotes_request_id ON repair_quotes(repair_request_id);
CREATE INDEX IF NOT EXISTS idx_repair_quotes_worker_id ON repair_quotes(worker_id);
CREATE INDEX IF NOT EXISTS idx_repair_quotes_status ON repair_quotes(status);
