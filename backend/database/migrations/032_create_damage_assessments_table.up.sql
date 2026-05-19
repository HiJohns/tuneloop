-- 032: Create damage_assessments table

-- damage_assessments: 定损评估表
CREATE TABLE IF NOT EXISTS damage_assessments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    order_id UUID NOT NULL,
    inspector_id UUID,
    description TEXT,
    estimated_cost DECIMAL(10,2),
    status VARCHAR(20) DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_damage_assessments_tenant_id ON damage_assessments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_damage_assessments_order_id ON damage_assessments(order_id);
