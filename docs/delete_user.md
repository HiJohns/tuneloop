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

# 2.2 微信号反查（若知道 openid；身份切换页看到的账户即来自这张表）
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
\\copy (SELECT * FROM site_members WHERE user_id='<LOCAL_USER_ID>') TO '/tmp/backup_tl_sitemembers_<LOCAL_USER_ID>.csv' CSV HEADER
\\copy (SELECT * FROM points_transactions WHERE user_id='<LOCAL_USER_ID>') TO '/tmp/backup_tl_points_<LOCAL_USER_ID>.csv' CSV HEADER
\\copy (SELECT * FROM referrals WHERE referee_id='<LOCAL_USER_ID>') TO '/tmp/backup_tl_referrals_<LOCAL_USER_ID>.csv' CSV HEADER
\\copy (SELECT * FROM pending_orders WHERE user_id='<LOCAL_USER_ID>') TO '/tmp/backup_tl_pendingorders_<LOCAL_USER_ID>.csv' CSV HEADER
\\copy (SELECT * FROM face_capture_batches WHERE user_id='<LOCAL_USER_ID>') TO '/tmp/backup_tl_facecapture_<LOCAL_USER_ID>.csv' CSV HEADER
\\copy (SELECT * FROM staff_invites WHERE invitee_user_id='<LOCAL_USER_ID>') TO '/tmp/backup_tl_staffinvites_<LOCAL_USER_ID>.csv' CSV HEADER
\\copy (SELECT * FROM point_batches WHERE user_id='<LOCAL_USER_ID>') TO '/tmp/backup_tl_pointbatches_<LOCAL_USER_ID>.csv' CSV HEADER
\\copy (SELECT * FROM user_addresses WHERE user_id='<LOCAL_USER_ID>') TO '/tmp/backup_tl_addresses_<LOCAL_USER_ID>.csv' CSV HEADER
\\copy (SELECT * FROM electronic_contracts WHERE user_id='<LOCAL_USER_ID>') TO '/tmp/backup_tl_contracts_<LOCAL_USER_ID>.csv' CSV HEADER
\\copy (SELECT * FROM order_payment_records WHERE user_id='<LOCAL_USER_ID>') TO '/tmp/backup_tl_payrecords_<LOCAL_USER_ID>.csv' CSV HEADER\""
```

> ⚠️ **占位符易混**：IAM 侧表用 `<IAM_USER_ID>`；tuneloop 侧 `users` 用 `iam_sub = <IAM_USER_ID>`，
> 而 `site_members`/`points_transactions`/`point_batches`/`referrals`/`pending_orders`/`face_capture_batches`/
> `user_addresses`/`electronic_contracts`/`order_payment_records`/`staff_invites` 用 `<LOCAL_USER_ID>`。

## 4. 删除（模式 B，两库按序）

### 4.1 IAM 库（beaconiam_pre_snapshot）——顺序执行

```sql
-- ① 微信绑定（必须删：否则该微信登录仍命中旧户，无法重新注册）
DELETE FROM wx_user_bindings WHERE user_id = '<IAM_USER_ID>';

-- ② 组织关系（顾客/员工 relation）
DELETE FROM user_org_relations WHERE user_id = '<IAM_USER_ID>';
--    注：`functional_roles` 是**本表上的 jsonb 列**（实测值如 ["customer"]），**不是独立表**，
--        随本行删除自动消失，无需单独处理。别去 `users` 表找 —— 该表没有此列。

-- ③ 会话/权限/任务/密码历史残留（易漏，删 users 行后会成为孤儿行）
--    ⚠️ 必须**逐条执行**：部分表在当前库根本不存在（2026-09-28 预生产实测：
--       IAM 库无 confirmation_sessions、无 password_histories），
--       报 relation does not exist 即跳过该条 —— 整批粘贴会让前面的语句一并回滚。
DELETE FROM confirmation_sessions    WHERE user_id = '<IAM_USER_ID>';
DELETE FROM password_histories       WHERE user_id = '<IAM_USER_ID>';
DELETE FROM user_task_queues         WHERE user_id = '<IAM_USER_ID>';
DELETE FROM user_permission_overrides WHERE user_id = '<IAM_USER_ID>';

-- ④ 用户记录（最后删）
DELETE FROM users WHERE id = '<IAM_USER_ID>';
```

> **保留不删**（审计轨迹，同 §0.3）：`audit_logs` / `audit_logs_archive`（均含 `user_id`）。
> **无需处理**：`customer_permissions` 是 namespace 级权限（无 `user_id`），与具体用户无关。

### 4.2 tuneloop 库（tuneloop_pre_snapshot）

```sql
-- ① 本地成员关系
DELETE FROM site_members      WHERE user_id = '<LOCAL_USER_ID>';
DELETE FROM merchant_members  WHERE user_id = '<LOCAL_USER_ID>';

