-- #1708/#1711: merge DamageAssessment into DamageReport.
-- 1. Add acceptance columns to damage_reports (model updated in #1709).
-- 2. Backfill damage_assessments rows into damage_reports (idempotent).
-- 3. Drop the deprecated damage_assessments table.
-- 生产/预生产 DDL 执行需用户确认（红线）。

-- 1. New columns
ALTER TABLE damage_reports ADD COLUMN IF NOT EXISTS condition    VARCHAR(20);
ALTER TABLE damage_reports ADD COLUMN IF NOT EXISTS notes        TEXT;
ALTER TABLE damage_reports ADD COLUMN IF NOT EXISTS scan_time    TIMESTAMP;
ALTER TABLE damage_reports ADD COLUMN IF NOT EXISTS overdue_days INTEGER NOT NULL DEFAULT 0;
ALTER TABLE damage_reports ADD COLUMN IF NOT EXISTS overdue_fee  DECIMAL(10,2) NOT NULL DEFAULT 0;

-- 2. Backfill assessments → reports (idempotent):
--    - existing report for the lease: fill acceptance fields if empty
--    - no report: insert one (fresh id; damaged rows get damage_amount from
--      estimated_cost and status=pending, others completed)
INSERT INTO damage_reports (id, tenant_id, org_id, lease_id, instrument_id, user_id,
                            damage_amount, damage_description, condition, notes, scan_time,
                            overdue_days, overdue_fee, status, assessed_by, assessed_at,
                            created_at, updated_at)
SELECT
    gen_random_uuid(),
    a.tenant_id, a.org_id, a.order_id, a.instrument_id, a.user_id,
    a.estimated_cost,
    a.description,
    a.condition,
    a.notes,
    a.scan_time,
    a.overdue_days,
    a.overdue_fee,
    CASE WHEN a.condition = 'damaged' THEN 'pending' ELSE 'completed' END,
    a.inspector_id,
    a.scan_time,
    a.created_at,
    a.updated_at
FROM damage_assessments a
WHERE NOT EXISTS (SELECT 1 FROM damage_reports r WHERE r.lease_id = a.order_id)
  AND a.condition IS NOT NULL;

-- Update existing reports with acceptance fields when the assessment has them
-- (legacy reports created before #1709 lack condition/scan_time/overdue).
UPDATE damage_reports r
SET condition    = COALESCE(r.condition,    a.condition),
    notes        = COALESCE(r.notes,        a.notes),
    scan_time    = COALESCE(r.scan_time,    a.scan_time),
    overdue_days = CASE WHEN r.overdue_days = 0 THEN a.overdue_days ELSE r.overdue_days END,
    overdue_fee  = CASE WHEN r.overdue_fee  = 0 THEN a.overdue_fee  ELSE r.overdue_fee  END
FROM damage_assessments a
WHERE a.order_id = r.lease_id
  AND (r.condition IS NULL OR r.scan_time IS NULL);

-- 3. Drop the deprecated table (down keeps it via restore).
DROP TABLE IF EXISTS damage_assessments;
