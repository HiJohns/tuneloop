-- Migration 016: Rollback
ALTER TABLE sites DROP COLUMN IF EXISTS updated_at;