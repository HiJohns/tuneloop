# 删除指定用户（测试注册用）操作手册

> **受众**：AI 助手 / 运维。目标：彻底清除指定用户（如「赵楠」）的全部身份记录，使该微信号/手机号可以**重新走完整注册流程**。
> **适用环境**：预生产（`*-pre_snapshot` 库）。**生产库执行前必须获得用户明确授权。**

---

## 0. 红线（先读）

1. **先备份，后删除**（见 §3）。删除不可逆。
2. 预生产/生产数据库的任何写操作，**必须先向用户确认**（项目红线）。
3. **业务数据保留**：`orders` / `point_batches` / `notifications` 等**不删除**（保留审计轨迹）。若必须清理业务数据，单独向用户确认。
4. 操作对象是**指定那一个用户**；禁止批量。

## 1. 选择模式

| 模式 | 用途 | 用户记录 | 重新注册 |
|------|------|---------|:---:|
| **A 标记删除**（保留可加回） | 停用账户但保留数据归属 | `status='inactive'`，relation 停用 | ❌（同手机号/邮箱仍占用） |
| **B 彻底清除**（**测试注册用**） | 释放 微信号/手机号/邮箱，允许全新注册 | IAM users 行删除 | ✅ |

> 测试注册场景（如「赵楠」）→ **用模式 B**。

## 2. 定位用户（两库各取 user_id）

```bash
# 容器名
C=$(ssh cadenza "docker ps --format '{{.Names}}' | grep -i postgres | head -1")

# 2.1 IAM 库（beaconiam_pre_snapshot）：按 姓名/用户名/手机/邮箱 模糊定位
ssh cadenza "docker exec $C psql -U beaconiam_user -d beaconiam_pre_snapshot -c \"
SELECT id, username, name, nickname, phone, email, status FROM users
WHERE name ILIKE '%<关键词>%' OR username ILIKE '%<关键词>%' OR phone LIKE '%<关键词>%' OR email ILIKE '%<关键词>%';\""

# 2.2 微信号反查（若知道 openid；切换账户页看到的账户即来自这张表）
ssh cadenza "docker exec $C psql -U beaconiam_user -d beaconiam_pre_snapshot -c \"
SELECT b.user_id, u.username, u.name, u.phone FROM wx_user_bindings b JOIN users u ON u.id=b.user_id WHERE b.openid='<openid>';\""

# 2.3 tuneloop 库（tuneloop_pre_snapshot）：按 iam_sub 关联（= IAM user_id）
ssh cadenza "docker exec $C psql -U tuneloop_user -d tuneloop_pre_snapshot -c \"
SELECT id, username, name, phone, iam_sub, created_at FROM users WHERE iam_sub='<IAM_USER_ID>';\""
```

> ⚠️ 一个微信号可能绑定多条 user（`wx_user_bindings` 一对多）——**逐条确认**要删哪个，不要凭 openid 全删。

## 3. 备份（删除前必做）

```bash
ssh cadenza "docker exec $C psql -U beaconiam_user -d beaconiam_pre_snapshot -c \"
\\copy (SELECT * FROM users WHERE id='<IAM_USER_ID>') TO '/tmp/backup_user_<IAM_USER_ID>.csv' CSV HEADER
\\copy (SELECT * FROM wx_user_bindings WHERE user_id='<IAM_USER_ID>') TO '/tmp/backup_bindings_<IAM_USER_ID>.csv' CSV HEADER
\\copy (SELECT * FROM user_org_relations WHERE user_id='<IAM_USER_ID>') TO '/tmp/backup_relations_<IAM_USER_ID>.csv' CSV HEADER\""
ssh cadenza "docker exec $C psql -U tuneloop_user -d tuneloop_pre_snapshot -c \"
\\copy (SELECT * FROM users WHERE iam_sub='<IAM_USER_ID>') TO '/tmp/backup_tl_user_<IAM_USER_ID>.csv' CSV HEADER
\\copy (SELECT * FROM site_members WHERE user_id='<IAM_USER_ID>') TO '/tmp/backup_tl_sitemembers_<IAM_USER_ID>.csv' CSV HEADER\""
```

