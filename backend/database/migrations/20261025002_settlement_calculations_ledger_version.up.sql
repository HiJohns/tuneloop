ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS ledger_version bigint NOT NULL DEFAULT 0;
WITH numbered AS (
  SELECT id, ROW_NUMBER() OVER (PARTITION BY order_id, trigger ORDER BY created_at, id) AS rn
  FROM settlement_calculations
)
UPDATE settlement_calculations s SET ledger_version = n.rn FROM numbered n WHERE s.id = n.id;
CREATE UNIQUE INDEX IF NOT EXISTS idx_settlement_calc_version ON settlement_calculations(order_id, trigger, ledger_version);
