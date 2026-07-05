-- Down migration: Remove cover_image column from instruments
ALTER TABLE instruments DROP COLUMN IF EXISTS cover_image;
