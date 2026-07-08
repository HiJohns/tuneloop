UPDATE site_members SET role = 'worker' WHERE role = 'repair_technician';
UPDATE roles SET code = 'worker', name = '维修工程师' WHERE code = 'repair_technician';
