-- #2073：全员富文本简介（创建用户时可用；师傅类型仍双写 technician_profiles.bio）
ALTER TABLE users ADD COLUMN IF NOT EXISTS bio text;
