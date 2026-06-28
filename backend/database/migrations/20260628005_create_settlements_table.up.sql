CREATE TABLE IF NOT EXISTS settlements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    actual_rent_days INT NOT NULL DEFAULT 0,
    actual_rent_amount DECIMAL(10,2) NOT NULL DEFAULT 0,
    original_rent_amount DECIMAL(10,2) NOT NULL DEFAULT 0,
    gift_points_refunded DECIMAL(10,2) NOT NULL DEFAULT 0,
    cash_refundable DECIMAL(10,2) NOT NULL DEFAULT 0,
    prepaid_refunded DECIMAL(10,2) NOT NULL DEFAULT 0,
    refund_method VARCHAR(20) NOT NULL DEFAULT 'prepaid',
    refund_status VARCHAR(20) NOT NULL DEFAULT 'pending',
    overdue_charges_total DECIMAL(10,2) NOT NULL DEFAULT 0,
    breakdown JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_settlements_order_id ON settlements(order_id);
