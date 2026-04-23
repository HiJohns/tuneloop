-- Migration: Add is_system_admin to users table
-- Description: Mark system administrator users for bootstrapping

DO $$
BEGIN
    -- Check if column already exists
    IF NOT EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'users' 
        AND column_name = 'is_system_admin'
    ) THEN
        ALTER TABLE users 
        ADD COLUMN is_system_admin BOOLEAN DEFAULT FALSE;
        
        RAISE NOTICE 'Column is_system_admin added to users table';
    ELSE
        RAISE NOTICE 'Column is_system_admin already exists in users table';
    END IF;
END
$$;

-- Add comment
COMMENT ON COLUMN users.is_system_admin IS 'Is system administrator (for bootstrapping)';

-- Optionally create index for fast lookup of system admins
CREATE INDEX IF NOT EXISTS idx_users_system_admin ON users(is_system_admin) WHERE is_system_admin = TRUE;

-- Verify column added (optional)
DO $$
BEGIN
    ASSERT EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'users' 
        AND column_name = 'is_system_admin'
    ), 'Column is_system_admin not found';
    
    RAISE NOTICE '✓ Migration verified: is_system_admin column exists';
END
$$;

