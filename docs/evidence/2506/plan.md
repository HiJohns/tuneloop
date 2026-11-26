## Accepted Plan — #2506 [Core Update Required] S1: IAM 功能角色「集合语义」地基 + 人员管理改角色主诉路径｜Rev.4

> Parent: #2505。本单解决「改角色后 IAM `functional_roles` 只增不减（权限回收失效）」的**地基 + 主诉路径**；其余角色变更路径见 S2（#2507）。
> Tags: bug, code:go, api-contract

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 改角色只做「顶层 role + ADD 模板」，无 remove / set | `backend/handlers/user_staff.go:1200-1217`（`UpdateUserRoleInOrgWithToken` + `AssignRoleTemplateToUserWithToken`） |
| tuneloop IAM 客户端**只有** assign，没有读取/覆盖功能角色的方法 | `backend/services/iam_client.go:1857-1885`（`AssignRoleTemplateToUser*`） |
| beaconiam **已有**集合语义端点：`PUT /api/v1/organizations/:id/users/:user_id/functional-roles`，body `{"roles":[...]}`，`relation.FunctionalRoles = req.Roles`（整体覆盖），校验每个 code 对应 active 模板，并刷新 perm_version | 对端 `beaconiam/internal/handler/org_handler.go:1499-1595`；路由 `beaconiam/cmd/api/main.go:293` |
| beaconiam 只读端点：`GET /api/v1/users/:id/roles?org_id=` → `[{org_id, functional_roles}]` | 对端 `beaconiam/internal/handler/permission_handler.go:137-171`；路由 `beaconiam/cmd/api/main.go:385` |
| 逐码 `DELETE /users/:id/roles/:role_id` 不可用于本场景：仅 `claims.IsOwner` 或职能角色 `role:write` 通过（site_admin=ADMIN 不满足） | 对端 `beaconiam/internal/handler/permission_handler.go:40-50,239-289` |
| #2482（done，分支 `issue-2482` 未合入 develop）已定义「改主角色 = 合并其它角色」语义与 `mergeSiteMemberRoles` | `719e64929`；`backend/handlers/site_member_common.go` |
| 本路径操作者是 `merchant_admin`（sys_perm bit18=user_update）；`PUT functional-roles` 要求 operator 在**目标 org 本身**为 OWNER/ADMIN（无层级继承）→ 商户管理员对网点子组织调必 403（Rev.4 修正：Rev.3 误判为「权限兼容」） | `backend/services/role_templates.go:28-31,45-47`；`backend/main.go:552`；对端 `beaconiam/internal/handler/org_handler.go:1534-1541`；二级组织关系 `backend/handlers/site.go:328-333`、`merchant.go:313-320` |
| 有效权限 = 直授权限 ∪ 角色权限（按位或，四层）：组织默认 / `user_org_relations.cus_perm` / `user_permission_overrides` / 角色模板 `cus_perm` → 角色集合一旦覆盖，旧角色权限自动消失；**直授字段不得被本单触碰** | 对端 `beaconiam/internal/service/permission_calc.go:20-79` |
| 授权例外（Owner 2026-11-26 裁定）：**商户管理员可管理网点成员在网点组织中的角色** → 需 beaconiam 侧新增「直接父组织管理员」例外 | 对端 Issue HiJohns/beaconiam#506 |

**结论**：把 `user_staff.UpdateUser` 的角色同步从「ADD 模板」改为**集合覆盖**（GET → 计算目标集合 → PUT），目标集合 = 本 org 本地角色全集（#2482 合并后）∪ 保留的非受管模板。这样降级即回收，且不误删 `customer` / `namespace_admin`。

### 执行前置（硬门，未满足则 HOLD 不得实现）

1. **#2482 合并语义已在基线**：`cd backend && grep -q "func mergeSiteMemberRoles" handlers/site_member_common.go`。
   缺失 → `holdon.sh hold 2506 --self-block --reason "awaiting #2482 integration into develop" --deps "#2482"` 并停止；**禁止**在本单内重写合并逻辑（避免双份语义）。
2. **beaconiam 授权例外已部署到预生产**（`HiJohns/beaconiam#506`：父组织 OWNER/ADMIN 可管理**直接子组织**成员的功能角色集合）。
   - 判定依据：beaconiam 侧发布包已部署 + 其单测（父→子 allow / 子→父、兄弟、隔代、inactive 关系 403）全绿；
   - **未就绪时禁止把 tuneloop 切换到集合覆盖端点**：旧行为下「只 ADD」尚能加角色（虽不回收），切换到集合覆盖后端到端会全部 403 → 比修复前更差。
   - 缺失 → `holdon.sh hold 2506 --self-block --reason "awaiting beaconiam#506 deployment"` 并停止。

### 变更点

