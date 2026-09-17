-- #1935 audit Bug4: sites.contact_name — UpdateAdminTransitSite 写入该列时因列不存在
-- 导致 500（column "contact_name" does not exist）；Create 亦需持久化联系人。
ALTER TABLE sites ADD COLUMN IF NOT EXISTS contact_name VARCHAR(255) NOT NULL DEFAULT '';
