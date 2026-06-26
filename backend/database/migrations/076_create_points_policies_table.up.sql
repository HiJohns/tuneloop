CREATE TABLE IF NOT EXISTS points_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope_type VARCHAR(20) NOT NULL CHECK (scope_type IN ('system', 'merchant', 'site')),
    scope_id UUID,
    max_pay_ratio DECIMAL(5,4),
    valid_days INTEGER,
    is_active BOOLEAN NOT NULL DEFAULT true
);
