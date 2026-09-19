-- #1941 回滚：移除 发票类型/抬头/税号 三字段
ALTER TABLE invoice_applications DROP COLUMN IF EXISTS tax_number;
ALTER TABLE invoice_applications DROP COLUMN IF EXISTS title;
ALTER TABLE invoice_applications DROP COLUMN IF EXISTS invoice_type;
