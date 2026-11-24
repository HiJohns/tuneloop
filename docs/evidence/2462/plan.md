## Accepted Plan — #2462（商户直属员工角色落库：users.role 落域 + merchant_members.role 不硬编码）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 创建用户时 `models.User{...}` **未设置 Role** → 新建用户 `users.role` 恒为空串 | `backend/handlers/user_staff.go:304-315` |
| `resolvedRole` 已正确推导（`merchant_direct` → `merchant_staff`），但只用于 IAM 模板与 `site_members.role` | `backend/handlers/user_staff.go:348-370` |
| 直属员工写 `merchant_members.Role = "site_member"`（硬编码），与 `resolvedRole` 冲突 | `backend/handlers/user_staff.go:537-540` |
| 列表对无网点用户优先取 `merchant_members.role` | `backend/handlers/user_staff.go:160-163` |
| 更新接口只要 `req.role` 非空就写 `users.role` 并同步 IAM（即使值未变） | `backend/handlers/user_staff.go:897-1015` |
| PC 编辑页角色初值取列表 `role`（= `merchant_members.role`） | `frontend-pc/src/pages/StaffEdit.jsx:69-75`、`:186-190` |
| 角色展示映射已含 `merchant_staff → 商户直属员工`（PC 列表/直属管理页） | `frontend-pc/src/pages/StaffManagement.jsx:493,504`、`DirectMemberManagement.jsx:17-18` |
| `toIAMRole("merchant_staff") = "STAFF"`；`AllRoleTemplates["merchant_staff"]` 已定义 | `backend/handlers/permission_manage.go:250`、`backend/services/role_templates.go:66` |

结论：角色**推导**正确但**落库**丢失，且编辑路径无条件回写 → 「直属员工」被持久化成网点角色 `site_member`。

### 变更点

1. `backend/handlers/user_staff.go` `CreateUser`：
   - `resolvedRole` 计算完成后（校验通过后）写入 `user.Role = resolvedRole`（随 `users` 行同事务落库）；
   - `merchant_members.Role` 改为 `resolvedRole`（`merchant_direct` 即 `merchant_staff`），不再硬编码 `site_member`；
   - `repair_technician` 与 `site_staff` 同样落 `users.role = resolvedRole`（口径统一，便于列表兜底与导出）。
2. `backend/handlers/user_staff.go` `UpdateUser`：`newRole` 与 `existingUser.Role` 相同时**视为未变更** —— 不写 `users.role`、不写 `site_members/merchant_members.role`、**不调用 IAM 角色同步**（消除「只改职位也触发角色同步」）；仅当值真正变化时才走既有 #2375 同步链。
3. `frontend-pc/src/pages/StaffEdit.jsx`：角色初值加一层域判定——当该用户**无网点归属**（列表无 `site_id`）且 `role` 属于网点角色（`site_member`/`site_admin`）时回填 `merchant_staff`（兼容存量脏数据，不做后端隐式改写）。
4. `scripts/report-merchant-direct-role-drift-2462.sh`：**只读**漂移清单（fixture 可测），列出「`merchant_members.role='site_member'` 且无 `site_members` 行」的直属成员及 `users.role` 现状；`--apply` 必须 `OWNER_APPROVED_MERCHANT_DIRECT_ROLE=1` 才允许执行，未授权 exit 2。
5. 文档：`docs/spec/permissions.md` 补 `users.role` 的域语义（含 `merchant_staff`）；`docs/spec/ui/ui.md` 补「角色列/编辑页预填」口径。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 创建直属员工双向落角色 | `backend/handlers/user_staff.go` | `POST /users`（`user_type=merchant_direct`） | 商户管理员上下文；fake IAM 200 | 后端 | `users.role=merchant_staff`；`merchant_members.role=merchant_staff`；`site_members` 0 行 | R1 |
| R2 创建网点员工落角色 | `backend/handlers/user_staff.go` | `POST /users`（`user_type=site_staff`，带 `site_id`） | 该网点已有成员；fake IAM 200 | 后端 | `users.role=site_member`；`site_members.role=site_member`；两者一致 | R2 |
| R3 创建维修师傅落角色 | `backend/handlers/user_staff.go` | `POST /users`（`user_type=repair_technician`） | fake IAM 200 | 后端 | `users.role=repair_technician`；`technician_profiles` 1 行；无 `merchant_members` 误写 | R3 |
| R4 列表角色列（直属） | `backend/handlers/user_staff.go` | `GET /users`（商户管理员列表） | 直属用户：无 `site_members`，`merchant_members.role=merchant_staff` | 后端 | 该行 `role=merchant_staff`、`site_name` 空；与 `users.role` 一致（`ROLE_MATCH=1`） | R4 |
| R5 编辑仅改职位不触发角色同步 | `backend/handlers/user_staff.go` | `PUT /users/:id` | body 携带与现值相同的 `role` + 新 `position` | 后端 | `users.role` 不变；fake IAM `UpdateUserRoleInOrg` 调用 **0** 次；`role_errors` 为空 | R5 |
| R6 编辑显式改角色仍同步 | `backend/handlers/user_staff.go` | `PUT /users/:id` | `role` 由 `site_member` 改为 `merchant_staff` | 后端 | `users.role=merchant_staff`；`merchant_members.role=merchant_staff`；IAM 角色同步 **1** 次且映射 `STAFF` | R6 |
| R7 存量漂移清单（只读） | `scripts/report-merchant-direct-role-drift-2462.sh`、`scripts/tests/test_merchant_direct_role_drift_2462.sh` | 脚本 fixture | 3 行 fixture（2 行漂移 + 1 行正常）；另做未授权 `--apply` | 后端 | 输出恰好 2 行 `ACTION=align`、1 行 `ACTION=none`；`READONLY=1`；未授权 `--apply` → exit 2 且 `DB_WRITES=0` | R7 |
| R8 编辑页角色预填（存量安全） | `frontend-pc/src/pages/StaffEdit.jsx` | PC 编辑员工页 | ①无网点 + `role=site_member`（存量脏行）②正常网点员工 | PC | ①预填 `merchant_staff`（`PREFILL_LEGACY=merchant_staff`）②保持 `site_member`（`PREFILL_NORMAL=site_member`） | R8 |
| R9 构建与定向回归 | `backend`、`frontend-pc` | `go build ./...`、定向测试、`npm run build` | 修后树 | 后端 + PC | build 0；定向测试 0 FAIL；PC build 0 | R9 |

