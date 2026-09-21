-- #2016 S3：废弃 tuneloop 本地 users.wx_openid / wx_unionid 缓存列
-- 前置（已交付）：openid 读写统一走 beaconiam wx_user_bindings（S1 内部解析端点 + S2 prepay/用户管理切换）；
-- 本地列写点（绑定/解绑/注册/同步）与读点（openidOfUser、user_staff 响应）均已移除。
ALTER TABLE users DROP COLUMN IF EXISTS wx_openid;
ALTER TABLE users DROP COLUMN IF EXISTS wx_unionid;
