## Accepted Plan — #2461（v2：IAM 角色更新改 JSON body + 调用点失败语义收口 + 既有 2375 mock 断言翻转登记）

> **v2 变更（采纳 2026-11-23 Plan Gap）**：① `### Files` 补 `backend/handlers/user_role_assign_2375_test.go`；② 新增 `### 被翻转的既有断言`（该 mock 原从 **query** 读 role，契约改 JSON body 后必失败，R10 否则不可达）；③ Decision Lock 增 D7（既有 mock 只改数据来源，期望值不放宽）；④ 明确 `:303/:308` 两个期望值保持不变。R1–R10 与 D1–D6 不变。

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| `UpdateUserRoleInOrg` 把 role 放 query、body 传 `nil` | `backend/services/iam_client.go:1293-1320` |
| `UpdateUserRoleInOrgWithToken` 同上 | `backend/services/iam_client.go:1458-1480` |
| beaconiam `UpdateUserRoleInOrg` 用 `c.Bind` 读 JSON body；PUT 不绑定 query → `role` 恒空 → 400 `role is required` | `beaconiam/internal/handler/org_handler.go`（契约自 `a7e2845` 改为 JSON body） |
| `BindUserToOrganization(WithToken)` 已用 JSON body（`BindUserRequest`），**不受影响** | `backend/services/iam_client.go:1171-1232` |
| 调用点 6 处，失败语义不一致：user_staff 进 `role_errors`；site_member 仅 `log`；merchant 降级仅 `log`；merchant_member / permission_manage / transit_site_admin 已 fail-fast | `backend/handlers/{user_staff,site_member,merchant,merchant_member,permission_manage,transit_site_admin}.go` |

**结论**：主因是客户端契约漂移（query vs JSON body），使所有角色同步 100% 失败；`site_member` / `merchant` 两处把失败降级成日志 → 本地改了、IAM 没改（红线：静默吞错）。

### 变更点

1. `backend/services/iam_client.go`：
   - `UpdateUserRoleInOrg(orgID, userID, role)` 改为 `PUT /api/v1/organizations/{org}/users/{user}/role`，body `{"role":"<role>"}`（`doRequest`，服务凭据）；
   - `UpdateUserRoleInOrgWithToken(token, orgID, userID, role)` 同上（`doRequestWithToken`）；
   - 错误映射保持：403 → `permission denied`、404 → `user or organization not found`、其余非 200 → 含状态码与响应体；**不再把 role 放 query**。
2. `backend/handlers/site_member.go` `UpdateMemberRole`：
   - IAM 主角色更新（`BindUserToOrganizationWithToken` / `UpdateUserRoleInOrgWithToken`）失败 → **502/50000 显式返回，且不写本地**（当前实现先 log 后继续本地写）；
   - 模板分配（`AssignRoleTemplateToUserWithToken`）失败 → 不阻断，写 `role_errors` 并在响应体返回（沿用 #2375 可见性契约）。
3. `backend/handlers/merchant.go` `UpdateMerchant`（换管理员）：
   - 新管理员 bind 失败 → 保留 `AdminPending=true` 语义并在响应 `data.admin_pending` 可见（禁止只 log）；
   - 旧管理员 demote 失败 → **补偿**：`UnbindUserFromOrganization` 解绑刚绑定的新管理员，返回 502 + `换管理员失败（旧管理员降级失败）`；补偿也失败 → 500 + `CRITICAL` 日志 + 明确 message（禁止静默）。
4. 测试：
   - **既有 mock 适配（v2）**：`backend/handlers/user_role_assign_2375_test.go:80` 由 `r.URL.Query().Get("role")` 改为解析请求 JSON body 的 `role` 字段；`:303`/`:308` 两处断言值（`repair_technician` / `STAFF`）**保持不变**，不 skip、不放宽。
   - `backend/services/iam_client_role_contract_2461_test.go`：fake IAM 断言 method=PUT、path=`/api/v1/organizations/ORG1/users/USER1/role`、body=`{"role":"STAFF"}`、带 Authorization；400 响应 → 返回错误且错误串含 `400` 与 `role is required`；服务凭据与用户令牌两条通路各一例。
   - `backend/handlers/member_role_iam_fail_2461_test.go`：site_member 主角色失败 → 502 且 `site_members.role` 不变；模板失败 → 200 + `role_errors`；merchant demote 失败 → 500 + 新管理员已解绑（`IAM_UNBIND_CALLS=1`）+ `merchants.admin_uid` 未变；user_staff IAM 失败 → 200 + `role_errors` + `users.role` 已更新。
   - `backend/handlers/role_failfast_regression_2461_test.go`：transit_site_admin(502)/permission_manage(500)/merchant_member(500) 既有 fail-fast 不变。
