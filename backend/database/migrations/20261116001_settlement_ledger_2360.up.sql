-- #2360 (#2268): 结算数据模型与支付分配账本。
--
-- v2.2 §3.1/§9/§12：
--   1) 租金/押金现金实收只能从 order_payment_allocations 读取（禁止反推）；
--   2) settlement_calculations 版本化 + 状态机 + 订单级唯一最终结算；
--   3) 子动作账本 settlement_action_ledger（幂等键预生成）；
--   4) 费用分类（逾期罚金/定损费/物流费）与凭证；
--   5) 价格规则快照；
--   6) 历史数据只标记 legacy_*，不重算。

-- ---------------------------------------------------------------- 结算表扩展
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS settlement_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS status VARCHAR(24) NOT NULL DEFAULT 'calculated';
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS algorithm_version VARCHAR(40) NOT NULL DEFAULT '';
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS ratio_num INTEGER;
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS ratio_den INTEGER;
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS ratio_source VARCHAR(20) NOT NULL DEFAULT '';
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS calculated_at TIMESTAMP;
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS accounts_json JSONB;

-- 分账户金额（分）
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS rent_cash_paid_net_cents BIGINT NOT NULL DEFAULT 0;
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS deposit_cash_collected_net_cents BIGINT NOT NULL DEFAULT 0;
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS deposit_held_cents BIGINT NOT NULL DEFAULT 0;

-- 费用分类 + 凭证
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS overdue_penalty_cents BIGINT NOT NULL DEFAULT 0;
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS damage_fee_cents BIGINT NOT NULL DEFAULT 0;
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS logistics_fee_cents BIGINT NOT NULL DEFAULT 0;
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS damage_report_id UUID;
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS logistics_ref VARCHAR(100) NOT NULL DEFAULT '';

-- 价格规则快照
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS pricing_policy_snapshot JSONB;
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS pricing_policy_version VARCHAR(40) NOT NULL DEFAULT '';

-- 历史标记（不重算金额）
ALTER TABLE settlement_calculations ADD COLUMN IF NOT EXISTS legacy_marker VARCHAR(40) NOT NULL DEFAULT '';

-- 历史行状态回填：confirm → 最新一条 confirmed，其余 superseded；preview 保持 calculated。
-- 先全部降为 superseded，再把每单最新 confirm 行提升为 confirmed，避免多行同时进入
-- active 集合导致下方 partial unique index 创建失败（幂等：重复执行结果一致）。
UPDATE settlement_calculations SET status = 'superseded' WHERE trigger = 'confirm' AND status IS DISTINCT FROM 'superseded';
WITH latest AS (
    SELECT DISTINCT ON (order_id) id
    FROM settlement_calculations
    WHERE trigger = 'confirm'
    ORDER BY order_id, ledger_version DESC, created_at DESC, id DESC
)
UPDATE settlement_calculations s
SET status = 'confirmed'
FROM latest l
WHERE s.id = l.id AND s.status IS DISTINCT FROM 'confirmed';

CREATE INDEX IF NOT EXISTS idx_settlement_order_version
    ON settlement_calculations (order_id, settlement_version);

-- 订单级唯一最终结算（5 种 active 状态；failed/冲正历史行不参与）。
CREATE UNIQUE INDEX IF NOT EXISTS idx_settlement_active_order
    ON settlement_calculations (order_id)
    WHERE status IN ('confirmed', 'processing', 'succeeded', 'partially_succeeded', 'failed_retryable');

-- ------------------------------------------------------------ 支付分配账本
CREATE TABLE IF NOT EXISTS order_payment_allocations (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id               UUID NOT NULL,
    tenant_id              UUID,
    source_type            VARCHAR(24) NOT NULL,           -- payment | refund | deduction | adjustment
    source_id              VARCHAR(64) NOT NULL,           -- payment record id / refund id / deduction id
    channel                VARCHAR(20) NOT NULL DEFAULT '',
    rent_cents             BIGINT NOT NULL DEFAULT 0,      -- 租金流入（退款为负）
    deposit_cents          BIGINT NOT NULL DEFAULT 0,      -- 押金流入（退款为负）
    fee_cents              BIGINT NOT NULL DEFAULT 0,
    fee_type               VARCHAR(24) NOT NULL DEFAULT '',-- overdue_penalty | damage_fee | logistics_fee
    deposit_deducted_cents BIGINT NOT NULL DEFAULT 0,      -- 押金被扣用（正数）
    created_at             TIMESTAMP NOT NULL DEFAULT now()
);

-- 幂等键：同一订单的同一来源只允许一行（支付/退款/扣用各一次）。
CREATE UNIQUE INDEX IF NOT EXISTS idx_opa_source
    ON order_payment_allocations (order_id, source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_opa_order ON order_payment_allocations (order_id);

-- ------------------------------------------------------------ 子动作账本
CREATE TABLE IF NOT EXISTS settlement_action_ledger (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    settlement_id         UUID NOT NULL,
    order_id              UUID NOT NULL,
    settlement_version    BIGINT NOT NULL DEFAULT 1,
    action_type           VARCHAR(32) NOT NULL,            -- rent_refund | deposit_refund | fee_charge | points_restore | reverse
    idempotency_key       VARCHAR(120) NOT NULL,
    payment_channel       VARCHAR(20) NOT NULL DEFAULT '',
    channel_txn_id        VARCHAR(64),
    original_txn_id       VARCHAR(64),
    executed_amount_cents BIGINT NOT NULL DEFAULT 0,
    executed_points       BIGINT NOT NULL DEFAULT 0,
    retry_count           INTEGER NOT NULL DEFAULT 0,
    status                VARCHAR(24) NOT NULL DEFAULT 'pending',
    error_message         TEXT,
    created_at            TIMESTAMP NOT NULL DEFAULT now(),
    updated_at            TIMESTAMP NOT NULL DEFAULT now()
);

-- 幂等键在执行前预生成；重复提交必须命中同一行而不是再打款。
CREATE UNIQUE INDEX IF NOT EXISTS idx_sal_idempotency
    ON settlement_action_ledger (idempotency_key);
CREATE INDEX IF NOT EXISTS idx_sal_settlement ON settlement_action_ledger (settlement_id);
CREATE INDEX IF NOT EXISTS idx_sal_order ON settlement_action_ledger (order_id);
