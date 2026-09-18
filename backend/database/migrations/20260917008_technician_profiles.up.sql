-- #1974 T1 师傅档案（直属商户）：photo/bio/experience/status
CREATE TABLE IF NOT EXISTS technician_profiles (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL,
    tenant_id  UUID NOT NULL,
    photo      VARCHAR(500),
    bio        TEXT,
    experience JSONB DEFAULT '[]',
    status     VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_technician_profiles_user ON technician_profiles (user_id);
CREATE INDEX IF NOT EXISTS idx_technician_profiles_tenant ON technician_profiles (tenant_id);
CREATE INDEX IF NOT EXISTS idx_technician_profiles_status ON technician_profiles (status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_technician_profiles_user_tenant ON technician_profiles (user_id, tenant_id);
