-- #2210: 商城页图片表（平台级全站配置，与租户无关）
CREATE TABLE IF NOT EXISTS mall_images (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    image_url  VARCHAR(500) NOT NULL,
    link_url   VARCHAR(500) DEFAULT '',
    title      VARCHAR(200) DEFAULT '',
    sort_order INT DEFAULT 0,
    status     VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_mall_images_sort ON mall_images(sort_order);
CREATE INDEX IF NOT EXISTS idx_mall_images_status ON mall_images(status);
