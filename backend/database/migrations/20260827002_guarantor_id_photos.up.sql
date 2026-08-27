-- #1782: guarantor ID card photos + identity fields for deposit-free application
ALTER TABLE guarantors ADD COLUMN IF NOT EXISTS id_card_no varchar(18);
ALTER TABLE guarantors ADD COLUMN IF NOT EXISTS id_photo_front text;
ALTER TABLE guarantors ADD COLUMN IF NOT EXISTS id_photo_back text;
ALTER TABLE guarantors ADD COLUMN IF NOT EXISTS other_cert_photo text;
