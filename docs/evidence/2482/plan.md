## Accepted Plan — #2482（改主角色时保留其它角色：`site_members.roles` 不再被压成单值）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| `roleChanged` 分支把 `site_members.roles` 写为**单值** `[newRole]` | `backend/handlers/user_staff.go:1226-1237`（`siteMemberRolesExpr([]string{newRole})` + `WHERE user_id AND tenant_id` 整批覆盖） |
| `roles` 语义是**全集**（主角色 `role` + 其余身份，如「员工 + 维修师傅」） | `backend/models/models.go:1071-1079`（#2034/#2035） |
| 修复前该写入恒报 `22P02`（写入失败 → `roles` 反而保留多角色，仅前端弹警告）；#2475 让写入成功后，**压缩语义生效** | #2475（已合入 develop，`siteMemberRolesExpr` 存在） |
| `site_members.roles` 是**授权判定来源**：`hasSiteRole(..., "repair_technician")` | `backend/handlers/repair.go:596-615`（调用点 `:275`、`:334`）、`backend/handlers/repair_request.go:810`；另 `siteAdminMerchantTenants` 以 `roles` 判 `site_admin`（`repair_service.go:843-860`） |
| IAM 侧**没有**解除角色模板的 API（只有 `AssignRoleTemplateToUser*`）→ 旧功能角色留在 IAM；本地被压缩后 → 本地拒维修、IAM 仍授权 | `backend/services/iam_client.go:1857-1885` |
| 多角色编辑入口（保留全集）另外存在 | `backend/handlers/site_member.go:174,268`（`PUT /api/sites/:id/members/:uid`） |

**结论**：`UpdateUser` 改主角色时不应**替换**全集，而应**合并**：保留其它角色、原位替换旧主角色、保证 `roles` 始终含新主角色。这样本地与 IAM（旧模板未被移除）**一致**，维修能力不再被本地门禁误拒。若 Owner 的真实语义是「改角色 = 整体替换」，则必须同时移除 IAM 侧旧模板（需 beaconiam unassign 端点）→ 见 D2 备选，超出本单。

### 变更点

1. `backend/handlers/site_member_common.go` 新增纯函数（可脱离 DB 单测）：
   ```go
   // mergeSiteMemberRoles 计算改主角色后的角色全集（#2482）：
   // 去空白/去重保序；保留其它角色；旧主角色原位替换为 newPrimary；结果必含 newPrimary。
   func mergeSiteMemberRoles(existing []string, oldPrimary, newPrimary string) []string
   ```
   - `oldPrimary` 不在 `existing` → 追加 `newPrimary`；
   - `existing` 为空/全空串 → `[]string{newPrimary}`；
   - `newPrimary == ""` → 原样返回（防御，不制造空主角色）。
2. `backend/handlers/user_staff.go`（`roleChanged` 分支）：
   - 改为**逐行**读取 `SELECT id, roles::text FROM site_members WHERE user_id = ? AND tenant_id = ?`；
   - 对每行 `merged := mergeSiteMemberRoles(row.Roles, existingUser.Role, newRole)`，用既有 `siteMemberRolesExpr(merged)` 写回 `{"role": newRole, "roles": expr}`（`WHERE id = ?`）；
   - 失败语义保持 #2475 D2：编码/写库失败 → `role_errors`（可见、不阻断其它字段）；
   - `users.role`、`merchant_members.role` 的既有单主角色写入**不变**（`role` 仍是主角色）。
3. 测试 `backend/handlers/user_staff_role_set_2482_test.go`：
   - R1 纯函数 5 类输入；R2 handler 红绿（双角色 → 保留 `repair_technician`）；R3 多网点逐行；R4 invariant（`roles` 含主角色、jsonb 合法、NOT NULL）；R5 非翻转回归（`roleChanged=false` 不写）。