1. `backend/services/iam_client.go` 新增两个方法（失败不吞错）：
   - `GetUserFunctionalRolesWithToken(token, userID, orgID string) ([]string, error)` → `GET /api/v1/users/{userID}/roles?org_id={orgID}`；解析 `[]struct{OrgID, FunctionalRoles}`，返回匹配 `org_id` 的集合；空列表返回空切片。
   - `SetUserFunctionalRolesWithToken(token, userID, orgID string, codes []string) error` → `PUT /api/v1/organizations/{orgID}/users/{userID}/functional-roles`，body `{"roles":codes}`；状态码映射：`403` → `permission denied: <body>`、`404` → `user or organization not found: ...`、`400` → `invalid functional roles: <body>`、其它非 200 → 带状态码错误；成功后 `c.IncrementPermVersion()`。
2. `backend/handlers/site_member_common.go` 新增（纯函数可脱 DB 单测）：
   - `managedFunctionalRoleCodes = []string{"site_admin","site_member","merchant_staff","repair_technician","merchant_admin"}` + `isManagedFunctionalRoleCode(code) bool`
   - `functionalRoleTargetSet(current, desired []string) []string`：desired 去空去重保序在前；随后追加 `current` 中**非受管**且未出现过者（保留 `customer` / `namespace_admin` / 自定义模板）。
   - `functionalRoleTargetSetWithout(current []string, remove ...string) []string`：精确移除指定码（S2 换管理员降级用），保序。
   - `syncFunctionalRoles(iamClient *services.IAMClient, userToken, iamSub, orgID string, desired []string) []gin.H`：
     a) GET current（失败 → 返回 `role_errors`，**不**发 PUT，因为无法保真非受管码）；
     b) `nsID := middleware.GetNamespaceID(ctx)` 非空时 `ListRoleTemplates(nsID)` 取模板列表，**必须解析 `is_active`**（Rev.4：`ListRoleTemplates` 会返回 `is_active=false` 模板，Rev.3 误以为只返回 active）：
        - 只把 `is_active=true` 的码视为有效；`current` 中的失效码被剔除并逐条记 `role_errors`（可见、不静默）；
        - **非受管码不受模板列表影响**：`functionalRoleTargetSet` 对非受管码无条件保留（即使其不在模板列表或被标 inactive），避免误删 `customer` / `namespace_admin` / 自定义码；
        - 受管码若命中 `is_active=false` → 剔除 + `role_errors`（不发送会导致 400 `invalid_role` 的码）；
        - 模板列表拉取失败或 `nsID` 为空 → 跳过过滤，**仅**剔除「受管但当前组织已不再允许」的码不可判定时不剔除，PUT 失败以 `role_errors` 暴露（D7 修订）。
     c) `target := functionalRoleTargetSet(filteredCurrent, desired)`；与 current 集合相等（忽略顺序）→ 跳过 PUT；
     d) PUT（失败 → `role_errors`）；成功返回空切片。
3. `backend/handlers/user_staff.go`（`roleChanged` 分支）：
   - 把 #2482 的「逐行读取 `site_members(id, roles, site_org_id)` + `mergeSiteMemberRoles`」**提前**到 IAM 同步之前，合并结果复用于后续本地写库（保持逐行 `WHERE id = ?` 不回归）。
   - 目标集合：本 org（`existingUser.OrgID`，为空时沿用既有 tenantID fallback）内各网点行合并集合的并集 ∪ `{newRole}`。
   - **本地读取失败短路（Rev.4，审计 Bug 5）**：`db.Raw` 读取 `site_members.roles` 失败时，`siteRows` 不得当作空集合继续（否则目标集合退化为 `{newRole}`，集合覆盖会**破坏性删除** `repair_technician` 等并存角色）——必须跳过 IAM 同步并把错误写入 `role_errors`（`PUT_CALLS=0`）。
   - 用 `trigger := syncFunctionalRoles(...)` 替换单点 `AssignRoleTemplateToUserWithToken(newRole)`；追加到 `roleErrors`。
   - `UpdateUserRoleInOrgWithToken`（顶层 role）与本地写库顺序、`role_errors` 契约（#2375/#2462 D3）不变。
4. 文档（Core Update）：见 `## Core Update`。
5. 测试与证据：`backend/services/iam_functional_roles_2505_test.go`、`backend/handlers/functional_role_sync_2505_test.go`、`scripts/verify-2505-s1.sh`。
6. **授权通道（Rev.4 新增）**：本单不改 beaconiam；依赖其授权例外（`HiJohns/beaconiam#506`）——父组织 OWNER/ADMIN 可管理**直接子组织**成员的功能角色集合，其余关系仍 403。
   - tuneloop 侧只负责把 403 映射为 `role_errors`（不吞、不重试、不降级回 ADD）；
   - 不得为了绕过 403 而使用平台/服务账号越权写，也不得回退 `POST /users/{id}/roles`（ADD-only）。

## 语义变更：被翻转的既有断言（AGENTS.md §语义变更强制）

| # | 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|---|
| 1 | `backend/handlers/user_role_assign_2375_test.go:191` | `tmplCalls=1`（POST `/users/{id}/roles`） | `tmplCalls=0`；断言 `GET /users/{id}/roles?org_id=` + `PUT /organizations/{org}/users/{user}/functional-roles` | assign 退役，集合覆盖接管 |
| 2 | `backend/handlers/user_staff_role_set_2482_test.go:195` | `assignCalls=1`；fake 无 GET roles | `assignCalls=0`；fake 增补 GET roles；`role_errors` 为空 | 同上 |
| 3 | `backend/handlers/site_member_roles_jsonb_2475_test.go:40-63,127` | fake 无 GET roles；`role_errors` 为空 | fake 增补 GET roles；`role_errors` 仍为空 | 同上 |

