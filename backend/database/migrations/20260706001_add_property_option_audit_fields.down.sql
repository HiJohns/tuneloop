ALTER TABLE property_options
  DROP COLUMN IF EXISTS submitter_id,
  DROP COLUMN IF EXISTS site_id,
  DROP COLUMN IF EXISTS merchant_id,
  DROP COLUMN IF EXISTS instrument_id;
