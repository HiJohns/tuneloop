-- #1945 回滚：移除 referral_ratio/referral_reg_points，恢复 refund_ratio
ALTER TABLE gift_policies ADD COLUMN IF NOT EXISTS refund_ratio double precision NOT NULL DEFAULT 0;
ALTER TABLE gift_policies DROP COLUMN IF EXISTS referral_reg_points;
ALTER TABLE gift_policies DROP COLUMN IF EXISTS referral_ratio;
