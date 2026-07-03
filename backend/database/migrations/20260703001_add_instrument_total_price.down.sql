-- Revert total_price and default pricing template

ALTER TABLE instruments DROP COLUMN IF EXISTS total_price;

DELETE FROM pricing_templates WHERE code = 'system_default';
