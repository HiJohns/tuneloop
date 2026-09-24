-- #2052 down
DROP INDEX IF EXISTS idx_staff_invites_invitee_user_id;
ALTER TABLE staff_invites DROP COLUMN IF EXISTS invitee_user_id;
