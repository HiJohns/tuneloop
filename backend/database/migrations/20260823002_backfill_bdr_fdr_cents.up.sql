-- #1741: Backfill legacy yuan-semantics base_daily_rent / final_daily_rent in
-- orders.pricing_breakdown to cents semantics (x100).
--
-- Yuan-semantics detection (per accepted plan):
--   bdr > 0 AND bdr < 200 AND _cents_migrated='true'
--   - cents-semantics daily rent is always >= 100 (yuan values like 36 are far below)
--   - _cents_migrated='true' restricts scope to P3-snapshot orders (total_amount already cents)
--
-- Batch marker: '_bdr_backfill_batch' = '20260823002' is written onto every
-- affected row so .down.sql can roll back EXACTLY this batch (plan requires
-- 仅回滚该批次) without touching legitimate cents rows in the overlap window.
--
-- Idempotent: after backfill bdr >= 10000 fails the < 200 guard on re-run.
--
-- ⚠️ 红线：生产/预生产数据库执行需用户明确授权（dry-run 工具见
-- scripts/dryrun_backfill_bdr_fdr_cents.sql，只读）。

UPDATE orders
SET pricing_breakdown = jsonb_set(
      jsonb_set(
        jsonb_set(pricing_breakdown, '{base_daily_rent}',
                  to_jsonb((pricing_breakdown->>'base_daily_rent')::numeric * 100)),
        '{final_daily_rent}',
        to_jsonb((pricing_breakdown->>'final_daily_rent')::numeric * 100)
      ),
      '{_bdr_backfill_batch}', '"20260823002"', true
    )
WHERE pricing_breakdown ? 'base_daily_rent'
  AND (pricing_breakdown->>'base_daily_rent')::numeric > 0
  AND (pricing_breakdown->>'base_daily_rent')::numeric < 200        -- 元语义判定
  AND pricing_breakdown->>'_cents_migrated' = 'true';               -- 仅 P3 后快照
