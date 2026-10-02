-- #2116 down
ALTER TABLE repair_requests DROP COLUMN IF EXISTS receive_photos;
ALTER TABLE repair_requests DROP COLUMN IF EXISTS received_by;
ALTER TABLE repair_requests DROP COLUMN IF EXISTS received_at;