5. 文档：`docs/topics/iam/iam-notes.md` 增加「组织角色更新契约 = JSON body（`a7e2845` 起），tuneloop 客户端已对齐；失败语义分级（主角色 fail-fast / 模板 role_errors）」；`docs/spec/api/api.md` 在成员角色相关段落补「IAM 失败语义」一句。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 服务凭据通路 JSON body | `backend/services/iam_client.go` | IAM `PUT /api/v1/organizations/{org}/users/{user}/role` | fake IAM 200；`UpdateUserRoleInOrg` | 后端 | 请求 method=PUT、path 无 `?role=`、body `{"role":"STAFF"}`、Authorization 为服务凭据 | R1 |
| R2 用户令牌通路 JSON body | `backend/services/iam_client.go` | 同上 | fake IAM 200；`UpdateUserRoleInOrgWithToken` | 后端 | 同上且 Authorization 为传入 token | R2 |
| R3 400 不再被吞（负例） | `backend/services/iam_client.go` | 同上 | fake IAM 400 `{"error":"role is required"}` | 后端 | 返回 error，错误串同时含 `400` 与 `role is required`（调用方可判定） | R3 |
| R4 网点成员改角色 fail-fast | `backend/handlers/site_member.go` | `PUT /api/sites/:id/members/:uid` | fake IAM 500（主角色更新失败） | 后端 | HTTP 502 + code 50000；`site_members.role` 保持旧值（`LOCAL_UNCHANGED=1`） | R4 |
| R5 模板分配失败可见 | `backend/handlers/site_member.go` | 同上 | 主角色更新 200、模板分配 500 | 后端 | HTTP 200 + `role_errors` 非空（`ROLE_ERRORS=1`）、`site_members.role` 已更新 | R5 |
| R6 换管理员补偿式全成败 | `backend/handlers/merchant.go` | `PUT /api/merchants/:id`（换 `admin_uid`） | bind 新管理员 200、demote 旧管理员 500 | 后端 | HTTP 502 + message 含「旧管理员降级失败」；`IAM_UNBIND_CALLS=1`（补偿解绑）；`merchants.admin_uid` 未变 | R6 |
| R7 换管理员 bind 失败可见 | `backend/handlers/merchant.go` | 同上 | bind 新管理员 500 | 后端 | HTTP 200 + `data.admin_pending=true`（禁止只 log；`BIND_WARNING=1`） | R7 |
| R8 员工改角色 role_errors 回归 | `backend/handlers/member_role_iam_fail_2461_test.go` | `PUT /users/:id`（该用例挂载既有 handler，handler 本体不改） | fake IAM 500 | 后端 | HTTP 200 + `role_errors` 非空 + `users.role` 已更新（#2375 契约不变） | R8 |
| R9 既有 fail-fast 调用点回归 | `backend/handlers/role_failfast_regression_2461_test.go` | 三个入口（transit 成员角色 / 权限管理角色设置 / 商户成员角色；三者 handler 不改，仅回归） | fake IAM 500 | 后端 | transit=502、permission_manage=500、merchant_member=500（均未降级） | R9 |
| R10 构建与定向回归 | `backend` | `go build ./...` + 定向测试 | 修后树 | 后端 | build 0；定向测试 0 FAIL | R10 |

## Input Truth & Tool Preflight

