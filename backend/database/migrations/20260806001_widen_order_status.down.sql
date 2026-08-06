-- Revert order status columns to varchar(20) (#1544)
-- NOTE: only safe if no pending_damage_response rows exist
ALTER TABLE orders ALTER COLUMN status TYPE varchar(20);
ALTER TABLE order_status_history ALTER COLUMN status_from TYPE varchar(20);
ALTER TABLE order_status_history ALTER COLUMN status_to TYPE varchar(20);
