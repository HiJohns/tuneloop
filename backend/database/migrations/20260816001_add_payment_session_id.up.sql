-- Add session_id to order_payment_records (#1664): the two-phase registration
-- session link must survive the WeChat payment callback — record.RawResponse
-- is overwritten by the callback result, so the association lives in its own
-- column instead.
ALTER TABLE order_payment_records ADD COLUMN IF NOT EXISTS session_id UUID;
