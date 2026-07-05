-- Down migration: Remove deposit column from instruments
ALTER TABLE instruments DROP COLUMN IF EXISTS deposit;
