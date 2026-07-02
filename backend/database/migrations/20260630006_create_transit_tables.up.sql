CREATE TABLE IF NOT EXISTS transit_routes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    controlled_site_id UUID NOT NULL,
    transit_site_id UUID NOT NULL,
    priority INT DEFAULT 0,
    is_default BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_transit_routes_controlled ON transit_routes(controlled_site_id);

CREATE TABLE IF NOT EXISTS transit_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL,
    transit_site_id UUID NOT NULL,
    controlled_site_id UUID NOT NULL,
    status VARCHAR(20) DEFAULT 'dispatching',
    unpack_photos JSONB DEFAULT '[]',
    repack_company VARCHAR(100),
    repack_tracking_number VARCHAR(100),
    transit_order_number VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_transit_orders_order ON transit_orders(order_id);
CREATE INDEX IF NOT EXISTS idx_transit_orders_status ON transit_orders(status);

CREATE TABLE IF NOT EXISTS repair_transit_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repair_request_id UUID,
    transit_site_id UUID NOT NULL,
    controlled_site_id UUID NOT NULL,
    status VARCHAR(20) DEFAULT 'inbound',
    unpack_photos JSONB DEFAULT '[]',
    repack_company VARCHAR(100),
    repack_tracking_number VARCHAR(100),
    transit_order_number VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_repair_transit_request ON repair_transit_orders(repair_request_id);
