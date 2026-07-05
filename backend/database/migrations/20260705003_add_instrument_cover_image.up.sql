-- Up migration: Add cover_image column to instruments table
ALTER TABLE instruments ADD COLUMN IF NOT EXISTS cover_image TEXT;
