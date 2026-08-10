-- Create gift_policies table (#1605, L-05)
-- Unified per-level gift point rules: pay_ratio (usage cap at payment)
-- and refund_ratio (rebate on refund completion). level_id=0 is the
-- default fallback row for unconfigured levels.
CREATE TABLE IF NOT EXISTS gift_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    level_id INTEGER NOT NULL UNIQUE,
    pay_ratio DECIMAL(5,4) NOT NULL DEFAULT 0.3000,
    refund_ratio DECIMAL(5,4) NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Default fallback row (level_id=0): pay_ratio from points_policies
-- max_pay_ratio (system scope, fallback 0.3), refund_ratio 0.
INSERT INTO gift_policies (level_id, pay_ratio, refund_ratio, is_active)
VALUES (0, 0.3000, 0, true)
ON CONFLICT (level_id) DO NOTHING;

-- Per-level rows: seed pay_ratio from system points_policy max_pay_ratio
-- if present, else default 0.3.
INSERT INTO gift_policies (level_id, pay_ratio, refund_ratio, is_active)
SELECT ml.id,
       COALESCE(pp.max_pay_ratio, 0.3000),
       0,
       true
FROM membership_levels ml
LEFT JOIN points_policies pp ON pp.scope_type = 'system' AND pp.is_active = true
ON CONFLICT (level_id) DO NOTHING;
