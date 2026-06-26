CREATE TABLE IF NOT EXISTS instrument_promo_overrides (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    instrument_id UUID NOT NULL REFERENCES instruments(id) ON DELETE CASCADE,
    override_type VARCHAR(20) NOT NULL CHECK (override_type IN ('discount', 'rebate')),
    enabled BOOLEAN NOT NULL DEFAULT true,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, instrument_id, override_type)
);
