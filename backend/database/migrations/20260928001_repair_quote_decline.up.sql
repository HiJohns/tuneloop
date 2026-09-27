-- #2093：顾客拒绝报价（终态 cancelled）——理由结构化存储
ALTER TABLE repair_requests ADD COLUMN IF NOT EXISTS quote_decline_reason varchar(30);
ALTER TABLE repair_requests ADD COLUMN IF NOT EXISTS quote_decline_note text;
