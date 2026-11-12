DROP INDEX IF EXISTS idx_orders_credit_state;

ALTER TABLE orders
    DROP COLUMN IF EXISTS credit_assessed_at,
    DROP COLUMN IF EXISTS credit_service_order_no,
    DROP COLUMN IF EXISTS credit_risk_amount_cents,
    DROP COLUMN IF EXISTS credit_state;
