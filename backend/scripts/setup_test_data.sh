#!/bin/bash
# Maintenance Workflow Test Data Setup Script
# This script creates test data for the maintenance workflow
# Run this after starting the backend service

set -e

echo "🔄 Setting up test data for maintenance workflow..."
echo ""

echo "Step 1: Creating service site..."
curl -s -X POST http://localhost:5554/api/merchant/sites \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TEST_JWT_TOKEN:-}" \
  -d '{
    "name": "中央维修中心",
    "address": "北京市朝阳区音乐街88号",
    "phone": "010-12345678",
    "business_hours": "09:00-18:00",
    "latitude": 39.9042,
    "longitude": 116.4074
  }' | jq . || echo "Warning: Site creation requires authentication"

echo ""
echo "✅ Test data setup instructions created!"
echo ""
echo "📋 Manual SQL Data Setup (Alternative Method):"
echo ""
echo "1. Connect to PostgreSQL:"
echo "   psql postgresql://tuneloop_user:tune_secret_2026@localhost:5432/tuneloop_db"
echo ""
echo "2. Run the following SQL to insert test data:"
echo ""
cat << 'EOF'

-- Insert test site
INSERT INTO sites (tenant_id, name, address, phone, business_hours, latitude, longitude, status)
VALUES (
    '00000000-0000-0000-0000-000000000000',
    '中央维修中心',
    '北京市朝阳区音乐街88号',
    '010-12345678',
    '09:00-18:00',
    39.9042,
    116.4074,
    'active'
);

-- Insert technician (requires site_id from above)
INSERT INTO technicians (tenant_id, org_id, site_id, name, phone)
VALUES (
    '00000000-0000-0000-0000-000000000000',
    NULL,
    (SELECT id FROM sites WHERE name = '中央维修中心' LIMIT 1),
    '张师傅',
    '13800138001'
);

-- Insert test instruments
INSERT INTO instruments (tenant_id, org_id, name, brand, level, level_name, description, stock_status, pricing)
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
    );

-- Verify data
SELECT 
    'Site' as type, name 
FROM sites 
WHERE name = '中央维修中心'
UNION ALL
SELECT 
    'Technician' as type, name 
FROM technicians 
WHERE name = '张师傅'
UNION ALL
SELECT 
    'Instrument' as type, name 
FROM instruments 
WHERE name LIKE '%雅马哈%' OR name LIKE '%泰勒%' OR name LIKE '%斯特拉%';

EOF

echo ""
echo "⚠️  Note: tenant_id should be replaced with your actual test tenant UUID"
echo ""
echo "📂 Test data files created:"
echo "   - /home/coder/tuneloop/backend/scripts/init_test_data.sql"
echo "   - /home/coder/tuneloop/backend/scripts/init_test_data.go"
echo ""
echo "✅ Setup complete!"