ALTER TABLE property_options ADD COLUMN IF NOT EXISTS scope_category_id UUID REFERENCES categories(id);
ALTER TABLE property_options ADD COLUMN IF NOT EXISTS scope_parent_value VARCHAR(255);
CREATE INDEX IF NOT EXISTS idx_property_options_scope_category ON property_options(scope_category_id);
CREATE INDEX IF NOT EXISTS idx_property_options_scope_parent ON property_options(scope_parent_value);

-- Ensure uniqueness per scope: global (all NULL), category, or property parent
CREATE UNIQUE INDEX IF NOT EXISTS uq_property_option_scoped
  ON property_options (property_name, value,
    COALESCE(scope_category_id::text, ''),
    COALESCE(scope_parent_value, ''));
