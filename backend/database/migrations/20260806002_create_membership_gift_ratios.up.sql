-- Create membership_gift_ratios table (#1536)
-- Per-level gift point ratios: self-spend loyalty (#1542), referral
-- registration bonus (#1534), referral spend commission (#1535).
CREATE TABLE IF NOT EXISTS membership_gift_ratios (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    level_id INTEGER NOT NULL UNIQUE REFERENCES membership_levels(id),
    self_spend_ratio DECIMAL(5,4) NOT NULL DEFAULT 0,
    referral_reg_points DECIMAL(10,2) NOT NULL DEFAULT 0,
    referral_spend_ratio DECIMAL(5,4) NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Default rows per membership level (all zeros; admin configures values)
INSERT INTO membership_gift_ratios (level_id, self_spend_ratio, referral_reg_points, referral_spend_ratio)
SELECT id, 0, 0, 0 FROM membership_levels
ON CONFLICT (level_id) DO NOTHING;
