-- #2626: users.roles = 派生自 IAM 的功能角色多值镜像。
--
-- 权威来源 = IAM user_org_relations（组织角色 role + 功能角色 functional_roles[]）；
-- 本列是它的镜像（functional_roles ∪ 派生主角色），派生实现见
-- backend/services/role_derive.go（DeriveRoles）。users.role 保留为派生主角色，
-- 仅用于兼容旧读路径；三步走（镜像多值 → 前端改读 → 删列）的第三步另立 Issue。
--
-- 本迁移只加列：存量行一律留空数组，**不在 SQL 内跨库推导或猜测角色值**
-- （IAM 在 beaconiam 库，跨库回填属部署侧；历史数据修复见 #2626 F1–F4 记录）。
ALTER TABLE users ADD COLUMN IF NOT EXISTS roles JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN users.roles IS
    '#2626 派生自 IAM user_org_relations（role + functional_roles[]）的多值镜像，禁止独立写；users.role 仅为派生主角色';
