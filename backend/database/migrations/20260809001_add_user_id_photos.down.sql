-- Drop ID photo columns from users table (#1598)
ALTER TABLE users DROP COLUMN IF EXISTS id_photo_front;
ALTER TABLE users DROP COLUMN IF EXISTS id_photo_back;
