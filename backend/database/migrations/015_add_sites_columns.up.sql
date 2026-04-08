-- Migration 015: Add missing columns to sites table
-- Issue #217: Missing column "parent_id" in sites table

-- Add parent_id column for self-referencing site hierarchy
ALTER TABLE sites ADD COLUMN parent_id UUID REFERENCES sites(id);

-- Add manager_id column for site manager relationship
ALTER TABLE sites ADD COLUMN manager_id UUID REFERENCES users(id);

-- Add type column for site classification
ALTER TABLE sites ADD COLUMN type VARCHAR(50);
