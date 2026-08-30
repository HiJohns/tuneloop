-- #1807: 第三证件类型（学生证/教师证/工作证等）— users.id_photo_other_type 列
ALTER TABLE users ADD COLUMN IF NOT EXISTS id_photo_other_type VARCHAR(50);
