-- Add role column to users table for IAM sync
ALTER TABLE users ADD COLUMN IF NOT EXISTS role VARCHAR(50) DEFAULT '';

-- Make org_id column nullable for IAM sync compatibility
ALTER TABLE users ALTER COLUMN org_id DROP NOT NULL;