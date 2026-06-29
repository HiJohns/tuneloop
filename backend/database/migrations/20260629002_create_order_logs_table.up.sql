CREATE TABLE order_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id),
    event VARCHAR(50) NOT NULL,
    operator_id VARCHAR(255),
    operator_name VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_order_logs_order_id ON order_logs(order_id);
CREATE INDEX idx_order_logs_event ON order_logs(event);
