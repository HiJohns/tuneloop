-- Migration 016: Add updated_at column to sites table
-- Issue #226: 创建网点失败 - missing updated_at column

ALTER TABLE sites ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP DEFAULT NOW();