4. 不改 IAM 调用序列（仍 `UpdateUserRoleInOrgWithToken` + `AssignRoleTemplateToUserWithToken(newRole)`），**不新增** unassign 调用（IAM 无该能力）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 合并纯函数 | `backend/handlers/site_member_common.go` | `go test ./handlers -run TestMergeSiteMemberRoles_2482` | 5 类输入（含空/重复/旧主缺失/新主已存在） | 后端 | 保留其它角色、原位替换、去重保序、必含新主角色 | R1 |
| R2 handler 红绿（主诉） | `backend/handlers/user_staff.go`、`backend/handlers/user_staff_role_set_2482_test.go` | `PUT /api/users/:id`（`UpdateUser`） | `site_members.roles=["site_member","repair_technician"]`，改 `role=site_admin` | 后端 | 改后 `role=site_admin`、`roles=["site_admin","repair_technician"]`；旧实现为 `["site_admin"]`（红） | R2 |
| R3 多网点逐行合并 | `backend/handlers/user_staff.go` | 同上 | 用户两行 `site_members`，roles 分别为 `[site_member,repair_technician]`、`[site_member]` | 后端 | 两行各自保留其其它角色，均含新主角色；`ROW1_OK=1`、`ROW2_OK=1` | R3 |
| R4 invariant（jsonb/NOT NULL/主角色包含） | `backend/handlers/user_staff.go`、`backend/handlers/site_member_common.go` | 同上 | 任意改角色 | 后端 | `roles` 合法 jsonb、非 NULL、包含 `role`；`NO_22P02=1`、`INVARIANT=1` | R4 |
| R5 既有语义不翻转 | `backend/handlers/user_staff.go` | 同上 | `roleChanged=false`（只改职位）；单角色 `["site_member"]` → `site_admin` | 后端 | 未变化时不写 `roles`、不调 IAM（#2462 R5）；单角色场景结果 `["site_admin"]`（无残留旧角色） | R5 |
| R6 IAM 双源一致（无 unassign 调用） | `backend/handlers/user_staff.go` | 同上 | 同 R2 | 后端 | `IAM_ASSIGN_CALLS=1`、`IAM_UNASSIGN_CALLS=0`；本地 `roles` 为 IAM 功能角色的超集/等价 → `DUAL_SOURCE_OK=1` | R6 |
| R7 构建与定向回归 | `backend` | `go build ./...` + 定向测试（2482 + 2475 + 2375 + 修复流程） | 修后树 | 后端 | `BUILD_EXIT=0`、`TESTS_FAIL=0` | R7 |
| R8 gate 四源 + 全量套件 | `docs/evidence/2482/pre-ready-2482.json`、`docs/evidence/2482/plan.md` | `--full-gate-cmd "cd backend && go test ./handlers -count=1 -timeout 15m && go test ./services -count=1 -timeout 15m"` + `plan-source.sh --issue 2482` | 修后树（干净） | 后端 | `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | R8 |

## Input Truth & Tool Preflight

- **Source of truth**：`site_members.roles` = 身份全集（#2034/#2035）+ `role` = 主角色；授权门禁以 `roles`（`SiteMember.EffectiveRoles()`）为准（`models.go:1093-1100`、`repair.go:596`）。
- **差异核对**：`grep -rn '"roles"' backend --include=*.go` → map 写 `roles` 仅 `user_staff.go`（本单改）与 `iam_proxy.go`（#2475 已修 IAM 纠偏，单角色映射**语义正确**，本单不动）；`site_member.go:268` 为多角色编辑入口（保留全集，不动）。
- **损坏输入拒绝**：`existing` 含空串/重复 → 去重去空；`newPrimary` 为空 → 原样返回（不写空主角色，`roleChanged` 已保证 `newRole != ""`）；读取 `roles::text` 解析失败 → 该行进 `role_errors`（不静默）。
- **工具盲区**：`iam_proxy.ensureSiteMember` 的 IAM 纠偏路径**故意**写单角色映射（IAM 是该路径的权威源）；本单只改「人员管理改角色」路径 → 两路径语义差异在证据中显式记录，避免审计误判为漏改。
- **影响面预检**：`UpdateUser` 的 `roles` 写入范围从「整批 WHERE user_id+tenant_id」改为「逐行 WHERE id」→ 行数不变、无新增查询爆炸（同用户网点行数 ≤ 少数）；`users.role`/`merchant_members.role` 不变。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 语义：改主角色 = **合并**（保留其它角色），不是整体替换 | Owner / Architect | 本计划锁定（Issue 期望 1；且与 IAM 无 unassign 能力的事实一致） | 若 Owner 要求「整体替换」→ 回 Architect（需 beaconiam unassign 端点 + 前端多角色编辑） |
| D2 | IAM 侧旧模板**不移除**（无 API）；本单通过「本地保留同角色」达成双源一致 | Architect | 本计划锁定 | 若要求移除 → 另立 beaconiam Issue（新增 unassign 端点） |
| D3 | 逐行合并（不同网点行可有不同 `roles`），不使用单次整批 UPDATE | Architect | 本计划锁定 | 阻塞实现 |
| D4 | 单角色场景必须保持「无残留旧角色」（`["site_member"]` → `["site_admin"]`），避免把「合并」误实现为「只追加」 | Architect | 本计划锁定 | 阻塞实现 |
| D5 | `iam_proxy` 纠偏路径（IAM 权威）写单角色映射**不变**，与本单差异显式记录 | Architect | 本计划锁定 | 若要求统一 → 回 Architect（会与 IAM 权威冲突） |

## Out of Scope

- IAM 侧角色模板移除（需 beaconiam 新端点）→ 另立跨仓 Issue（D2）。
- 前端多角色编辑 UI（#2035 交互）。
- `iam_proxy.ensureSiteMember` 的单角色映射语义（D5）。
- 存量 `users.role=''` 回填（#2462/#2483）。
- 双归属判定（#2483）。
- 全量 `go test ./...` 作为合同行（由 R8 gate 承担）。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `mergeSiteMemberRoles(["site_member","repair_technician"],"site_member","site_admin")`；`(["site_member","repair_technician"],"site_member","repair_technician")`；`(["x"],"missing","site_admin")`；`([],"" ,"site_admin")`；`(["a"],"a","")` | `go test` | `docs/evidence/2482/R1-merge-fn.txt` | 四态分别 `[site_admin,repair_technician]`、去重后不含重复、追加、`[site_admin]`、原样 `[a]`；`ORDER_STABLE=1` | 丢 `repair_technician` → FAIL |
| R2 | 用户 U、网点 S：`site_members(role=site_member, roles=["site_member","repair_technician"])`；fake IAM 200 | `PUT /api/users/:id` `{"role":"site_admin"}` | `docs/evidence/2482/R2-handler.txt` | `ROLES_JSON=["site_admin","repair_technician"]`、`ROLE=site_admin`、`RED_GREEN=1`（旧实现 `["site_admin"]`） | 仍丢角色 → FAIL |
| R3 | 同一用户两行网点，roles 分别为含/不含 `repair_technician` | 同上 | `docs/evidence/2482/R3-multi-site.txt` | 两行各自保留；`ROW1_OK=1`、`ROW2_OK=1` | 被统一覆盖 → FAIL |
| R4 | 同 R2 | 同上 | `docs/evidence/2482/R4-invariant.txt` | `NO_22P02=1`、`INVARIANT=1`、`ROLES_NOT_NULL=1` | `22P02` 或 NULL → FAIL |
| R5 | (a) `role=site_member` 不改角色只改职位；(b) `roles=["site_member"]` 改 `site_admin` | 同上 | `docs/evidence/2482/R5-no-flip.txt` | (a) `IAM_ROLE_CALLS=0`、`ROLES_UNCHANGED=1`；(b) `ROLES_JSON=["site_admin"]`、`NO_OLD_RESIDUE=1` | 只追加旧角色残留 → FAIL |
| R6 | 同 R2，fake IAM 记录 assign | 同上 | `docs/evidence/2482/R6-dual-source.txt` | `IAM_ASSIGN_CALLS=1`、`IAM_UNASSIGN_CALLS=0`、`DUAL_SOURCE_OK=1` | 新增 unassign 调用 → FAIL |
| R7 | 修后 backend | `go build ./...` + 定向测试 | `docs/evidence/2482/R7-build.txt` | `BUILD_EXIT=0`、`TESTS_FAIL=0` | 任一非 0 → FAIL |
| R8 | 修后干净树 + gate | 读校验 | `docs/evidence/2482/R8-gate.txt` | `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | `P5=NA` → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2482.sh r1` | `docs/evidence/2482/R1-merge-fn.txt` | exit 0；grep `CASE1=`、`ORDER_STABLE=1`、`EMPTY_NEW=` | PASS | 丢角色 → FAIL | dev |
| R2 | `bash scripts/verify-2482.sh r2` | `docs/evidence/2482/R2-handler.txt` | exit 0；grep `ROLES_JSON=["site_admin","repair_technician"]`、`RED_GREEN=1` | PASS | 丢 `repair_technician` → FAIL | dev |
| R3 | `bash scripts/verify-2482.sh r3` | `docs/evidence/2482/R3-multi-site.txt` | exit 0；grep `ROW1_OK=1`、`ROW2_OK=1` | PASS | 统一覆盖 → FAIL | dev |
| R4 | `bash scripts/verify-2482.sh r4` | `docs/evidence/2482/R4-invariant.txt` | exit 0；grep `NO_22P02=1`、`INVARIANT=1` | PASS | `22P02` → FAIL | dev |
| R5 | `bash scripts/verify-2482.sh r5` | `docs/evidence/2482/R5-no-flip.txt` | exit 0；grep `IAM_ROLE_CALLS=0`、`NO_OLD_RESIDUE=1` | PASS | 旧角色残留 → FAIL | dev |
| R6 | `bash scripts/verify-2482.sh r6` | `docs/evidence/2482/R6-dual-source.txt` | exit 0；grep `IAM_UNASSIGN_CALLS=0`、`DUAL_SOURCE_OK=1` | PASS | 调用 unassign → FAIL | dev |
| R7 | `bash scripts/verify-2482.sh r7` | `docs/evidence/2482/R7-build.txt` | exit 0；grep `BUILD_EXIT=0`、`TESTS_FAIL=0` | PASS | 任一非 0 → FAIL | dev |
| R8 | `bash scripts/verify-2482.sh r8` | `docs/evidence/2482/R8-gate.txt` | exit 0；grep `GATE_RESULT=PASS`、`PLAN_SOURCE=PASS` | PASS | `P5=NA` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2482/R1-merge-fn.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2482/R2-handler.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2482/R3-multi-site.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2482/R4-invariant.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2482/R5-no-flip.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2482/R6-dual-source.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2482/R7-build.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2482/R8-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 | 触发条件 | 根因 | 修复点 | 覆盖行 | 负例断言 |
|---|---|---|---|---|---|---|
| B1 | 改角色后维修能力本地被拒 | 用户在网点有「员工+维修师」双角色 | `roles` 被写为单值 | 变更点 1/2 | R2 | 丢 `repair_technician` → R2 失败 |
| B2 | 本地与 IAM 双源不一致 | IAM 旧模板未移除 | 本地压缩 vs IAM 保留 | 变更点 1（合并）+ D2 | R6 | `DUAL_SOURCE_OK=0` → R6 失败 |
| B3 | 多网点用户被统一覆盖 | 两行 `site_members` | 整批 UPDATE | 变更点 2（逐行） | R3 | `ROW1_OK=0` → R3 失败 |
| B4 | 只追加不替换 → 旧主角色残留 | 单角色改主角色 | 合并实现错误 | 变更点 1 + D4 | R5 | 残留 → R5 失败 |
| B5 | jsonb 写入回归 22P02 | 改 `roles` 序列化 | 误改 helper | 变更点 2 | R4 | `22P02` → R4 失败 |
| B6 | 只改职位也写 `roles` | `roleChanged=false` | 门控失效 | 变更点 2（保持门控） | R5 | `IAM_ROLE_CALLS>0` → R5 失败 |
| B7 | 违反 NOT NULL | 合并结果为空 | 空集合处理 | 变更点 1（空 → `[new]`） | R4 | `ROLES_NOT_NULL=0` → R4 失败 |
| B8 | 静默吞错 | 逐行写失败 | 无 `role_errors` | 变更点 2（沿用 #2475 D2） | R2/R4 | 失败无 `role_errors` → R2 失败 |
| B9 | 把 IAM 纠偏路径一起改错 | 误改 `iam_proxy` | 未区分两条路径 | D5 | R2/R6 | `iam_proxy` 改动 → R6 失败 |

### Workload

- Size: M
- Basis: 1 个纯函数（`site_member_common.go`）+ 1 个 handler 分支改造（逐行合并）+ 1 个测试文件 + 1 个 verify 脚本 + 8 条证据行；无 migration、无前端
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/site_member_common.go`
- `backend/handlers/user_staff.go`
- `backend/handlers/user_staff_role_set_2482_test.go`
- `scripts/verify-2482.sh`
- `docs/evidence/2482/plan.md`
- `docs/evidence/2482/pre-ready-2482.json`
- `docs/evidence/2482/R1-merge-fn.txt`
- `docs/evidence/2482/R2-handler.txt`
- `docs/evidence/2482/R3-multi-site.txt`
- `docs/evidence/2482/R4-invariant.txt`
- `docs/evidence/2482/R5-no-flip.txt`
- `docs/evidence/2482/R6-dual-source.txt`
- `docs/evidence/2482/R7-build.txt`
- `docs/evidence/2482/R8-gate.txt`
- `docs/evidence/2482/evidence.json`

## Dependencies

- 无

### 依赖与关联说明（非硬依赖）

- **#2475（done，已在 develop）**：提供 `siteMemberRolesExpr`（jsonb 序列化）——本单在其之上做「合并」而非「单值」。
- **#2483（accepted）**：双归属编辑页解锁（同属人员域，但不同层：本单改角色集合，后者改编辑页模式）。
- **#2034 / #2035**：`roles` 全集语义与多角色编辑入口（`PUT /api/sites/:id/members/:uid`，本单不改）。
- **#2472 / #2473（Ops）**：预生产回执中若涉及改角色，需在本单合入后复测 `roles` 不再被压缩。
- **beaconiam（另立）**：若不采用 D1（合并），而要求「改角色=整体替换」，需 beaconiam 提供解除角色模板端点 → 届时另立 Issue。
- **部署侧**：预生产「双角色员工改角色后仍保留维修能力」回评由 `[Ops-Followup of #2482]` 承载。

*Model: DeepSeek V4.1 Flash 1017*

