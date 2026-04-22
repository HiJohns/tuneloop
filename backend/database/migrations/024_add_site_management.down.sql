-- 024_add_site_management.down.sql
-- 回滚网点管理功能增强

-- 删除外键约束（如果已添加）
-- ALTER TABLE sites DROP CONSTRAINT IF EXISTS fk_sites_manager;
-- ALTER TABLE sites DROP CONSTRAINT IF EXISTS fk_sites_parent;

-- 删除索引
DROP INDEX IF EXISTS idx_sites_manager_id;
DROP INDEX IF EXISTS idx_sites_parent_id;
DROP INDEX IF EXISTS idx_sites_deleted_at;

-- 删除 sites 表新增字段（注意：这会丢失数据）
ALTER TABLE sites 
DROP COLUMN IF EXISTS site_type,
DROP COLUMN IF EXISTS address,
DROP COLUMN IF EXISTS contact_phone,
DROP COLUMN IF EXISTS manager_id,
DROP COLUMN IF EXISTS parent_id,
DROP COLUMN IF EXISTS deleted_at;

-- users 表回滚
DROP INDEX IF EXISTS idx_users_site_id;
DROP INDEX IF EXISTS idx_users_user_type;
DROP INDEX IF EXISTS idx_users_deleted_at;

ALTER TABLE users 
DROP COLUMN IF EXISTS position,
DROP COLUMN IF EXISTS user_type,
DROP COLUMN IF EXISTS site_id,
DROP COLUMN IF EXISTS deleted_at;
