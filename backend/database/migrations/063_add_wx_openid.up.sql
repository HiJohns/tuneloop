ALTER TABLE users ADD COLUMN IF NOT EXISTS wx_openid varchar(128);
CREATE INDEX IF NOT EXISTS idx_users_wx_openid ON users(wx_openid);