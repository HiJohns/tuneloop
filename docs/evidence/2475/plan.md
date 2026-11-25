## Accepted Plan — #2475（`site_members.roles` jsonb 写入修复：map Update 不再传 Go `[]string`）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| `SiteMember.Roles` 是 **jsonb** + `serializer:json`（**模型级** Create/Struct Update 正常） | `backend/models/models.go:1074` |
| GORM **map Update 绕过 serializer**：`Updates(map{"roles": []string{...}})` 会渲染成 SQL 数组 `('site_member')` → 对 jsonb 列报 `22P02` | GORM 既有行为；预生产日志实证见 #2475 正文 |
| **缺陷点 1（本次复现）**：改角色时同步 `site_members.roles` | `backend/handlers/user_staff.go:1228` |
| **缺陷点 2（同类）**：IAM 同步纠偏角色时写 `roles` | `backend/handlers/iam_proxy.go:1077` |
| **已正确写法（可复用范式）** | `backend/handlers/site_member.go:266-268`：`json.Marshal` + `gorm.Expr("?::jsonb", …)` |
| 全仓扫描结论 | `grep -rn '"roles"' backend --include=*.go` 仅上述两处为 map 写入 `[]string`；其余为模型 struct 赋值（走 serializer）或读取 |

**结论**：写入语义缺一层 JSON 序列化；同类路径两处，必须一起修（只修被点名一处 → 违反同类扫描规则）。

### 变更点

1. 新增共享 helper（放在 `backend/handlers/site_member_common.go`，供两处复用，避免再复制 sql 片段）：
   ```go
   // siteMemberRolesExpr 把角色集合序列化为 jsonb 表达式（map Update 不走 serializer，#2475）。
   func siteMemberRolesExpr(roles []string) (gorm.Expr, error)
   ```
   语义：去空串 + 去重保序；空集合 → `[]`（避免 NOT NULL 违约）；返回 `gorm.Expr("?::jsonb", string(b))`。
2. `backend/handlers/user_staff.go:1228`：改用 helper；序列化失败 → 进 `role_errors`（可见，不静默），**不阻断**其它字段更新（沿用 #2375 契约）。
3. `backend/handlers/iam_proxy.go:1077`：改用 helper；失败 → `log.Printf` + 跳过该成员的 roles 修正（保留 `role` 主列更新），并在返回摘要中计入失败数（若该函数已有统计结构则复用，否则至少日志含 user/site）。
4. 测试：`backend/handlers/site_member_roles_jsonb_2475_test.go`
   - R1 user_staff 路径红绿（缺陷版：`Updates` 直接传 `[]string` → 断言 `22P02`；修复版：合法 JSON 数组）
   - R2 iam_proxy `ensureSiteMember` 纠偏路径（有既有 role 且与 IAM 不一致时写 roles → 合法 JSON）
   - R3 多重角色场景：写入后 `SELECT roles FROM site_members` 解析为 `["site_member"]`（单主角色语义不变），且列 `NOT NULL` 保持
   - R4 既有正确路径回归：`site_member.UpdateMemberRole`（多角色 jsonb）仍通过

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 改角色同步 roles（user_staff） | `backend/handlers/user_staff.go`、`backend/handlers/site_member_common.go` | `PUT /users/:id` | 目标用户已有 `site_members` 行，改角色 `site_member → site_admin` | 后端 | 不再出现 `22P02`；`roles` 为合法 JSON 数组 `["site_admin"]`；`role=site_admin`；`role_errors` 为空 | R1 |
| R2 IAM 纠偏同步 roles（iam_proxy） | `backend/handlers/iam_proxy.go` | `POST /iam/users/sync`（`ensureSiteMember`） | 本地 role=`site_member`、IAM role=ADMIN（不一致） | 后端 | 写入成功，`roles=["site_admin"]`、`role=site_admin`；无 `22P02` | R2 |
| R3 多角色与 NOT NULL 保持 | `backend/handlers/site_member_common.go` | 单测（helper） | 输入 `["site_member","site_member","","repair_technician"]`、空集合 `[]`、`nil` | 后端 | 去重保序 → `["site_member","repair_technician"]`；空/nil → `[]`（非 NULL） | R3 |
| R4 既有正确路径回归 | `backend/handlers/site_member.go` | `PUT /api/sites/:id/members/:uid` | 多角色 `roles=["site_member","repair_technician"]` | 后端 | 行为不变（仍走 `?::jsonb`），断言与 #2035 一致 | R4 |
| R5 gate 四源一致 + 全量套件 | `docs/evidence/2475/pre-ready-2475.json`、`docs/evidence/2475/plan.md` | `/work` 以 `--full-gate-cmd` 跑 pre-ready（`cd backend && go test ./handlers -count=1 -timeout 15m` 与 `./services`），随后 `plan-source.sh --issue 2475` | 修后树（干净） | 后端 | gate `result=PASS` 且含 `FULL_SUITE_EXIT=0 FAIL_COUNT=0 PANIC_COUNT=0`；`PLAN_SOURCE: PASS` | R5 |
| R6 构建与定向回归 | `backend` | `go build ./...` + 定向测试（2475 + 2035 + 2375） | 修后树 | 后端 | `BUILD_EXIT=0`、`TESTS_FAIL=0` | R6 |

