ALTER TABLE properties ADD COLUMN IF NOT EXISTS scope_type VARCHAR(20) NOT NULL DEFAULT 'global';
ALTER TABLE properties ADD COLUMN IF NOT EXISTS related_category_id UUID;
ALTER TABLE properties ADD COLUMN IF NOT EXISTS related_property_id UUID;
CREATE INDEX IF NOT EXISTS idx_properties_scope_type ON properties(scope_type);
CREATE INDEX IF NOT EXISTS idx_properties_related_category ON properties(related_category_id);
CREATE INDEX IF NOT EXISTS idx_properties_related_property ON properties(related_property_id);
