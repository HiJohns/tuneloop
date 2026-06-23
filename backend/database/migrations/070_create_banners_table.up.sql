CREATE TABLE IF NOT EXISTS banners (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID NOT NULL,
    image_url  VARCHAR(500) NOT NULL,
    link_url   VARCHAR(500) DEFAULT '',
    title      VARCHAR(200) DEFAULT '',
    sort_order INT DEFAULT 0,
    status     VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_banners_tenant ON banners(tenant_id);
CREATE INDEX IF NOT EXISTS idx_banners_sort ON banners(sort_order);
CREATE INDEX IF NOT EXISTS idx_banners_status ON banners(status);
