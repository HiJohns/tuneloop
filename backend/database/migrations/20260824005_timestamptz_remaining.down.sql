-- #1759 rollback: timestamptz → timestamp without time zone (back to Beijing wall-clock)
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_name = 'merchant_members' AND column_name = 'created_at'
               AND data_type = 'timestamp with time zone') THEN
    ALTER TABLE merchant_members ALTER COLUMN created_at TYPE timestamp WITHOUT TIME ZONE USING created_at AT TIME ZONE 'Asia/Shanghai';
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_name = 'merchant_members' AND column_name = 'updated_at'
               AND data_type = 'timestamp with time zone') THEN
    ALTER TABLE merchant_members ALTER COLUMN updated_at TYPE timestamp WITHOUT TIME ZONE USING updated_at AT TIME ZONE 'Asia/Shanghai';
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_name = 'settlement_calculations' AND column_name = 'created_at'
               AND data_type = 'timestamp with time zone') THEN
    ALTER TABLE settlement_calculations ALTER COLUMN created_at TYPE timestamp WITHOUT TIME ZONE USING created_at AT TIME ZONE 'Asia/Shanghai';
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_name = 'order_payment_records' AND column_name = 'reminded_at'
               AND data_type = 'timestamp with time zone') THEN
    ALTER TABLE order_payment_records ALTER COLUMN reminded_at TYPE timestamp WITHOUT TIME ZONE USING reminded_at AT TIME ZONE 'Asia/Shanghai';
  END IF;
END $$;
