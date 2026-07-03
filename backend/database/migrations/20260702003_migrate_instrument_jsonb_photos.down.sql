-- Remove instrument-linked JSONB photos that were migrated to instrument_media
-- This is a destructive operation — only run if rolling back the migration.
-- We delete by batch_type to target only migrated records.
DELETE FROM instrument_media WHERE batch_type IN ('repair', 'repaired', 'receiving');
