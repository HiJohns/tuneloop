-- 回滚至 072 种子命名（金额不变）
UPDATE membership_levels SET name = '初级', min_amount_cents = 0       WHERE id = 1;
UPDATE membership_levels SET name = '中级', min_amount_cents = 500000  WHERE id = 2;
UPDATE membership_levels SET name = '高级', min_amount_cents = 1000000 WHERE id = 3;
