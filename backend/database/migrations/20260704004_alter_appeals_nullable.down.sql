-- Down migration: Revert appeals columns to NOT NULL
ALTER TABLE appeals
    ALTER COLUMN damage_report_id SET NOT NULL,
    ALTER COLUMN user_id SET NOT NULL,
    ALTER COLUMN appeal_reason SET NOT NULL;
