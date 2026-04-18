-- Add logistics tracking fields to orders table
ALTER TABLE orders 
ADD COLUMN IF NOT EXISTS tracking_number VARCHAR(100),
ADD COLUMN IF NOT EXISTS courier_company VARCHAR(100),
ADD COLUMN IF NOT EXISTS shipped_at TIMESTAMP;

-- Add indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_orders_tracking_number ON orders(tracking_number);

-- Fix: Create order_status_history table if it doesn't exist
CREATE TABLE IF NOT EXISTS order_status_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    org_id UUID,
    order_id UUID NOT NULL,
    status_from VARCHAR(20) NOT NULL,
    status_to VARCHAR(20) NOT NULL,
    changed_by UUID,
    changed_at TIMESTAMP NOT NULL DEFAULT NOW(),
    notes TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Add indexes for order_status_history
CREATE INDEX IF NOT EXISTS idx_order_status_history_tenant_id ON order_status_history(tenant_id);
CREATE INDEX IF NOT EXISTS idx_order_status_history_order_id ON order_status_history(order_id);
CREATE INDEX IF NOT EXISTS idx_order_status_history_changed_at ON order_status_history(changed_at);

-- Fix: Allow NULL values for org_id and category_id in instruments table
ALTER TABLE instruments ALTER COLUMN org_id DROP NOT NULL;
ALTER TABLE instruments ALTER COLUMN category_id DROP NOT NULL;
