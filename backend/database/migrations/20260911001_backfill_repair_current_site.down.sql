-- #1889 down: no-op.
-- The backfill derives current_site_id from existing data; reverting it would
-- re-introduce the acceptance blocker. Intentionally left as a no-op.
SELECT 1;
