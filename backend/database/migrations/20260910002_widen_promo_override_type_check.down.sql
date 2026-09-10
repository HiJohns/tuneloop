-- #1863 rollback: restore the two-value CHECK. rent_to_own rows are removed first
-- because the narrower constraint would reject them. Idempotent.
DELETE FROM instrument_promo_overrides WHERE override_type = 'rent_to_own';
ALTER TABLE instrument_promo_overrides DROP CONSTRAINT IF EXISTS instrument_promo_overrides_override_type_check;
ALTER TABLE instrument_promo_overrides ADD CONSTRAINT instrument_promo_overrides_override_type_check
    CHECK (override_type IN ('discount', 'rebate'));