-- ② 注册即写入的关联行（必须一并清，否则删 users 行后残留孤儿行）
--    来源：注册有两条路径 —— PostRegister(handlers/auth.go) 与 IAM webhook
--          (handlers/iam_callback.go handleUserRegistered)，两者都会产生下列行。
DELETE FROM points_transactions  WHERE user_id = '<LOCAL_USER_ID>';
DELETE FROM face_capture_batches WHERE user_id = '<LOCAL_USER_ID>';  -- 人脸/证件采集批次（含采集影像），PII，必删
DELETE FROM user_addresses      WHERE user_id = '<LOCAL_USER_ID>';  -- 收货地址 PII（收件人/手机/省市区/详址），必删
DELETE FROM pending_orders      WHERE user_id = '<LOCAL_USER_ID>';  -- #2041：payload JSONB 含下单表单 PII，必须删
--    referrals 是双向关系：只删「本人是被推荐人」这一侧
DELETE FROM referrals            WHERE referee_id = '<LOCAL_USER_ID>';
--    ⚠️ 若该测试用户还是别人的推荐人（referrer_id 指向他人），不要盲删 —— 先确认影响面再单独处理
--    #2052：清掉邀请指向，避免 staff_invites 指向已删用户
UPDATE staff_invites SET invitee_user_id = NULL WHERE invitee_user_id = '<LOCAL_USER_ID>';

-- ③ 本地用户缓存行
--    判定：以下任一表有记录 → 必须走匿名化（模式 A），不可删 users 行
--          （否则这些业务行的 user_id 全部变孤儿，违反 §0.3 可追溯）：
--          orders / order_payment_records / electronic_contracts /
--          leases / deposits / repair_requests
--    上述全为 0 → 纯测试户，可直接删：
DELETE FROM users WHERE iam_sub = '<IAM_USER_ID>';
--    有业务数据 → 改为匿名化保留（勿删行），业务行继续指向本地 users.id：
--    ⚠️ 必须清空全部 PII 列，否则「彻底清除」名不副实（#2057 intro_letter_url、#2073 bio 为新增列）
-- UPDATE users SET name='已删除用户', nickname=NULL, phone=NULL, email=NULL, ref_code=NULL,
--                  real_name=NULL, id_card_no=NULL, id_card_expire=NULL,
--                  id_card_authority=NULL, id_card_address=NULL,
--                  id_photo_front=NULL, id_photo_back=NULL,
--                  id_photo_other=NULL, id_photo_other_type=NULL,
--                  intro_letter_url=NULL, bio=NULL,
--                  status='inactive', deleted_at=now() WHERE iam_sub='<IAM_USER_ID>';

-- ④ 乐币余额批次（与 ② 的 points_transactions **必须成对清**）
--    否则余额表有值（注册赠送，如 9900 分）而流水表为空，数据自相矛盾
DELETE FROM point_batches WHERE user_id = '<LOCAL_USER_ID>';

-- ⑤ 通知（可选清理；默认保留）
-- DELETE FROM notifications WHERE user_id = '<LOCAL_USER_ID>';
```

> 本地 `users.id` ≠ IAM `users.id`：tuneloop 侧以 **`iam_sub` = IAM user_id** 关联（`users.id` 是本地独立 uuid）。
>
> **业务数据保留（不可删）**：`orders` / `order_payment_records` / `electronic_contracts` /
> `leases` / `deposits` / `repair_requests` / `invoice_applications` / `guarantors` /
> `ownership_certificates` / `maintenance_tickets` —— 这些行的 `user_id` 指向**保留（匿名化）后**的
> 本地 `users.id`，是订单与合同的追溯依据（§0.3）。只删 `users` 行会让它们全部变孤儿。
> 唯一例外是「重新注册」场景：`wx_user_bindings` 必须删净（§4.1 ①），否则该微信仍命中旧户。

## 5. 验证清单（全过才算完成）

```bash
# ① IAM：核心三表 + 关联表均已无该用户
ssh cadenza "docker exec $C psql -U beaconiam_user -d beaconiam_pre_snapshot -c \"
SELECT (SELECT count(*) FROM users WHERE id='<IAM_USER_ID>') AS u,
       (SELECT count(*) FROM wx_user_bindings WHERE user_id='<IAM_USER_ID>') AS b,
       (SELECT count(*) FROM user_org_relations WHERE user_id='<IAM_USER_ID>') AS r,
       (SELECT count(*) FROM password_histories WHERE user_id='<IAM_USER_ID>') AS ph,
       (SELECT count(*) FROM user_task_queues WHERE user_id='<IAM_USER_ID>') AS tq,
       (SELECT count(*) FROM user_permission_overrides WHERE user_id='<IAM_USER_ID>') AS po;\""
# 期望：0 / 0 / 0 / 0 / 0 / 0
# 注：confirmation_sessions / audit_logs 存在残留不阻塞（前者可过期清理，后者按 §0.3 保留审计）