## Input Truth & Tool Preflight

- **Source of truth**：`resolvedRole` 推导（`user_staff.go:348-370`）与 `toIAMRole`/`AllRoleTemplates` 为角色域真值；`merchant_members` 记录直属域、`site_members` 记录网点域（#2375 单一主角色约定）。
- **差异核对（预生产实况）**：`测试员工2372`（`users.id=1dac4ae3…`）→ `merchant_members.role=site_member`、`site_members` 0 行、`users.role=site_member`；IAM `cadenza` 组织 `role=STAFF`、`functional_roles=["merchant_staff","site_member"]`。即**三方不一致**：域记录、users.role、IAM 模板。
- **损坏输入拒绝**：`user_type` 不在白名单 → 40001；`role` 不在 `{site_member, site_admin, merchant_staff, repair_technician}` → 40301；`site_staff` 缺 `site_id` → 40001（既有）；编辑时 `role` 空白 → 40001。
- **工具盲区**：PC 端无测试运行器（`frontend-pc/package.json` 仅 dev/build/lint/preview）→ R8 用源码结构断言（`evidence_type: static`，Owner 豁免 D5）；`users.role` 无 DB 约束，需测试逐列断言。
- **影响面预检**：`grep -rn "user.Role ==" backend --include=*.go` → 无按 `users.role` 字符串分支的权限判定（权限以 JWT claim 为准）；列表兜底 `item["role"] = user.Role` 与导出是唯一消费点 → 写入 `merchant_staff` 不会改变鉴权行为。
- **跨单依赖**：R6 的 IAM 角色同步真实成功取决于 #2461（JSON body 契约）；本单用 fake IAM 只断言「调用次数与映射」，不把 #2461 的缺陷计入本单验收。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | `users.role` 语义 = **域角色**（`site_member`/`site_admin`/`merchant_staff`/`repair_technician`），创建时按 `resolvedRole` 写入 | Architect | 本计划锁定 | 阻塞实现 |
| D2 | 直属员工在两处必须同为 `merchant_staff`：`users.role` 与 `merchant_members.role` | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 角色未变化时不得写角色列、不得调 IAM 角色同步（幂等） | Architect | 本计划锁定 | 阻塞实现 |
| D4 | 存量数据：只出**只读**清单（R7），回填需 Owner 明确授权，禁止静默改名/回填 | Owner / ops | `OWNER_APPROVED_MERCHANT_DIRECT_ROLE=1` 才允许 `--apply` | 未授权 → 作为 `[Ops-Followup of #2462]` 跟踪 Issue（`waitForConfirm`，无 `status:*`） |
| D5 | R8 为纯前端预填接线，`frontend-pc` 无测试运行器 → `evidence_type: static` 源码结构断言 + 构建，**Owner 豁免**（Architect 授予） | Architect | 本计划锁定 | 若审计要求 render → 另立前端测试基建 Issue |
| D6 | 前端预填的 legacy 映射只做**显示/初始值**修正，不回写后端、不改列表返回 | Architect | 本计划锁定 | 阻塞实现 |
| D7 | IAM 客户端契约（JSON body）归 #2461；本单不改 `iam_client.go` | Architect | 本计划锁定 | 若在本单改 IAM 客户端 → 回 Architect |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R8 | static | 源码结构断言（域判定优先于历史 role 的预填分支），Owner 豁免见 D5 |

## Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/permissions.md` | 仅在 §#2129 说明 `merchant_staff` 角色模板与 cus_perm | 在角色/人员章节补一段「`users.role` 是域角色落库字段：`merchant_direct` → `merchant_staff`，`site_staff` → `site_member`/`site_admin`，`repair_technician` → `repair_technician`；`merchant_members.role` 与之一致」 | 本单把 `users.role` 从「恒空」改为「按域写入」，属权限-人员矩阵语义变更 |
| `docs/spec/ui/ui.md` | §员工列表/编辑说明只写「角色下拉含四项」 | 补「角色列对直属员工显示 `商户直属员工`；编辑页角色初值：无网点归属且历史值为网点角色的存量行回填 `商户直属员工`」 | 前端预填规则变更需与实现一致 |

## Out of Scope

- IAM 客户端 `UpdateUserRoleInOrg*` 的 query→JSON body 契约修复与静默吞错收口 → #2461。
- `role_errors` 在前端的展示样式（warning 弹窗文案）改造。
- 存量数据回填执行（部署侧，Owner 授权后 ops 执行；本单只交付只读清单脚本）。
- 员工上下文/组织名展示口径（#2455）、网点名唯一约束（#2455）。
- `merchant_members` 表结构/约束变更（无 migration）。
- 全量 `go test ./...`（由独立 Issue 的 gate 承担）。

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|---|---|---|---|
| R1 | 商户管理员 actor（`tenant_id=T`、`org=T`）；body `{user_type:"merchant_direct", name:"测试直属", username:"u-direct-2462"}`；fake IAM 200 | `POST /users` | `docs/evidence/2462/R1-create-direct.txt` |
| R2 | 网点 S（`org_id=O`）+ 既有 1 名成员；body `{user_type:"site_staff", site_id:S, username:"u-site-2462"}` | `POST /users` | `docs/evidence/2462/R2-create-site.txt` |
| R3 | body `{user_type:"repair_technician", username:"u-tech-2462"}` | `POST /users` | `docs/evidence/2462/R3-create-tech.txt` |
| R4 | 直属用户：`users.role=merchant_staff`、`merchant_members.role=merchant_staff`、无 `site_members` | `GET /users` | `docs/evidence/2462/R4-list-role.txt` |
| R5 | 直属用户现值 `role=merchant_staff`；body `{role:"merchant_staff", position:"首席"}；fake IAM 计数器 | `PUT /users/:id` | `docs/evidence/2462/R5-noop-role.txt` |
| R6 | 网点用户现值 `site_member`；body `{role:"merchant_staff"}` | `PUT /users/:id` | `docs/evidence/2462/R6-role-change.txt` |
| R7 | fixture 3 行：2 行（`mm.role=site_member` + 无 site_members）、1 行（`mm.role=merchant_staff`）；另做未授权 `--apply` | `scripts/report-merchant-direct-role-drift-2462.sh --report` | `docs/evidence/2462/R7-drift.txt` |
| R8 | 修后 `frontend-pc/src/pages/StaffEdit.jsx`（两分支：无网点 legacy / 正常网点） | PC 编辑页预填分支 | `docs/evidence/2462/R8-prefill.txt` |
| R9 | 修后 backend + frontend-pc | `go build ./...` + 定向测试 + `npm run build` | `docs/evidence/2462/R9-build.txt` |

### Assertions

- R1：`USERS_ROLE=merchant_staff`、`MM_ROLE=merchant_staff`、`SITE_MEMBERS_ROWS=0`；负例：`USERS_ROLE=`（空）或 `MM_ROLE=site_member` → FAIL。
- R2：`USERS_ROLE=site_member`、`SM_ROLE=site_member`、`ROLE_MATCH=1`；负例：`USERS_ROLE=` 或两者不等 → FAIL。
- R3：`USERS_ROLE=repair_technician`、`TECH_PROFILES=1`、`MM_ROWS=0`；负例：`USERS_ROLE=site_member` → FAIL。
- R4：`LIST_ROLE=merchant_staff`、`ROLE_MATCH=1`、`SITE_NAME_EMPTY=1`；负例：`LIST_ROLE=site_member` → FAIL。
- R5：`USERS_ROLE_UNCHANGED=1`、`IAM_ROLE_CALLS=0`、`ROLE_ERRORS=0`、`POSITION=首席`；负例：`IAM_ROLE_CALLS=1`（无变化仍同步）→ FAIL。
- R6：`USERS_ROLE=merchant_staff`、`MM_ROLE=merchant_staff`、`IAM_ROLE_CALLS=1`、`IAM_ROLE_VALUE=STAFF`；负例：`IAM_ROLE_CALLS=0`（漏同步）→ FAIL。
- R7：`ALIGN_ROWS=2`、`NONE_ROWS=1`、`READONLY=1`、`UNAUTHORIZED_APPLY_EXIT=2`、`DB_WRITES=0`；负例：未授权 apply 成功或出现 `UPDATE` 语句 → FAIL。
- R8：`PREFILL_LEGACY=merchant_staff`、`PREFILL_NORMAL=site_member`、`DOMAIN_GUARD=1`；负例：legacy 仍回填 `site_member` → FAIL。
- R9：`BUILD_EXIT=0`、`TESTS_FAIL=0`、`PC_BUILD=0`；负例：任一非 0 → FAIL。
- 通用 provenance：每个 artifact 首行 `# build=1.0.<fix> # commit=<sha> # dirty=false`。
- 负例（跨行）：任何 artifact 出现 `merchant_members.role=site_member`（直属）或 `users.role=` 空 → FAIL。

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2462/R1-create-direct.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2462/R2-create-site.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2462/R3-create-tech.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2462/R4-list-role.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2462/R5-noop-role.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2462/R6-role-change.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2462/R7-drift.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2462/R8-prefill.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2462/R9-build.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| symptom | trigger | root cause | test blind spot | same-class paths | fix layer | red-green | prevention |
|---|---|---|---|---|---|---|---|
| 直属员工列表显示「成员」 | 创建商户直属员工 | `merchant_members.role` 硬编码 `site_member` | 无「直属域角色」断言 | 邀请直达（`staff_invite.go:289` 已用 merchant_staff）、`member_invite_2114.go:401` | handler 创建路径 | R1/R4 先写断言（旧实现 mm.role=site_member → FAIL）再改 | D2 双写口径 |
| 新建用户 `users.role` 恒空 | 任意后台建号 | `models.User{}` 未赋 Role | 测试只断言 id/iam_sub | 维修师傅/网点员工创建 | handler 创建路径 | R1–R3 红绿 | D1 |
| 只改职位也触发 IAM 角色同步并失败 | 编辑直属员工改职位 | `newRole` 非空即同步，未比较现值 | 无「无变化」用例 | 网点成员/商户成员改角色（#2375） | handler 更新路径 | R5 红绿 | D3 |
| 编辑后直属员工变成网点角色 | 编辑页预填 `site_member` 后保存 | 预填源=历史脏值 + 后端无条件覆盖 | 无 legacy fixture | `DirectMemberManagement` 编辑入口 | 前端预填 + 后端域写入 | R8 + R1 红绿 | D6 |
| 角色在三方不一致 | 存量脏行 | 无一致性清单 | 无漂移用例 | `users.role` 导出/统计 | 只读工具 | R7 红绿 | D4 |
| 修复师傅 users.role 仍空 | 创建维修师傅 | 同一缺省赋值 | 同上 | 网点首用户 site_admin | handler 创建路径 | R3 红绿 | D1 |
| 列表回退到 `users.role` 出现空角色 | 无 `merchant_members` 行 | `users.role` 未写 | 无兜底断言 | 批量导入/邀请直达 | handler | R4 红绿 | D1 |
| 存量回填被静默执行 | 运维脚本误跑 | 无授权闸门 | 无未授权用例 | #2457 同类漂移工具 | 脚本 | R7 `UNAUTHORIZED_APPLY_EXIT=2` | D4 |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2462.sh r1` | `docs/evidence/2462/R1-create-direct.txt` | exit 0；grep `USERS_ROLE=merchant_staff`、`MM_ROLE=merchant_staff`、`SITE_MEMBERS_ROWS=0` | PASS | `USERS_ROLE=` 空或 `MM_ROLE=site_member` → FAIL | dev |
| R2 | `bash scripts/verify-2462.sh r2` | `docs/evidence/2462/R2-create-site.txt` | exit 0；grep `USERS_ROLE=site_member`、`ROLE_MATCH=1` | PASS | 两者不等 → FAIL | dev |
| R3 | `bash scripts/verify-2462.sh r3` | `docs/evidence/2462/R3-create-tech.txt` | exit 0；grep `USERS_ROLE=repair_technician`、`TECH_PROFILES=1` | PASS | `USERS_ROLE=site_member` → FAIL | dev |
| R4 | `bash scripts/verify-2462.sh r4` | `docs/evidence/2462/R4-list-role.txt` | exit 0；grep `LIST_ROLE=merchant_staff`、`ROLE_MATCH=1` | PASS | `LIST_ROLE=site_member` → FAIL | dev |
| R5 | `bash scripts/verify-2462.sh r5` | `docs/evidence/2462/R5-noop-role.txt` | exit 0；grep `IAM_ROLE_CALLS=0`、`ROLE_ERRORS=0`、`USERS_ROLE_UNCHANGED=1` | PASS | `IAM_ROLE_CALLS=1` → FAIL | dev |
| R6 | `bash scripts/verify-2462.sh r6` | `docs/evidence/2462/R6-role-change.txt` | exit 0；grep `IAM_ROLE_CALLS=1`、`IAM_ROLE_VALUE=STAFF`、`USERS_ROLE=merchant_staff` | PASS | `IAM_ROLE_CALLS=0` → FAIL | dev |
| R7 | `bash scripts/verify-2462.sh r7` | `docs/evidence/2462/R7-drift.txt` | exit 0；grep `ALIGN_ROWS=2`、`READONLY=1`、`UNAUTHORIZED_APPLY_EXIT=2` | PASS | `READONLY=0` 或未授权 apply 成功 → FAIL | dev |
| R8 | `bash scripts/verify-2462.sh r8` | `docs/evidence/2462/R8-prefill.txt` | exit 0；grep `PREFILL_LEGACY=merchant_staff`、`PREFILL_NORMAL=site_member` | PASS | legacy 仍 `site_member` → FAIL | dev |
| R9 | `bash scripts/verify-2462.sh r9` | `docs/evidence/2462/R9-build.txt` | exit 0；grep `BUILD_EXIT=0`、`TESTS_FAIL=0`、`PC_BUILD=0` | PASS | 任一非 0 → FAIL | dev |

