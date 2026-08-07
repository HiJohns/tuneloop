-- Create guarantors + order_guarantors tables and orders.deposit_waived (#1557)
CREATE TABLE IF NOT EXISTS guarantors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    name VARCHAR(100) NOT NULL,
    phone VARCHAR(50) NOT NULL,
    company VARCHAR(200),
    title VARCHAR(100),
    address VARCHAR(500),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS order_guarantors (
    order_id UUID NOT NULL REFERENCES orders(id),
    guarantor_id UUID NOT NULL REFERENCES guarantors(id),
    PRIMARY KEY (order_id, guarantor_id)
);

ALTER TABLE orders ADD COLUMN IF NOT EXISTS deposit_waived BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_guarantors_user ON guarantors(user_id);
CREATE INDEX IF NOT EXISTS idx_order_guarantors_guarantor ON order_guarantors(guarantor_id);
