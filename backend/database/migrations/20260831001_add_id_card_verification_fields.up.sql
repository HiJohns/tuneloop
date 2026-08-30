-- #1807: 实名核身人工审核补全字段 — 身份证有效期/签发机关/住址（员工按证件照填写）
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS id_card_expire varchar(20),
  ADD COLUMN IF NOT EXISTS id_card_authority varchar(100),
  ADD COLUMN IF NOT EXISTS id_card_address varchar(200);
