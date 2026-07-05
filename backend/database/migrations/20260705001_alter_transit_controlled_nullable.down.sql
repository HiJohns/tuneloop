-- Down migration: Restore NOT NULL on repair_transit_orders.controlled_site_id
ALTER TABLE repair_transit_orders ALTER COLUMN controlled_site_id SET NOT NULL;
