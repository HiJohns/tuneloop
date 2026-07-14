ALTER TABLE orders DROP COLUMN IF EXISTS current_payment_session_id;
DROP TABLE IF EXISTS session_order_links;
DROP TABLE IF EXISTS payment_sessions;
