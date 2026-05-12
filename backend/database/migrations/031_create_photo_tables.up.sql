-- 031: Create instrument_photo_specs and instrument_photo_batches tables

-- instrument_photo_specs: 乐器拍照要求规范表
CREATE TABLE IF NOT EXISTS instrument_photo_specs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    category_id UUID NOT NULL,
    photo_requirements JSONB DEFAULT '[]',
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_instrument_photo_specs_tenant_id ON instrument_photo_specs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_instrument_photo_specs_category_id ON instrument_photo_specs(category_id);

-- instrument_photo_batches: 乐器照片批次表
CREATE TABLE IF NOT EXISTS instrument_photo_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instrument_id UUID NOT NULL,
    instrument_sn VARCHAR(100) NOT NULL,
    batch_type VARCHAR(20) NOT NULL DEFAULT 'outbound',
    storage_path VARCHAR(500),
    operator_id VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_instrument_photo_batches_instrument_id ON instrument_photo_batches(instrument_id);
CREATE INDEX IF NOT EXISTS idx_instrument_photo_batches_type ON instrument_photo_batches(batch_type);
