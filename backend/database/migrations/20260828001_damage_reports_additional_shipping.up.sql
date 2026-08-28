-- #1801: 归还验收追缴费用区块 — 新增追加物流费字段
ALTER TABLE damage_reports ADD COLUMN IF NOT EXISTS additional_shipping_fee bigint DEFAULT 0;
