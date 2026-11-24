## Accepted Plan — #2375 Rev.3（补 Files：verify 脚本/evidence.json；R4 走包装器；R7 由 Owner 签）

### 背景

#2364 D1 决策：创建时**自动角色分配失败 → 只警告、不回滚**；并且商户管理员登录后**能看到新建账号、能为其分配角色**。
现状缺口（已扫）：`CreateUser` 角色失败仅 `log.Printf`（`user_staff.go:426-428`）；`UpdateUser` 请求体**没有** `role`（`user_staff.go:657-664`），`StaffEdit.jsx:100-105` 的角色下拉提交后被丢弃；`ListStaff` 不返回角色字段。

### 变更点

1. `CreateUser`：角色分配失败收集到 `data.role_errors:[{code,error}]`，创建仍成功（HTTP 200）。
2. `UpdateUser` 新增 `role`（白名单 `site_member/site_admin/merchant_staff/repair_technician`）：
   - 本地：`users.role` + 目标域（`site_members.role/roles` 或 `merchant_members.role`）；
   - IAM：`UpdateUserRoleInOrgWithToken` + `AssignRoleTemplateToUserWithToken`（幂等，失败进 `role_errors` 而非 500）；
   - 审计：`writeAuditLog(action="user.role_update")`（fire-and-forget，与 #2062 口径一致）。
3. `ListStaff` 返回 `role`（优先级：`site_members.role` → `merchant_members.role` → `users.role`）。
4. 前端：`StaffEdit` 角色下拉真正生效（提交含 role，失败展示 `role_errors`）；`StaffManagement` 创建成功但 `role_errors` 非空 → `message.warning` + 列表新增「角色」列。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| 创建时角色失败不阻断 | `backend/handlers/user_staff.go` | `POST /api/users` | IAM 功能角色返回 404 | 后端 | HTTP 200；`data.role_errors` 非空；`users` 行存在 | R1 |
| 管理员改角色落库 + IAM 同步 | `backend/handlers/user_staff.go` | `PUT /api/users/:id` | `{role:"repair_technician"}` | 后端 | 200；`users.role` 更新；IAM 收到 role + 模板调用；审计写入 | R2 |
| 越权角色被拒 | `backend/handlers/user_staff.go` | `PUT /api/users/:id` | `{role:"merchant_admin"}` / 跨商户用户 | 后端 | 403（白名单外）/ 404（跨租户不可见） | R3 |
| 列表展示角色 | `frontend-pc/src/pages/StaffManagement.jsx` | `/staff` 列表 | 任一用户 | PC 前端 | 列表出现「角色」列，值来自后端 `role` | R4 |
| pending 账号改角色不 500 | `backend/handlers/user_staff.go` | `PUT /api/users/:id` | IAM role 端点 404 | 后端 | HTTP 200 + `data.role_errors` ≥1；本地 `users.role` 已更新 | R5 |
| IAM 目标参数正确 | 同上 | `PUT /api/users/:id` | `{role:"repair_technician"}` / `{role:"merchant_staff"}` | 后端 | mock 收到 `org_id=existingUser.OrgID`；`iam_role` 分别 `repair_technician` / `STAFF` | R6 |
| 角色列真实渲染 | `frontend-pc/src/pages/StaffManagement.jsx` | `/staff` 列表 | 有角色用户 | PC 浏览器 | 「角色」列可见且值=后端 role（Owner 人工回执） | R7 |

## Input Truth & Tool Preflight

- **Source of truth**：`handlers/permission_manage.go:240 toIAMRole`（角色 → IAM 关系角色映射）、`models.SiteMember.Roles`（#2034 多重角色）、`models.MerchantMember.Role`。
- **白名单核对**：仅 `site_member` / `site_admin` / `merchant_staff` / `repair_technician`（与 `user_staff.go:324 validRoles` 一致）；**禁止** `merchant_admin`、`OWNER`、平台角色（提权面）。
- **作用域核对**：`UpdateUser` 现有查询已带 `tenant_id = ?` + `deleted_at IS NULL`；改为角色写入时**必须复用同一行查询结果**，不得新增无 scope 的 UPDATE。
- **IAM 能力核对**：`UpdateUserRoleInOrgWithToken`、`AssignRoleTemplateToUserWithToken` 均存在（`services/iam_client.go:1368/1687`）；pending 用户模板分配在 beaconiam#495 前会 404 → 必须进 `role_errors`。
- **审计**：复用 `handlers/audit_helper.go writeAuditLog`（jsonb details，fire-and-forget）；不新增审计中间件。
- **损坏输入拒绝**：`role` 为空 → 视为"不修改角色"（保持向后兼容）；空白字符串 → 400。

## Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 角色失败是否回滚创建 | 否（#2364 D1）；只警告 |
| D2 | 是否允许改 `merchant_admin` | 否；商户负责人由 #1718 独立流程管理 |
| D3 | pending 账号改角色 | 允许提交，靠 `role_errors` 告知"待激活后生效"（依赖 beaconiam#495 实现生效） |
| D4 | 多重角色 | 本任务只改主角色（`users.role` + `site_members.role`/`roles=[role]` 覆盖单值）；多角色 UI 另立 Issue |

## Out of Scope

- 多角色勾选 UI（#2034 的 `roles` 集合语义仅做最小同步）。
- 平台员工角色（`/platform-staff`，独立权限域）。
- IAM 侧 pending 用户的队列化实现（beaconiam#495）。
- 角色变更后的权限缓存即时刷新（沿用 `IncrementPermVersion` 现状）。

## G0 Golden Acceptance

| 行 | fixture（精确输入/seed） | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 测试库 + IAM mock：`POST /users` 返回 404 on `roles` 端点；请求 `{name,username:"t1",auto_generate:true,user_type:"repair_technician"}` | `POST /api/users` | `docs/evidence/2375/R1-role-errors.txt` | HTTP 200；`data.role_errors` 长度 ≥1；`users` 1 行 | 返回 500 或回滚用户 → 测试失败 |
| R2 | 已有用户 `u2`（`role=site_member`）+ IAM mock 记录 role/template 调用 | `PUT /api/users/u2` | `docs/evidence/2375/R2-role-update.txt` | 200；DB `users.role='repair_technician'`；mock 收到 1 次 role + 1 次 template；`audit_logs` 1 行 `action=user.role_update` | 未调 IAM（仅本地改）→ 测试失败 |
| R3 | 请求 `{role:"merchant_admin"}`；另一用例：另一 tenant 的用户 ID | `PUT /api/users/:id` | `docs/evidence/2375/R3-forbidden.txt` | 白名单外 403 `40301`；跨租户 404 | 提权成功或跨租户可改 → 测试失败 |
| R4 | 前端构建产物 + 列表渲染源 | `/staff` | `docs/evidence/2375/R4-list-role.txt` | 产物/grep 断言：`StaffManagement.jsx` 含 `角色` 列且取 `record.role`；`StaffEdit.jsx` 提交含 `role` | 列表无角色列 → 断言失败 |
| R5 | `bash scripts/verify-2375.sh r5-pending` | docs/evidence/2375/R5-update-pending.txt | exit 0；含 `--- PASS: TestUpdateUserPendingRoleErrors_2375` | PASS | 返回 500 → FAIL | dev |
| R6 | `bash scripts/verify-2375.sh r6-iam-params` | docs/evidence/2375/R6-iam-mapping.txt | exit 0；含 `--- PASS: TestUpdateUserRoleIAMPayload_2375` | PASS | org/role 与预期不符 → FAIL | dev |
| R7 | `grep -q 'RESULT: PASS' docs/evidence/2375/R7-role-column.md` | docs/evidence/2375/R7-role-column.md | exit 0 | PASS | 回执缺失 → exit 1 | dev |

## Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2375.sh r1-role-errors` | docs/evidence/2375/R1-role-errors.txt | exit 0；含 `--- PASS: TestCreateUserRoleErrors_2375` | PASS | 角色失败导致 500/回滚 → FAIL | dev |
| R2 | `bash scripts/verify-2375.sh r2-role-update` | docs/evidence/2375/R2-role-update.txt | exit 0；含 `--- PASS: TestUpdateUserRole_2375`；用例断言 mock 调用次数与 audit 行数 | PASS | 只改本地未调 IAM → FAIL | dev |
| R3 | `bash scripts/verify-2375.sh r3-forbidden` | docs/evidence/2375/R3-forbidden.txt | exit 0；含 `--- PASS: TestUpdateUserRoleForbidden_2375` | PASS | 提权/跨租户通过 → FAIL | dev |
| R4 | `bash scripts/verify-2375.sh r4-list-role` | docs/evidence/2375/R4-list-role.txt | exit 0；含 `built in` 且首行 PROVENANCE | PASS | 构建失败 → exit 1 | dev |
| R5 | `bash scripts/verify-2375.sh r5-pending` | docs/evidence/2375/R5-update-pending.txt | exit 0；含 `--- PASS: TestUpdateUserPendingRoleErrors_2375` | PASS | 返回 500 → FAIL | dev |
| R6 | `bash scripts/verify-2375.sh r6-iam-params` | docs/evidence/2375/R6-iam-mapping.txt | exit 0；含 `--- PASS: TestUpdateUserRoleIAMPayload_2375` | PASS | org/role 与预期不符 → FAIL | dev |
| R7 | `grep -q 'RESULT: PASS' docs/evidence/2375/R7-role-column.md` | docs/evidence/2375/R7-role-column.md | exit 0 | PASS | 回执缺失 → exit 1 | dev |


## Artifact Provenance

产物清单（人工回执为回执正文；机器产物首行含 provenance 头）：

- `docs/evidence/2375/R1-role-errors.txt`
- `docs/evidence/2375/R2-role-update.txt`
- `docs/evidence/2375/R3-forbidden.txt`
- `docs/evidence/2375/R4-list-role.txt`
- `docs/evidence/2375/R5-update-pending.txt`
- `docs/evidence/2375/R6-iam-mapping.txt`
- `docs/evidence/2375/R7-role-column.md`
- `scripts/verify-2375.sh`
- `docs/evidence/2375/evidence.json`

机器产物首行格式（三项缺一不可）：`# build=1.0.<git 短码>`、`# commit=<40 位 sha>`、`# dirty=false`。

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## 计划修订（Rev.3）

| 项 | 裁定 |
|---|---|
| Files | 补 `scripts/verify-2375.sh`、`docs/evidence/2375/evidence.json` |
| R4 命令 | 改为 `bash scripts/verify-2375.sh r4-list-role`（脚本负责写 PROVENANCE 头 + 构建） |
| R5/R6 | 测试已就绪（commit `1a969f83a`），按合同生成证据 |
| R7 | **Owner 人工回执**；Developer 完成 R1–R6 后置 `status:analysis` 交接，不得代签 |

## Dependencies

- 前置：#2367（同文件归属解析修复；避免与 `user_staff.go` 大改冲突，先合入 #2367 再动 `UpdateUser`）。
- 外部：pending 账号的角色真正生效依赖 HiJohns/beaconiam#495（E2）；未部署时 `role_errors` 保留提示。
- 消费方：前端角色展示与告警供 #2372 创建成功流程复用。

### Files（变更文件清单）

- `backend/handlers/user_staff.go`
- `backend/handlers/user_role_assign_2375_test.go`（新增）
- `frontend-pc/src/pages/StaffEdit.jsx`
- `frontend-pc/src/pages/StaffManagement.jsx`
- `docs/evidence/2375/R1-role-errors.txt`
- `docs/evidence/2375/R2-role-update.txt`
- `docs/evidence/2375/R3-forbidden.txt`
- `docs/evidence/2375/R4-list-role.txt`
- `docs/evidence/2375/R5-update-pending.txt`
- `docs/evidence/2375/R6-iam-mapping.txt`
- `docs/evidence/2375/R7-role-column.md`
- `scripts/verify-2375.sh`
- `docs/evidence/2375/evidence.json`

*Model: DeepSeek V4.1 Flash 1017 (deepseek/deepseek-flash)*



