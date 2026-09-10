-- #1863: widen override_type CHECK to allow rent_to_own (per-instrument 租购转化 config).
-- Migration 077 created the constraint with only ('discount','rebate'); the feature
-- adds rent_to_own, so the constraint must be re-created to include it. Idempotent.
ALTER TABLE instrument_promo_overrides DROP CONSTRAINT IF EXISTS instrument_promo_overrides_override_type_check;
ALTER TABLE instrument_promo_overrides ADD CONSTRAINT instrument_promo_overrides_override_type_check
    CHECK (override_type IN ('discount', 'rebate', 'rent_to_own'));
