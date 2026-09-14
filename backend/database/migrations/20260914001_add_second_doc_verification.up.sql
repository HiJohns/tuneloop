-- #1924: second-document verification state and review-batch kind.
ALTER TABLE users ADD COLUMN IF NOT EXISTS id_photo_other_verified BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE face_capture_batches ADD COLUMN IF NOT EXISTS kind VARCHAR(20) NOT NULL DEFAULT 'registration';
