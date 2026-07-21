-- Backfill instrument.org_id for instruments with NULL org_id
-- Priority 1: use site's org_id
UPDATE instruments i SET org_id = s.org_id
FROM sites s
WHERE i.site_id = s.id AND i.org_id IS NULL;

-- Priority 2: fallback to tenant_id (merchant-level instruments)
UPDATE instruments SET org_id = tenant_id WHERE org_id IS NULL;
