-- Drop NOT NULL constraint on name (removed from Go model, instruments identified by SN)
ALTER TABLE instruments ALTER COLUMN name DROP NOT NULL;
