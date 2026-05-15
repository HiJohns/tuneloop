-- Migration 034: Add description column to properties table
-- Issue #543: Frontend uses prop.description but DB column is missing

ALTER TABLE properties ADD COLUMN IF NOT EXISTS description TEXT;
