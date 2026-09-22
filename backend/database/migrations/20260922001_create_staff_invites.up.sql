-- #2031 邀请制自助加入：管理员只发邀请码，本人在自己账户上接受（一人一号，无 SMS）
CREATE TABLE IF NOT EXISTS staff_invites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    org_id UUID NOT NULL,
    site_id UUID,
    role VARCHAR(20) NOT NULL,
    code VARCHAR(32) NOT NULL UNIQUE,
    created_by UUID,
    expires_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    accepted_by UUID,
    accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_staff_invites_tenant ON staff_invites (tenant_id);
CREATE INDEX IF NOT EXISTS idx_staff_invites_site ON staff_invites (site_id);
CREATE INDEX IF NOT EXISTS idx_staff_invites_code ON staff_invites (code);
