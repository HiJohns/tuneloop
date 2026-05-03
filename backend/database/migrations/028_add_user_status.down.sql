-- Rollback: remove status column from users table
ALTER TABLE users DROP COLUMN IF EXISTS status;