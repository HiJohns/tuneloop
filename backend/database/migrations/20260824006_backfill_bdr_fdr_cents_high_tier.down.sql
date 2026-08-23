-- #1741 rollback of batch 20260824006: high-tier yuan-semantics backfill,
-- divide back by 100 and remove the batch marker.

UPDATE orders
SET pricing_breakdown = jsonb_set(
      jsonb_set(
        jsonb_set(pricing_breakdown, '{base_daily_rent}',
                  to_jsonb((pricing_breakdown->>'base_daily_rent')::numeric / 100)),
        '{final_daily_rent}',
        to_jsonb((pricing_breakdown->>'final_daily_rent')::numeric / 100)
      ),
      '{_bdr_backfill_batch}', NULL, true
    )
WHERE pricing_breakdown->>'_bdr_backfill_batch' = '20260824006';
