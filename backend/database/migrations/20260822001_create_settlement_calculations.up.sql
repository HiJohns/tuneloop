-- #1738 P2: settlement_calculations — append-only audit trail of every
-- settlement computation (preview via GET calculate, confirm via POST).
-- Stores the order input snapshot and the computed result (cents JSONB)
-- so any displayed amount can be traced to a persisted calculation.
CREATE TABLE IF NOT EXISTS settlement_calculations (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id       UUID NOT NULL,
    tenant_id      UUID,
    trigger        VARCHAR(20) NOT NULL DEFAULT 'preview',
    input_snapshot JSONB,
    result         JSONB,
    actual_days    INTEGER NOT NULL DEFAULT 0,
    created_at     TIMESTAMP NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sc_order_id  ON settlement_calculations (order_id);
CREATE INDEX IF NOT EXISTS idx_sc_tenant_id ON settlement_calculations (tenant_id);
