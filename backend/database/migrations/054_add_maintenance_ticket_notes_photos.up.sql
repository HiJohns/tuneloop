-- Migration 054: Add completion_notes and completion_photos to maintenance_tickets
ALTER TABLE maintenance_tickets ADD COLUMN IF NOT EXISTS completion_notes TEXT;
ALTER TABLE maintenance_tickets ADD COLUMN IF NOT EXISTS completion_photos JSONB DEFAULT '[]';
