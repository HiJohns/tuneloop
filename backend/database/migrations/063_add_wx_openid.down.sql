DROP INDEX IF EXISTS idx_users_wx_openid;
ALTER TABLE users DROP COLUMN IF EXISTS wx_openid;