## 4. 删除（模式 B，两库按序）

### 4.1 IAM 库（beaconiam_pre_snapshot）——顺序执行

```sql
-- ① 微信绑定（必须删：否则该微信登录仍命中旧户，无法重新注册）
DELETE FROM wx_user_bindings WHERE user_id = '<IAM_USER_ID>';

-- ② 组织关系（含顾客/员工 relation 与 functional_roles）
DELETE FROM user_org_relations WHERE user_id = '<IAM_USER_ID>';

-- ③ 审计/会话残留（存在才删；表缺列则跳过该条）
DELETE FROM confirmation_sessions WHERE user_id = '<IAM_USER_ID>';

-- ④ 用户记录（最后删）
DELETE FROM users WHERE id = '<IAM_USER_ID>';
```

### 4.2 tuneloop 库（tuneloop_pre_snapshot）

```sql
-- ① 本地成员关系
DELETE FROM site_members      WHERE user_id = '<LOCAL_USER_ID>';
DELETE FROM merchant_members  WHERE user_id = '<LOCAL_USER_ID>';

-- ② 本地用户缓存行
--    测试户（无订单等业务数据）→ 直接删：
DELETE FROM users WHERE iam_sub = '<IAM_USER_ID>';
--    若该户有订单等业务数据 → 改为匿名化保留（勿删行）：
-- UPDATE users SET name='已删除用户', nickname=NULL, phone=NULL, email=NULL,
--                  status='inactive', deleted_at=now() WHERE iam_sub='<IAM_USER_ID>';

-- ③ 乐币批次/通知（可选清理；默认保留）
-- DELETE FROM point_batches   WHERE user_id = '<LOCAL_USER_ID>';
-- DELETE FROM notifications   WHERE user_id = '<LOCAL_USER_ID>';
```

> 本地 `users.id` ≠ IAM `users.id`：tuneloop 侧以 **`iam_sub` = IAM user_id** 关联（`users.id` 是本地独立 uuid）。

## 5. 验证清单（全过才算完成）

```bash
# ① IAM：三表均已无该用户
ssh cadenza "docker exec $C psql -U beaconiam_user -d beaconiam_pre_snapshot -c \"
SELECT (SELECT count(*) FROM users WHERE id='<IAM_USER_ID>') AS u,
       (SELECT count(*) FROM wx_user_bindings WHERE user_id='<IAM_USER_ID>') AS b,
       (SELECT count(*) FROM user_org_relations WHERE user_id='<IAM_USER_ID>') AS r;\""
# 期望：0 / 0 / 0

# ② 该微信登录 → wx-accounts 不再返回该账户（0 或仅剩其他合法绑定）
#    （小程序「切换账户」页同步确认）

# ③ 重新注册走通：微信进入注册页 → 全新注册成功（新 user_id）→ 登录正常
```

## 6. 常见坑

| 坑 | 说明 |
|----|------|
| 只删 tuneloop 不删 IAM | 微信登录走 IAM `wx_user_bindings` → 旧户仍能登录，注册仍被占用 |
| 只删 IAM 不删本地 | tuneloop `users` 缓存残留 → 列表/搜索出现幽灵用户 |
| 凭 openid 删全部绑定 | 一个 openid 可绑多户（历史多户）→ 只删目标户的绑定行 |
| UUID 列写空串 | Postgres `uuid` 列拒绝 `''`（SQLSTATE 22P02）→ 置 `NULL` 或省略 |
| 忘记备份 | 先跑 §3 再动手 |

## 7. 示例话术（供 AI 直接使用）

> 「根据 docs/delete_user.md 的流程，删除赵楠（预生产）」
> → AI 应：按 §2 定位（姓名/手机）→ §3 备份 → §4 顺序删除两库 → §5 验证 → 汇报删除的 user_id 与验证结果。

---
*关联：#2025（身份模型：删除=标记删除+解除关联）、beaconiam `PurgeUser`（`DELETE /api/v1/users/:id/purge`，等效 §4.1 的 API 路径）、#2036（预生产合并执行窗口）*
