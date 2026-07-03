-- Migrate existing JSONB photos from instrument-linked tables to instrument_media
-- These are historical records; new writes go to instrument_media via handler dual-write.

-- maintenance_tickets.repair_photos → instrument_media (batch_type='repair')
INSERT INTO instrument_media (tenant_id, org_id, instrument_id, batch_id, batch_type, file_name, file_type, storage_key, is_display, sort_order, created_at)
SELECT
  t.tenant_id,
  t.org_id,
  t.instrument_id,
  gen_random_uuid() AS batch_id,
  'repair' AS batch_type,
  'repair_photo.jpg' AS file_name,
  'image' AS file_type,
  trim(BOTH '"' FROM photo) AS storage_key,
  false AS is_display,
  row_number() OVER (PARTITION BY t.id) - 1 AS sort_order,
  t.created_at
FROM maintenance_tickets t,
LATERAL jsonb_array_elements_text(t.repair_photos) AS photo
WHERE t.repair_photos IS NOT NULL AND t.repair_photos != '[]'::jsonb
  AND t.instrument_id IS NOT NULL;

-- maintenance_tickets.completion_photos → instrument_media (batch_type='repaired')
INSERT INTO instrument_media (tenant_id, org_id, instrument_id, batch_id, batch_type, file_name, file_type, storage_key, is_display, sort_order, created_at)
SELECT
  t.tenant_id,
  t.org_id,
  t.instrument_id,
  gen_random_uuid() AS batch_id,
  'repaired' AS batch_type,
  'completion_photo.jpg' AS file_name,
  'image' AS file_type,
  trim(BOTH '"' FROM photo) AS storage_key,
  false AS is_display,
  row_number() OVER (PARTITION BY t.id) - 1 AS sort_order,
  t.completed_at
FROM maintenance_tickets t,
LATERAL jsonb_array_elements_text(t.completion_photos) AS photo
WHERE t.completion_photos IS NOT NULL AND t.completion_photos != '[]'::jsonb
  AND t.instrument_id IS NOT NULL;

-- damage_assessments.photos → instrument_media (batch_type='receiving')
INSERT INTO instrument_media (tenant_id, org_id, instrument_id, batch_id, batch_type, file_name, file_type, storage_key, is_display, sort_order, created_at)
SELECT
  d.tenant_id,
  d.org_id,
  d.instrument_id,
  gen_random_uuid() AS batch_id,
  'receiving' AS batch_type,
  'assessment_photo.jpg' AS file_name,
  'image' AS file_type,
  trim(BOTH '"' FROM photo) AS storage_key,
  false AS is_display,
  row_number() OVER (PARTITION BY d.id) - 1 AS sort_order,
  d.created_at
FROM damage_assessments d,
LATERAL jsonb_array_elements_text(d.photos) AS photo
WHERE d.photos IS NOT NULL AND d.photos != '[]'::jsonb
  AND d.instrument_id IS NOT NULL;

-- NOTE: repair_records photos are not backfilled here. The repair_records table
-- was never created by an earlier migration (see 20260702005), so it holds no
-- historical data. New repair-record photos are written to instrument_media
-- directly via the handler dual-write path (handlers/repair.go).
