-- #1759 follow-up: convert remaining timestamp without time zone columns
-- (merchant_members, settlement_calculations, order_payment_records.reminded_at)
-- that were absent from the prerelease information_schema snapshot used to
-- generate 20260824002. Idempotent via DO block.

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_name = 'merchant_members' AND column_name = 'created_at'
               AND data_type = 'timestamp without time zone') THEN
    ALTER TABLE merchant_members ALTER COLUMN created_at TYPE timestamptz USING created_at AT TIME ZONE 'Asia/Shanghai';
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_name = 'merchant_members' AND column_name = 'updated_at'
               AND data_type = 'timestamp without time zone') THEN
    ALTER TABLE merchant_members ALTER COLUMN updated_at TYPE timestamptz USING updated_at AT TIME ZONE 'Asia/Shanghai';
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_name = 'settlement_calculations' AND column_name = 'created_at'
               AND data_type = 'timestamp without time zone') THEN
    ALTER TABLE settlement_calculations ALTER COLUMN created_at TYPE timestamptz USING created_at AT TIME ZONE 'Asia/Shanghai';
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_name = 'order_payment_records' AND column_name = 'reminded_at'
               AND data_type = 'timestamp without time zone') THEN
    ALTER TABLE order_payment_records ALTER COLUMN reminded_at TYPE timestamptz USING reminded_at AT TIME ZONE 'Asia/Shanghai';
  END IF;
END $$;
