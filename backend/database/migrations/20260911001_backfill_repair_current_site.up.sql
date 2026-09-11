-- #1889: backfill current_site_id for instruments currently in the repair flow.
-- Instruments entering repair before this fix had current_site_id unset, which
-- made the acceptance gate (AcceptRepair) fail with "instrument has no site".
-- Derive the site from the most recent repair record's worker membership.
-- Scope: only instruments with an active repair_status (user-confirmed 2026-09-11).
UPDATE instruments i
SET current_site_id = sub.site_id
FROM (
    SELECT DISTINCT ON (r.instrument_id) r.instrument_id, sm.site_id
    FROM repair_records r
    JOIN users u ON u.iam_sub = r.worker_id
    JOIN site_members sm ON sm.user_id = u.id
    WHERE r.instrument_id IN (
        SELECT id FROM instruments
        WHERE repair_status IS NOT NULL AND current_site_id IS NULL
    )
    ORDER BY r.instrument_id, r.created_at DESC
) sub
WHERE i.id = sub.instrument_id
  AND i.current_site_id IS NULL;
