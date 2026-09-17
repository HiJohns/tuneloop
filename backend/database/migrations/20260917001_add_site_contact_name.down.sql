-- Rollback #1935: drop sites.contact_name
ALTER TABLE sites DROP COLUMN IF EXISTS contact_name;
