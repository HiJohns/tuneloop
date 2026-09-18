-- #1942 维修服务（单项服务商品）：repair_requests 扩展 + 分段物流费 + 评价表
-- up/down 幂等（IF NOT EXISTS / IF EXISTS）

ALTER TABLE repair_requests
    ADD COLUMN IF NOT EXISTS type VARCHAR(20) NOT NULL DEFAULT 'warranty',
    ADD COLUMN IF NOT EXISTS repair_code VARCHAR(6),
    ADD COLUMN IF NOT EXISTS technician_id UUID,
    ADD COLUMN IF NOT EXISTS quote_repair_cents BIGINT,
    ADD COLUMN IF NOT EXISTS quote_logistics_cents BIGINT,
    ADD COLUMN IF NOT EXISTS quote_status VARCHAR(20) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS adjusted_quote_cents BIGINT,
    ADD COLUMN IF NOT EXISTS incurred_repair_cents BIGINT;

CREATE INDEX IF NOT EXISTS idx_repair_requests_type ON repair_requests (type);
-- 唯一编码：repair_code 为 NULL 的报修单不参与唯一约束（PostgreSQL 唯一索引允许多 NULL）
CREATE UNIQUE INDEX IF NOT EXISTS idx_repair_requests_repair_code ON repair_requests (repair_code);

CREATE TABLE IF NOT EXISTS repair_logistics_fees (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repair_id    UUID NOT NULL,
    leg          INT NOT NULL,
    amount_cents BIGINT NOT NULL DEFAULT 0,
    filled_by    VARCHAR(255),
    created_at   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_repair_logistics_fees_repair ON repair_logistics_fees (repair_id);

CREATE TABLE IF NOT EXISTS repair_reviews (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repair_id  UUID NOT NULL,
    user_id    UUID NOT NULL,
    rating     INT NOT NULL,
    message    TEXT,
    photos     JSONB DEFAULT '[]',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_repair_reviews_repair ON repair_reviews (repair_id);
CREATE INDEX IF NOT EXISTS idx_repair_reviews_user ON repair_reviews (user_id);
