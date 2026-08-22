-- #1741 down: reverse the yuan→cents backfill (÷100) for EXACTLY the batch
-- flagged by _bdr_backfill_batch='20260823002', then drop the marker.
-- Rows never touched by the up migration carry no marker and are left alone.

UPDATE orders
SET pricing_breakdown = jsonb_set(
      jsonb_set(
        (pricing_breakdown - '_bdr_backfill_batch'),
        '{base_daily_rent}',
        to_jsonb((pricing_breakdown->>'base_daily_rent')::numeric / 100)
      ),
      '{final_daily_rent}',
      to_jsonb((pricing_breakdown->>'final_daily_rent')::numeric / 100)
    )
WHERE pricing_breakdown ? '_bdr_backfill_batch'
  AND pricing_breakdown->>'_bdr_backfill_batch' = '20260823002';
