-- Remove user_type column from users table
-- user_type was a redundant field; use role to display user type in Chinese instead

ALTER TABLE users DROP COLUMN IF EXISTS user_type;
DROP INDEX IF EXISTS idx_users_user_type;
