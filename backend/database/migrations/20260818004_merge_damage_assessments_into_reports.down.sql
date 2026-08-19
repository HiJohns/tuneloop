-- #1708/#1711: reverse the merge — recreate damage_assessments.
-- Data is NOT copied back (reports are authoritative post-migration); the
-- restored table is empty and legacy reads fall back to damage_reports.
CREATE TABLE IF NOT EXISTS damage_assessments (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL,
    org_id        UUID,
    order_id      UUID NOT NULL,
    instrument_id UUID,
    user_id       UUID,
    inspector_id  UUID,
    condition     VARCHAR(20),
    description   TEXT,
    photos        JSONB,
    notes         TEXT,
    estimated_cost DECIMAL(10,2),
    overdue_days  INTEGER NOT NULL DEFAULT 0,
    overdue_fee   DECIMAL(10,2) NOT NULL DEFAULT 0,
    scan_time     TIMESTAMP,
    status        VARCHAR(20) DEFAULT 'pending',
    created_at    TIMESTAMP NOT NULL DEFAULT now(),
    updated_at    TIMESTAMP NOT NULL DEFAULT now()
);