- **Source of truth**：beaconiam `a7e2845` 后的契约 = JSON body（`c.Bind(&struct{Role string `json:"role"`})`）；Echo `DefaultBinder` 对 PUT 不解析 query（`bindData` 仅 GET/DELETE/HEAD 取 query）→ query 方案不可能成功。
- **差异核对**：`grep -rn "users/%s/role?role=" backend --include=*.go` 仅命中 `iam_client.go:1294,1459` 两处；`BindUserToOrganization*` 已是 JSON body（`BindUserRequest`），因此**爆炸半径就是这两个方法**，不需要改 bind 通路。
- **既有断言真值**：`backend/handlers/user_role_assign_2375_test.go:80` 用 `rec.lastRole = r.URL.Query().Get("role")` 采集 IAM 角色；`:303`/`:308` 断言 `repair_technician` / `STAFF`。契约改 JSON body 后该 mock 恒读空值 → `go test ./handlers` 必 FAIL → R10 `TESTS_FAIL=0` 不可达。`grep -rn 'Query().Get("role")' backend --include=*.go` 仅命中该 mock（`iam_client.go` 的两处待改点不含此模式），爆炸半径已核实。
- **调用点真值核对**（与 Issue 表格的差异）：`merchant_member.go:452`、`permission_manage.go:212`、`transit_site_admin.go:323` 实际**已 fail-fast**（log 后 `c.JSON` + `return`）；真正静默的是 `site_member.go:221`（bind 与 updateRole 两个分支）与 `merchant.go:601,608`（bind 新管理员 / demote 旧管理员）。本计划按**核实后**的结论收口，不重复修改已 fail-fast 的路径。
- **损坏输入拒绝**：`role` 为空 → 客户端不发请求（调用方已校验）；IAM 返回 400/403/404 → 返回可判别错误，禁止降级为成功。
- **工具盲区**：`merchant.go` 换管理员涉及三方状态（IAM bind/demote + 本地 `merchants`），单测只能覆盖「补偿调用发生 + 本地未提交」，真实 IAM 一致性需部署侧实测（D5）。
- **运行环境**：`handlers`/`services` 包 `TestMain` + 真实测试库；fake IAM 用 `httptest` + `services.SetIAMInternalURLForTesting`（沿用 `backend/handlers/create_user_2068_test.go:23-45` 模式）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 客户端契约唯一形态 = JSON body；不保留 query 兼容、不做双写 | Architect | 本计划锁定 | 阻塞实现 |
| D2 | 失败语义分级：**主角色同步失败 = fail-fast（本地不写）**；**模板/附加权限失败 = `role_errors` 可见**（沿用 #2375） | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 换管理员 = 补偿式全成败：bind 新 → demote 旧；demote 失败则解绑新并 502；补偿失败 → 500 + CRITICAL 日志 | Architect | 本计划锁定 | 若 Owner 要求「先本地后补偿」→ 回 Architect |
| D4 | 不改 beaconiam（契约已就绪，`a7e2845`） | Architect | 本计划锁定 | 阻塞实现 |
| D5 | 预生产实测（Issue 期望 4：日志出现 `Updated user role with user token` 且 IAM `user_org_relations.role` 变化）属**部署侧**，需 beaconiam（含 `a7e2845`）+ tuneloop 同批部署后执行 | Owner / ops | 部署完成后执行并回评测 | 未部署 → 关闭时创建 `[Ops-Followup of #2461]`（`waitForConfirm`，无 `status:*`） |
| D6 | 不改前端 `role_errors` 展示样式；本单只保证错误可见（响应体字段） | Architect | 本计划锁定 | 阻塞实现 |
| D7 | 既有 2375 mock 仅把 **数据来源** 从 query 改为 JSON body；`:303`/`:308` 期望值、调用次数断言一律不放宽，禁止 `t.Skip` / 删除用例 | Architect | 本计划锁定 | 若审计要求放宽 → 回 Architect |

## Out of Scope