# ② 该微信登录 → wx-accounts 不再返回该账户（0 或仅剩其他合法绑定）
#    （小程序「切换身份」页同步确认）

# ③ 重新注册走通：微信进入注册页 → 全新注册成功（新 user_id）→ 登录正常

# ④ tuneloop 侧无孤儿行（新增表/列残留自检）
ssh cadenza "docker exec $C psql -U tuneloop_user -d tuneloop_pre_snapshot -c \"
SELECT (SELECT count(*) FROM referrals            WHERE referee_id='<LOCAL_USER_ID>')     AS ref,
       (SELECT count(*) FROM points_transactions  WHERE user_id='<LOCAL_USER_ID>')      AS pts,
       (SELECT count(*) FROM point_batches        WHERE user_id='<LOCAL_USER_ID>')      AS pbat,
       (SELECT count(*) FROM pending_orders       WHERE user_id='<LOCAL_USER_ID>')      AS pend,
       (SELECT count(*) FROM face_capture_batches WHERE user_id='<LOCAL_USER_ID>')      AS face,
       (SELECT count(*) FROM user_addresses       WHERE user_id='<LOCAL_USER_ID>')      AS addr,
       (SELECT count(*) FROM staff_invites        WHERE invitee_user_id='<LOCAL_USER_ID>') AS inv;\""
# 期望：0 / 0 / 0 / 0 / 0 / 0 / 0
# 注：orders / order_payment_records / electronic_contracts 属**保留**项 —— 模式 A 下应保持原值（≠0），不是孤儿，勿当失败。
```

## 6. 常见坑

| 坑 | 说明 |
|----|------|
| 只删 tuneloop 不删 IAM | 微信登录走 IAM `wx_user_bindings` → 旧户仍能登录，注册仍被占用 |
| 只删 IAM 不删本地 | tuneloop `users` 缓存残留 → 列表/搜索出现幽灵用户 |
| 凭 openid 删全部绑定 | 一个 openid 可绑多户（历史多户）→ 只删目标户的绑定行 |
| UUID 列写空串 | Postgres `uuid` 列拒绝 `''`（SQLSTATE 22P02）→ 置 `NULL` 或省略 |
| 忘记备份 | 先跑 §3 再动手 |
| 只删 `users` 不删注册副产物 | 注册会写 `points_transactions`/`point_batches`/`referrals`/`face_capture_batches`/`user_addresses`/`pending_orders`/`staff_invites` → 删用户行后残留孤儿行，且 `pending_orders.payload` 含下单表单 PII、`user_addresses` 含收货地址 PII |
| 乐币只删一张 | `point_batches`（余额）与 `points_transactions`（流水）必须成对清，否则余额有值而流水为空 |
| 整批粘贴 SQL 中途报错 | 部分表在库中不存在（实测 IAM 库无 `confirmation_sessions`/`password_histories`）→ 报错前后的语句一并回滚。**逐条执行**，单条 `relation does not exist` 即跳过 |
| 盲删 `referrals` | 该表双向：`referee_id`=本人（删）、`referrer_id`=他人（勿删） |
| 漏清新列 PII | `intro_letter_url`(#2057 介绍信影像)、`bio`(#2073 富文本简介) 为后续新增列，匿名化时未清 → 模式 A 残留个人影像与简介 |
| 以为 DB 删净就干净 | **uploads 磁盘文件仍在**（证件照/介绍信影像等）。本手册不提供自动删除，如需清理由人工定位处理（红线：预生产/生产写操作须先获用户授权） |

## 7. 示例话术（供 AI 直接使用）

> 「根据 docs/delete_user.md 的流程，删除赵楠（预生产）」
> → AI 应：按 §2 定位（姓名/手机）→ §3 备份 → §4 顺序删除两库 → §5 验证 → 汇报删除的 user_id 与验证结果。

---
*关联：#2025（身份模型：删除=标记删除+解除关联）、beaconiam `PurgeUser`（`DELETE /api/v1/users/:id/purge`，等效 §4.1 的 API 路径）、#2036（预生产合并执行窗口）、#2041（`pending_orders`）、#2052（`staff_invites.invitee_user_id`）、#2057（`users.intro_letter_url`）、#2073（`users.bio`）*

*最后核对：2026-09-28 —— 依代码复核（`backend/handlers/auth.go` 注册路径、`backend/models/models.go:38-39,118-125,1039-1047`、`beaconiam/internal/models/*`），并在**预生产库实测校验**：以测试用户「王纯羽」逐一盘点（tuneloop 库 26 张含 `user_id` 的表、IAM 库 7 张），据此补入 `user_addresses`/`electronic_contracts`/`order_payment_records`，并修正 `functional_roles` 的归属（`user_org_relations` 的 jsonb 列）。其他环境表结构可能不同，删除前建议先只读盘点。*