负例：任何路径仍调用 `POST /api/v1/users/{id}/roles` → FAIL。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| 目标集合纯函数（受管覆盖 + 非受管保留） | `backend/handlers/site_member_common.go` | `go test ./handlers -run TestFunctionalRoleTargetSet_2505 -count=1 -timeout 15m` | 4 组输入：无/有受管旧码；有 `customer`；重复与乱序 | 后端 | `site_admin→site_member` 得 `["site_member"]`；`merchant_staff→repair_technician` 得 `["repair_technician"]`；`customer` 保留；保序去重 | R1（合同 R1） |
| IAM 客户端契约（路径/查询/body/错误） | `backend/services/iam_client.go` | `go test ./services -run TestFunctionalRolesContract_2505` | fake IAM：200 / 403 / 404 | 后端 | GET 路径与 `org_id` 查询正确；PUT 路径 + `{"roles":[...]}` body 正确；403/404 返回 error 且含关键字 | R2（合同 R2） |
| 主诉 A：`site_admin → site_member` | `backend/handlers/user_staff.go` | `PUT /api/users/:id`（`UpdateUser`） | IAM current=`["site_admin","site_member"]`，本地 `site_members.roles=["site_admin"]` | 后端 | PUT body `roles=["site_member"]`；改后受管集合无 `site_admin`；红绿：旧实现残留 `site_admin` | R3（合同 R3） |
| 主诉 B：`merchant_staff → repair_technician` | `backend/handlers/user_staff.go` | 同上 | IAM current=`["merchant_staff","repair_technician"]`，`merchant_members.role=merchant_staff` | 后端 | PUT body `roles=["repair_technician"]`；无 `merchant_staff` 残留 | R4（合同 R4） |
| 双角色保留（#2482 不回退） | `backend/handlers/user_staff.go` | 同上 | 本地 `roles=["site_member","repair_technician"]`，改主角色为 `site_admin` | 后端 | PUT body `roles=["site_admin","repair_technician"]`；`repair_technician` 未被移除 | R5（合同 R5） |
| 非受管模板保留 | `backend/handlers/site_member_common.go` + `user_staff.go` | 同上 | IAM current=`["site_member","customer"]`，目标 `{site_member}` | 后端 | PUT body 含 `customer`；`customer` 不被移除 | R6（合同 R6） |
| 失败语义（不静默） | `backend/handlers/site_member_common.go` | 同上 | GET 500；PUT 403 | 后端 | 响应 20000 且 `role_errors` 非空；GET 失败时 `PUT_CALLS=0`；无「失败却无痕迹」 | R7（合同 R7） |
| 旧 ADD 语义退场（负例） | 全部变更路径 | `bash scripts/verify-2505-s1.sh r9` | 修后树 | 后端 | `LEGACY_ASSIGN_CALLS=0` | R9（合同 R9） |
| 编译 + 定向回归 | `backend` | `go build ./...` + 受影响用例定向测试（`go test ./handlers -run 'Test.*_2505|Test.*_2482|Test.*_2461|Test.*_2475|Test.*_2375|Test.*_2462|Test.*_2466' -count=1 -timeout 15m`，由 R8 脚本执行） | 修后树 | 后端 | `BUILD_EXIT=0`、`TESTS_FAIL=0`（含 2482/2461/2475/2506 用例） | R8（合同 R8） |
| 授权例外可用（前置门，beaconiam 侧契约） | beaconiam 仓库（外部前置，无需在本单 Files 登记）+ `scripts/verify-2505-s1.sh` | beaconiam 单测：父 org OWNER/ADMIN → 直接子 org 允许；子→父 / 兄弟 / 隔代 / inactive 关系 / 跨 namespace → 403 | 对端 handler | 后端 | `AUTHZ_PARENT_CHILD_OK=1`、`AUTHZ_SIBLING_DENIED=1`、`AUTHZ_GRANDPARENT_DENIED=1`、`AUTHZ_INACTIVE_DENIED=1` | R10（合同 R10） |
| inactive 模板过滤（is_active 解析） | `backend/handlers/site_member_common.go` | 同上（fake IAM 模板列表含 `is_active=false` 的 `site_admin` 与不在列表的 `customer`） | current=`["site_admin","site_member","customer"]` | 后端 | PUT 不含 `site_admin`；`customer` 保留；`role_errors` 含失效码说明；不出现 400 `invalid_role` | R11（合同 R11） |
| fixture 保真（主诉场景真实形态） | `backend/handlers/functional_role_sync_2505_test.go` | 同上 | 用户 `users.org_id = siteOrg`（**≠ tenant**）；操作者关系在商户（父）org；网点 org 为父 org 的子组织 | 后端 | `SITE_ORG_NE_TENANT=1`、`OPERATOR_ORG=merchant`、`PUT_ORG=site_org`、`USERS_ORG_IS_TENANT=0` | R12（合同 R12） |
| 本地角色读取失败不破坏性 PUT | `backend/handlers/user_staff.go` | 同上 | `site_members.roles` 读取失败（注入错误） | 后端 | `PUT_CALLS=0`、`ROLE_ERRORS>=1`、`SILENT_200=0` | R13（合同 R13） |
| 红绿可证伪（不是字面量推导） | `backend/handlers/functional_role_sync_2505_test.go` | 同上 | 同一 fixture 分别跑「旧 ADD-only 目标集合算法」与「新集合覆盖算法」 | 后端 | `LEGACY_STALE_SITE_ADMIN=1`、`NEW_STALE_SITE_ADMIN=0`、`RED_GREEN_FALSIFIABLE=1` | R14（合同 R14） |

