-- #1789 T1: 实名核身五态状态机 — users.face_verify_method 列 + face_capture_batches 表
-- 背景：#1787 实名核身流程；face_verified 布尔 + 批次审核表，五态派生
-- （none/uploaded/pending_review/verified/rejected）见 docs/cases/id-photos.md C6。
ALTER TABLE users ADD COLUMN IF NOT EXISTS face_verify_method VARCHAR(10);

CREATE TABLE IF NOT EXISTS face_capture_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',          -- pending/approved/rejected
    reject_reason TEXT,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_by VARCHAR(255),                                -- 平台员工（本地 users 缓存 name/ID）
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_face_capture_batches_user ON face_capture_batches (user_id, submitted_at DESC);
CREATE INDEX IF NOT EXISTS idx_face_capture_batches_status ON face_capture_batches (status);
