-- #1867 down: drop the recommendation letter column.
ALTER TABLE orders DROP COLUMN IF EXISTS recommendation_letter;