## Input Truth & Tool Preflight

- **Source of truth**：beaconiam 端点语义以对端代码为准（`org_handler.go:1499-1595` 覆盖式写入 + 模板校验 + perm_version；`permission_handler.go:137-171` 只读返回数组）。本单不改 beaconiam。
- **差异核对**：`grep -rn "AssignRoleTemplateToUserWithToken" backend` → 5 处角色变更路径（本单仅改 `user_staff.go`，其余 S2 #2507）+ 4 处「新增成员/邀请接受」路径（只增不减属正确语义，不动）。
- **损坏输入拒绝**：`desired` 含空串 → 去空；重复 → 去重；`orgID` 为空（tenantID fallback 也空）→ 直接 `role_errors` 不发请求；GET 返回中无匹配 org → 视为空集合（不伪造）。
- **工具盲区**：`site_members.roles` 为 jsonb，测试使用真实 Postgres 夹具（`testfixtures`）+ 原生 `roles::text` 读取；不得用 sqlite 替代（`::jsonb`/`::text` 语义不同）。fake IAM 以 `services.SetIAMInternalURLForTesting` 注入，断言 method/path/query/body。
- **影响面预检**：`PUT functional-roles` 会覆盖整组功能角色 → 必须先 GET 再计算，禁止直接 PUT `desired`（否则删掉 `customer`）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 采用「GET current → PUT 集合覆盖」，不用逐码 DELETE（DELETE 仅 IsOwner/`role:write` 可用；模板里无 `role:write` 码 → 实际只有 IsOwner 能删） | Architect | 本计划锁定（对端代码已证） | 阻塞实现 |
| D8 | 授权前提 = **beaconiam 例外**：目标 org 的直接父组织 OWNER/ADMIN 可管理其成员功能角色；其余关系（子管父/兄弟/隔代/inactive 关系/跨 namespace）一律 403。本单不绕行、不降级 | Architect + Owner（2026-11-26 裁定） | 依赖 `HiJohns/beaconiam#506` 已部署；未部署 → 执行前置 2 HOLD | 未就绪就切端点 → 端到端全 403，比修复前更差（禁止） |
| D9 | 有效权限 = 直授 ∪ 角色（按位或，`permission_calc.go:20-79`）⇒ 本单**只覆盖角色集合**，不读写 `cus_perm` / `cus_perm_ext` / `user_permission_overrides`；不新增「权限清理」逻辑 | Architect | 本计划锁定 | 若要求同时改直授 → 另立 Issue（权限模型变更） |
| D10 | 403/404/400 一律映射为 `role_errors`（可见），禁止吞错、禁止回退 `POST /users/{id}/roles` | Architect | 本计划锁定（红线：不静默吞错） | 阻塞实现 |
| D2 | 非受管模板（`customer` / `namespace_admin` / 自定义）一律保留；#2505「只剩 X」按 **受管模板集合** 解释 | Architect + Owner | 本计划锁定（移除 customer 会破坏顾客上下文 #2027/#2031） | Owner 若要求连 customer 一并清空 → 回 Architect 重写语义 |
| D3 | 受管模板码固定为 `site_admin/site_member/merchant_staff/repair_technician/merchant_admin` | Architect | 本计划锁定（与 `services.AllRoleTemplates` 及 #2129/#2375 白名单一致） | 新增业务角色 → 先改本清单（新 Issue） |
| D4 | GET 失败时**不发** PUT（无法保真非受管码），以 `role_errors` 显式暴露 | Architect | 本计划锁定 | 阻塞实现 |
| D5 | 执行前置：#2482 合并语义必须已在基线（`mergeSiteMemberRoles`）；否则 HOLD | Architect | 本计划锁定 | 缺失 → HOLD，等集成，不在本单重复实现 |
| D6 | `users.org_id` 为空时沿用既有 tenantID fallback，不扩范围；失败以 `role_errors` 可见 | Architect | 本计划锁定 | 若要求改为「org 缺失即 400」→ 回 Architect（影响面更大） |
| D7 | 模板过滤口径（Rev.4 修正）：`ListRoleTemplates` 返回含 inactive → **必须解析 `is_active`**；受管码只保留 active，失效码剔除并记 `role_errors`；**非受管码无条件保留**（不受模板列表影响）；列表拉取失败/`nsID` 空 → 跳过过滤，PUT 失败以 `role_errors` 暴露 | Architect | 本计划锁定 | 阻塞实现（不得静默过滤、不得误删 customer） |

