-- Revert instrument_levels table changes

-- Drop foreign key constraint and level_id column
ALTER TABLE instruments DROP COLUMN IF EXISTS level_id;

-- Drop instrument_levels table
DROP TABLE IF EXISTS instrument_levels;