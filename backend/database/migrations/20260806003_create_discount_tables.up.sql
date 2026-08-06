-- Create discount code system tables (#1539)
CREATE TABLE IF NOT EXISTS discount_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    rent_discount DECIMAL(5,4) NOT NULL DEFAULT 1,
    deposit_discount DECIMAL(5,4) NOT NULL DEFAULT 1,
    shipping_discount DECIMAL(5,4) NOT NULL DEFAULT 1,
    max_amount DECIMAL(10,2) NOT NULL DEFAULT 0,
    valid_from TIMESTAMP,
    valid_to TIMESTAMP,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS discount_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(50) NOT NULL UNIQUE,
    policy_id UUID NOT NULL REFERENCES discount_policies(id),
    max_uses INTEGER NOT NULL DEFAULT 0,
    usage_count INTEGER NOT NULL DEFAULT 0,
    expires_at TIMESTAMP,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS discount_code_usages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code_id UUID NOT NULL REFERENCES discount_codes(id),
    order_id UUID,
    user_id UUID,
    discount_amount DECIMAL(10,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_discount_codes_policy ON discount_codes(policy_id);
CREATE INDEX IF NOT EXISTS idx_discount_usages_code ON discount_code_usages(code_id);
CREATE INDEX IF NOT EXISTS idx_discount_usages_order ON discount_code_usages(order_id);
