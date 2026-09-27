-- #2085：新服务流维修报价增加料钱（初付 = 修理费 + 料钱 + 物流费预估；加价补差仍仅修理费）
ALTER TABLE repair_requests ADD COLUMN IF NOT EXISTS quote_material_cents bigint;
