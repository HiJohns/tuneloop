ALTER TABLE order_refund_records ALTER COLUMN payment_record_id DROP NOT NULL;
ALTER TABLE order_refund_records DROP CONSTRAINT IF EXISTS order_refund_records_payment_record_id_fkey;
