-- #1901 down: drop the attribute (behavior reverts to cash-basis everywhere).
ALTER TABLE coupons DROP COLUMN IF EXISTS points_basis;
