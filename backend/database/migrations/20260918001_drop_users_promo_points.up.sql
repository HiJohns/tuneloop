-- #1983 阶段 2：读路径切换为 SUM(未过期批次)，废弃 users.promo_points 快照。
--
-- 漂移对账（保护过渡期「快照-only」写入/管理员历史调整）：
--   对「快照 > 未过期批次合计」的用户补建 migration 批次（差额），
--   确保删除快照列后用户可用余额不减少；
--   「快照 < 批次合计」不回拔（对用户有利方向）。
INSERT INTO point_batches (user_id, source_type, source_ref, amount_cents, remaining_cents, acquired_at, expires_at, created_at, updated_at)
SELECT s.user_id, 'migration', 'stage2_sync', s.delta, s.delta, now(),
       (date_trunc('month', (now() AT TIME ZONE 'Asia/Shanghai') + interval '2 years') + interval '1 month') AT TIME ZONE 'Asia/Shanghai',
       now(), now()
FROM (
    SELECT u.id AS user_id,
           COALESCE(u.promo_points, 0) - COALESCE((
               SELECT SUM(b.remaining_cents) FROM point_batches b
               WHERE b.user_id = u.id
                 AND b.remaining_cents > 0
                 AND (b.expires_at IS NULL OR b.expires_at >= now())
           ), 0) AS delta
    FROM users u
) s
WHERE s.delta > 0;

ALTER TABLE users DROP COLUMN IF EXISTS promo_points;
