-- Up migration: Add appeal desensitization/forwarding fields (v3 repair-appeals)
ALTER TABLE appeals
    ADD COLUMN IF NOT EXISTS reviewer_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS desensitized_description TEXT,
    ADD COLUMN IF NOT EXISTS forwarded_to VARCHAR(255);
