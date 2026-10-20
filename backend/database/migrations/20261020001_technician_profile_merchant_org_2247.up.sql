-- #2247：维修师档案归属锚规范化为「商户 org」。
-- 依据：users.tenant_id（历史值）经 merchants.org_id 映射到唯一商户组织。
UPDATE technician_profiles tp
SET tenant_id = m.org_id
FROM users u
JOIN merchants m ON m.org_id = u.tenant_id
WHERE tp.user_id = u.id
  AND tp.tenant_id <> m.org_id;
