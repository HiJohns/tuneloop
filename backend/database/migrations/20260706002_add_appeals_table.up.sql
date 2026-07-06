CREATE TABLE IF NOT EXISTS appeals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    org_id UUID,
    site_id UUID,
    category VARCHAR(30),
    object_type VARCHAR(30),
    object_id UUID,
    appellant_id VARCHAR(255),
    description TEXT,
    images JSONB DEFAULT '[]',
    damage_report_id UUID,
    user_id UUID,
    appeal_reason TEXT,
    reviewer_id VARCHAR(255),
    desensitized_description TEXT,
    forwarded_to VARCHAR(255),
    status VARCHAR(20) DEFAULT 'pending',
    submitted_at TIMESTAMP NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMP,
    resolution VARCHAR(20),
    final_amount DECIMAL(10,2),
    manager_comment TEXT DEFAULT '',
    resolved_by UUID,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_appeals_site ON appeals(site_id);
CREATE INDEX IF NOT EXISTS idx_appeals_object ON appeals(object_id);
CREATE INDEX IF NOT EXISTS idx_appeals_tenant ON appeals(tenant_id);
CREATE INDEX IF NOT EXISTS idx_appeals_status ON appeals(status);
CREATE INDEX IF NOT EXISTS idx_appeals_damage_report ON appeals(damage_report_id);
CREATE INDEX IF NOT EXISTS idx_appeals_user ON appeals(user_id);
