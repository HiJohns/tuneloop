-- 024_add_site_management.up.sql
-- 网点管理功能增强

-- 1. sites 表增强
ALTER TABLE sites 
ADD COLUMN IF NOT EXISTS site_type VARCHAR(20) DEFAULT '直营',  -- 加盟、直营、合作店
ADD COLUMN IF NOT EXISTS address TEXT,
ADD COLUMN IF NOT EXISTS contact_phone VARCHAR(50),
ADD COLUMN IF NOT EXISTS manager_id UUID,
ADD COLUMN IF NOT EXISTS parent_id UUID,
ADD COLUMN IF NOT EXISTS created_at TIMESTAMP NOT NULL DEFAULT NOW(),
ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP;

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_sites_tenant_id ON sites(tenant_id);
CREATE INDEX IF NOT EXISTS idx_sites_parent_id ON sites(parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sites_manager_id ON sites(manager_id) WHERE manager_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_sites_deleted_at ON sites(deleted_at) WHERE deleted_at IS NOT NULL;

-- 外键约束（可选，如果希望添加外键）
-- ALTER TABLE sites 
-- ADD CONSTRAINT fk_sites_manager FOREIGN KEY (manager_id) REFERENCES users(id),
-- ADD CONSTRAINT fk_sites_parent FOREIGN KEY (parent_id) REFERENCES sites(id);

-- 2. users 表增强
ALTER TABLE users 
ADD COLUMN IF NOT EXISTS position VARCHAR(100),
ADD COLUMN IF NOT EXISTS user_type VARCHAR(20) DEFAULT '员工',  -- 管理、员工
ADD COLUMN IF NOT EXISTS site_id UUID,
ADD COLUMN IF NOT EXISTS created_at TIMESTAMP NOT NULL DEFAULT NOW(),
ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP;

-- users 表索引
CREATE INDEX IF NOT EXISTS idx_users_site_id ON users(site_id) WHERE site_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_user_type ON users(user_type);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users(deleted_at) WHERE deleted_at IS NOT NULL;

-- 唯一性约束：姓名、电邮、电话三者唯一（允许 NULL）
-- 需要根据业务需求决定是否添加

-- 3. 初始数据迁移（可选）
-- 将现有数据标记为有效
UPDATE sites SET 
  created_at = COALESCE(created_at, NOW()), 
  updated_at = COALESCE(updated_at, NOW())
WHERE created_at IS NULL;

UPDATE users SET 
  created_at = COALESCE(created_at, NOW()), 
  updated_at = COALESCE(updated_at, NOW())
WHERE created_at IS NULL;
