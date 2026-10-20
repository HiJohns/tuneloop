-- #2250：merchant_members 归属锚规范化为商户 org（原为共享 tenant）。
UPDATE merchant_members mm
SET tenant_id = m.org_id
FROM merchants m
WHERE mm.merchant_id = m.id
  AND mm.tenant_id <> m.org_id;
