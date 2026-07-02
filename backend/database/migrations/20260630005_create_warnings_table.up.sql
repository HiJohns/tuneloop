CREATE TABLE IF NOT EXISTS warnings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID,
    merchant_id UUID,
    reason VARCHAR(50) NOT NULL,
    category VARCHAR(30),
    level VARCHAR(10) DEFAULT 'low',
    object_type VARCHAR(30),
    object_id UUID,
    description TEXT,
    status VARCHAR(20) DEFAULT 'open',
    created_at TIMESTAMP DEFAULT NOW(),
    resolved_at TIMESTAMP,
    resolved_by UUID
);
CREATE INDEX IF NOT EXISTS idx_warnings_status ON warnings(status);
CREATE INDEX IF NOT EXISTS idx_warnings_level ON warnings(level);
CREATE INDEX IF NOT EXISTS idx_warnings_site ON warnings(site_id);
CREATE INDEX IF NOT EXISTS idx_warnings_object ON warnings(object_id);
