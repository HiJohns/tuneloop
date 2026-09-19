-- #1983 回滚：恢复 users.promo_points 快照列，并以当前未过期批次合计回填。
ALTER TABLE users ADD COLUMN IF NOT EXISTS promo_points bigint NOT NULL DEFAULT 0;

UPDATE users u
SET promo_points = COALESCE((
    SELECT SUM(b.remaining_cents) FROM point_batches b
    WHERE b.user_id = u.id
      AND b.remaining_cents > 0
      AND (b.expires_at IS NULL OR b.expires_at >= now())
), 0);
