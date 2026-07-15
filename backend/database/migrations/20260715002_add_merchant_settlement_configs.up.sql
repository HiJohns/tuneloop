CREATE TABLE IF NOT EXISTS merchant_settlement_configs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    receiver_type varchar(20) NOT NULL DEFAULT 'merchant',
    receiver_account varchar(128) NOT NULL,
    profit_share_ratio decimal(5,2) NOT NULL DEFAULT 0,
    is_enabled boolean NOT NULL DEFAULT true,
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now()
);

CREATE INDEX idx_settlement_configs_tenant ON merchant_settlement_configs (tenant_id);
