-- #2327 S2: 微信支付分（免押租借）信用状态快照
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS credit_state varchar(20) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS credit_risk_amount_cents bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS credit_service_order_no varchar(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS credit_assessed_at timestamptz NULL;

CREATE INDEX IF NOT EXISTS idx_orders_credit_state ON orders (credit_state);

COMMENT ON COLUMN orders.credit_state IS '#2327 支付分信用状态：空=未走支付分|pending|approved|rejected|expired';
COMMENT ON COLUMN orders.credit_risk_amount_cents IS '#2327 提交微信的风险金额（分）=减免额度；full 档为 0';
COMMENT ON COLUMN orders.credit_service_order_no IS '#2327 支付分服务订单号（out_order_no），用于查询/完结/取消';
COMMENT ON COLUMN orders.credit_assessed_at IS '#2327 信用评估完成时间（approved/rejected 落库时间）';
