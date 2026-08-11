-- Drop shipping_fee from damage_assessments (#1621)
ALTER TABLE damage_assessments DROP COLUMN IF EXISTS shipping_fee;
