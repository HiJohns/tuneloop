-- #2455: rollback the same-tenant site-name unique index.
-- Data is never touched by down: only the index created by up is dropped.
DROP INDEX IF EXISTS uq_sites_tenant_name;
