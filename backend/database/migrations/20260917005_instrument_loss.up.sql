-- #1948 乐器丢失与找回：丢失记录表（员工裁量 + 分场景结算 + 找回冲正）
CREATE TABLE IF NOT EXISTS instrument_loss_records (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID,
    instrument_id        UUID NOT NULL,
    order_id             UUID,
    responsible_party    VARCHAR(20) NOT NULL,
    user_ratio           INTEGER NOT NULL DEFAULT 0,
    compensation_cents   BIGINT NOT NULL DEFAULT 0,
    user_burden_cents    BIGINT NOT NULL DEFAULT 0,
    description          TEXT,
    photos               JSONB DEFAULT '[]',
    settled_at           TIMESTAMPTZ,
    settle_breakdown     JSONB DEFAULT '{}',
    created_by           VARCHAR(255),
    restored_at          TIMESTAMPTZ,
    restored_damaged     BOOLEAN NOT NULL DEFAULT FALSE,
    restore_description  TEXT,
    restore_photos       JSONB DEFAULT '[]',
    reversed_at          TIMESTAMPTZ,
    reversed_amount_cents BIGINT NOT NULL DEFAULT 0,
    deducted_damage_cents BIGINT NOT NULL DEFAULT 0,
    deducted_idle_cents   BIGINT NOT NULL DEFAULT 0,
    reverse_note          TEXT,
    created_at            TIMESTAMPTZ,
    updated_at            TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_instrument_loss_instrument ON instrument_loss_records (instrument_id);
CREATE INDEX IF NOT EXISTS idx_instrument_loss_order ON instrument_loss_records (order_id);
CREATE INDEX IF NOT EXISTS idx_instrument_loss_tenant ON instrument_loss_records (tenant_id);
