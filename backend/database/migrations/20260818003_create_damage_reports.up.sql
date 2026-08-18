-- damage_reports (#1706): damage accept/reject records referenced by the
-- damage notification (ref_type=damage_report). The table was missing from
-- migrations while InspectReturn already wrote to it — production insert
-- failed silently and the notification's accept/reject buttons never rendered.
CREATE TABLE IF NOT EXISTS damage_reports (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID NOT NULL,
    org_id             UUID,
    lease_id           UUID NOT NULL,
    instrument_id      UUID NOT NULL,
    user_id            UUID NOT NULL,
    damage_amount      DECIMAL(10,2),
    damage_description TEXT,
    assessed_by        UUID,
    assessed_at        TIMESTAMP,
    deposit_deducted   DECIMAL(10,2) NOT NULL DEFAULT 0,
    status             VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at         TIMESTAMP NOT NULL DEFAULT now(),
    updated_at         TIMESTAMP NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_damage_reports_tenant ON damage_reports (tenant_id);
CREATE INDEX IF NOT EXISTS idx_damage_reports_lease ON damage_reports (lease_id);
CREATE INDEX IF NOT EXISTS idx_damage_reports_user ON damage_reports (user_id);
CREATE INDEX IF NOT EXISTS idx_damage_reports_status ON damage_reports (status);