## Out of Scope

- 其余角色变更路径（网点成员 / 商户直属 / 中转网点 / 权限管理 / 换管理员降级）→ S2 #2507。
- 预生产存量漂移清单执行（只读，需 Owner/运维在预生产库执行）→ S2 脚本 + Ops 回评。
- beaconiam 侧授权例外的实现与发布（在 `HiJohns/beaconiam#506` 处理；本单只消费并验证前置门）。
- 前端 PC/小程序 UI 与文案。
- #2482 自身的合并语义与集成动作（由集成方 merge `issue-2482`）。
- 全量 `go test ./...`（由父/批次 gate 承担）。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `functionalRoleTargetSet(["site_admin","site_member"],["site_member"])`；`(["merchant_staff","repair_technician"],["repair_technician"])`；`(["site_member","customer"],["site_member"])`；`(["a","a"],["b"])` | `go test` 纯函数 | `docs/evidence/2506/R1-target-set.txt` | 4 组分别 `["site_member"]`、`["repair_technician"]`、`["site_member","customer"]`、`["b","a"]`；`ORDER_STABLE=1` | 残留 `site_admin` 或删 `customer` → FAIL |
| R2 | fake IAM：`GET /api/v1/users/USER/roles?org_id=ORG` 返回 `[{"org_id":"ORG","functional_roles":["site_admin","site_member"]}]`；`PUT .../functional-roles` 200；403/404 变体 | `go test` 客户端 | `docs/evidence/2506/R2-iam-contract.txt` | `GET_PATH`、`GET_QUERY=org_id=ORG`、`PUT_BODY={"roles":["site_member"]}`、`FORBIDDEN_ERR=1`、`NOTFOUND_ERR=1` | 403 被吞成 nil → FAIL |
| R3 | 用户 U（`iam_sub=S`，`org_id=ORG`）、网点 S1（org=ORG）`site_members.role=site_admin, roles=["site_admin"]`；IAM current=`["site_admin","site_member"]` | `PUT /api/users/U {"role":"site_member"}` | `docs/evidence/2506/R3-demote-admin.txt` | `PUT_ROLES=["site_member"]`、`STALE_SITE_ADMIN=0`、`RED_GREEN=1`（旧实现残留） | PUT body 仍含 `site_admin` → FAIL |
| R4 | 用户 U2（`merchant_members.role=merchant_staff`）；IAM current=`["merchant_staff","repair_technician"]` | `PUT /api/users/U2 {"role":"repair_technician"}` | `docs/evidence/2506/R4-merchant-to-repair.txt` | `PUT_ROLES=["repair_technician"]`、`STALE_MERCHANT_STAFF=0` | 残留 `merchant_staff` → FAIL |
| R5 | 本地 `site_members.roles=["site_member","repair_technician"]`；IAM current=`["site_member","repair_technician"]` | `PUT /api/users/U {"role":"site_admin"}` | `docs/evidence/2506/R5-dual-role.txt` | `PUT_ROLES=["site_admin","repair_technician"]`、`DROPPED_REPAIR=0` | 丢 `repair_technician` → FAIL |
| R6 | IAM current=`["site_member","customer"]`；本地目标 `{site_member}` | 同上 | `docs/evidence/2506/R6-unmanaged-preserve.txt` | `PUT_ROLES` 含 `customer`；`CUSTOMER_REMOVED=0` | 删 `customer` → FAIL |
| R7 | fake IAM：GET → 500；PUT → 403 | 同上 | `docs/evidence/2506/R7-failure-visible.txt` | `GET_FAIL_ROLE_ERRORS=1`、`PUT_SKIPPED_ON_GET_FAIL=1`、`PUT_FAIL_ROLE_ERRORS=1`、`SILENT_200=0` | 失败仍 200 且无 `role_errors` → FAIL |
| R8 | 修后 `backend` 树 | `go build ./...` + `go test ./handlers -run '2505|2506|2482|2461|2475|2375|2462|2466' -count=1 -timeout 15m` | `docs/evidence/2506/R8-build.txt` | `BUILD_EXIT=0`、`TESTS_FAIL=0`、`PANIC=0` | 任一非 0 → FAIL |
| R10 | 对端仓库 beaconiam 的 `TestUpdateUserFunctionalRoles_AuthorizationHierarchy`（父→子 allow；子→父/兄弟/隔代/inactive/跨 namespace deny） | `bash scripts/verify-2505-s1.sh r10` | `docs/evidence/2506/R10-beaconiam-authz.txt` | `AUTHZ_PARENT_CHILD_OK=1`、`AUTHZ_SIBLING_DENIED=1`、`AUTHZ_GRANDPARENT_DENIED=1`、`AUTHZ_INACTIVE_DENIED=1` | 任一 deny 用例变成 200 → FAIL（越权） |
| R11 | fake 模板列表：`site_admin(is_active=false)`、`site_member(active)`、`customer(不在列表)`；current=`["site_admin","site_member","customer"]` | `bash scripts/verify-2505-s1.sh r11` | `docs/evidence/2506/R11-inactive-filter.txt` | `INACTIVE_DROPPED=1`、`CUSTOMER_KEPT=1`、`PUT_NO_INACTIVE=1`、`INVALID_ROLE_400=0` | `site_admin` 被发送（→400）或 `customer` 被删 → FAIL |
| R12 | 夹具：`users.org_id = siteOrg`、`siteOrg.parent_id = merchantOrg`、操作为 merchant OWNER | `bash scripts/verify-2505-s1.sh r12` | `docs/evidence/2506/R12-fixture-fidelity.txt` | `SITE_ORG_NE_TENANT=1`、`USERS_ORG_IS_TENANT=0`、`PUT_ORG=site_org` | 夹具仍把 users.org_id 设成 tenant → FAIL（红例不成立） |
| R13 | 注入 `site_members.roles` 读取错误 | `bash scripts/verify-2505-s1.sh r13` | `docs/evidence/2506/R13-local-read-failure.txt` | `PUT_CALLS=0`、`ROLE_ERRORS>=1`、`SILENT_200=0` | 读失败仍 PUT（会删并存角色）→ FAIL |
| R14 | 同一 fixture 跑旧/新算法 | `bash scripts/verify-2505-s1.sh r14` | `docs/evidence/2506/R14-red-green.txt` | `LEGACY_STALE_SITE_ADMIN=1`、`NEW_STALE_SITE_ADMIN=0`、`RED_GREEN_FALSIFIABLE=1` | 两算法结果相同（红侧不可证伪）→ FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2505-s1.sh r1` | `docs/evidence/2506/R1-target-set.txt` | exit 0；grep `CASE_ADMIN_TO_MEMBER=["site_member"]`、`CASE_MERCHANT_TO_REPAIR=["repair_technician"]`、`PRESERVE_CUSTOMER=1`、`ORDER_STABLE=1` | PASS | 残留 `site_admin` → FAIL | dev |
| R2 | `bash scripts/verify-2505-s1.sh r2` | `docs/evidence/2506/R2-iam-contract.txt` | exit 0；grep `GET_QUERY=org_id=ORG`、`PUT_PATH=/api/v1/organizations/ORG/users/USER/functional-roles`、`PUT_BODY={"roles":["site_member"]}`、`FORBIDDEN_ERR=1` | PASS | 403 被吞 → FAIL | dev |
| R3 | `bash scripts/verify-2505-s1.sh r3` | `docs/evidence/2506/R3-demote-admin.txt` | exit 0；grep `PUT_ROLES=["site_member"]`、`STALE_SITE_ADMIN=0`、`RED_GREEN=1` | PASS | `STALE_SITE_ADMIN=1` → FAIL | dev |
| R4 | `bash scripts/verify-2505-s1.sh r4` | `docs/evidence/2506/R4-merchant-to-repair.txt` | exit 0；grep `PUT_ROLES=["repair_technician"]`、`STALE_MERCHANT_STAFF=0` | PASS | `STALE_MERCHANT_STAFF=1` → FAIL | dev |
| R5 | `bash scripts/verify-2505-s1.sh r5` | `docs/evidence/2506/R5-dual-role.txt` | exit 0；grep `PUT_ROLES=["site_admin","repair_technician"]`、`DROPPED_REPAIR=0` | PASS | `DROPPED_REPAIR=1` → FAIL | dev |
| R6 | `bash scripts/verify-2505-s1.sh r6` | `docs/evidence/2506/R6-unmanaged-preserve.txt` | exit 0；grep `CUSTOMER_REMOVED=0`、`UNMANAGED_KEPT=1` | PASS | `CUSTOMER_REMOVED=1` → FAIL | dev |
| R7 | `bash scripts/verify-2505-s1.sh r7` | `docs/evidence/2506/R7-failure-visible.txt` | exit 0；grep `GET_FAIL_ROLE_ERRORS=1`、`PUT_SKIPPED_ON_GET_FAIL=1`、`PUT_FAIL_ROLE_ERRORS=1`、`SILENT_200=0` | PASS | `SILENT_200=1` → FAIL | dev |
| R8 | `bash scripts/verify-2505-s1.sh r8` | `docs/evidence/2506/R8-build.txt` | exit 0；grep `BUILD_EXIT=0`、`TESTS_FAIL=0`、`PANIC=0` | PASS | 编译失败 → FAIL | dev |
| R10 | `bash scripts/verify-2505-s1.sh r10` | `docs/evidence/2506/R10-beaconiam-authz.txt` | exit 0；grep `AUTHZ_PARENT_CHILD_OK=1`、`AUTHZ_SIBLING_DENIED=1` | PASS | 越权放行 → FAIL | dev |
| R11 | `bash scripts/verify-2505-s1.sh r11` | `docs/evidence/2506/R11-inactive-filter.txt` | exit 0；grep `INACTIVE_DROPPED=1`、`CUSTOMER_KEPT=1` | PASS | 发送 inactive 码 → FAIL | dev |
| R12 | `bash scripts/verify-2505-s1.sh r12` | `docs/evidence/2506/R12-fixture-fidelity.txt` | exit 0；grep `SITE_ORG_NE_TENANT=1`、`PUT_ORG=site_org` | PASS | 夹具保真不足 → FAIL | dev |
| R13 | `bash scripts/verify-2505-s1.sh r13` | `docs/evidence/2506/R13-local-read-failure.txt` | exit 0；grep `PUT_CALLS=0`、`SILENT_200=0` | PASS | 读失败仍 PUT → FAIL | dev |
| R14 | `bash scripts/verify-2505-s1.sh r14` | `docs/evidence/2506/R14-red-green.txt` | exit 0；grep `LEGACY_STALE_SITE_ADMIN=1`、`NEW_STALE_SITE_ADMIN=0` | PASS | 红侧恒真 → FAIL | dev |
| R9 | `bash scripts/verify-2505-s1.sh r9` | `docs/evidence/2506/R9-legacy-assign.txt` | exit 0；grep `LEGACY_ASSIGN_CALLS=0` | PASS | 仍调用 `POST /users/{id}/roles` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2506/R1-target-set.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2506/R2-iam-contract.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2506/R3-demote-admin.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2506/R4-merchant-to-repair.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2506/R5-dual-role.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2506/R6-unmanaged-preserve.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2506/R7-failure-visible.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2506/R8-build.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2506/R9-legacy-assign.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2506/R10-beaconiam-authz.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2506/R11-inactive-filter.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2506/R12-fixture-fidelity.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2506/R13-local-read-failure.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2506/R14-red-green.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 symptom | 触发条件 trigger | 根因 root cause | 测试盲区 test blind spot | 同类路径 same-class | 修复层 fix layer | 红绿 red-green | 预防 prevention |
|---|---|---|---|---|---|---|---|---|
| B1 | 降级后仍持有旧职能（token `fn_roles` 含 `site_admin`） | 人员管理改角色（`site_admin→site_member`） | 只 ADD 模板，无集合覆盖 | 既有测试只断言 assign 被调用 1 次 | `site_member.go` 等 5 处（S2） | IAM 同步层（`syncFunctionalRoles`） | R3 旧实现 `STALE_SITE_ADMIN=1` → 新 0 | 合同行断言 PUT body 精确集合 |
| B2 | 商户直属员工降级维修师傅后仍带 `merchant_staff` | `merchant_staff→repair_technician` | 同上 | 无「移除」断言 | 同上 | 同上 | R4 红绿 | 断言 `PUT_ROLES` 精确值 |
| B3 | 修复误伤顾客身份（`customer` 被删） | 集合覆盖时直接 PUT `desired` | 未区分受管/非受管模板 | 无 customer 保持用例 | `namespace_admin` 同类 | 纯函数 `functionalRoleTargetSet` | R6 | D2 + R6 负例 |
| B4 | 修复回退 #2482（维修师傅能力被删） | 双角色用户改主角色 | 目标集合未走合并语义 | #2482 用例未覆盖 IAM 侧 | `transit_site_admin`（S2） | 目标集合 = 合并集合 ∪ 新角色 | R5 | D5 前置 + R5 断言 |
| B5 | 读失败后仍 PUT → 越过非受管模板 | GET 5xx | 无失败短路 | 无失败路径用例 | 所有同步路径 | `syncFunctionalRoles` | R7 `PUT_SKIPPED_ON_GET_FAIL=1` | 合同 R7 |
| B6 | 403（非本 org 管理员）被吞成成功 | 平台管理员/越权操作 | 客户端未校验状态码 | 端到端未见 403 用例 | `SetUserFunctionalRolesWithToken` | IAM 客户端 | R2 `FORBIDDEN_ERR=1` | 负例断言 |
| B7 | 本地与 IAM 再次漂移（本地已改、IAM 未改） | PUT 失败 | 未进 `role_errors` | 前端只弹通用提示 | 全路径 | handler 失败语义 | R7 `PUT_FAIL_ROLE_ERRORS=1` | 沿用 #2375/#2461 `role_errors` |
| B8 | 模板校验缺失 → 400 `invalid_role` 阻断整组同步 | current 含已失效码 | 无模板过滤 | 无失效码用例 | 全路径 | `syncFunctionalRoles` 过滤 + `role_errors` | R7 组合断言 | D7 |
| B9 | 只改职位也触发同步 | `roleChanged=false` | 门控被改动 | #2462 D3 用例 | `user_staff.go` | 保持 `roleChanged` 门控 | R8 回归含 2475/2462 用例 |
| B10 | 主诉红例不成立（夹具把 `users.org_id` 设成 tenant，等于「站点 org == 商户 org」） | 夹具构造偷懒 | fake IAM 不校验 operator 关系，掩盖真实 403 | 无「site org ≠ tenant」用例 | 所有人员改角色用例 | 夹具按二级组织真实形态重建 | R12 `SITE_ORG_NE_TENANT=1` | 夹具必须复刻生产 ID 域关系 |
| B11 | 改角色整体 400 `invalid_role`，权限一个都没改成 | `ListRoleTemplates` 含 `is_active=false` 模板且客户端只解析 code | 无 inactive 模板用例 | 无 | 所有模板同步路径 | 解析 `is_active` + 受管码过滤 + `role_errors` | R11 `INACTIVE_DROPPED=1` | 模板列表消费必须校验 active |
| B12 | 集合覆盖把并存角色（如 `repair_technician`）静默删掉 | 本地 `site_members.roles` 读取失败被当作空集合 | 无 DB 读失败用例 | 无 | 所有「先读后写」同步路径 | 读失败短路 + `role_errors` | R13 `PUT_CALLS=0` | 破坏性写前必须确认读取成功 |
| B13 | 上线后所有改角色 403（比修复前更差：旧 ADD 至少还能加） | 误判「商户管理员对网点 org 有 OWNER/ADMIN」 | 未核对对端 per-org 关系校验 | 无 | 所有跨级授权调用 | 依赖 beaconiam 例外（父→直接子）+ 前置门 | R10 `AUTHZ_PARENT_CHILD_OK=1` | 跨仓授权必须读对端实现，不能按角色名推断 | 定向回归 |

### Workload

- Size: M
- Basis: 3 个实现文件（`iam_client.go`、`site_member_common.go`、`user_staff.go`）+ 2 个测试文件 + 1 个 verify 脚本 + 2 个核心文档 + 8 条证据行；无 migration、无前端
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/user_role_assign_2375_test.go`
- `backend/handlers/user_staff_role_set_2482_test.go`
- `backend/handlers/site_member_roles_jsonb_2475_test.go`

