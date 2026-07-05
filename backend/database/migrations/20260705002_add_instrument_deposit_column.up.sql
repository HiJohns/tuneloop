-- Up migration: Add deposit column to instruments table (v2, was missing from schema)
ALTER TABLE instruments ADD COLUMN IF NOT EXISTS deposit DECIMAL(10,2) NOT NULL DEFAULT 0;
