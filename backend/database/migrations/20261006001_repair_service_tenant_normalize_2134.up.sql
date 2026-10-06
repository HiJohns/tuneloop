-- #2134 P3：维修服务单 tenant_id 规范化（叶组织 id → 根租户 id）
--
-- 背景：写入链曾把商户 org_id 当 tenant_id 落库（本商户数据 tenant_id=org_id 混用，
-- 见 #2125），而 IAMInterceptor 会把 JWT 的 tid/oid 向上追溯为「根租户」用于 GORM
-- 自动租户范围（addTenantScope），导致 staff 上下文（scope=mine/site）过滤掉这些行（#2134）。
--
-- 依据：merchants(org_id → tenant_id) 映射（本表编码了 org→tenant 关系）。
-- 幂等：仅更新能在 merchants.org_id 命中、且 tenant_id 确有差异的行；重复执行无副作用。
-- 不改动 updated_at（避免影响 #2128 的「近 30 天」窗口语义）。
-- 数据迁移，不涉及表/列变更 → 无需 defaultMigrationArtifacts 工件断言登记。
--
-- ⚠️ 数据回填：本迁移会修改既有行。按 #2134 计划，执行需用户授权
-- （随 deploy 应用即视为授权；如需单独控制，请先与维护者确认）。

UPDATE repair_requests rr
SET tenant_id = m.tenant_id
FROM merchants m
WHERE rr.tenant_id = m.org_id
  AND rr.tenant_id IS DISTINCT FROM m.tenant_id;

UPDATE technician_profiles tp
SET tenant_id = m.tenant_id
FROM merchants m
WHERE tp.tenant_id = m.org_id
  AND tp.tenant_id IS DISTINCT FROM m.tenant_id;
