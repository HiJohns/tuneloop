-- Add overdue fee fields to damage_assessments for staged return settlement (#1494)
-- overdue_days / overdue_fee: computed once at return inspection, persisted for
-- the refund calculation (replaces legacy per-day overdue_charges)
ALTER TABLE damage_assessments ADD COLUMN IF NOT EXISTS overdue_days integer DEFAULT 0;
ALTER TABLE damage_assessments ADD COLUMN IF NOT EXISTS overdue_fee decimal(10,2) DEFAULT 0;
