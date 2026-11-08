CREATE TABLE IF NOT EXISTS order_term_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    version int NOT NULL,
    start_date date,
    end_date date,
    days int NOT NULL DEFAULT 0,
    tier_segments jsonb NOT NULL DEFAULT '[]'::jsonb,
    total_amount bigint NOT NULL DEFAULT 0,
    payment_record_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (order_id, version)
);
CREATE INDEX IF NOT EXISTS idx_order_term_versions_order ON order_term_versions(order_id);
