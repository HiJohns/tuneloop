-- #1891: backfill current_site_id for instruments still in the repair flow
-- whose site could not be derived from repair_records (#1889 / migration
-- 20260911001). Instruments with no repair_records (legacy rows) stayed NULL,
-- which hides them from the site-scoped pending list (#1882) and blocks
-- takeover, since both require an instrument site.
--
-- Derivation rule: when the instrument's org maps to exactly ONE site, that
-- site is unambiguous. Multi-site orgs are skipped rather than guessed.
-- Idempotent: only touches rows with current_site_id IS NULL.
UPDATE instruments i
SET current_site_id = sub.site_id
FROM (
    SELECT s.org_id, (array_agg(s.id ORDER BY s.id))[1] AS site_id
    FROM sites s
    GROUP BY s.org_id
    HAVING COUNT(*) = 1
) sub
WHERE i.org_id = sub.org_id
  AND i.current_site_id IS NULL
  AND i.repair_status IS NOT NULL
  AND i.repair_status <> '';
