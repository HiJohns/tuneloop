-- #1807: 回滚实名核身人工审核补全字段
ALTER TABLE users
  DROP COLUMN IF EXISTS id_card_expire,
  DROP COLUMN IF EXISTS id_card_authority,
  DROP COLUMN IF EXISTS id_card_address;
