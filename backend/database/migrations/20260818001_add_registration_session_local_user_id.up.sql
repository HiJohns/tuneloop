-- Two-phase registration (#1688): the reserved local users cache id stored
-- on the registration session so the payment record's user_id references the
-- real local user (consumption records queryable after activation).
ALTER TABLE registration_sessions ADD COLUMN IF NOT EXISTS local_user_id UUID;
