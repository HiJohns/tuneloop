-- #1741 follow-up: backfill high-tier yuan-semantics base_daily_rent /
-- final_daily_rent (bdr >= 200, e.g. CB-111 205 yuan) that the <200 guard
-- in 20260823002 intentionally excluded.
--
-- Detection: nearest-neighbor against the instrument's current
-- base_daily_rate (cents) — same rule as resolveBaseDailyRentCents
-- (handlers/pricing_helpers.go): if bdr*100 is closer to the current rate
-- than bdr is, the snapshot is yuan-semantics → multiply by 100.
--
-- Batch marker: '_bdr_backfill_batch' = '20260824006' so .down.sql rolls
-- back exactly this batch. Rows already marked by 20260823002 are skipped.
--
-- ⚠️ 红线：生产/预生产数据库执行需用户明确授权。

UPDATE orders o
SET pricing_breakdown = jsonb_set(
      jsonb_set(
        jsonb_set(o.pricing_breakdown, '{base_daily_rent}',
                  to_jsonb((o.pricing_breakdown->>'base_daily_rent')::numeric * 100)),
        '{final_daily_rent}',
        to_jsonb((o.pricing_breakdown->>'final_daily_rent')::numeric * 100)
      ),
      '{_bdr_backfill_batch}', '"20260824006"', true
    )
FROM instruments i
WHERE i.id = o.instrument_id
  AND i.base_daily_rate IS NOT NULL
  AND o.pricing_breakdown ? 'base_daily_rent'
  AND (o.pricing_breakdown->>'base_daily_rent')::numeric > 0
  AND o.pricing_breakdown->>'_cents_migrated' = 'true'
  AND o.pricing_breakdown->>'_bdr_backfill_batch' IS NULL
  AND ABS((o.pricing_breakdown->>'base_daily_rent')::numeric * 100 - i.base_daily_rate)
      < ABS((o.pricing_breakdown->>'base_daily_rent')::numeric - i.base_daily_rate);
