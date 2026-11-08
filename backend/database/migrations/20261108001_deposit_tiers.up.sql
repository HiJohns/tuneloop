-- #2321: 押金两档（全额/差额）+ 零押金；历史订单不追溯
ALTER TABLE instruments
    ADD COLUMN IF NOT EXISTS deposit_reduced bigint NULL;

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS deposit_tier varchar(10) NULL,
    ADD COLUMN IF NOT EXISTS deposit_full bigint NULL,
    ADD COLUMN IF NOT EXISTS deposit_reduced bigint NULL;

COMMENT ON COLUMN instruments.deposit_reduced IS '#2321 差额押金（分）；NULL=无差额档，0=差额 0，>0=差额金额';
COMMENT ON COLUMN orders.deposit_tier IS '#2321 押金档位快照：full|reduced|zero；历史订单留空';
COMMENT ON COLUMN orders.deposit_full IS '#2321 下单时全额押金快照（分）';
COMMENT ON COLUMN orders.deposit_reduced IS '#2321 下单时差额押金快照（分；NULL=未配置）';
