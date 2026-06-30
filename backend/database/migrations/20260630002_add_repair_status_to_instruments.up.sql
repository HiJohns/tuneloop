ALTER TABLE instruments ADD COLUMN IF NOT EXISTS repair_status VARCHAR(20);
ALTER TABLE instruments ADD COLUMN IF NOT EXISTS repair_worker_id UUID;
CREATE INDEX IF NOT EXISTS idx_instruments_repair_status ON instruments(repair_status);
CREATE INDEX IF NOT EXISTS idx_instruments_repair_worker ON instruments(repair_worker_id);
