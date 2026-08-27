-- #1782 rollback
ALTER TABLE guarantors DROP COLUMN IF EXISTS id_card_no;
ALTER TABLE guarantors DROP COLUMN IF EXISTS id_photo_front;
ALTER TABLE guarantors DROP COLUMN IF EXISTS id_photo_back;
ALTER TABLE guarantors DROP COLUMN IF EXISTS other_cert_photo;
