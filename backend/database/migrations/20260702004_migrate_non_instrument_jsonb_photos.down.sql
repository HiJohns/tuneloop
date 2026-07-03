-- Remove non-instrument-related media records migrated from JSONB
DELETE FROM instrument_media WHERE object_type IN ('repair_request', 'repair_request_record', 'transit_order', 'repair_transit_order', 'appeal');
