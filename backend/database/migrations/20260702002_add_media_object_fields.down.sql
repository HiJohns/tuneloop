-- Down migration: revert object_type and object_id changes
DROP INDEX IF EXISTS idx_media_object;

ALTER TABLE instrument_media DROP COLUMN IF EXISTS object_id;
ALTER TABLE instrument_media DROP COLUMN IF EXISTS object_type;

UPDATE instrument_media SET instrument_id = '00000000-0000-0000-0000-000000000000' WHERE instrument_id IS NULL;
ALTER TABLE instrument_media ALTER COLUMN instrument_id SET NOT NULL;

ALTER TABLE instrument_media ADD CONSTRAINT instrument_media_instrument_id_fkey
    FOREIGN KEY (instrument_id) REFERENCES instruments(id) ON DELETE CASCADE;
