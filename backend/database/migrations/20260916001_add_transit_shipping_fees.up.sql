-- #1934: forwarding-session segment tracking fields + transit shipping fee
-- segments (docs/cases/transit.md v1: fee split matrix — customer pays
-- segments 1+2+3, merchant pays segment 4).

ALTER TABLE forwarding_sessions
    ADD COLUMN IF NOT EXISTS tracking_company VARCHAR(100) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS tracking_number VARCHAR(100) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS photos JSONB,
    ADD COLUMN IF NOT EXISTS logistics_fee_cents BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS transit_shipping_fees (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id      UUID NOT NULL,
    direction     VARCHAR(20) NOT NULL,       -- outbound / return
    segment       INT NOT NULL,               -- 1=受控→中转 2=中转→顾客 3=顾客→中转 4=中转→受控
    amount        BIGINT NOT NULL DEFAULT 0,
    paid_by       VARCHAR(20) NOT NULL,       -- customer / merchant
    recorded_by   VARCHAR(255) NOT NULL,
    created_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_transit_shipping_fees_order ON transit_shipping_fees (order_id);