-- #1945 Sub-B：赠点策略（乐币规则）比例配置化
-- 增 referral_ratio（裂变比例，按推荐人级别）、referral_reg_points（邀请奖乐币，按级别）
-- 删 refund_ratio（取消"退款返点给自己"）
ALTER TABLE gift_policies ADD COLUMN IF NOT EXISTS referral_ratio double precision NOT NULL DEFAULT 0;
ALTER TABLE gift_policies ADD COLUMN IF NOT EXISTS referral_reg_points double precision NOT NULL DEFAULT 10;
ALTER TABLE gift_policies DROP COLUMN IF EXISTS refund_ratio;

-- 预设（手册值覆盖）：乐手 2% / 首席 5% / 演奏家 8%
UPDATE gift_policies gp
SET referral_ratio = CASE ml.name
    WHEN '乐手' THEN 0.02
    WHEN '首席' THEN 0.05
    WHEN '演奏家' THEN 0.08
    ELSE gp.referral_ratio
  END
FROM membership_levels ml
WHERE gp.level_id = ml.id;

-- level-0 兜底 2%
UPDATE gift_policies SET referral_ratio = 0.02 WHERE level_id = 0;

-- 邀请奖乐币统一 10（手册）
UPDATE gift_policies SET referral_reg_points = 10 WHERE referral_reg_points IS DISTINCT FROM 10;
