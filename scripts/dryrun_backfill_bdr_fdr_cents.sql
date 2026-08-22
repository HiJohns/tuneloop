-- #1741 T2: dry-run report for bdr/fdr yuan→cents backfill (READ-ONLY).
-- Safe to run on any environment; performs no writes.
--
-- Usage:
--   ssh cadenza "docker exec -i <pg-container> psql -U tuneloop_user -d <db>" \
--     < scripts/dryrun_backfill_bdr_fdr_cents.sql
--
-- Environments:
--   预生产: -d tuneloop_pre_snapshot    生产: -d tuneloop

\echo '=== 1) Affected rows detail (order_id / status / current / after) ==='
SELECT id AS order_id,
       status,
       pricing_breakdown->>'base_daily_rent'  AS bdr_now,
       pricing_breakdown->>'final_daily_rent' AS fdr_now,
       (pricing_breakdown->>'base_daily_rent')::numeric * 100  AS bdr_after,
       (pricing_breakdown->>'final_daily_rent')::numeric * 100 AS fdr_after,
       (pricing_breakdown ? 'final_daily_rent'
        AND pricing_breakdown->>'final_daily_rent' IS NOT NULL) AS fdr_present,
       total_amount
FROM orders
WHERE pricing_breakdown ? 'base_daily_rent'
  AND (pricing_breakdown->>'base_daily_rent')::numeric > 0
  AND (pricing_breakdown->>'base_daily_rent')::numeric < 200
  AND pricing_breakdown->>'_cents_migrated' = 'true'
ORDER BY created_at;

\echo '=== 2) Grouped by status ==='
SELECT status, count(*) AS affected_rows
FROM orders
WHERE pricing_breakdown ? 'base_daily_rent'
  AND (pricing_breakdown->>'base_daily_rent')::numeric > 0
  AND (pricing_breakdown->>'base_daily_rent')::numeric < 200
  AND pricing_breakdown->>'_cents_migrated' = 'true'
GROUP BY status
ORDER BY affected_rows DESC;

\echo '=== 3) Edge case: affected rows missing/NULL final_daily_rent (expect 0 rows) ==='
SELECT id AS order_id,
       pricing_breakdown->>'base_daily_rent'  AS bdr_now,
       pricing_breakdown->>'final_daily_rent' AS fdr_now
FROM orders
WHERE pricing_breakdown ? 'base_daily_rent'
  AND (pricing_breakdown->>'base_daily_rent')::numeric > 0
  AND (pricing_breakdown->>'base_daily_rent')::numeric < 200
  AND pricing_breakdown->>'_cents_migrated' = 'true'
  AND (NOT pricing_breakdown ? 'final_daily_rent'
       OR pricing_breakdown->>'final_daily_rent' IS NULL);

\echo '=== 4) Post-execution residual check (run AFTER backfill; expect 0 rows) ==='
SELECT id FROM orders
WHERE pricing_breakdown->>'_cents_migrated' = 'true'
  AND pricing_breakdown ? 'base_daily_rent'
  AND (pricing_breakdown->>'base_daily_rent')::numeric > 0
  AND (pricing_breakdown->>'base_daily_rent')::numeric < 200;

\echo '=== 5) Spot check af3f8cf2 (after backfill expect bdr/fdr = 3600 cents) ==='
SELECT id,
       pricing_breakdown->>'base_daily_rent'  AS bdr,
       pricing_breakdown->>'final_daily_rent' AS fdr,
       pricing_breakdown->>'_bdr_backfill_batch' AS batch_marker
FROM orders
WHERE id::text LIKE 'af3f8cf2%';
