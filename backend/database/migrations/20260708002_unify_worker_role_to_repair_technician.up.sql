UPDATE site_members SET role = 'repair_technician' WHERE role = 'worker';
UPDATE roles SET code = 'repair_technician', name = '维修师傅' WHERE code = 'worker';
