-- Restore user_type column on users table

ALTER TABLE users ADD COLUMN user_type VARCHAR(20) DEFAULT '员工';
CREATE INDEX IF NOT EXISTS idx_users_user_type ON users(user_type);
