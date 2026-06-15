DROP INDEX IF EXISTS idx_notifications_action_type;
ALTER TABLE notifications
    DROP COLUMN IF EXISTS action_data,
    DROP COLUMN IF EXISTS action_type;
