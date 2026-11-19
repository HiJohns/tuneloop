-- #2361 (#2268): 结算冲正/差额版本化。
--
-- v2.2 §9：已执行的最终结算不可覆盖，只能生成新版本：
--   - 旧行 status → 'superseded'（金额列不变）；
--   - 新行 status = 'calculated' + supersedes = 旧行 id；
--   - active 唯一索引（20261116001）仅覆盖
--     confirmed/processing/succeeded/partially_succeeded/failed_retryable，
--     calculated/superseded 不参与 → 新旧版本不冲突。
-- status 列为 VARCHAR(24)，'superseded' 为新增值，无需枚举变更。

ALTER TABLE settlement_calculations
    ADD COLUMN IF NOT EXISTS supersedes UUID;

CREATE INDEX IF NOT EXISTS idx_settlement_supersedes
    ON settlement_calculations (supersedes);
