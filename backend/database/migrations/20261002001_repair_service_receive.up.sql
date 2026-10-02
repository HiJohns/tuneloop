-- #2116: 维修服务单收货环节——师傅直收（repairing）/ 员工代收（pending_repair）
ALTER TABLE repair_requests ADD COLUMN IF NOT EXISTS receive_photos JSONB DEFAULT '[]'::jsonb;
ALTER TABLE repair_requests ADD COLUMN IF NOT EXISTS received_by UUID;
ALTER TABLE repair_requests ADD COLUMN IF NOT EXISTS received_at TIMESTAMPTZ;
