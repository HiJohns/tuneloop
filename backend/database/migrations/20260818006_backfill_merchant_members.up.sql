-- #1715: backfill merchant_members from the authoritative IAM org bindings.
-- merchant_members never existed before 20260818005, so real memberships live
-- in beaconiam.user_org_relations (org_id = merchant.org_id). This migration
-- copies active relations into merchant_members, idempotent (skip existing).
-- Cross-DB note: tuneloop cannot read beaconiam directly — the mapping below
-- is expressed via the local users.iam_sub ↔ IAM user_id equivalence; the
-- INSERT selects from a VALUES list of (merchant_id, iam_user_id, role) built
-- from the org relations snapshot.
-- 生产/预生产 DDL 执行需用户确认（红线）。

INSERT INTO merchant_members (id, tenant_id, merchant_id, user_id, role, status, created_at, updated_at)
SELECT
    gen_random_uuid(),
    m.tenant_id,
    src.merchant_id,
    u.id,
    CASE WHEN src.role = 'OWNER' THEN 'merchant_admin' ELSE 'site_member' END,
    'active',
    now(),
    now()
FROM (VALUES
    -- (merchant_id, iam_user_id, role) — snapshot of beaconiam user_org_relations
    ('060bb8cf-8794-4eb3-af92-1416d5453f92'::uuid, '1745e9d7-c616-48bf-9fb4-e2dffee45231'::uuid, 'OWNER'),  -- 卡丹萨 明月
    ('060bb8cf-8794-4eb3-af92-1416d5453f92'::uuid, '6025663a-1cfb-4363-8a36-52ebbe2b6534'::uuid, 'member'),  -- 卡丹萨 南哥
    ('060bb8cf-8794-4eb3-af92-1416d5453f92'::uuid, '8278bc17-3153-4466-814e-934db3cce3ab'::uuid, 'member'),  -- 卡丹萨 维修张师傅
    ('3911392b-fdb9-4658-a1b0-8b41293718e3'::uuid, '07323754-e3bc-4b7c-8256-21451803d4d9'::uuid, 'OWNER'),  -- cadenza 张三
    ('3911392b-fdb9-4658-a1b0-8b41293718e3'::uuid, '85c51a93-dec8-4677-9f74-f1a3207c125b'::uuid, 'member'),  -- cadenza 王五
    ('3911392b-fdb9-4658-a1b0-8b41293718e3'::uuid, '98d1fbe0-155b-4940-b0fe-a01af4914e37'::uuid, 'member'),  -- cadenza 南哥
    ('3911392b-fdb9-4658-a1b0-8b41293718e3'::uuid, 'f42c8130-3788-4295-8e52-650011a8641c'::uuid, 'member')   -- cadenza 挺好听
) AS src(merchant_id, iam_user_id, role)
JOIN merchants m ON m.id = src.merchant_id
JOIN users u ON u.iam_sub = src.iam_user_id::text
WHERE NOT EXISTS (
    SELECT 1 FROM merchant_members mm
    WHERE mm.merchant_id = src.merchant_id AND mm.user_id = u.id
);
