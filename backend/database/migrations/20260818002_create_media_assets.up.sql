-- Unified media asset registry (#1692): tracks every physical file written under
-- uploads/media/ for orphan detection and periodic cleanup. instrument_media
-- remains the authoritative source for business media; media_assets is an index.
CREATE TABLE IF NOT EXISTS media_assets (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    storage_key        VARCHAR(500) NOT NULL,
    source_type        VARCHAR(30)  NOT NULL,
    source_id          VARCHAR(100),
    is_referenced      BOOLEAN      NOT NULL DEFAULT true,
    ref_count          INT          NOT NULL DEFAULT 1,
    file_size          BIGINT,
    file_type          VARCHAR(10),
    created_at         TIMESTAMP    NOT NULL DEFAULT now(),
    last_referenced_at TIMESTAMP    NOT NULL DEFAULT now(),
    CONSTRAINT uq_media_assets_storage_key UNIQUE (storage_key)
);

CREATE INDEX IF NOT EXISTS idx_media_assets_unreferenced
    ON media_assets (is_referenced, last_referenced_at);
