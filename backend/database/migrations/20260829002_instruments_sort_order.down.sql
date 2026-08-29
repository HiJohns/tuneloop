DROP INDEX IF EXISTS idx_instruments_category_sort;
ALTER TABLE instruments DROP COLUMN IF EXISTS sort_order;
