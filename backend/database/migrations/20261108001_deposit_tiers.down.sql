ALTER TABLE orders
    DROP COLUMN IF EXISTS deposit_reduced,
    DROP COLUMN IF EXISTS deposit_full,
    DROP COLUMN IF EXISTS deposit_tier;

ALTER TABLE instruments
    DROP COLUMN IF EXISTS deposit_reduced;
