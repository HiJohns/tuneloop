-- #2248：维修服务单归属从共享 tenant 规范化为商户 org（可推导行）。
-- 列均为 uuid：直接等值比较（不得与 ''/text 比较，否则启动迁移 FATAL）。
-- 1) 经 technician_id (= users.id) → users.tenant_id → merchants.org_id
UPDATE repair_requests rr
SET tenant_id = m.org_id
FROM users u
JOIN merchants m ON m.org_id = u.tenant_id
WHERE rr.type = 'service'
  AND rr.technician_id IS NOT NULL
  AND rr.technician_id = u.id
  AND rr.tenant_id IS DISTINCT FROM m.org_id;

-- 2) 经 site_id → sites.tenant_id → merchants.org_id
UPDATE repair_requests rr
SET tenant_id = m.org_id
FROM sites s
JOIN merchants m ON m.org_id = s.tenant_id
WHERE rr.type = 'service'
  AND rr.site_id IS NOT NULL
  AND rr.site_id = s.id
  AND rr.tenant_id IS DISTINCT FROM m.org_id;
