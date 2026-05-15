-- Migration 034 rollback: Remove description column from properties table

ALTER TABLE properties DROP COLUMN IF EXISTS description;
