-- Pricing system: tiered discount pricing with merchant-level config
-- Issue #689

-- 1. System pricing template types
CREATE TABLE pricing_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    config_schema JSONB NOT NULL DEFAULT '{}',
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT now()
);

-- 2. Merchant-level pricing configuration
CREATE TABLE merchant_pricing_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID UNIQUE NOT NULL,
    template_id UUID NOT NULL REFERENCES pricing_templates(id),
    config JSONB NOT NULL DEFAULT '{}',
    updated_at TIMESTAMP DEFAULT now(),
    updated_by VARCHAR(255)
);

-- 3. Instrument pricing fields
ALTER TABLE instruments ADD COLUMN base_daily_rate DECIMAL(10,2);
ALTER TABLE instruments ADD COLUMN pricing_overrides JSONB DEFAULT '{}';

-- Seed initial pricing template
INSERT INTO pricing_templates (id, code, name, description, config_schema) VALUES (
    '00000000-0000-0000-0000-000000000001',
    'tiered_discount_v1',
    '阶梯折扣定价',
    '按租赁天数区间设置折扣率，天数越长折扣越大',
    '{
        "fields": [
            {
                "key": "tiers",
                "label": "定价阶梯",
                "type": "array",
                "required": true,
                "item": {
                    "days_max": {"label": "天数上限 (-1=不限)", "type": "int", "required": true},
                    "discount_percent": {"label": "折扣率(%)", "type": "int", "required": true, "min": 0, "max": 90}
                },
                "min_items": 1
            },
            {
                "key": "deposit_mode",
                "label": "押金模式",
                "type": "select",
                "required": true,
                "options": [
                    {"value": "fixed", "label": "固定金额"},
                    {"value": "ratio", "label": "按日均价倍率"}
                ]
            },
            {
                "key": "deposit_ratio",
                "label": "押金倍率",
                "type": "float",
                "min": 0.5,
                "max": 5,
                "visible_when": {"deposit_mode": "ratio"}
            },
            {
                "key": "deposit_fixed",
                "label": "固定押金金额",
                "type": "float",
                "min": 0,
                "visible_when": {"deposit_mode": "fixed"}
            }
        ],
        "defaults": {
            "tiers": [
                {"days_max": 30, "discount_percent": 0},
                {"days_max": 365, "discount_percent": 20},
                {"days_max": -1, "discount_percent": 40}
            ],
            "deposit_mode": "ratio",
            "deposit_ratio": 2.0
        }
    }'::jsonb
);
