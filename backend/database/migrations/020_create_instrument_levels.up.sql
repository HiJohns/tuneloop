-- Create instrument_levels table
CREATE TABLE instrument_levels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    caption VARCHAR(50) NOT NULL UNIQUE,
    code VARCHAR(20) NOT NULL UNIQUE,
    sort_order INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Insert default levels
INSERT INTO instrument_levels (caption, code, sort_order) VALUES
    ('入门', 'entry', 1),
    ('专业', 'professional', 2),
    ('大师', 'master', 3);

-- Add level_id foreign key to instruments table
ALTER TABLE instruments ADD COLUMN IF NOT EXISTS level_id UUID REFERENCES instrument_levels(id);
CREATE INDEX IF NOT EXISTS idx_instruments_level_id ON instruments(level_id);

-- Migrate existing data
-- Map level strings to level_id
UPDATE instruments i SET level_id = il.id 
FROM instrument_levels il 
WHERE (i.level = il.code OR i.level_name = il.caption OR i.level = il.caption)
AND i.level_id IS NULL;