-- #2114 审计修复：staff_invites 待处理唯一约束（计划 §2.1 部分唯一索引）
-- 目标：同 (tenant_id, site_id, 对象) 在 pending_approval/pending 下唯一，杜绝并发重复申请/重复直邀。
-- 幂等：先对存量重复项保留最新一条、其余置 cancelled；再建部分唯一索引。
UPDATE staff_invites s SET status = 'cancelled', updated_at = now()
WHERE s.status IN ('pending_approval', 'pending')
  AND EXISTS (
    SELECT 1 FROM staff_invites t
    WHERE t.status IN ('pending_approval', 'pending')
      AND t.tenant_id = s.tenant_id
      AND COALESCE(t.site_id::text, '') = COALESCE(s.site_id::text, '')
      AND COALESCE(t.invitee_user_id::text, t.invitee_identifier) = COALESCE(s.invitee_user_id::text, s.invitee_identifier)
      AND (t.created_at, t.id) > (s.created_at, s.id)
  );

CREATE UNIQUE INDEX IF NOT EXISTS uniq_staff_invites_pending_2114
  ON staff_invites (
    tenant_id,
    COALESCE(site_id::text, ''),
    COALESCE(invitee_user_id::text, invitee_identifier)
  )
  WHERE status IN ('pending_approval', 'pending');
