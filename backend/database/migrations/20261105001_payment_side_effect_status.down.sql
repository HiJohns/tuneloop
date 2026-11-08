ALTER TABLE order_payment_records
    DROP COLUMN IF EXISTS side_effect_attempts,
    DROP COLUMN IF EXISTS side_effect_error,
    DROP COLUMN IF EXISTS side_effect_status;