## Input Truth & Tool Preflight

- **Source of truth**：`site_members.roles` 列类型 `jsonb NOT NULL DEFAULT '[]'`（`information_schema` 与 `models.go:1074` 双证）；多重角色语义见 #2034/#2035（主角色 `role` + 全集 `roles`）。
- **差异核对**：`site_member.go`（HTTP 改角色）已正确序列化；`user_staff.go` / `iam_proxy.go` 两处 map Update 未序列化 → 预生产 `22P02` 实证。**不是**列/模型定义问题，故**无 migration**。
- **损坏输入拒绝**：`roles` 中含空串 → 剔除；全为空 → 写 `[]`；JSON 序列化失败（理论不可达）→ 显式错误/`role_errors`，禁止吞掉。
- **工具盲区**：GORM `serializer:json` 只覆盖 struct Create/`Updates(&struct)` 路径，map Update 一律绕过——测试必须**走 handler 入口**（而非直接 `db.Model().Updates(&struct)`），否则无法复现缺陷。
- **影响面预检**：其它写 `roles` 的路径均为 struct 赋值（`staff_invite.go:358`、`site_member_common.go:364`、`site.go:483`、`confirmation_session.go:383`、`user_staff.go:518`、`iam_proxy.go:1089`）→ 走 serializer，不改。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 修复方式：新增 `siteMemberRolesExpr` helper（`json.Marshal` + `gorm.Expr("?::jsonb", …)`），两处 map Update 统一复用；不引入 `datatypes.JSON` 列类型变更 | Architect | 本计划锁定 | 阻塞实现 |
| D2 | 失败语义：user_staff 路径失败 → `role_errors`（可见、不阻断其它字段）；iam_proxy 路径失败 → 日志 + 跳过该成员 roles（`role` 主列仍更新），不使整批同步 500 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 语义不变：`role`=主角色、`roles`=全集；本单**不**把改角色扩展成多角色编辑（#2035 交互另行） | Architect | 本计划锁定 | 若要求多角色编辑 → 回 Architect |
| D4 | 无 migration：列已为 jsonb，仅修正写入序列化 | Architect | 本计划锁定 | 若发现列类型不符 → 回 Architect |

## Out of Scope

- #2035 多角色前端交互（站点成员编辑多角色）扩展。
- `site_members` 表结构/约束变更（无 migration）。
- IAM 侧角色同步契约（#2461 已 done）。
- 角色模板定义与权限位。
- 全量 `go test ./...` 作为合同行（由 R5 gate 承担）。

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|---|---|---|---|
| R1 | 租户 T、网点 S、用户 U（`site_members` 行 role=site_member, roles=["site_member"]）；body `{role:"site_admin"}`；fake IAM 200 | `PUT /users/:id`（`UpdateUser`） | `docs/evidence/2475/R1-staff-roles.txt` |
| R2 | 用户 U 本地 role=site_member；fake IAM 返回 role=ADMIN；调用同步 | `ensureSiteMember`（`POST /iam/users/sync` 内部） | `docs/evidence/2475/R2-proxy-roles.txt` |
| R3 | helper 输入：`["site_member","site_member","","repair_technician"]`、`[]`、`nil` | 单测 | `docs/evidence/2475/R3-helper.txt` |
| R4 | 多角色成员（roles=["site_member","repair_technician"]）改主角色 | `PUT /api/sites/:id/members/:uid` | `docs/evidence/2475/R4-multi-role.txt` |
| R5 | 修后干净树 + `--full-gate-cmd` gate 报告 | 读校验（`plan-source.sh` + gate 解析） | `docs/evidence/2475/R5-gate.txt` |
| R6 | 修后 backend | `go build ./...` + 定向测试 | `docs/evidence/2475/R6-build.txt` |

### Assertions

