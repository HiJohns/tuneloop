-- #2248：维修服务单归属从共享 tenant 规范化为商户 org（可推导行）。
-- 1) 经 technician_id → users.tenant_id → merchants.org_id
UPDATE repair_requests rr
SET tenant_id = m.org_id
FROM users u
JOIN merchants m ON m.org_id = u.tenant_id
WHERE rr.type = 'service'
  AND rr.technician_id IS NOT NULL AND rr.technician_id <> ''
  AND rr.technician_id = u.id::text
  AND rr.tenant_id <> m.org_id;

-- 2) 经 site_id → sites.tenant_id → merchants.org_id
UPDATE repair_requests rr
SET tenant_id = m.org_id
FROM sites s
JOIN merchants m ON m.org_id = s.tenant_id
WHERE rr.type = 'service'
  AND rr.site_id ~ '^[0-9a-fA-F-]{36}$'
  AND rr.site_id::uuid = s.id
  AND rr.tenant_id <> m.org_id;
