-- Create repair_records table (backfills the table for the RepairRecord model
-- introduced in issue-1104, which was never given a migration).
CREATE TABLE IF NOT EXISTS repair_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instrument_id UUID NOT NULL,
    worker_id VARCHAR(255) NOT NULL,
    comment TEXT,
    photos JSONB DEFAULT '[]',
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_repair_records_instrument ON repair_records(instrument_id);
