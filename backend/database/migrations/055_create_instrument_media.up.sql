-- Migration 055: Create instrument_media and system_settings tables
CREATE TABLE IF NOT EXISTS instrument_media (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL,
    org_id        UUID,
    instrument_id UUID NOT NULL REFERENCES instruments(id) ON DELETE CASCADE,
    batch_id      UUID NOT NULL,
    batch_type    VARCHAR(20) NOT NULL,
    file_name     VARCHAR(255) NOT NULL,
    file_type     VARCHAR(10) NOT NULL,
    file_size     BIGINT DEFAULT 0,
    storage_key   VARCHAR(500) NOT NULL,
    is_display    BOOLEAN DEFAULT false,
    sort_order    INT DEFAULT 0,
    created_at    TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_media_tenant ON instrument_media(tenant_id);
CREATE INDEX IF NOT EXISTS idx_media_org ON instrument_media(org_id);
CREATE INDEX IF NOT EXISTS idx_media_instrument ON instrument_media(instrument_id);
CREATE INDEX IF NOT EXISTS idx_media_batch ON instrument_media(batch_id);
CREATE INDEX IF NOT EXISTS idx_media_display ON instrument_media(instrument_id, is_display);

CREATE TABLE IF NOT EXISTS system_settings (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID NOT NULL,
    setting_key   VARCHAR(100) NOT NULL,
    setting_value TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMP DEFAULT NOW(),
    updated_by VARCHAR(255),
    UNIQUE(tenant_id, setting_key)
);

CREATE INDEX IF NOT EXISTS idx_settings_tenant ON system_settings(tenant_id);
CREATE INDEX IF NOT EXISTS idx_settings_key ON system_settings(setting_key);
