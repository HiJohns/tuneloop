## Accepted Plan — #2647（#2626 Round 2 增量验收：R7 `users.roles` jsonb 写形状 + R8 改角色同步镜像 + R9 镜像来源可判定 + R2 写侧 6 点）

> **来源**：#2626 Round 2 审计（REJECTED，5 项失败）中的**新增验收面**在 Owner 2026-11-28「计划冻结」后另立本单。#2626 按其已接受合同（Rev.4，R1–R6）验收，本单不改其 Accepted Plan / `docs/evidence/2626/*` / `scripts/verify-2626.sh`（历史生成器）。
> **本单四件事**（Issue 正文 + 审计「下一轮需要补的矩阵行」逐条对齐）：
> ① **R2**：写侧收口改为「**6 个写点**（5 个文件内的 5 点 + `user_staff.go` 改角色点）都经 `DeriveRoles` 同时写 `role` + `roles`」，断言 `ROLE_ASSIGN_WITHOUT_MIRROR=0`，并补**变量形态**负例（`R2_NEGATIVE_CONTROL_VAR=1`）；审计新发现的 3 个建号初始化写点进 allowlist（带理由与 ref）。
> ② **R7**：`users.roles` 的 map 写入统一走显式 jsonb 序列化（`?::jsonb`），运行期断言 `jsonb_typeof(roles)='array'`；负例：裸 `[]string` 写入必须失败。
> ③ **R8**：`PUT /api/users/:id` 改角色后 `users.roles` 与 IAM `functional_roles`（ADD-only 结果集）一致，主角色同步；读回走镜像优先。
> ④ **R9**：镜像只能由 `DeriveRoles` 产出（静态：无第二处赋值来源）；`SyncUsers` 在取不到 `functional_roles` 时**不得降级覆盖非空镜像**（`SYNC_USES_FUNCTIONAL_ROLES=1` 不可得 → 采 `SYNC_SKIPS_NONEMPTY_MIRROR_OVERWRITE=1`，审计二选一）。
> **不改隔离/权威口径**：#2626 D1–D8 不变；本单只补镜像写入形态、改角色同步与来源判定。

## 语义变更：被翻转的既有断言

