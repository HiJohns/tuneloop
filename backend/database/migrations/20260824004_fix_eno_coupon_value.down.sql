-- #1751 rollback: ENO back to the erroneous 1‰ seed value.
UPDATE coupons SET value = 1 WHERE code = 'ENO' AND type = 'percent' AND value = 10;
