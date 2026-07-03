-- Migrate existing JSONB photos from non-instrument tables to instrument_media
-- These use object_type + object_id instead of instrument_id.

-- repair_requests.photos → instrument_media (object_type='repair_request')
INSERT INTO instrument_media (tenant_id, org_id, object_type, object_id, batch_id, batch_type, file_name, file_type, storage_key, is_display, sort_order, created_at)
SELECT
  r.tenant_id,
  NULL AS org_id,
  'repair_request' AS object_type,
  r.id AS object_id,
  gen_random_uuid() AS batch_id,
  'repair' AS batch_type,
  'repair_photo.jpg' AS file_name,
  'image' AS file_type,
  trim(BOTH '"' FROM photo) AS storage_key,
  false AS is_display,
  row_number() OVER (PARTITION BY r.id) - 1 AS sort_order,
  r.created_at
FROM repair_requests r,
LATERAL jsonb_array_elements_text(r.photos::jsonb) AS photo
WHERE r.photos IS NOT NULL AND r.photos != '[]'::jsonb;

-- repair_request_records.photos → instrument_media (object_type='repair_request_record')
-- tenant_id is derived from the parent repair_request (records carry no tenant_id).
INSERT INTO instrument_media (tenant_id, org_id, object_type, object_id, batch_id, batch_type, file_name, file_type, storage_key, is_display, sort_order, created_at)
SELECT
  req.tenant_id,
  NULL AS org_id,
  'repair_request_record' AS object_type,
  rr.id AS object_id,
  gen_random_uuid() AS batch_id,
  'repair' AS batch_type,
  'record_photo.jpg' AS file_name,
  'image' AS file_type,
  trim(BOTH '"' FROM photo) AS storage_key,
  false AS is_display,
  row_number() OVER (PARTITION BY rr.id) - 1 AS sort_order,
  rr.created_at
FROM repair_request_records rr
JOIN repair_requests req ON req.id = rr.repair_request_id,
LATERAL jsonb_array_elements_text(rr.photos::jsonb) AS photo
WHERE rr.photos IS NOT NULL AND rr.photos != '[]'::jsonb;

-- transit_orders.unpack_photos → instrument_media (object_type='transit_order')
-- tenant_id/org_id are derived from the parent order (transit_orders carry neither).
INSERT INTO instrument_media (tenant_id, org_id, object_type, object_id, batch_id, batch_type, file_name, file_type, storage_key, is_display, sort_order, created_at)
SELECT
  o.tenant_id,
  o.org_id,
  'transit_order' AS object_type,
  t.id AS object_id,
  gen_random_uuid() AS batch_id,
  'relaying' AS batch_type,
  'unpack_photo.jpg' AS file_name,
  'image' AS file_type,
  trim(BOTH '"' FROM photo) AS storage_key,
  false AS is_display,
  row_number() OVER (PARTITION BY t.id) - 1 AS sort_order,
  t.updated_at
FROM transit_orders t
JOIN orders o ON o.id = t.order_id,
LATERAL jsonb_array_elements_text(t.unpack_photos::jsonb) AS photo
WHERE t.unpack_photos IS NOT NULL AND t.unpack_photos != '[]'::jsonb;

-- repair_transit_orders.unpack_photos → instrument_media (object_type='repair_transit_order')
-- tenant_id is derived from the parent repair_request (transit orders carry no tenant_id).
INSERT INTO instrument_media (tenant_id, org_id, object_type, object_id, batch_id, batch_type, file_name, file_type, storage_key, is_display, sort_order, created_at)
SELECT
  req.tenant_id,
  NULL AS org_id,
  'repair_transit_order' AS object_type,
  t.id AS object_id,
  gen_random_uuid() AS batch_id,
  'relaying' AS batch_type,
  'unpack_photo.jpg' AS file_name,
  'image' AS file_type,
  trim(BOTH '"' FROM photo) AS storage_key,
  false AS is_display,
  row_number() OVER (PARTITION BY t.id) - 1 AS sort_order,
  t.updated_at
FROM repair_transit_orders t
JOIN repair_requests req ON req.id = t.repair_request_id,
LATERAL jsonb_array_elements_text(t.unpack_photos::jsonb) AS photo
WHERE t.unpack_photos IS NOT NULL AND t.unpack_photos != '[]'::jsonb;

-- appeals.images → instrument_media (object_type='appeal')
INSERT INTO instrument_media (tenant_id, org_id, object_type, object_id, batch_id, batch_type, file_name, file_type, storage_key, is_display, sort_order, created_at)
SELECT
  a.tenant_id,
  a.org_id,
  'appeal' AS object_type,
  a.id AS object_id,
  gen_random_uuid() AS batch_id,
  'repair' AS batch_type,
  'appeal_image.jpg' AS file_name,
  'image' AS file_type,
  trim(BOTH '"' FROM photo) AS storage_key,
  false AS is_display,
  row_number() OVER (PARTITION BY a.id) - 1 AS sort_order,
  a.created_at
FROM appeals a,
LATERAL jsonb_array_elements_text(a.images::jsonb) AS photo
WHERE a.images IS NOT NULL AND a.images != '[]'::jsonb;
