-- #2034 一店多角色：site_members 支持多重角色（IAM functional_roles 已支持多重）
ALTER TABLE site_members ADD COLUMN IF NOT EXISTS roles JSONB NOT NULL DEFAULT '[]'::jsonb;
-- backfill：存量单角色 → roles=[role]（仅当 roles 为空且 role 非空）
UPDATE site_members
   SET roles = jsonb_build_array(role)
 WHERE (roles = '[]'::jsonb OR roles IS NULL)
   AND role IS NOT NULL
   AND role <> '';
