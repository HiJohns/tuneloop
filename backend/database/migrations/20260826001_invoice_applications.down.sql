DROP TABLE IF EXISTS invoice_application_orders;
DROP TABLE IF EXISTS invoice_applications;
ALTER TABLE orders DROP COLUMN IF EXISTS invoice_applied_at;
ALTER TABLE orders DROP COLUMN IF EXISTS invoice_applied;
