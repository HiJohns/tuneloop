ALTER TABLE merchants DROP COLUMN IF EXISTS admin_pending;
ALTER TABLE sites DROP COLUMN IF EXISTS manager_pending;
ALTER TABLE site_members DROP COLUMN IF EXISTS status;
ALTER TABLE site_members DROP COLUMN IF EXISTS iam_task_id;
