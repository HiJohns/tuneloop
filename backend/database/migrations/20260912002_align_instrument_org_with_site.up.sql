-- #1894: align instruments.org_id with their site's org. Legacy rows were
-- created with org_id = tenant_id, so site-scoped staff (ApplyOrgScope uses
-- the site org; site_member sees exactly their own org) could not see
-- instruments that physically sit at their own site (204/209 in prod/pre).
-- The single-create path already resolves org from the site
-- (instrument.go), so this backfill restores the intended invariant.
-- Idempotent: only rows whose org differs from the site org are touched.
UPDATE instruments i
SET org_id = s.org_id
FROM sites s
WHERE i.site_id = s.id
  AND s.org_id IS NOT NULL
  AND i.org_id IS DISTINCT FROM s.org_id;
