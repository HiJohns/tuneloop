-- #1945 审计返工：按 level_id 重放裂变比例（referral_ratio）手册预设。
-- 背景：原迁移 20260917011 的预设以 membership_levels.name 匹配
--   ('乐手'/'首席'/'演奏家')；级别名称被管理员改名后 CASE 未命中，
--   导致 1/2/3 级 referral_ratio 实测为 0（手册应为 2%/5%/8%），裂变返佣静默失效。
-- 修复：改用 level_id 匹配（与对齐迁移 20260917004 的既有约定一致）。
-- 幂等：反复执行结果一致；仅覆盖 {1,2,3}，不动其他行（含 level-0 兜底 0.02）。
UPDATE gift_policies SET referral_ratio = 0.02 WHERE level_id = 1;
UPDATE gift_policies SET referral_ratio = 0.05 WHERE level_id = 2;
UPDATE gift_policies SET referral_ratio = 0.08 WHERE level_id = 3;
