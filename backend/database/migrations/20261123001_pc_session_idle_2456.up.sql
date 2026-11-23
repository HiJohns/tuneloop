-- #2456: PC 管理后台静置超时（idle timeout）会话锚点表。
--
-- 只有 PC 入口（OAuth 回调 /auth/callback 与账密登录 /auth/login）写入本表；
-- H5 与微信小程序（/auth/callback?client_type=wx、/auth/wx-login）不写本表，
-- 因此 /auth/refresh 仅在 refresh_hash 命中本表时执行 idle 校验，
-- H5/小程序令牌保持既有 7 天续期语义（Decision Lock D5）。
--
-- refresh_hash = sha256(refresh_token)。IAM 刷新不轮换 refresh_token
-- （见 #2456 Input Truth），因此该摘要可作为稳定的会话键。
CREATE TABLE IF NOT EXISTS pc_sessions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    iam_sub          VARCHAR(255) NOT NULL,
    refresh_hash     VARCHAR(64) NOT NULL,
    last_activity_at TIMESTAMP NOT NULL DEFAULT NOW(),
    revoked_at       TIMESTAMP,
    created_at       TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_pc_sessions_refresh_hash ON pc_sessions(refresh_hash);
CREATE INDEX IF NOT EXISTS idx_pc_sessions_iam_sub ON pc_sessions(iam_sub);
