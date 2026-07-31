-- Add order creation snapshots for audit/compliance
-- request_snapshot: raw creation request params (dates, days, instrument, address)
-- pricing_config_snapshot: merchant pricing config + calculation inputs at order time
ALTER TABLE orders ADD COLUMN IF NOT EXISTS request_snapshot jsonb DEFAULT '{}';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS pricing_config_snapshot jsonb DEFAULT '{}';
