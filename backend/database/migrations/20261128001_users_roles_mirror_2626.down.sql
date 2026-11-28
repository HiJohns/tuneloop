-- #2626: rollback users.roles mirror column.
ALTER TABLE users DROP COLUMN IF EXISTS roles;
