CREATE TABLE IF NOT EXISTS overdue_charges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    charge_date DATE NOT NULL,
    amount DECIMAL(10,2) NOT NULL,
    deducted_from_prepaid DECIMAL(10,2) NOT NULL DEFAULT 0,
    remaining_balance DECIMAL(10,2) NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'success',
    failure_reason VARCHAR(500),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_overdue_charges_order_id ON overdue_charges(order_id);
CREATE INDEX idx_overdue_charges_charge_date ON overdue_charges(charge_date);
CREATE INDEX idx_overdue_charges_status ON overdue_charges(status);
