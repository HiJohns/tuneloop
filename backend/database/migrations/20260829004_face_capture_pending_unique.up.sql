-- #1790 T2 R2 M3: 核身批次并发幂等 — 部分唯一索引
-- 同一用户同时仅允许一个 pending 批次（并发提交时 DB 级兜底）。
CREATE UNIQUE INDEX IF NOT EXISTS uq_face_capture_batches_user_pending
    ON face_capture_batches (user_id) WHERE status = 'pending';
