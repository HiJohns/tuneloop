-- #1715: merchant_members table was introduced with the model + CRUD API in
-- #1506 but the migration was never written — production and prerelease both
-- lacked the table and the member-management tab silently returned empty.
CREATE TABLE IF NOT EXISTS merchant_members (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID NOT NULL,
    merchant_id    UUID NOT NULL,
    user_id        UUID NOT NULL,
    role           VARCHAR(20) NOT NULL DEFAULT 'site_member',
    status         VARCHAR(20) NOT NULL DEFAULT 'active',
    cus_perm_codes TEXT[] NOT NULL DEFAULT '{}',
    created_at     TIMESTAMP NOT NULL DEFAULT now(),
    updated_at     TIMESTAMP NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_mm_tenant   ON merchant_members (tenant_id);
CREATE INDEX IF NOT EXISTS idx_mm_merchant ON merchant_members (merchant_id);
CREATE INDEX IF NOT EXISTS idx_mm_unique   ON merchant_members (user_id);
