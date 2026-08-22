-- #1751: ENO percent coupon value was seeded as 1 (1‰) — per the
-- permille contract (#1728, prepay ÷1000) 1% = 10‰, so ENO must be 10.
-- Without this, ENO payments charged 1/10 of the intended discount.
UPDATE coupons SET value = 10 WHERE code = 'ENO' AND type = 'percent' AND value = 1;
