-- #1945 回滚：恢复预设修复前的状态（1/2/3 级 referral_ratio = 0）。
UPDATE gift_policies SET referral_ratio = 0 WHERE level_id IN (1, 2, 3);
