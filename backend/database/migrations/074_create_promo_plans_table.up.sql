CREATE TABLE IF NOT EXISTS promo_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_type VARCHAR(20) NOT NULL DEFAULT 'promo_campaign' CHECK (plan_type IN ('discount_policy', 'promo_campaign')),
    scope_type VARCHAR(20) NOT NULL CHECK (scope_type IN ('system', 'merchant', 'site')),
    scope_id UUID,
    name VARCHAR(100) NOT NULL,
    start_date DATE,
    end_date DATE,
    stackable BOOLEAN NOT NULL DEFAULT false,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    CHECK ((plan_type = 'discount_policy' AND scope_type != 'site') OR (plan_type = 'promo_campaign'))
);
