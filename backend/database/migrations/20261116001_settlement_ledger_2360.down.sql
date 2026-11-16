-- #2360 (#2268): down migration — 逆向 20261116001_settlement_ledger_2360.up.sql。
-- 只删结构；不改动历史金额（up 也只标记，不重算）。

DROP TABLE IF EXISTS settlement_action_ledger;
DROP TABLE IF EXISTS order_payment_allocations;

DROP INDEX IF EXISTS idx_settlement_active_order;
DROP INDEX IF EXISTS idx_settlement_order_version;

ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS legacy_marker;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS pricing_policy_version;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS pricing_policy_snapshot;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS logistics_ref;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS damage_report_id;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS logistics_fee_cents;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS damage_fee_cents;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS overdue_penalty_cents;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS deposit_held_cents;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS deposit_cash_collected_net_cents;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS rent_cash_paid_net_cents;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS accounts_json;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS calculated_at;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS ratio_source;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS ratio_den;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS ratio_num;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS algorithm_version;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS status;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS settlement_version;