| 位置 | 旧断言 | 新预期 | 理由 |
|---|---|---|---|
| #2626 R2（`WRITE_SITES=5` / `MIRROR_WRITE_DELEGATED=1` / `ALLOWLIST_HITS=0`） | 文件级 `derive_calls>0` 即算「写侧已收口」；`user_staff.go` 改角色点不写 `roles` 仍 PASS | #2647 R2：按**写点**计数 `WRITE_SITES=6`，且每个 `updates["role"] =` 必须伴随 `updates["roles"] =`（`ROLE_ASSIGN_WITHOUT_MIRROR=0`）；变量形态负例必须命中（`R2_NEGATIVE_CONTROL_VAR=1`）；审计新发现的 3 个建号初始化点显式 allowlist（`ALLOWLIST_ENTRIES=3`、`ALLOWLIST_UNLINKED=0`） | 文件级自证式断言不覆盖变量写（Round 2 审计失败项 1/5） |
| #2626 R4（`PC_USES_ROLES_SET=1` / `ROLES_SOURCE=iam`） | 只证明「纯函数集合优先」与「来源不是页面自造」 | 由 #2647 R8 补「写 → 读一致」（改角色后读回 `roles[]` 含新角色且主角色正确），R4 本身不重定义 | 镜像过期时读侧集合优先会显示旧角色（Round 2 审计语义挑战 2） |

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R2 写侧 6 点收口（含改角色点）+ 变量形态负例 | `scripts/verify-2647.sh`（`r2`）、`backend/handlers/user_staff.go`、`backend/handlers/iam_callback.go`、`backend/handlers/merchant_member.go`、`backend/handlers/merchant.go`、`backend/handlers/iam_proxy.go` | `bash scripts/verify-2647.sh r2` | 修后树（静态扫描） | 后端（静态 + 合成负例） | `WRITE_SITES=6`、`WRITE_INLINE_LITERAL=0`、`ROLE_ASSIGN_WITHOUT_MIRROR=0`、`R2_NEGATIVE_CONTROL_VAR=1`、`ALLOWLIST_ENTRIES=3`、`ALLOWLIST_UNLINKED=0` | R2 |
| R7 `users.roles` jsonb 写入形态 | `backend/handlers/user_roles_jsonb.go`（新 helper）、`backend/handlers/user_roles_mirror_2647_test.go` | `bash scripts/verify-2647.sh r7` | 改角色/建商户/同步三条写路径经 helper 落库 | 后端（一次性 PG） | `USERS_MIRROR_JSONB_EXPR=1`（4 个写点均走 `?::jsonb`）、`USERS_MIRROR_RAW_SLICE=0`、`JSONB_TYPE_IS_ARRAY=1`（`jsonb_typeof(roles)='array'` 且非 NULL）、`RAW_SLICE_WRITE_ERROR=1`（负例：裸 `[]string` 在 simple protocol 下报错，证明断言器不是盲的） | R7 |
| R8 改角色同步镜像（写 → 读一致） | `backend/handlers/user_staff.go`（`UpdateUser`）、`backend/handlers/user_roles_mirror_2647_test.go` | `PUT /api/users/:id`（`StaffHandler.UpdateUser`） | 目标用户 `role=site_member`、`roles=["site_member"]`；请求 `{"role":"site_admin"}`；fake IAM 成功 | 后端（一次性 PG + fake IAM） | HTTP 200；`users.role=site_admin`；`users.roles` 含 `site_admin` 与旧 `site_member`（IAM ADD-only 结果集）；读回（`GetStaffUser`）`roles[]` 含 `site_admin`（镜像优先）；`ROLE_ASSIGN_WITHOUT_MIRROR=0` | R8 |
| R9 镜像来源可判定 + 同步不降级 | `backend/handlers/iam_proxy.go`（`SyncUsers`）、`scripts/verify-2647.sh`（`r9`）、`backend/handlers/user_roles_mirror_2647_test.go` | `POST /api/iam/users/sync`（`IAMProxyHandler.SyncUsers`） | IAM 列表返回关系角色 `STAFF` 且**无** `functional_roles`；本地用户镜像非空（`["repair_technician","site_member"]`） | 后端（一次性 PG + fake IAM） | 同步后镜像**不降级**（仍含 `repair_technician`），`SYNC_SKIPS_NONEMPTY_MIRROR_OVERWRITE=1`、`SYNC_DEGRADES_NONEMPTY_MIRROR=0`；静态：`MIRROR_SOURCE_DERIVE_ONLY=1`（`services.DeriveRoles` 是唯一镜像来源）、`WRITE_SIDE_SECOND_ASSIGNMENT=0` | R9 |

## Input Truth & Tool Preflight

