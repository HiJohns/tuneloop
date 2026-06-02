ALTER TABLE damage_assessments ADD COLUMN IF NOT EXISTS org_id UUID;
ALTER TABLE damage_assessments ADD COLUMN IF NOT EXISTS instrument_id UUID;
ALTER TABLE damage_assessments ADD COLUMN IF NOT EXISTS user_id UUID;
ALTER TABLE damage_assessments ADD COLUMN IF NOT EXISTS condition VARCHAR(20);
ALTER TABLE damage_assessments ADD COLUMN IF NOT EXISTS photos JSONB;
ALTER TABLE damage_assessments ADD COLUMN IF NOT EXISTS notes TEXT;
ALTER TABLE damage_assessments ADD COLUMN IF NOT EXISTS scan_time TIMESTAMP;