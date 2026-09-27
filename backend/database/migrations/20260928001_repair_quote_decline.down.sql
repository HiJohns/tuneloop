-- #2093 down
ALTER TABLE repair_requests DROP COLUMN IF EXISTS quote_decline_note;
ALTER TABLE repair_requests DROP COLUMN IF EXISTS quote_decline_reason;
