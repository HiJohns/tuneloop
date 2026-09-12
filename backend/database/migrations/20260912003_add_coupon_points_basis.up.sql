-- #1901: test coupons (OREZ/ENO) may count the full rent toward membership
-- spend and rebate points so the points flow can be exercised without real
-- cash. The flag only takes effect when TEST_COUPON_GROSS_POINTS=true, which
-- must only be set in non-production environments.
ALTER TABLE coupons ADD COLUMN IF NOT EXISTS points_basis VARCHAR(10) NOT NULL DEFAULT 'cash';
UPDATE coupons SET points_basis = 'gross' WHERE UPPER(code) IN ('OREZ', 'ENO');
