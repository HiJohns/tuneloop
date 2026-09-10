-- #1867: deposit-free applications require a recommendation letter.
-- Stores the uploaded letter URL (POST /upload result) on the order.
ALTER TABLE orders ADD COLUMN IF NOT EXISTS recommendation_letter varchar(500) NOT NULL DEFAULT '';
