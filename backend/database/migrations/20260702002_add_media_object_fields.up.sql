-- Migration: Add object_type and object_id to instrument_media
-- Makes instrument_id nullable for non-instrument entity attachments

ALTER TABLE instrument_media DROP CONSTRAINT IF EXISTS instrument_media_instrument_id_fkey;

ALTER TABLE instrument_media ALTER COLUMN instrument_id DROP NOT NULL;

ALTER TABLE instrument_media ADD COLUMN IF NOT EXISTS object_type VARCHAR(30);
ALTER TABLE instrument_media ADD COLUMN IF NOT EXISTS object_id UUID;

CREATE INDEX IF NOT EXISTS idx_media_object ON instrument_media(object_id);
