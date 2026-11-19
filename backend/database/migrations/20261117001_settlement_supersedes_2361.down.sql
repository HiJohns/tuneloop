-- #2361 (#2268): down migration — 逆向 20261117001_settlement_supersedes_2361.up.sql。
-- 只删结构；不改历史行状态/金额（up 亦不改金额）。

DROP INDEX IF EXISTS idx_settlement_supersedes;
ALTER TABLE settlement_calculations DROP COLUMN IF EXISTS supersedes;
