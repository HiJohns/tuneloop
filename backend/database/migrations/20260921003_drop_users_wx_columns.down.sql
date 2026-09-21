-- #2016 S3 down：重建本地 users.wx_openid / wx_unionid 列与索引（结构恢复，数据不可恢复）
ALTER TABLE users ADD COLUMN IF NOT EXISTS wx_openid varchar(128);
CREATE INDEX IF NOT EXISTS idx_users_wx_openid ON users(wx_openid);

ALTER TABLE users ADD COLUMN IF NOT EXISTS wx_unionid VARCHAR(128);
CREATE INDEX IF NOT EXISTS idx_users_wx_unionid ON users(wx_unionid);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_wx_unionid_tenant_unique ON users(wx_unionid, tenant_id) WHERE wx_unionid <> '';
CREATE UNIQUE INDEX IF NOT EXISTS users_wx_unionid_nonempty_idx ON users (wx_unionid) WHERE wx_unionid != '';
