-- Two-phase registration (#1682): the reserved init user (IAM id) stored on
-- the registration session so the payment callback activates it instead of
-- re-creating the account (atomic reservation at session creation).
ALTER TABLE registration_sessions ADD COLUMN IF NOT EXISTS iam_user_id UUID;
