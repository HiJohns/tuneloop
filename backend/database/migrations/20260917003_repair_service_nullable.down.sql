-- 回滚前需确保无 NULL 行（未选师的服务单必须先回填或清理）。
ALTER TABLE repair_requests ALTER COLUMN site_id SET NOT NULL;
ALTER TABLE repair_requests ALTER COLUMN tenant_id SET NOT NULL;
