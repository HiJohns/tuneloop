-- #2057 裁定3：学生证作为第二证件时，介绍信为必传（仅此情形要求）
ALTER TABLE users ADD COLUMN IF NOT EXISTS intro_letter_url varchar(500);
