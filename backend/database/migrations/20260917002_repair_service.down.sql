-- Rollback #1942 维修服务
DROP INDEX IF EXISTS idx_repair_reviews_user;
DROP INDEX IF EXISTS idx_repair_reviews_repair;
DROP TABLE IF EXISTS repair_reviews;

DROP INDEX IF EXISTS idx_repair_logistics_fees_repair;
DROP TABLE IF EXISTS repair_logistics_fees;

DROP INDEX IF EXISTS idx_repair_requests_repair_code;
DROP INDEX IF EXISTS idx_repair_requests_type;

ALTER TABLE repair_requests
    DROP COLUMN IF EXISTS incurred_repair_cents,
    DROP COLUMN IF EXISTS adjusted_quote_cents,
    DROP COLUMN IF EXISTS quote_status,
    DROP COLUMN IF EXISTS quote_logistics_cents,
    DROP COLUMN IF EXISTS quote_repair_cents,
    DROP COLUMN IF EXISTS technician_id,
    DROP COLUMN IF EXISTS repair_code,
    DROP COLUMN IF EXISTS type;
