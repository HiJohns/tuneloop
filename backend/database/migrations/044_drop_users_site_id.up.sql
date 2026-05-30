-- Remove redundant users.site_id column; use site_members table for
-- user-site relationships. Issue #693.
ALTER TABLE users DROP COLUMN IF EXISTS site_id;
