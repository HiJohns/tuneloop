CREATE TABLE IF NOT EXISTS promo_plan_details (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    promo_plan_id UUID NOT NULL REFERENCES promo_plans(id) ON DELETE CASCADE,
    level_id INTEGER NOT NULL REFERENCES membership_levels(id),
    rent_discount DECIMAL(5,4),
    deposit_discount DECIMAL(5,4),
    overdue_discount DECIMAL(5,4)
);
