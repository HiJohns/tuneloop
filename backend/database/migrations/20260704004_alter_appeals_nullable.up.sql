-- Up migration: Make appeal damage_report_id, user_id, appeal_reason nullable (v3 repair-appeals don't fill these)
ALTER TABLE appeals
    ALTER COLUMN damage_report_id DROP NOT NULL,
    ALTER COLUMN user_id DROP NOT NULL,
    ALTER COLUMN appeal_reason DROP NOT NULL;