- `backend/services/iam_client.go`
- `backend/handlers/site_member_common.go`
- `backend/handlers/user_staff.go`
- `backend/services/iam_functional_roles_2505_test.go`
- `backend/handlers/functional_role_sync_2505_test.go`
- `scripts/verify-2505-s1.sh`
- `docs/spec/api/api.md`
- `docs/spec/permissions.md`
- `docs/evidence/2506/plan.md`
- `docs/evidence/2506/evidence.json`
- `docs/evidence/2506/pre-ready-2506.json`
- `docs/evidence/2506/R1-target-set.txt`
- `docs/evidence/2506/R2-iam-contract.txt`
- `docs/evidence/2506/R3-demote-admin.txt`
- `docs/evidence/2506/R4-merchant-to-repair.txt`
- `docs/evidence/2506/R5-dual-role.txt`
- `docs/evidence/2506/R6-unmanaged-preserve.txt`
- `docs/evidence/2506/R7-failure-visible.txt`
- `docs/evidence/2506/R8-build.txt`
- `docs/evidence/2506/R9-legacy-assign.txt`
- `docs/evidence/2506/R10-beaconiam-authz.txt`
- `docs/evidence/2506/R11-inactive-filter.txt`
- `docs/evidence/2506/R12-fixture-fidelity.txt`
- `docs/evidence/2506/R13-local-read-failure.txt`
- `docs/evidence/2506/R14-red-green.txt`

## Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §`PUT /api/users/:id` 的 `site_id` 语义（#2466）只描述网点迁移与重绑 | 新增「角色同步 = 集合语义」小节：`PUT /organizations/{org}/users/{user}/functional-roles`（body `{"roles":[...]}`）、受管码集合、非受管码保留、`role_errors` 契约 | 契约变更必须落到 api.md（前后端与审计的唯一真源） |
| `docs/spec/permissions.md` | §4.2 有 #2462 `users.role` 域语义，未定义「角色 → IAM 模板集合」映射与并存白名单 | 新增 §「#2505 角色→功能角色模板映射」：受管码 = site_admin/site_member/merchant_staff/repair_technician/merchant_admin；非受管保留 = customer/namespace_admin/自定义；并存组合以本地 `site_members.roles` 为准（repair_technician 可与 site_member/site_admin 并存） | #2505 期望 1 要求文档列出白名单 |

## Dependencies

- #2482

### 依赖与关联说明（非硬依赖）

- **#2482**（done，代码在 `issue-2482`）：提供 `mergeSiteMemberRoles` 合并语义；未合入 develop 时按「执行前置」HOLD。
- **#2505**：父任务；本单完成后由 S2（#2507）收口其余路径，父任务做集成验收。
- **#2461 / #2475**：本路径既有失败语义与 jsonb 写入修复，本单沿用不翻转。
- **beaconiam（外部前置）**：`HiJohns/beaconiam#506` —— 给 `PUT /organizations/:id/users/:user_id/functional-roles` 增加「直接父组织 OWNER/ADMIN 管理子组织成员角色」例外（Owner 2026-11-26 裁定）。该单未合并/未部署前，本单按「执行前置 2」HOLD。
- **权限合成口径**：有效权限 = 直授 ∪ 角色（`permission_calc.go:20-79` 四层按位或）；本单只改角色集合，直授字段不动。

*Model: DeepSeek V4.1 Flash 1017*