- **Source of truth**：
  - 权威与镜像口径 = #2626 Decision Lock D1/D2/D4（IAM `user_org_relations` 的 `role` + `functional_roles[]` 权威；`users.roles` 为多值镜像；`users.role` 为派生主角色）。
  - 派生唯一实现 = `backend/services/role_derive.go` 的 `DeriveRoles(orgLevel, relationRole, functionalRoles) DerivedRoles`（`Primary` / `All`，D4 优先级）。
  - 改角色现状 = `backend/handlers/user_staff.go` 的 `UpdateUser`：`roleChanged` 分支只写 `updates["role"] = newRole`，随后调 IAM `UpdateUserRoleInOrgWithToken` + `AssignRoleTemplateToUserWithToken`（ADD-only）→ `users.roles` 不更新（Round 2 失败项 1，可复现：读回 `resolveInitialRole` 得旧角色）。
  - jsonb 写约定 = 仓内既有 `siteMemberRolesExpr`（`backend/handlers/site_member.go`）+ `gorm.Expr("?::jsonb", …)`；GORM v1.31.1 map 更新不经 `field.Serializer`，裸 `[]string` 在 `TUNELOOP_DB_SIMPLE_PROTOCOL=1` 下报 `42804`（Round 2 失败项 3）。
  - 同步现状 = `backend/handlers/iam_proxy.go` 的 `SyncUsers`：`DeriveRoles(orgLevel, matchedRole, nil)`（`functional_roles` 取不到，列表接口不返回）→ 直接覆盖非空镜像（Round 2 失败项 2）。
  - 写点盘点（本单口径）：**在册 6 个写点** = `iam_callback.go`（IAM payload 镜像）/ `merchant_member.go`（建缓存）/ `user_staff.go` CreateUser / `user_staff.go` UpdateUser 改角色 / `merchant.go`（商户管理员建缓存）/ `iam_proxy.go` SyncUsers；**新发现 3 个建号初始化点**（不在 6 内）= `site.go` CreateSite 的 `localUser`（管理员 `site_admin` / 成员 `site_member` 两处）+ `site_member_common.go` `addMembersCore` 的 `localUser`（`site_member`）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `grep -n 'updates\["role"\] =' backend/handlers/user_staff.go` → 修前 1 处且无相邻 `updates["roles"]`；
  - `grep -c '?::jsonb' backend/handlers/{iam_callback,merchant,iam_proxy,user_staff}.go` → 修前 0（除 site_members 相关外）；
  - `grep -n 'DeriveRoles(.*nil)' backend/handlers/iam_proxy.go` → 修前 ≥1；
  - `git merge-base --is-ancestor origin/issue-2626 origin/develop` → 前置必须为 true。
- **损坏输入 / 边界拒绝规则**：
  - 改角色时 `existingUser.Roles` 为空（存量未回填）→ 镜像 = `DeriveRoles(level, iamRole, [newRole])`（不得写空集合，也不得写 `null`）；
  - IAM 角色同步失败（`roleErrors` 非空）→ 本地镜像仍按「已命令的 ADD-only 结果」写并保留 `role_errors`，**不得**静默跳过镜像写入；
  - 同步路径取不到 `functional_roles` 且本地镜像非空 → **不覆盖**（保留旧镜像），只允许在镜像为空时按关系角色回填；
  - 镜像值为 `nil` 时序列化为 `[]`（禁止 `null`）。
- **工具盲区**：
  - jsonb 形态必须在**运行期**（`jsonb_typeof`）验证，静态 grep 只能证明写法存在；
  - `TUNELOOP_DB_SIMPLE_PROTOCOL=1` 是负例生效的前提（simple protocol 下裸 `[]string` 报 `42804`）；若测试环境未启用该协议，负例断言改为 `JSONB_TYPE_OF_RAW_SLICE=array=0`；
  - 静态「第二处赋值」扫描只能覆盖 handler 目录的 map/struct 赋值形态，跨包写入不在扫描面（须在 Work Summary 记录人工复核结论）；
  - `frontend-pc` 读侧不在本单（#2626 R4 已覆盖），本单只验服务端读回。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | `WRITE_SITES` 口径 = **写点**（语句级），不是文件级：6 = 5 个在册文件内的 5 个既有写点 + `user_staff.go` 改角色点。审计新发现的 3 个建号初始化写点（`site.go`×2 + `site_member_common.go`×1）进 allowlist（`ALLOWLIST_ENTRIES=3`），理由「建号初始化，值确定且读侧有回退链；镜像由后续 sync/改角色刷新」，ref 指向 #2626 D1/D7 | Architect | 本计划锁定 | 若审计要求一并收口 → 回 Architect（会变成 9 个写点，需重估 size） |