- 直属员工角色落库错乱的数据成因（`users.role` 未写、`merchant_members.role` 硬编码）→ #2462。
- beaconiam 侧契约实现（已存在）；`BindUserToOrganization` 通路（已合规）。
- 前端 `role_errors` 渲染/文案改造、批量角色导入。
- 存量 `site_members`/`merchant_members` 角色漂移回填（#2462 的只读清单工具承担）。
- 预生产实测与部署（部署侧，D5）。
- 全量 `go test ./...`（由独立 Issue 的 gate 承担）。

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|---|---|---|---|
| R1 | fake IAM 记录 method/path/body/Authorization；`UpdateUserRoleInOrg("ORG1","USER1","STAFF")` | `services` 单测 | `docs/evidence/2461/R1-client-service.txt` |
| R2 | 同上，`UpdateUserRoleInOrgWithToken("tok-abc","ORG1","USER1","ADMIN")` | `services` 单测 | `docs/evidence/2461/R2-client-token.txt` |
| R3 | fake IAM 返回 400 `{"error":"role is required"}` | `services` 单测 | `docs/evidence/2461/R3-client-400.txt` |
| R4 | 网点 S（`org_id=O`）+ 成员 user（`site_members.role=site_member`）；fake IAM 对 role 更新返回 500 | `PUT /api/sites/S/members/U` | `docs/evidence/2461/R4-site-failfast.txt` |
| R5 | 同上但 role 更新 200、`AssignRoleTemplate` 返回 500 | 同上 | `docs/evidence/2461/R5-template-errors.txt` |
| R6 | 商户 M（`org_id=MO`，admin=oldUser）；body `{admin_uid:newUser}`；fake IAM：bind 200、role update 500、unbind 200 | `PUT /api/merchants/M` | `docs/evidence/2461/R6-merchant-compensate.txt` |
| R7 | 同上但 bind 返回 500 | 同上 | `docs/evidence/2461/R7-merchant-bind-pending.txt` |
| R8 | 直属/网点用户（`users.role=site_member`）；fake IAM role 更新 500 | `PUT /users/:id` | `docs/evidence/2461/R8-staff-role-errors.txt` |
| R9 | transit 成员 / 权限管理角色设置 / 商户成员角色；fake IAM 500 | 三个入口 | `docs/evidence/2461/R9-failfast-regression.txt` |
| R10 | 修后 backend | `go build ./...` + 定向测试 | `docs/evidence/2461/R10-build.txt` |

### Assertions

- R1：`METHOD=PUT`、`PATH=/api/v1/organizations/ORG1/users/USER1/role`、`HAS_QUERY=0`、`BODY={"role":"STAFF"}`、`AUTH=service`；负例：`HAS_QUERY=1`（仍用 query）→ FAIL。
- R2：同上且 `AUTH=user-token`、`BODY={"role":"ADMIN"}`；负例：`BODY=`（nil body）→ FAIL。
- R3：`ERR_CONTAINS_400=1`、`ERR_CONTAINS_ROLE_REQUIRED=1`；负例：错误为空/被吞 → FAIL。
- R4：`STATUS=502`、`LOCAL_UNCHANGED=1`、`IAM_ROLE_CALLS=1`；负例：`STATUS=200` 或本地被改 → FAIL。
- R5：`STATUS=200`、`ROLE_ERRORS=1`、`LOCAL_ROLE=site_admin`；负例：`ROLE_ERRORS=0`（静默）→ FAIL。
- R6：`STATUS=502`、`MSG_CONTAINS=旧管理员降级失败`、`IAM_UNBIND_CALLS=1`、`ADMIN_UID_UNCHANGED=1`；负例：`STATUS=200`（吞掉失败）→ FAIL。
- R7：`STATUS=200`、`ADMIN_PENDING=true`；负例：`ADMIN_PENDING=false` 且无告警字段 → FAIL。
- R8：`STATUS=200`、`ROLE_ERRORS=1`、`USERS_ROLE_UPDATED=1`；负例：`ROLE_ERRORS=0` → FAIL。
- R9：`TRANSIT=502`、`PERMISSION=500`、`MERCHANT_MEMBER=500`；负例：任一处返回 200 → FAIL。
- R10：`BUILD_EXIT=0`、`TESTS_FAIL=0`；负例：任一非 0 → FAIL。
- 既有断言（v2）：`user_role_assign_2375_test.go:303` 仍断言 `repair_technician`、`:308` 仍断言 `STAFF` 且 `roleCalls=2`；负例：两处期望被改/用例被 skip → FAIL。
- 通用 provenance：每个 artifact 首行 `# build=1.0.<fix> # commit=<sha> # dirty=false`。
- 负例（跨行）：任何 artifact 出现 `?role=` 或 `log only`/`仅记日志` 语义 → FAIL。

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2461/R1-client-service.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2461/R2-client-token.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2461/R3-client-400.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2461/R4-site-failfast.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2461/R5-template-errors.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2461/R6-merchant-compensate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2461/R7-merchant-bind-pending.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2461/R8-staff-role-errors.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2461/R9-failfast-regression.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2461/R10-build.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### 被翻转的既有断言（AGENTS.md §语义变更，强制）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `backend/handlers/user_role_assign_2375_test.go:80` | `rec.lastRole = r.URL.Query().Get("role")`（从 query 采集） | 解析请求 JSON body 的 `role` 字段后写入 `rec.lastRole` | 客户端契约改为 JSON body（beaconiam `a7e2845`，D1）；继续读 query 会使断言恒为空 |
| `backend/handlers/user_role_assign_2375_test.go:303` | `lastRole == "repair_technician"` | **保持** `"repair_technician"`（仅数据来源变化） | 角色映射 `toIAMRole("repair_technician")` 未变 |
| `backend/handlers/user_role_assign_2375_test.go:308` | `lastRole == "STAFF"`、`roleCalls == 2` | **保持** `"STAFF"` 与 `2`（仅数据来源变化） | `toIAMRole("merchant_staff") = "STAFF"` 未变 |

