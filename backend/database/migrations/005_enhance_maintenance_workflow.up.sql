-- Add technician assignment and completion report fields to maintenance_tickets
-- This enhances the maintenance workflow with auto-assignment and completion tracking

-- Add technician_id for automatic assignment
ALTER TABLE maintenance_tickets 
ADD COLUMN IF NOT EXISTS technician_id UUID REFERENCES technicians(id);

CREATE INDEX IF NOT EXISTS idx_maintenance_technician ON maintenance_tickets(technician_id);

-- Add repair report field for technician to document completion details
ALTER TABLE maintenance_tickets 
ADD COLUMN IF NOT EXISTS repair_report TEXT,
ADD COLUMN IF NOT EXISTS repair_photos JSONB DEFAULT '[]',
ADD COLUMN IF NOT EXISTS completed_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_maintenance_completed ON maintenance_tickets(completed_at);