| D2 | R7 序列化实现 = 新增 `backend/handlers/user_roles_jsonb.go` 的 `userRolesExpr([]string) clause.Expr`（`gorm.Expr("?::jsonb", json)`，`nil → []`），4 个写点全部替换；不复制 `siteMemberRolesExpr` 的第二份逻辑（同语义、不同表） | Architect | 本计划锁定 | 若要求共用同一个 helper 覆盖 `site_members` → 回 Architect |
| D3 | R8 镜像来源 = **IAM ADD-only 结果集**（`UpdateUserRoleInOrgWithToken` + `AssignRoleTemplateToUserWithToken(newRole)`，无 unassign API）→ 镜像 = 本地既有镜像 ∪ {newRole}，主角色由 `DeriveRoles` 产出；不新增 IAM 读回依赖（列表/详情接口当前不返回 `functional_roles`，跨仓改动另立 Issue） | Architect | 本计划锁定 | 若要求「以 IAM 实时读回为唯一来源」→ 阻塞，需 beaconiam 先补字段 |
| D4 | R9 采 `SYNC_SKIPS_NONEMPTY_MIRROR_OVERWRITE=1`（审计二选一中的第二项）：取不到 `functional_roles` 时不得用关系角色覆盖非空镜像；`SYNC_USES_FUNCTIONAL_ROLES=1` 需 beaconiam 列表接口补字段，不在本单 | Architect | 本计划锁定 | 若 Owner 要求第一项 → 跨仓前置，回 Architect |
| D5 | 读侧回退链按**已实现口径**登记（`roles → role → site_role → merchant_role`），#2626 D1 文本里的「IAM 实时查询」段**不在本单**（同 D3 理由）；本单只在 Work Summary 与本节登记该偏离，不改代码 | Architect | 本计划锁定 | 若审计要求实现 IAM 实时查询 → 回 Architect（跨仓前置） |
| D6 | 执行前置：#2626 **已合入 `origin/develop`**；未满足则 `/work` HOLD（禁止在 #2626 分支上叠加、禁止 cherry-pick） | Architect | 本计划锁定 | HOLD（不阻塞批次其余任务） |
| D7 | #2626 的 Accepted Plan / 证据 / `scripts/verify-2626.sh` **冻结**：本单以自己的 R2/R7/R8/R9 承接修正口径 | Architect | 本计划锁定 | 若要求回改 #2626 产物 → 回 Architect |

## Out of Scope

- `site.go` / `site_member_common.go` 的 3 个建号初始化写点收口（allowlist，见 D1）。
- IAM 列表/详情接口补 `functional_roles`（跨仓；`SYNC_USES_FUNCTIONAL_ROLES` 由此解冻）。
- 读侧 IAM 实时查询回退段（D5 登记，不改代码）。
- `users.role` 删列（#2626 D7 三步走）。
- 前端读侧改动（#2626 R4 已覆盖）。
- `users.tenant_id` / 角色历史脏值回填（#2527 / #2527 族）。
- #2626 的 Accepted Plan、`docs/evidence/2626/*`、`scripts/verify-2626.sh`（冻结）。
- 全量 `go test ./...` 作为合同行（由批次 gate / P5 baseline 差分承担）。
- 预生产真机复验（`[Ops-Followup of #2647]`）。

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|---|---|---|---|
| R2 | 修后工作树 + 合成负例文件（`updates["role"] = roleVar` 变量形态、`m["roles"] = x`） | `bash scripts/verify-2647.sh r2`（写点扫描 + allowlist 校验 + 负例注入） | `docs/evidence/2647/R2-write-sites.txt` |
| R7 | 一次性 PG + 目标用户（`roles=["site_member"]`）→ 改角色 `site_admin`；另一用例注入裸 `[]string` 写入作负例 | `PUT /api/users/:id`（`StaffHandler.UpdateUser`）+ helper 直调 | `docs/evidence/2647/R7-users-roles-jsonb.txt` |
| R8 | 同上（`role=site_member`、`roles=["site_member"]` → 请求 `{"role":"site_admin"}`；fake IAM 成功） | `PUT /api/users/:id` + 读回 `GetStaffUser` | `docs/evidence/2647/R8-role-change-mirror.txt` |
| R9 | fake IAM 列表：关系角色 `STAFF`、无 `functional_roles`；本地镜像 `["repair_technician","site_member"]` | `POST /api/iam/users/sync`（`SyncUsers`）+ 静态来源扫描 | `docs/evidence/2647/R9-mirror-source.txt` |

### Assertions

