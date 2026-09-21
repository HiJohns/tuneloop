-- #2010 S4：废弃 lease_sessions 表
-- 前置（同批/前序已交付）：收货地址已并入 orders.delivery_address（20260921001），
-- 读方（GetOrder/GetInstrumentByID）已切换（S2），写方（更新类 S3 / 创建 S4）已移除。
DROP TABLE IF EXISTS lease_sessions;
