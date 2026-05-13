-- Add delivered_at column to orders table for delivery confirmation
ALTER TABLE orders ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMP;
