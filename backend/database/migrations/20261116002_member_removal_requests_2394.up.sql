-- #2394 成员移除审批单（网点管理员申请 → 商户管理员批准/拒绝）。
CREATE TABLE IF NOT EXISTS member_removal_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    merchant_id uuid NOT NULL,
    site_id uuid,
    target_user_id uuid NOT NULL,
    requester_id uuid NOT NULL,
    reason varchar(200) NOT NULL,
    status varchar(20) NOT NULL DEFAULT 'pending_approval',
    decided_by uuid,
    decided_at timestamptz,
    reject_reason varchar(200),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_member_removal_requests_tenant
    ON member_removal_requests (tenant_id);
CREATE INDEX IF NOT EXISTS idx_member_removal_requests_merchant
    ON member_removal_requests (merchant_id);
CREATE INDEX IF NOT EXISTS idx_member_removal_requests_site
    ON member_removal_requests (site_id);
CREATE INDEX IF NOT EXISTS idx_member_removal_requests_target
    ON member_removal_requests (target_user_id);
CREATE INDEX IF NOT EXISTS idx_member_removal_requests_requester
    ON member_removal_requests (requester_id);
CREATE INDEX IF NOT EXISTS idx_member_removal_requests_status
    ON member_removal_requests (status);

-- 同一 (site_id, target_user_id) 同时只允许一条待审批申请。
-- site_id 为 NULL（商户直属成员）时 Postgres 视为互不相等，唯一性由 handler 预检保证。
CREATE UNIQUE INDEX IF NOT EXISTS uq_member_removal_pending_site_user
    ON member_removal_requests (site_id, target_user_id)
    WHERE status = 'pending_approval';
