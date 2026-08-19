-- #1715 down: remove backfilled merchant_members rows (keep table).
DELETE FROM merchant_members
WHERE user_id IN (
    SELECT id FROM users
    WHERE iam_sub IN (
        '1745e9d7-c616-48bf-9fb4-e2dffee45231',
        '6025663a-1cfb-4363-8a36-52ebbe2b6534',
        '8278bc17-3153-4466-814e-934db3cce3ab',
        '07323754-e3bc-4b7c-8256-21451803d4d9',
        '85c51a93-dec8-4677-9f74-f1a3207c125b',
        '98d1fbe0-155b-4940-b0fe-a01af4914e37',
        'f42c8130-3788-4295-8e52-650011a8641c'
    )
);
