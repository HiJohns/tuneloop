DROP INDEX IF EXISTS idx_properties_related_property;
DROP INDEX IF EXISTS idx_properties_related_category;
DROP INDEX IF EXISTS idx_properties_scope_type;
ALTER TABLE properties DROP COLUMN IF EXISTS related_property_id;
ALTER TABLE properties DROP COLUMN IF EXISTS related_category_id;
ALTER TABLE properties DROP COLUMN IF EXISTS scope_type;
