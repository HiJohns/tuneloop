DROP INDEX IF EXISTS idx_face_capture_batches_status;
DROP INDEX IF EXISTS idx_face_capture_batches_user;
DROP TABLE IF EXISTS face_capture_batches;
ALTER TABLE users DROP COLUMN IF EXISTS face_verify_method;
