-- #2114 down
DROP INDEX IF EXISTS idx_staff_invites_tenant_status;
DROP INDEX IF EXISTS idx_staff_invites_requested_by;
ALTER TABLE staff_invites DROP COLUMN IF EXISTS reject_reason;
ALTER TABLE staff_invites DROP COLUMN IF EXISTS approved_at;
ALTER TABLE staff_invites DROP COLUMN IF EXISTS approved_by;
ALTER TABLE staff_invites DROP COLUMN IF EXISTS request_note;
ALTER TABLE staff_invites DROP COLUMN IF EXISTS requested_by;
ALTER TABLE staff_invites DROP COLUMN IF EXISTS invitee_name;
ALTER TABLE staff_invites DROP COLUMN IF EXISTS invitee_identifier;
ALTER TABLE staff_invites DROP COLUMN IF EXISTS kind;
