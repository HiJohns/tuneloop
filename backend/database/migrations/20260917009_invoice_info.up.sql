-- #1941 发票申请增 发票类型/抬头/税号（按商户分组，每个分组=一张发票）
ALTER TABLE invoice_applications ADD COLUMN IF NOT EXISTS invoice_type VARCHAR(20) NOT NULL DEFAULT '普通';
ALTER TABLE invoice_applications ADD COLUMN IF NOT EXISTS title VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE invoice_applications ADD COLUMN IF NOT EXISTS tax_number VARCHAR(50) DEFAULT '';
