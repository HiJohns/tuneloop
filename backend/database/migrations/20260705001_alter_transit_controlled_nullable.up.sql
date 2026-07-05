-- Up migration: Make repair_transit_orders.controlled_site_id nullable (v3 flow: inbound order created before site chosen)
ALTER TABLE repair_transit_orders ALTER COLUMN controlled_site_id DROP NOT NULL;
