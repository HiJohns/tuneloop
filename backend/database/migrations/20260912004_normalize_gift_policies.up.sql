-- #1900: normalize gift policy tiers. Prerelease carried test values of
-- 100%/100% (pay/refund), which allowed zero-pay rentals and unbounded point
-- inflation; production had 0% and never granted rebates. New baseline:
--   level 1: pay 5%   refund 0.5%
--   level 2: pay 10%  refund 1%
--   level 3: pay 15%  refund 2%
-- (pay tiers mirror the retained production defaults; refund tiers mirror the
-- rebate_config values that 方案 A retires. level_id=0 fallback untouched.)
UPDATE gift_policies SET pay_ratio = 0.05, refund_ratio = 0.005, is_active = true, updated_at = NOW() WHERE level_id = 1;
UPDATE gift_policies SET pay_ratio = 0.10, refund_ratio = 0.010, is_active = true, updated_at = NOW() WHERE level_id = 2;
UPDATE gift_policies SET pay_ratio = 0.15, refund_ratio = 0.020, is_active = true, updated_at = NOW() WHERE level_id = 3;