**禁止**：删除/跳过该用例、放宽两处期望值、或把 body 解析写成"读到空也算通过"。

**本单不翻转的同类断言（已逐点核实，避免误伤）**：
- `backend/handlers/site_member_test.go`（`UpdateMemberRole` 用例）：`users.iam_sub` 留空 → 不走 IAM 分支，R4 fail-fast 改造不影响其断言。
- `backend/handlers/merchant_setup_test.go:330`（同挂 `UpdateMemberRole`）：测试 user 无 `iam_sub` → 同上。
- `grep -rn 'UpdateMerchant\b' backend/handlers/*_test.go` → 无既有用例；R6/R7 为新增用例，不构成翻转。

## Bug Reflection Matrix

| symptom | trigger | root cause | test blind spot | same-class paths | fix layer | red-green | prevention |
|---|---|---|---|---|---|---|---|
| 改角色提示「部分权限设置失败」，IAM 实际未变 | 任何角色修改 | 客户端 query vs IAM JSON body | 无客户端契约单测（路径/body 断言） | 所有 `UpdateUserRoleInOrg*` 调用 | IAM client | R1–R3 先写契约用例（旧实现 HAS_QUERY=1 → FAIL）再改 | D1 契约固化 + 单测 |
| 本地已改、IAM 未改（权限漂移） | site_member 改角色 | 失败仅 `log` 后继续本地写 | 无 IAM 失败用例 | merchant 换管理员 | handler | R4 红绿 | D2 fail-fast |
| 旧管理员仍保留 OWNER | 换商户管理员 | demote 失败仅 log | 无补偿用例 | 解绑/降级类操作 | handler + 补偿 | R6 红绿 | D3 补偿式全成败 |
| 新管理员绑定失败无感知 | 换商户管理员 | 只设 `AdminPending` 且无响应提示 | 无 pending 可见性断言 | 邀请直达管理员 | handler | R7 红绿（`admin_pending=true`） | 响应字段可见 |
| 模板分配失败静默 | 多角色成员 | 模板失败仅 log | 无 role_errors 断言 | #2375 角色模板链 | handler | R5 红绿 | D2 分级 |
| 「已 fail-fast」误判重复改造 | 审计沿用 Issue 表格 | 未逐调用点核实 | — | merchant_member/permission/transit | 计划 | R9 断言保持不变 | Input Truth 逐点核对 |
| 400 错误信息不可诊断 | IAM 400 | 客户端未保留响应体 | 无负例 | 403/404 映射 | IAM client | R3 红绿 | 错误串含状态码+body |
| 修复后角色同步仍失败（部署滞后） | 只发布 tuneloop | 跨仓版本错配 | 无部署侧验证 | IAM 其他契约 | 部署 | D5 Ops Follow-up | 部署顺序写进 iam-notes |
| 改完客户端后既有 2375 用例必失败 | 跑 `go test ./handlers` | 既有 mock 从 query 采角色 | 计划未登记该 mock 适配 | 其它按 query 断言 body 的 mock（已 grep 确认无） | 测试适配 | R10 `TESTS_FAIL=0` 红绿 | v2 明确登记 + D7 |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2461.sh r1` | `docs/evidence/2461/R1-client-service.txt` | exit 0；grep `METHOD=PUT`、`HAS_QUERY=0`、`BODY={"role":"STAFF"}` | PASS | `HAS_QUERY=1` → FAIL | dev |
| R2 | `bash scripts/verify-2461.sh r2` | `docs/evidence/2461/R2-client-token.txt` | exit 0；grep `AUTH=user-token`、`BODY={"role":"ADMIN"}` | PASS | `BODY=` 空 → FAIL | dev |
| R3 | `bash scripts/verify-2461.sh r3` | `docs/evidence/2461/R3-client-400.txt` | exit 0；grep `ERR_CONTAINS_400=1`、`ERR_CONTAINS_ROLE_REQUIRED=1` | PASS | 错误被吞 → FAIL | dev |
| R4 | `bash scripts/verify-2461.sh r4` | `docs/evidence/2461/R4-site-failfast.txt` | exit 0；grep `STATUS=502`、`LOCAL_UNCHANGED=1` | PASS | `STATUS=200` 或本地被改 → FAIL | dev |
| R5 | `bash scripts/verify-2461.sh r5` | `docs/evidence/2461/R5-template-errors.txt` | exit 0；grep `ROLE_ERRORS=1`、`LOCAL_ROLE=site_admin` | PASS | `ROLE_ERRORS=0` → FAIL | dev |
| R6 | `bash scripts/verify-2461.sh r6` | `docs/evidence/2461/R6-merchant-compensate.txt` | exit 0；grep `IAM_UNBIND_CALLS=1`、`ADMIN_UID_UNCHANGED=1`、`STATUS=502` | PASS | `STATUS=200` → FAIL | dev |
| R7 | `bash scripts/verify-2461.sh r7` | `docs/evidence/2461/R7-merchant-bind-pending.txt` | exit 0；grep `ADMIN_PENDING=true`、`STATUS=200` | PASS | `ADMIN_PENDING=false` → FAIL | dev |
| R8 | `bash scripts/verify-2461.sh r8` | `docs/evidence/2461/R8-staff-role-errors.txt` | exit 0；grep `ROLE_ERRORS=1`、`USERS_ROLE_UPDATED=1` | PASS | `ROLE_ERRORS=0` → FAIL | dev |
| R9 | `bash scripts/verify-2461.sh r9` | `docs/evidence/2461/R9-failfast-regression.txt` | exit 0；grep `TRANSIT=502`、`PERMISSION=500`、`MERCHANT_MEMBER=500` | PASS | 任一处 200 → FAIL | dev |
| R10 | `bash scripts/verify-2461.sh r10` | `docs/evidence/2461/R10-build.txt` | exit 0；grep `BUILD_EXIT=0`、`TESTS_FAIL=0` | PASS | 任一非 0 → FAIL | dev |

### Workload

- Size: M
- Basis: 3 个实现文件（`iam_client.go`、`site_member.go`、`merchant.go`）+ 3 个新测试文件 + **1 个既有测试适配（2375 mock，仅数据来源）** + 1 个 verify 脚本 + 2 篇文档 + 10 条证据行；无 migration、无前端改动
- Confidence: medium（`merchant.go` 补偿路径需 IAM 三态 stub；预生产实测属部署侧）

### Files（变更文件清单）

- `backend/services/iam_client.go`
- `backend/handlers/site_member.go`
- `backend/handlers/merchant.go`
- `backend/services/iam_client_role_contract_2461_test.go`
- `backend/handlers/member_role_iam_fail_2461_test.go`
- `backend/handlers/role_failfast_regression_2461_test.go`
- `backend/handlers/user_role_assign_2375_test.go`
- `scripts/verify-2461.sh`
- `docs/topics/iam/iam-notes.md`
- `docs/spec/api/api.md`
- `docs/evidence/2461/plan.md`
- `docs/evidence/2461/R1-client-service.txt`
- `docs/evidence/2461/R2-client-token.txt`
- `docs/evidence/2461/R3-client-400.txt`
- `docs/evidence/2461/R4-site-failfast.txt`
- `docs/evidence/2461/R5-template-errors.txt`
- `docs/evidence/2461/R6-merchant-compensate.txt`
- `docs/evidence/2461/R7-merchant-bind-pending.txt`
- `docs/evidence/2461/R8-staff-role-errors.txt`
- `docs/evidence/2461/R9-failfast-regression.txt`
- `docs/evidence/2461/R10-build.txt`
- `docs/evidence/2461/evidence.json`

## Dependencies

- 无代码前置。**关联 #2462**（直属员工域角色落库）：#2462 减少「无变化也同步」的无效调用，但两单互不阻塞。
- 部署侧：预生产实测需 beaconiam（含 `a7e2845`）与 tuneloop 同批部署；未部署前不得宣告「角色同步已恢复」→ 关闭时创建 `[Ops-Followup of #2461]`（`waitForConfirm`，无 `status:*`）。
- 关联 #2375（`role_errors` 契约）、#2035（多角色模板分配）。

*Model: DeepSeek V4.1 Flash 1017*