### Workload

- Size: M
- Basis: 1 个后端 handler 文件（创建+更新两处）+ 1 个后端测试文件 + 1 个 PC 页面 + 1 个只读漂移脚本 + 1 个脚本 fixture 测试 + 1 个 verify 脚本 + 2 篇文档 + 9 条证据行；无 migration、无 IAM 客户端改动
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/user_staff.go`
- `backend/handlers/user_role_domain_2462_test.go`
- `frontend-pc/src/pages/StaffEdit.jsx`
- `scripts/report-merchant-direct-role-drift-2462.sh`
- `scripts/tests/test_merchant_direct_role_drift_2462.sh`
- `scripts/verify-2462.sh`
- `docs/spec/permissions.md`
- `docs/spec/ui/ui.md`
- `docs/evidence/2462/plan.md`
- `docs/evidence/2462/R1-create-direct.txt`
- `docs/evidence/2462/R2-create-site.txt`
- `docs/evidence/2462/R3-create-tech.txt`
- `docs/evidence/2462/R4-list-role.txt`
- `docs/evidence/2462/R5-noop-role.txt`
- `docs/evidence/2462/R6-role-change.txt`
- `docs/evidence/2462/R7-drift.txt`
- `docs/evidence/2462/R8-prefill.txt`
- `docs/evidence/2462/R9-build.txt`
- `docs/evidence/2462/evidence.json`

## Dependencies

- 无代码前置。**关联 #2461**（IAM `UpdateUserRoleInOrg` query→JSON body）：本单 R6 仅用 fake IAM 断言「调用次数 + 角色映射」，不依赖真实 IAM 成功；预生产实测（issue 期望 6）属部署侧，须 #2461 落地后执行。
- 关联 #2375（`role_errors` 契约，本单沿用）、#2129（`merchant_staff` 模板）、#2455（显示口径，不阻塞）。
- 部署侧：存量回填清单执行需 Owner 授权 → 关闭时以 `[Ops-Followup of #2462]`（`waitForConfirm`，无 `status:*`）承载。

*Model: DeepSeek V4.1 Flash 1017*