- R2：`WRITE_SITES=6`、`WRITE_INLINE_LITERAL=0`、`ROLE_ASSIGN_WITHOUT_MIRROR=0`、`R2_NEGATIVE_CONTROL_VAR=1`、`ALLOWLIST_ENTRIES=3`、`ALLOWLIST_UNLINKED=0`。
  - 负例：把 `updates["roles"]` 从改角色分支删掉 → `ROLE_ASSIGN_WITHOUT_MIRROR>0` 必须 FAIL；变量形态负例检测不到（`R2_NEGATIVE_CONTROL_VAR=0`）→ FAIL。
- R7：`USERS_MIRROR_JSONB_EXPR=1`、`USERS_MIRROR_RAW_SLICE=0`、`JSONB_TYPE_IS_ARRAY=1`、`JSONB_IS_NULL=0`、`RAW_SLICE_WRITE_ERROR=1`。
  - 负例：任一写点回到 `updates["roles"] = derived.All`（裸切片）→ `USERS_MIRROR_RAW_SLICE>0` 且 `RAW_SLICE_WRITE_ERROR=0` → FAIL；`jsonb_typeof` 非 `array`（字符串/对象/NULL）→ FAIL。
- R8：`HTTP=200`、`ROLE_ASSIGN_WITHOUT_MIRROR=0`、`MIRROR_PRIMARY=site_admin`、`MIRROR_ALL_CONTAINS_SITE_ADMIN=1`、`MIRROR_ALL_CONTAINS_OLD=1`、`READBACK_PREFERS_MIRROR=1`、`TEST_EXIT=0`。
  - 负例：只写 `role` 不写 `roles` → `MIRROR_ALL_CONTAINS_SITE_ADMIN=0` → FAIL；读回仍返回旧角色（集合未刷新）→ `READBACK_PREFERS_MIRROR=0` → FAIL。
- R9：`MIRROR_SOURCE_DERIVE_ONLY=1`、`WRITE_SIDE_SECOND_ASSIGNMENT=0`、`SYNC_SKIPS_NONEMPTY_MIRROR_OVERWRITE=1`、`SYNC_DEGRADES_NONEMPTY_MIRROR=0`。
  - 负例：同步用关系角色覆盖非空镜像 → `SYNC_DEGRADES_NONEMPTY_MIRROR=1` → FAIL；写侧出现 `DeriveRoles` 之外的第二处镜像赋值（如内联 `[]string{...}`）→ `WRITE_SIDE_SECOND_ASSIGNMENT>0` → FAIL。
