-- #2052：邀请通知需记录被邀请人，以便接受时校验「接受人=被邀请人」
ALTER TABLE staff_invites ADD COLUMN IF NOT EXISTS invitee_user_id uuid;
CREATE INDEX IF NOT EXISTS idx_staff_invites_invitee_user_id ON staff_invites(invitee_user_id);
