-- #1894 down: no-op.
-- The backfill only aligns org_id with the instrument's own site org;
-- reverting would re-hide the instruments from site-scoped staff. Same
-- convention as #1889/#1891 backfills.
SELECT 1;
