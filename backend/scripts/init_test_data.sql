-- Maintenance Workflow Test Data Script
-- This script creates test data for maintenance workflow testing
-- Run this after applying all migrations

-- Note: tenant_id should be replaced with actual test tenant UUID
-- For testing purposes, we'll use a placeholder UUID

-- Insert test site (service center)
INSERT INTO sites (tenant_id, org_id, name, address, phone, business_hours, latitude, longitude, status)
VALUES (
    '00000000-0000-0000-0000-000000000000',  -- tenant_id (placeholder)
    NULL,                                      -- org_id (optional)
    '中央维修中心',                            -- name
    '北京市朝阳区音乐街88号',                  -- address
    '010-12345678',                            -- phone
    '09:00-18:00',                            -- business_hours
    39.9042,                                   -- latitude
    116.4074,                                  -- longitude
    'active'                                   -- status
)
ON CONFLICT (id) DO NOTHING;

-- Get the site ID for reference
WITH inserted_site AS (
    SELECT id FROM sites 
    WHERE name = '中央维修中心' AND tenant_id = '00000000-0000-0000-0000-000000000000'
    LIMIT 1
)

-- Insert technician (worker)
INSERT INTO technicians (tenant_id, org_id, site_id, name, phone)
SELECT 
    '00000000-0000-0000-0000-000000000000',  -- tenant_id (placeholder)
    NULL,                                      -- org_id (optional)
    inserted_site.id,                          -- site_id (from inserted site)
    '张师傅',                                  -- name
    '13800138001'                              -- phone
FROM inserted_site
ON CONFLICT (id) DO NOTHING;

-- Insert test instruments (3 pieces)
-- First, ensure a category exists
INSERT INTO categories (tenant_id, name, icon, parent_id)
VALUES 
    ('00000000-0000-0000-0000-000000000000', '钢琴', '🎹', NULL),
    ('00000000-0000-0000-0000-000000000000', '吉他', '🎸', NULL),
    ('00000000-0000-0000-0000-000000000000', '小提琴', '🎻', NULL)
ON CONFLICT (id) DO NOTHING;

-- Insert instruments with proper tenant isolation
INSERT INTO instruments (
    tenant_id, 
    org_id,
    name, 
    brand, 
    level, 
    level_name, 
    description,
    stock_status,
    pricing
)
VALUES 
    (
        '00000000-0000-0000-0000-000000000000',
        NULL,
        '雅马哈立式钢琴 U1',
        'Yamaha',
        'professional',
        '专业级',
        '日本原装进口，音色纯净，适合进阶学习者',
        'available',
        '{"3month": 800, "6month": 750, "12month": 700}'
    ),
    (
        '00000000-0000-0000-0000-000000000000',
        NULL,
        '泰勒民谣吉他 214ce',
        'Taylor',
        'professional',
        '专业级',
        '美国品牌，云杉面板，玫瑰木背侧板',
        'available',
        '{"3month": 600, "6month": 550, "12month": 500}'
    ),
    (
        '00000000-0000-0000-0000-000000000000',
        NULL,
        '斯特拉迪瓦里小提琴 4/4',
        'Stradivarius',
        'master',
        '大师级',
        '意大利手工制作，音质卓越',
        'available',
        '{"3month": 1200, "6month": 1100, "12month": 1000}'
    )
ON CONFLICT (id) DO NOTHING;

-- Verify the data was inserted
SELECT 
    '站点' as type,
    COUNT(*) as count
FROM sites 
WHERE tenant_id = '00000000-0000-0000-0000-000000000000' AND name = '中央维修中心'

UNION ALL

SELECT 
    '技师' as type,
    COUNT(*) as count
FROM technicians 
WHERE tenant_id = '00000000-0000-0000-0000-000000000000' AND name = '张师傅'

UNION ALL

SELECT 
    '乐器' as type,
    COUNT(*) as count
FROM instruments 
WHERE tenant_id = '00000000-0000-0000-0000-000000000000' 
AND name IN ('雅马哈立式钢琴 U1', '泰勒民谣吉他 214ce', '斯特拉迪瓦里小提琴 4/4');
