-- Remove non-instrument-related media records migrated from JSONB
DELETE FROM instrument_media WHERE object_type IN ('repair_request', 'transit_order', 'repair_transit_order', 'appeal');
