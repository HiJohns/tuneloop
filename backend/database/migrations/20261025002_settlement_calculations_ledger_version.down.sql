DROP INDEX IF EXISTS idx_settlement_calc_version;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS ledger_version;
