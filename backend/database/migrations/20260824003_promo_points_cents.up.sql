-- #1757: promo_points yuan → cents (1 点 = 1 元 = 100 分)
-- Existing yuan values ×100 become cents.
ALTER TABLE users ALTER COLUMN promo_points TYPE bigint USING (promo_points * 100)::bigint;
