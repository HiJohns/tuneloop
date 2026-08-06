-- Widen order status columns to varchar(40) for pending_damage_response (23 chars) (#1544)
ALTER TABLE orders ALTER COLUMN status TYPE varchar(40);
ALTER TABLE order_status_history ALTER COLUMN status_from TYPE varchar(40);
ALTER TABLE order_status_history ALTER COLUMN status_to TYPE varchar(40);
