-- #2114 审计修复 down：移除部分唯一索引（存量 cancelled 不回滚）
DROP INDEX IF EXISTS uniq_staff_invites_pending_2114;
