-- Down migration: Drop repair request tables (Issue #1110)
DROP TABLE IF EXISTS repair_request_records;
DROP TABLE IF EXISTS repair_requests;
DROP TABLE IF EXISTS user_instruments;
