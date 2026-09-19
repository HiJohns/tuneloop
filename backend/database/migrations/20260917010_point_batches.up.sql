-- #1947 Sub-D 乐币批次化记账：批次表 + 扣减留痕表 + 存量迁移
CREATE TABLE IF NOT EXISTS point_batches (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid NOT NULL,
    source_type     varchar(20) NOT NULL,
    source_ref      varchar(64) DEFAULT '',
    amount_cents    bigint NOT NULL,
    remaining_cents bigint NOT NULL,
    acquired_at     timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz,
    expired_cents   bigint NOT NULL DEFAULT 0,
    expired_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_point_batches_user ON point_batches(user_id);
CREATE INDEX IF NOT EXISTS idx_point_batches_expires ON point_batches(expires_at);
CREATE INDEX IF NOT EXISTS idx_point_batches_user_expires ON point_batches(user_id, expires_at);

CREATE TABLE IF NOT EXISTS point_batch_consumptions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id uuid NOT NULL,
    batch_id       uuid NOT NULL,
    amount_cents   bigint NOT NULL,
    consumed_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_point_batch_consumptions_tx ON point_batch_consumptions(transaction_id);
CREATE INDEX IF NOT EXISTS idx_point_batch_consumptions_batch ON point_batch_consumptions(batch_id);

-- 存量迁移（幂等）：promo_points>0 且尚无 migration 批次的用户，各建一条批次。
-- 口径（#1947 裁定 D）：expires_at = now + 2 年，且归一化为「次月首日 00:00（北京时间）」。
INSERT INTO point_batches (user_id, source_type, source_ref, amount_cents, remaining_cents, acquired_at, expires_at, created_at, updated_at)
SELECT u.id, 'migration', 'legacy', u.promo_points, u.promo_points, now(),
       (date_trunc('month', (now() AT TIME ZONE 'Asia/Shanghai') + interval '2 years') + interval '1 month') AT TIME ZONE 'Asia/Shanghai',
       now(), now()
FROM users u
WHERE u.promo_points > 0
  AND NOT EXISTS (
      SELECT 1 FROM point_batches pb
      WHERE pb.user_id = u.id AND pb.source_type = 'migration'
  );
