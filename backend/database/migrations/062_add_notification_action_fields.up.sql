ALTER TABLE notifications
    ADD COLUMN IF NOT EXISTS action_type VARCHAR(20) NOT NULL DEFAULT 'info',
    ADD COLUMN IF NOT EXISTS action_data JSONB;
CREATE INDEX IF NOT EXISTS idx_notifications_action_type ON notifications(action_type);
