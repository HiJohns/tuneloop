# IAM 孤儿账号清理 SOP（pending-user-orphan-cleanup）

> 关联：#2369（父 #2364）。适用角色：系统管理员（namespace admin）+ 运维。
> 目标：创建用户时本地事务失败 → 回滚 purge 被 IAM 拒绝（仅 namespace admin 可 purge）→ 留下
> **pending 且无本地记录**的 IAM 账号，导致同邮箱/用户名重试永远 409。本 SOP 说明如何**识别**与**收尾**。

## 1. 什么是孤儿（判定四条，缺一不可）

| # | 条件 | 说明 |
|---|---|---|
| 1 | IAM `users.status = 'pending'` | 从未激活 |
| 2 | IAM `users.org_id IS NULL` | 未绑定具体组织（`include_orgs` 的 `organizations` 不计） |
| 3 | tuneloop 本地 `users` 中**不存在** `iam_sub = <id>` | 创建后本地事务已回滚——这是与「正常邀请中用户」的关键区别 |
| 4 | `created_at` 早于 **60 分钟** | 避免与正在进行的创建流程竞争 |

> **不要**只按「pending + 无组织关系」判定：正常邀请制用户在被接受前同样是 pending，
> 但他们有本地 `users` 行，必须排除。

## 2. 本次实例（2026-11-16，预生产）

```text
[CreateUser] rollback purge failed for 6a08ff3b-...: PurgeUser returned status 403:
             {"error":"only namespace admin can purge users"}
[CreateUser] 500 → 同邮箱/用户名重试 5 次全部 409（IAM 已注册）
```

- 现象：管理员连续创建失败，每次 500；重试同邮箱/用户名恒 409，无法自愈。
- 根因：tuneloop 回滚使用 **service 身份**调用 `DELETE /api/v1/users/{id}/purge`，IAM 仅允许
  namespace admin（或本人）清理 → 403；本地事务已回滚，IAM 账号成为孤儿。
- 处置：按本 SOP 第 4 节清理该 `iam_user_id`，再重新创建用户（首次即成功）。

## 3. 识别（只读核对）

### 3.1 UI 入口（推荐）

1. PC 管理后台 → **系统 → 用户管理**（`/system/user-management`）。
2. 点击工具栏 **「IAM 孤儿账号」**。
3. 面板列出：登录名 / 姓名 / 邮箱 / 手机 / 创建时间 / 已挂起分钟数；支持单个或勾选批量清理。

### 3.2 API（等价，便于脚本核对）

```bash
# 需要系统管理员 token（cookie token 或 Bearer）
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  https://preweb.cadenzayueqi.com/api/admin/iam-orphans | jq '.data'
```

返回 `data.list[]`（孤儿）、`data.scanned`（本次扫描的 pending 总数）、`data.min_age_minute`（60）。

### 3.3 IAM 侧校验 SQL（只读）

```sql
-- IAM 数据库（预生产 beaconiam_pre_snapshot / 生产 beaconiam）
SELECT id, username, email, phone, status, org_id, created_at
FROM users
WHERE status = 'pending'
  AND org_id IS NULL
  AND created_at < now() - interval '60 minutes'
ORDER BY created_at;
```

### 3.4 本地校验 SQL（只读）

```sql
-- tuneloop 数据库
SELECT id, iam_sub, username, email, status
FROM users
WHERE iam_sub = '<IAM_USER_ID>';
-- 0 行 = 本地无记录（孤儿）；有行 = 正常邀请/账号，禁止清理（D5）
```

## 4. 清理步骤

### 4.1 UI（推荐）

1. 打开「IAM 孤儿账号」面板，核对第 3 节 SQL 结果与面板一致。
2. 单个：行末「清理」→ 二次确认；批量：勾选后「批量清理」→ 二次确认。
3. 成功提示「已清理 N 个」；失败条目会在提示中显示数量，详细信息见浏览器 Console
   （`[iam-orphans] failed entries`）。
4. 点击「刷新」确认列表已空。

### 4.2 API（等价）

```bash
curl -s -X POST -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"user_ids":["<IAM_USER_ID>"]}' \
  https://preweb.cadenzayueqi.com/api/admin/iam-orphans/purge | jq '.data'
# 期望：{"purged":1,"failed":[],"notes":["IAM purge 不会清理 user_task_queues ..."]}
```

保护规则（后端强制）：

- 本地 `users` 中存在该 `iam_sub` → 该 ID 进 `failed`，**不删除**（D5）。
- 非孤儿（org_id 非空 / 年龄不足 / 非 pending）→ 进 `failed`，不删除。
- `user_ids` 为空 → 400 `40001`（不静默成功）。

## 5. `user_task_queues` 收尾（IAM 侧）

`DELETE /api/v1/users/{id}/purge` **不会**清理 `user_task_queues`。若该账号曾被排队绑定
（`bind_org` / `role` 任务），需在 IAM 数据库手工核对并删除，否则队列会保留已不存在用户的待办。

```sql
-- 1) 核对（IAM 数据库）
SELECT id, user_id, org_id, role, task_type, status, created_at
FROM user_task_queues
WHERE user_id = '<IAM_USER_ID>'
ORDER BY created_at;

-- 2) 确认该 user_id 已不在 users 表（purge 成功）
SELECT count(*) FROM users WHERE id = '<IAM_USER_ID>';  -- 期望 0

-- 3) 删除遗留队列行（仅在上一步为 0 时执行）
DELETE FROM user_task_queues WHERE user_id = '<IAM_USER_ID>';
```

- **红线**：tuneloop 后端不直连 IAM 数据库（`grep -rn user_task_queues backend` 必须 0 命中）；
  队列清理只能由运维在 IAM 侧按本节执行。
- 批量清理后可用 `SELECT user_id, count(*) FROM user_task_queues q
  WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = q.user_id) GROUP BY user_id;`
  找出「队列指向已删用户」的历史残留。

## 6. 预防与监控

- 创建用户失败响应现在会带 `data.orphan=true` + `data.iam_user_id`（#2369 R1）；日志标记
  `create_user_orphan`，可直接检索：

  ```bash
  journalctl -u tuneloop-pre --since '1 hour ago' --no-pager | grep create_user_orphan
  ```

- 见到该标记时：按第 3 节确认，再按第 4 节清理；清理后重试创建应立即成功。
- 孤儿面板每 60 分钟为年龄门槛，刚失败（<60 分钟）的账号暂时不会出现，稍后刷新即可。
