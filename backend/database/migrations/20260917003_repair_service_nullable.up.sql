-- #1942 阶段2：维修服务单在「用户创建 → 选维修师」之间尚无网点/租户归属，
-- site_id/tenant_id 必须可空（选师时回填师傅所属网点的 site/tenant）。
-- v3 报修（type='warranty'）创建即带 site/tenant，存量行不受影响。
ALTER TABLE repair_requests ALTER COLUMN site_id DROP NOT NULL;
ALTER TABLE repair_requests ALTER COLUMN tenant_id DROP NOT NULL;
