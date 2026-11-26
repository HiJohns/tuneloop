-- #2543：确认会话持久化表（Owner 2026-11-26 裁定「确认会话可以建表」）。
--
-- 背景：confirmation_sessions 此前只存在于 GORM 模型里，db.go 将
-- ConfirmationSession 列为「无表」豁免，目标环境（预生产/生产）都没有该表，
-- 任何创建/确认都会因缺表失败。本迁移建表后，该模型从豁免清单移除。
--
-- 主体域（Decision Lock D2）：确认执行的 IAM 主体必须是 users.iam_sub，
-- 本地 users.id 只用于本地查询。表本身只存 user_id（本地行），iam_sub 由
-- executeAction 在事务外查询并校验。
CREATE TABLE IF NOT EXISTS confirmation_sessions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID NOT NULL,
    org_id           UUID NOT NULL,
    user_id          UUID NOT NULL,
    confirm_type     VARCHAR(20) NOT NULL,
    confirm_target   VARCHAR(255) NOT NULL,
    merchant_id      UUID,
    action_type      VARCHAR(50) NOT NULL,
    action_target_id UUID,
    iam_session_id   VARCHAR(255),
    callback_url     VARCHAR(500),
    status           VARCHAR(20) NOT NULL DEFAULT 'waiting',
    message          TEXT,
    token            VARCHAR(100) NOT NULL,
    expires_at       TIMESTAMPTZ NOT NULL,
    confirmed_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_confirmation_sessions_token
    ON confirmation_sessions(token);
CREATE INDEX IF NOT EXISTS idx_confirmation_sessions_tenant_id
    ON confirmation_sessions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_confirmation_sessions_org_id
    ON confirmation_sessions(org_id);
CREATE INDEX IF NOT EXISTS idx_confirmation_sessions_user_id
    ON confirmation_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_confirmation_sessions_merchant_id
    ON confirmation_sessions(merchant_id);
CREATE INDEX IF NOT EXISTS idx_confirmation_sessions_status
    ON confirmation_sessions(status);
CREATE INDEX IF NOT EXISTS idx_confirmation_sessions_iam_session_id
    ON confirmation_sessions(iam_session_id);