- 通用 provenance：每个 artifact 首行 `# build=1.0.<fix短码> # commit=<fix完整哈希> # dirty=false`。
- 负例（跨行）：任何 artifact 出现「镜像为 null 视为空」「用关系角色覆盖非空镜像」的语义 → FAIL。

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2647/R2-write-sites.txt` | `# build=1.0.<fix> # commit=<fix> # dirty=false` |
| `docs/evidence/2647/R7-users-roles-jsonb.txt` | `# build=1.0.<fix> # commit=<fix> # dirty=false` |
| `docs/evidence/2647/R8-role-change-mirror.txt` | `# build=1.0.<fix> # commit=<fix> # dirty=false` |
| `docs/evidence/2647/R9-mirror-source.txt` | `# build=1.0.<fix> # commit=<fix> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| symptom | trigger | root cause | test blind spot | same-class paths | fix layer | red-green | prevention |
|---|---|---|---|---|---|---|---|
| 管理员改角色后编辑页仍显示旧角色 | `UpdateUser` 只写 `users.role`，不写 `users.roles` | 改角色点未纳入收口，文件级扫描算作已收口 | 文件级 `derive_calls>0`；无「写→读」一致用例 | `site.go` / `site_member_common.go` 建号初始化 | handler 写侧 + 运行期用例 | R8 先红（读回旧角色）后绿 | R2 按写点计数 + `ROLE_ASSIGN_WITHOUT_MIRROR=0` |
| `users.roles` map 写入报 `42804` 或写成字符串 | 裸 `[]string` 塞进 `map[string]interface{}` | GORM map 更新不经 serializer | 静态 grep 只证写法存在，不验落库形态 | `merchant.go` / `iam_callback.go` / `iam_proxy.go` | 统一 helper（`?::jsonb`） | R7 `JSONB_TYPE_IS_ARRAY` 红绿 | 单一 helper + 运行期 `jsonb_typeof` 断言 |
| 同步后多角色丢失（`repair_technician` 消失） | 列表接口无 `functional_roles`，同步以关系角色覆盖 | 输入缺失时仍整体覆盖 | 无「非空镜像 + 未知 functional_roles」用例 | 其它 IAM 拉取路径（回调/代理） | 同步写侧（不覆盖非空） | R9 `SYNC_DEGRADES_NONEMPTY_MIRROR` 红绿 | 未知不改 + D4 口径 |
| 写侧出现第二个镜像来源 | 新代码内联角色集合 | 无「唯一来源」断言 | 只扫字面量，不扫赋值形态 | 未来新增 handler | 静态来源扫描 | R9 `WRITE_SIDE_SECOND_ASSIGNMENT=0` | `MIRROR_SOURCE_DERIVE_ONLY=1` |
| 建号用户 `roles=[]` 靠读侧兜底掩盖 | CreateSite / addMembers 初始化只写 `role` | 建号点未收口 | 无初始化形态用例 | `merchant_member.go` 建缓存 | allowlist（D1）+ 读侧回退 | R2 `ALLOWLIST_ENTRIES=3` 登记 | allowlist 带理由与 ref |
| 负例不覆盖变量写法 | 只注入字面量负例 | 断言器形态盲 | 只有字面量负例 | 扫描类验收全部 | 扫描器（`r2`） | `R2_NEGATIVE_CONTROL_VAR` 红绿 | 变量形态负例强制 |
| 归因错到前端 | 只改读侧集合优先 | 症状出现在 UI | 无服务端写→读用例 | 其它缓存镜像字段 | 写侧而非读侧 | R8 服务端读回 | 写侧为唯一修复层 |
| 镜像 `null` 与 `[]` 混淆 | 序列化遗漏 | `nil` 直接落库 | 无 NULL 断言 | `site_members.roles` | helper（`nil → []`） | R7 `JSONB_IS_NULL=0` | helper 内归一 |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R2 | `bash scripts/verify-2647.sh r2` | `docs/evidence/2647/R2-write-sites.txt` | exit 0；grep `WRITE_SITES=6`、`WRITE_INLINE_LITERAL=0`、`ROLE_ASSIGN_WITHOUT_MIRROR=0`、`R2_NEGATIVE_CONTROL_VAR=1`、`ALLOWLIST_ENTRIES=3`、`ALLOWLIST_UNLINKED=0` | PASS | 删掉改角色的 `updates["roles"]` 后 `ROLE_ASSIGN_WITHOUT_MIRROR>0`，或变量形态负例未命中 → FAIL | dev |
| R7 | `bash scripts/verify-2647.sh r7` | `docs/evidence/2647/R7-users-roles-jsonb.txt` | exit 0；grep `USERS_MIRROR_JSONB_EXPR=1`、`USERS_MIRROR_RAW_SLICE=0`、`JSONB_TYPE_IS_ARRAY=1`、`JSONB_IS_NULL=0`、`RAW_SLICE_WRITE_ERROR=1` | PASS | 任一写点回到裸切片（`USERS_MIRROR_RAW_SLICE>0`）或 `jsonb_typeof` 非 array → FAIL | dev |
| R8 | `bash scripts/verify-2647.sh r8` | `docs/evidence/2647/R8-role-change-mirror.txt` | exit 0；grep `HTTP=200`、`ROLE_ASSIGN_WITHOUT_MIRROR=0`、`MIRROR_PRIMARY=site_admin`、`MIRROR_ALL_CONTAINS_SITE_ADMIN=1`、`MIRROR_ALL_CONTAINS_OLD=1`、`READBACK_PREFERS_MIRROR=1` | PASS | 只写 role（`MIRROR_ALL_CONTAINS_SITE_ADMIN=0`）或读回旧角色（`READBACK_PREFERS_MIRROR=0`）→ FAIL | dev |
| R9 | `bash scripts/verify-2647.sh r9` | `docs/evidence/2647/R9-mirror-source.txt` | exit 0；grep `MIRROR_SOURCE_DERIVE_ONLY=1`、`WRITE_SIDE_SECOND_ASSIGNMENT=0`、`SYNC_SKIPS_NONEMPTY_MIRROR_OVERWRITE=1`、`SYNC_DEGRADES_NONEMPTY_MIRROR=0` | PASS | 同步覆盖非空镜像（`SYNC_DEGRADES_NONEMPTY_MIRROR=1`）或出现第二处镜像赋值 → FAIL | dev |

### Workload

- Size: L
- Basis: 6 个 Go 实现/改造文件（`user_roles_jsonb.go` 新增 + `user_staff.go`/`iam_proxy.go`/`iam_callback.go`/`merchant.go`/`merchant_member.go`）+ 1 个测试文件 + 1 个证据脚本 + 4 条证据行（R2/R7/R8/R9）+ 前置依赖（#2626 合并）
- Confidence: medium（R7 负例依赖 `TUNELOOP_DB_SIMPLE_PROTOCOL=1`；R9 采「未知不改」口径，`functional_roles` 跨仓补齐另立）

### Files（变更文件清单）

- `backend/handlers/user_roles_jsonb.go`
- `backend/handlers/user_staff.go`
- `backend/handlers/iam_proxy.go`
- `backend/handlers/iam_callback.go`
- `backend/handlers/merchant.go`
- `backend/handlers/merchant_member.go`
- `backend/handlers/user_roles_mirror_2647_test.go`
- `scripts/verify-2647.sh`
- `docs/evidence/2647/R2-write-sites.txt`
- `docs/evidence/2647/R7-users-roles-jsonb.txt`
- `docs/evidence/2647/R8-role-change-mirror.txt`
- `docs/evidence/2647/R9-mirror-source.txt`
- `docs/evidence/2647/evidence.json`
- `docs/evidence/2647/plan.md`
- `docs/evidence/2647/pre-ready-2647.json`

## Dependencies

- #2626

### 依赖与关联说明（非硬依赖）

- **硬前置 #2626**：本单四行都要改 #2626 分支上的实现（`user_staff.go`、`iam_proxy.go` 等）与其证据口径。#2626 当前 `status:ready`（Rev.4 计划冻结，Round 3 提交）且**未合入 `origin/develop`**（`git merge-base --is-ancestor origin/issue-2626 origin/develop` = false）→ `/work` 必须 HOLD。
- **开工前校验**：① `gh issue view 2626 --json labels` 含 `status:done`；② `git merge-base --is-ancestor origin/issue-2626 origin/develop` = true；③ `origin/develop` 上存在 `backend/services/role_derive.go` 与 `backend/database/migrations/20261128001_users_roles_mirror_2626.up.sql`。任一不满足 → 保持 HOLD 并在 Work Summary 记录原因，不改计划。
- **Round 2 失败项分工**：失败项 1（改角色不写镜像）→ 本单 R8；失败项 2（同步降级）→ 本单 R9；失败项 3（jsonb 形态 + 吞错）→ 本单 R7（吞错修复随 helper 落地：`merchant.go` / `iam_callback.go` 必须检查 `Updates` 返回值）；失败项 4（D1 回退链偏离）→ 本单 D5 登记（不改代码）；失败项 5（建号初始化）→ 本单 D1 allowlist。
- **关联**：`#2505`（`functional_roles` 集合端点 / unassign 缺失）、`#2580`（role 缓存多选）、`#2527`（缓存回填）。
- **部署侧**：无 DDL、无迁移、无 API 契约变更（`roles[]` 已在 #2626 的 `docs/spec/api/api.md` 登记）；真机复验归 `[Ops-Followup of #2647]`。

*Model: DeepSeek V4.1 Flash 1017*

