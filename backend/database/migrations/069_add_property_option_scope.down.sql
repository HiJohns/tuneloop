DROP INDEX IF EXISTS idx_property_options_scope_category;
DROP INDEX IF EXISTS idx_property_options_scope_parent;
ALTER TABLE property_options DROP COLUMN IF EXISTS scope_parent_value;
ALTER TABLE property_options DROP COLUMN IF EXISTS scope_category_id;
