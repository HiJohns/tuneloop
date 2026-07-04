-- Down migration: Remove appeal desensitization/forwarding fields
ALTER TABLE appeals
    DROP COLUMN IF EXISTS reviewer_id,
    DROP COLUMN IF EXISTS desensitized_description,
    DROP COLUMN IF EXISTS forwarded_to;
