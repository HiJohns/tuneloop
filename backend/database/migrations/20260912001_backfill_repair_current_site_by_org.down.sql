-- #1891 down: no-op.
-- The backfill only fills current_site_id for in-flow instruments that lacked
-- it; reverting would re-introduce the invisibility/takeover blocker. Same
-- convention as #1889 / 20260911001.
SELECT 1;
