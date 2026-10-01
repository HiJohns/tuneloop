-- #2114：邀请与成员管理层级——staff_invites 扩展（申请 + 邀请一条流水）
-- 说明：site_id 保持可空（商户/平台级邀请为 NULL）；新增申请/审批留痕与 P3 标识。
ALTER TABLE staff_invites ADD COLUMN IF NOT EXISTS kind VARCHAR(20) NOT NULL DEFAULT 'site_member';
ALTER TABLE staff_invites ADD COLUMN IF NOT EXISTS invitee_identifier VARCHAR(255);
ALTER TABLE staff_invites ADD COLUMN IF NOT EXISTS invitee_name VARCHAR(255);
ALTER TABLE staff_invites ADD COLUMN IF NOT EXISTS requested_by UUID;
ALTER TABLE staff_invites ADD COLUMN IF NOT EXISTS request_note TEXT;
ALTER TABLE staff_invites ADD COLUMN IF NOT EXISTS approved_by UUID;
ALTER TABLE staff_invites ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ;
ALTER TABLE staff_invites ADD COLUMN IF NOT EXISTS reject_reason TEXT;
CREATE INDEX IF NOT EXISTS idx_staff_invites_requested_by ON staff_invites(requested_by);
CREATE INDEX IF NOT EXISTS idx_staff_invites_tenant_status ON staff_invites(tenant_id, status);
