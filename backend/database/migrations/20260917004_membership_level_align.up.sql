-- #1939 Sub-C（#1946）：等级数据对齐《平台会员规则与权益手册》（2026-09-01 起执行）
-- 乐手（min 0）/ 首席（5000 元 = 500000 分）/ 演奏家（10000 元 = 1000000 分）
-- 幂等：按既定 id（072 种子 1/2/3）对齐命名与门槛；金额仍可经 PC 管理页调整（#1939 配置化）
UPDATE membership_levels SET name = '乐手',   min_amount_cents = 0       WHERE id = 1;
UPDATE membership_levels SET name = '首席',   min_amount_cents = 500000  WHERE id = 2;
UPDATE membership_levels SET name = '演奏家', min_amount_cents = 1000000 WHERE id = 3;
