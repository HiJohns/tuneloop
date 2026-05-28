-- Unify role codes: convert short codes to long codes to match AllRoleTemplates
-- Issue #665

UPDATE roles SET code = 'merchant_admin', name = '商户管理员' WHERE code = 'owner' AND is_system = true;
UPDATE roles SET code = 'site_admin', name = '网点管理员' WHERE code = 'admin' AND is_system = true;
UPDATE roles SET code = 'site_member', name = '网点员工' WHERE code = 'staff' AND is_system = true;
