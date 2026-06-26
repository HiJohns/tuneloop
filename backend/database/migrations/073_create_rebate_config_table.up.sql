CREATE TABLE IF NOT EXISTS rebate_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    level_id INTEGER NOT NULL UNIQUE REFERENCES membership_levels(id),
    rent_ratio DECIMAL(5,4) NOT NULL DEFAULT 0.01,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

INSERT INTO rebate_config (level_id, rent_ratio) VALUES
    (1, 0.005),
    (2, 0.01),
    (3, 0.02);
