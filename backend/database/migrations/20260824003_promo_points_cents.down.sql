-- #1757 rollback: promo_points cents → yuan
ALTER TABLE users ALTER COLUMN promo_points TYPE decimal USING (promo_points / 100.0)::decimal;