- R1：`NO_22P02=1`、`ROLES_JSON=["site_admin"]`、`ROLE=site_admin`、`ROLE_ERRORS=0`；负例：出现 `22P02` 或 `ROLES_JSON` 非 JSON → FAIL。
- R2：`NO_22P02=1`、`ROLES_JSON=["site_admin"]`；负例：写入失败或 roles 仍为旧值 → FAIL。
- R3：`DEDUP=["site_member","repair_technician"]`、`EMPTY=[]`、`NIL=[]`；负例：空集合写成 `NULL` → FAIL。
- R4：`STATUS=200`、`ROLES_JSON` 含两项；负例：多角色被压成单角色（行为变化）→ FAIL。
- R5：`FULL_SUITE_EXIT=0`、`FAIL_COUNT=0`、`PANIC_COUNT=0`、`PLAN_SOURCE=PASS`；负例：`P5=NA` → FAIL。
- R6：`BUILD_EXIT=0`、`TESTS_FAIL=0`；负例：任一非 0 → FAIL。
- 通用 provenance：每个 artifact 首行 `# build=1.0.<fix> # commit=<sha> # dirty=false`。
- 负例（跨行）：任何 artifact 出现 `invalid input syntax for type json` 或 `22P02` → FAIL。

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2475/R1-staff-roles.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2475/R2-proxy-roles.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2475/R3-helper.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2475/R4-multi-role.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2475/R5-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2475/R6-build.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| symptom | trigger | root cause | test blind spot | same-class paths | fix layer | red-green | prevention |
|---|---|---|---|---|---|---|---|
| 改角色后前端弹「部分角色权限同步失败」 | 目标用户已有 `site_members` 行 | map Update 传 `[]string` 绕过 serializer → jsonb 22P02 | 既有测试 fixture 的 `WHERE user_id+tenant_id` 命中 0 行（rows:0），错误不触发 | `iam_proxy.go:1077` | handler 写路径 | R1 红绿（缺陷版必现 22P02） | D1 统一 helper |
| IAM 同步纠偏静默失败 | `SyncUsers` 纠偏 role | 同上 | 无该路径 jsonb 断言 | 角色变更/成员添加 | handler | R2 红绿 | 同类扫描 + 回归 |
| 本地 `roles` 与 `users.role`/IAM 三方不一致 | 改角色 | roles 写入失败被吞（仅日志） | 无三方一致性断言 | #2462 / #2461 | handler | R1/R2 断言 roles JSON | 失败进 `role_errors` |
| 多角色被意外压成单角色 | 改主角色 | 语义混淆（primary vs 全集） | 无多角色回归 | #2035 | handler | R4 回归 | D3 锁定语义 |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2475.sh r1` | `docs/evidence/2475/R1-staff-roles.txt` | exit 0；grep `NO_22P02=1`、`ROLE=site_admin`、`ROLE_ERRORS=0` | PASS | 出现 `22P02` → FAIL | dev |
| R2 | `bash scripts/verify-2475.sh r2` | `docs/evidence/2475/R2-proxy-roles.txt` | exit 0；grep `NO_22P02=1`、`ROLES_JSON=["site_admin"]` | PASS | roles 未写入 → FAIL | dev |
| R3 | `bash scripts/verify-2475.sh r3` | `docs/evidence/2475/R3-helper.txt` | exit 0；grep `DEDUP=`、`EMPTY=[]`、`NIL=[]` | PASS | 空集合写成 NULL → FAIL | dev |
| R4 | `bash scripts/verify-2475.sh r4` | `docs/evidence/2475/R4-multi-role.txt` | exit 0；grep `STATUS=200`、`ROLES_KEPT=1` | PASS | 多角色被压成单项 → FAIL | dev |
| R5 | `bash scripts/verify-2475.sh r5` | `docs/evidence/2475/R5-gate.txt` | exit 0；grep `FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | PASS | `P5=NA` → FAIL | dev |
| R6 | `bash scripts/verify-2475.sh r6` | `docs/evidence/2475/R6-build.txt` | exit 0；grep `BUILD_EXIT=0`、`TESTS_FAIL=0` | PASS | 任一非 0 → FAIL | dev |

### Workload

- Size: S
- Basis: 2 个后端源文件（各 1 处调用改动）+ 1 个共享 helper + 1 个测试文件 + 1 个 verify 脚本 + 6 条证据行；无 migration、无前端改动
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/site_member_common.go`
- `backend/handlers/user_staff.go`
- `backend/handlers/iam_proxy.go`
- `backend/handlers/site_member.go`
- `backend/handlers/site_member_roles_jsonb_2475_test.go`
- `scripts/verify-2475.sh`
- `docs/evidence/2475/plan.md`
- `docs/evidence/2475/pre-ready-2475.json`
- `docs/evidence/2475/R1-staff-roles.txt`
- `docs/evidence/2475/R2-proxy-roles.txt`
- `docs/evidence/2475/R3-helper.txt`
- `docs/evidence/2475/R4-multi-role.txt`
- `docs/evidence/2475/R5-gate.txt`
- `docs/evidence/2475/R6-build.txt`
- `docs/evidence/2475/evidence.json`

## Dependencies

- 无

### 依赖与关联说明（非硬依赖）

- **#2461 / #2462（已 done）**：IAM 角色同步与域角色落库已修复；本单是其残留的**本地 jsonb 写入**缺陷，二者使流程真正走到该行才暴露。
- **#2466（已 done）**：网点迁移路径复用 `site_members` 更新，本单修复后其 `roles` 同步亦安全。
- **#2468 / #2438**：本单是其预生产回执通过的前置（改角色不再报 22P02）。
- **部署侧**：预生产改角色回执（列表/三方一致）由 `[Ops-Followup of #2475]`（`waitForConfirm`）承载。

*Model: DeepSeek V4.1 Flash 1017*

