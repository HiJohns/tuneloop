ALTER TABLE orders
    DROP COLUMN IF EXISTS credit_package_expire_at,
    DROP COLUMN IF EXISTS credit_package;
