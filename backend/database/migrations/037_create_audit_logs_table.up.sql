CREATE TABLE IF NOT EXISTS audit_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL,
    org_id      UUID,
    user_id     UUID        NOT NULL,
    actor_role  VARCHAR(50),
    action      VARCHAR(50) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id VARCHAR(100),
    details     JSONB,
    request_body JSONB,
    ip_address  VARCHAR(45),
    user_agent  VARCHAR(500),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_tenant_id ON audit_logs (tenant_id);
CREATE INDEX idx_audit_logs_org_id ON audit_logs (org_id);
CREATE INDEX idx_audit_logs_user_id ON audit_logs (user_id);
CREATE INDEX idx_audit_logs_resource ON audit_logs (resource_type, resource_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs (created_at);
