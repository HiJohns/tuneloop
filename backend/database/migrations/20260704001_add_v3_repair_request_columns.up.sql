-- Up migration: Add v3 fields to repair_requests (Issue #1201)
ALTER TABLE repair_requests
    ADD COLUMN IF NOT EXISTS merchant_type VARCHAR(10) NOT NULL DEFAULT 'full',
    ADD COLUMN IF NOT EXISTS transit_site_id UUID,
    ADD COLUMN IF NOT EXISTS controlled_site_id UUID,
    ADD COLUMN IF NOT EXISTS accepted_quote_id UUID,
    ADD COLUMN IF NOT EXISTS check_fee_snapshot DECIMAL(10,2),
    ADD COLUMN IF NOT EXISTS paid_amount DECIMAL(10,2),
    ADD COLUMN IF NOT EXISTS expire_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reminder_sent BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_repair_requests_transit_site_id ON repair_requests(transit_site_id);
CREATE INDEX IF NOT EXISTS idx_repair_requests_controlled_site_id ON repair_requests(controlled_site_id);
CREATE INDEX IF NOT EXISTS idx_repair_requests_accepted_quote_id ON repair_requests(accepted_quote_id);
