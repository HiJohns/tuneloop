-- Add ID photo columns to users table (#1598)
ALTER TABLE users ADD COLUMN IF NOT EXISTS id_photo_front VARCHAR(500);
ALTER TABLE users ADD COLUMN IF NOT EXISTS id_photo_back VARCHAR(500);
