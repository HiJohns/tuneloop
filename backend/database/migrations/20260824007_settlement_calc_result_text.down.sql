-- Restore JSONB typing for settlement_calculations.result (reverse of
-- 20260824007). Stored TEXT payloads are valid JSON and cast cleanly.
ALTER TABLE settlement_calculations ALTER COLUMN result TYPE JSONB USING result::jsonb;
