-- Migration 053: Add accepted_at column to maintenance_tickets
ALTER TABLE maintenance_tickets ADD COLUMN IF NOT EXISTS accepted_at TIMESTAMP;
