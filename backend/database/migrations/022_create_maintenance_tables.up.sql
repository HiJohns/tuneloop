-- 022_create_maintenance_tables.up.sql
-- 维修系统相关表

-- 1. maintenance_workers 维修师傅表
CREATE TABLE IF NOT EXISTS maintenance_workers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    org_id UUID,
    site_id UUID,
    name VARCHAR(100) NOT NULL,
    phone VARCHAR(50),
    join_date DATE,
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP,
    
    CONSTRAINT fk_maintenance_workers_tenant 
        FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT fk_maintenance_workers_org 
        FOREIGN KEY (org_id) REFERENCES orgs(id),
    CONSTRAINT fk_maintenance_workers_site 
        FOREIGN KEY (site_id) REFERENCES sites(id)
);

CREATE INDEX idx_maintenance_workers_tenant_id ON maintenance_workers(tenant_id);
CREATE INDEX idx_maintenance_workers_org_id ON maintenance_workers(org_id);
CREATE INDEX idx_maintenance_workers_site_id ON maintenance_workers(site_id);
CREATE INDEX idx_maintenance_workers_status ON maintenance_workers(status);
CREATE INDEX idx_maintenance_workers_deleted_at ON maintenance_workers(deleted_at);

-- 2. maintenance_sessions 维修会话表
CREATE TABLE IF NOT EXISTS maintenance_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    org_id UUID,
    maintenance_ticket_id UUID NOT NULL,
    worker_id UUID,
    status VARCHAR(20) DEFAULT 'pending',
    start_time TIMESTAMP,
    end_time TIMESTAMP,
    progress_notes TEXT,
    completion_notes TEXT,
    inspection_result VARCHAR(20),
    inspection_comment TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    CONSTRAINT fk_maintenance_sessions_tenant 
        FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT fk_maintenance_sessions_org 
        FOREIGN KEY (org_id) REFERENCES orgs(id),
    CONSTRAINT fk_maintenance_sessions_ticket 
        FOREIGN KEY (maintenance_ticket_id) REFERENCES maintenance_tickets(id),
    CONSTRAINT fk_maintenance_sessions_worker 
        FOREIGN KEY (worker_id) REFERENCES maintenance_workers(id)
);

CREATE INDEX idx_maintenance_sessions_tenant_id ON maintenance_sessions(tenant_id);
CREATE INDEX idx_maintenance_sessions_org_id ON maintenance_sessions(org_id);
CREATE INDEX idx_maintenance_sessions_worker_id ON maintenance_sessions(worker_id);
CREATE INDEX idx_maintenance_sessions_status ON maintenance_sessions(status);

-- 3. maintenance_session_records 维修记录表
CREATE TABLE IF NOT EXISTS maintenance_session_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    session_id UUID NOT NULL,
    record_type VARCHAR(20),
    content TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    CONSTRAINT fk_maintenance_session_records_tenant 
        FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT fk_maintenance_session_records_session 
        FOREIGN KEY (session_id) REFERENCES maintenance_sessions(id) ON DELETE CASCADE
);

CREATE INDEX idx_maintenance_session_records_tenant_id ON maintenance_session_records(tenant_id);
CREATE INDEX idx_maintenance_session_records_session_id ON maintenance_session_records(session_id);

-- 更新时间戳自动更新
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_maintenance_workers_updated_at 
    BEFORE UPDATE ON maintenance_workers 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_maintenance_sessions_updated_at 
    BEFORE UPDATE ON maintenance_sessions 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE maintenance_workers IS '维修师傅表';
COMMENT ON TABLE maintenance_sessions IS '维修会话表';
COMMENT ON TABLE maintenance_session_records IS '维修记录表';