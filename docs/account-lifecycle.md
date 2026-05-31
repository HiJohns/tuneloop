# 账户生命周期与数据完整性

> 一个可登录、有权限、有关联网点的用户，必须在三个系统中同时存在且数据一致。

## 系统分布

| 系统 | 数据库 | 表 | 存储内容 |
|------|--------|-----|----------|
| **IAM** | `beaconiam_debug` | `users` | IAM 侧用户身份 |
| **IAM** | `beaconiam_debug` | `user_org_relations` | 组织绑定 + 权限位图 |
| **Tuneloop** | `tuneloop_debug` | `users` | 本地用户记录 |
| **Tuneloop** | `tuneloop_debug` | `site_members` | 网点关联 |

## 创建流程

```mermaid
sequenceDiagram
    participant F as 前端
    participant B as Tuneloop
    participant I as IAM
    
    F->>B: POST /api/users {name, phone, email, username, site_id}
    B->>B: 校验唯一性（phone/email/username）
    B->>I: POST /api/v1/users (创建 IAM 用户)
    I-->>B: {user_id: "..."} ← 注意响应格式是扁平 JSON
    B->>B: 校验 iamResp.UserID != ""
    B->>I: BindUserToOrganization (组织绑定)
    B->>I: SetUserCustomerPermissions (设置权限位图)
    B->>B: db.Create(&user) (写入本地 users 表)
    B->>B: db.Create(&site_member) (写入 site_members 表)
    B-->>F: {code: 20000}
```

## 用户数据完整性清单

一个完整的用户账户，必须满足以下全部条件：

### 1. IAM 侧
- [ ] `beaconiam_debug.users` 有记录 → `id` 即为 IAM 用户 ID
- [ ] `beaconiam_debug.user_org_relations` 有记录:
  - `user_id` = IAM 用户 ID
  - `org_id` = 所属组织的 ID（朝阳店 = `65ad8d7f-...`）
  - `role` = 业务角色（`site_member` / `site_admin` 等）
  - `cus_perm` ≠ 0（非零权限位图）
  - `sys_perm` 按角色需求设定

### 2. Tuneloop 侧
- [ ] `tuneloop_debug.users` 有记录:
  - `iam_sub` = IAM 用户 ID（与 IAM 侧的 `id` 一致）
  - `tenant_id` = 租户 ID
  - `org_id` = 组织 ID
- [ ] `tuneloop_debug.site_members` 有记录:
  - `user_id` = 本地用户 ID
  - `site_id` = 关联网点 ID
  - `role` = 网点角色（`site_member`）

## 常见缺失模式与症状

| 缺失环节 | 症状 |
|----------|------|
| `iam_sub` 为空 | IAM 用户已创建但本地未关联 → 可登录 IAM 但 JWT 无 tid/oid → 40104 被踢回登录页 |
| 无 `user_org_relations` | IAM 用户有但无组织绑定 → JWT 无 tid/oid → 同上 |
| `cus_perm = 0` | 组织绑定有但权限位图未设 → 前端菜单全部隐藏，显示"权限不足" |
| 无 `site_members` | 本地记录有但无网点关联 → 人员管理列表"关联网点"为空 |

## 关键陷阱

### IAM CreateUser 响应格式
IAM API 返回**扁平**格式 `{"user_id": "xxx"}`，但 `iam_client.go` 先尝试解析嵌套格式 `{"data": {"user_id": "xxx"}}`。
- 第一层 `json.Unmarshal` 对扁平响应**静默成功**（缺失的 `data` 键→零值，不报错）
- `UserID` 为空 → 后续绑定全部跳过
- **修复**：`err != nil || result.Data.UserID == ""` 条件触发 fallback

### 角色权限位图
`cus_perm` 是位图整数，由 `ComputeCusPermBitmapExt` 根据 `PermissionRegistry` 的 bit 映射计算:
- site_member (值=8679): instrument:create/read/update/maintain + order:create/read/update + audit_log:read
- site_admin (值=待确认): 包含以上 + instrument:delete + order:cancel + appeal:read/handle
- `sys_perm` 为 0 时中间件 pass-through，不阻塞请求

## 手动修复参考

```bash
# 1. 设 iam_sub
docker exec -it jobmaster-postgres psql -U tuneloop_user -d tuneloop_debug \
  -c "UPDATE users SET iam_sub = '<IAM_USER_ID>' WHERE id = '<LOCAL_USER_ID>';"

# 2. 建立 IAM 组织绑定
docker exec -it jobmaster-postgres psql -U iam_user -d beaconiam_debug \
  -c "INSERT INTO user_org_relations (id, user_id, org_id, role, is_active, created_at, updated_at)
       VALUES (gen_random_uuid(), '<IAM_USER_ID>', '<ORG_ID>', 'site_member', true, now(), now());"

# 3. 设 IAM 权限位图
docker exec -it jobmaster-postgres psql -U iam_user -d beaconiam_debug \
  -c "UPDATE user_org_relations SET cus_perm = 8679
       WHERE user_id = '<IAM_USER_ID>' AND org_id = '<ORG_ID>';"

# 4. 建本地网点关联
docker exec -it jobmaster-postgres psql -U tuneloop_user -d tuneloop_debug \
  -c "INSERT INTO site_members (tenant_id, site_id, user_id, role)
       VALUES ('<TENANT_ID>', '<SITE_ID>', '<LOCAL_USER_ID>', 'site_member');"
```

> 来源: #701 事故 — IAM 响应格式变迁导致 `iam_sub` 为空，连锁引发绑定、权限、网点三缺。
