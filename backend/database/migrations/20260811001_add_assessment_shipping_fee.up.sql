-- Add shipping_fee to damage_assessments (#1621): staff sets logistics
-- fee at return inspection, deducted from deposit at settlement.
ALTER TABLE damage_assessments ADD COLUMN IF NOT EXISTS shipping_fee DECIMAL(10,2) NOT NULL DEFAULT 0;
