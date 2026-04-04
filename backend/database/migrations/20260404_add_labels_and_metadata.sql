-- Create labels table for tag normalization system
CREATE TABLE IF NOT EXISTS labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name VARCHAR(100) NOT NULL,
    alias JSONB DEFAULT '[]',
    audit_status VARCHAR(20) DEFAULT 'pending',
    normalized_to_id UUID,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_labels_tenant_status ON labels(tenant_id, audit_status);
CREATE INDEX IF NOT EXISTS idx_labels_name ON labels(tenant_id, name);
CREATE INDEX IF NOT EXISTS idx_labels_normalized ON labels(normalized_to_id) WHERE normalized_to_id IS NOT NULL;

-- Add metadata column to instruments table
ALTER TABLE instruments ADD COLUMN IF NOT EXISTS metadata JSONB DEFAULT '{}';

-- Update timestamp trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create trigger for labels table
DROP TRIGGER IF EXISTS update_labels_updated_at ON labels;
CREATE TRIGGER update_labels_updated_at BEFORE UPDATE ON labels
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();