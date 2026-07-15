-- Replace wx_unionid UNIQUE constraint with partial index that excludes empty strings
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_wx_unionid_key;
CREATE UNIQUE INDEX IF NOT EXISTS users_wx_unionid_nonempty_idx ON users (wx_unionid) WHERE wx_unionid != '';
