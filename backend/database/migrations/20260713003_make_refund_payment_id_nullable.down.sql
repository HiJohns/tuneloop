ALTER TABLE order_refund_records ALTER COLUMN payment_record_id SET NOT NULL;
ALTER TABLE order_refund_records ADD CONSTRAINT order_refund_records_payment_record_id_fkey
  FOREIGN KEY (payment_record_id) REFERENCES order_payment_records(id) ON DELETE CASCADE;
