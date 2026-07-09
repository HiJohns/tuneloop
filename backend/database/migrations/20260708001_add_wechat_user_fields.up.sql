ALTER TABLE users ADD COLUMN IF NOT EXISTS wx_unionid VARCHAR(128);
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_profile_completed BOOLEAN DEFAULT false;
CREATE INDEX IF NOT EXISTS idx_users_wx_unionid ON users(wx_unionid);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_wx_unionid_unique ON users(wx_unionid) WHERE wx_unionid <> '';
