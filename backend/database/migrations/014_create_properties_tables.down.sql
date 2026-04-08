-- Migration 014: Down migration for property management tables
-- Issue #218: Rollback property tables creation

-- Drop instrument_properties table first (has foreign keys)
DROP TABLE IF EXISTS instrument_properties;

-- Drop property_options table
DROP TABLE IF EXISTS property_options;

-- Drop properties table
DROP TABLE IF EXISTS properties;
