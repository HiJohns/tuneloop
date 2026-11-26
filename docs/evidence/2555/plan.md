## Accepted Plan — #2555 #2526 可见性口径回归：`TestActivateUser2309_*` 三个回滚用例恒 404（夹具缺员工角色）

> Tags: bug, code:go
> 关联：#2526（引入 by-id 统一可见性判定，未同步本夹具）、#2540（wip：激活路径将新增功能角色模板分配，其分支上已有同名 mock 适配）、#2309（「新建 IAM 用户后回滚必须 PurgeUser」原始契约）、#2336（GORM 回调清理）

### 现状核对（develop @ `c37655eb0`，代码为据）

| # | 事实 | 位置 |
|---|---|---|
| 1 | `ActivateUser` 已是 by-id 行操作，先走 `loadVisibleStaffUser` 再进 IAM | `backend/handlers/user_staff.go:618-635` |
| 2 | 可见性唯一判定：`membership ∪（旧 tenant/org 口径 ∧ 非顾客角色）`；`lower(role) ∈ {"", "user"}` 一律按顾客处理 → 不可见 | `backend/handlers/personnel.go:473-480`（`isCustomerUserRole`）、`:513-568`（`personnelUserVisible`） |
| 3 | site_admin 分支：先查 `site_members→sites.org_id`，再走 `legacyTenantOrgVisible`（`users.tenant_id = tid` + `ApplyOrgScope`），但 `customer` 为真时提前返回 false | `backend/handlers/personnel.go:534-546`、`:571-587` |
| 4 | 三个回滚用例的夹具 `models.User` **只设了 TenantID/OrgID/Name/Phone/Email/Status，未设 `Role`** → 按 #2526 口径判为纯顾客 → 404，`PurgeUser` 从未执行 | `backend/handlers/user_staff_rollback_2309_test.go:172-176`、`:197-201`、`:222-226` |
| 5 | mock 的 `/api/v1/namespaces/` 只回 `{"data": []}`，**不含 `role_templates`** → `ListRoleTemplates` 反序列化失败 | `backend/handlers/user_staff_rollback_2309_test.go:79-81`；客户端解析 `backend/services/iam_client.go:1753-1785` |
| 6 | mock 对 `/api/v1/organizations/` 全部返回同一响应、**不区分 bind/unbind**（两者共用同一 path，仅 body `action` 不同） | `backend/handlers/user_staff_rollback_2309_test.go:66-78`；`backend/services/iam_client.go:1256-1262`（bind）、`:1349-1355`（unbind） |
| 7 | #2540 的激活路径补「功能角色模板分配」只存在于 `origin/issue-2540`（`12cc82cec`），**不是** `origin/develop` 的祖先（`git merge-base --is-ancestor` rc=1）；该分支 tip 上仍含同名 mock 适配块（本单 Issue 所述「已在 `150ff8d85` 还原」在本仓不可验证：该对象不存在） | `git show origin/issue-2540:backend/handlers/user_staff_rollback_2309_test.go`（含 `role-templates` 分支） |
| 8 | 三个用例当前断言的是 `500` + `purges()==1` + `softDeletes()==0`，一旦被 404 短路，`require.Equal(500)` 直接失败 | `backend/handlers/user_staff_rollback_2309_test.go:189-191`、`:214-216`、`:252-254` |

### 根因

1. **夹具缺员工身份（主因）**：#2526 把「空 `users.role`」收窄为纯顾客（B10 数据保护），而本夹具沿用旧形态（只有 tenant/org、无 role）。可见性判定在 IAM 调用之前就返回 404，三条回滚分支（bind 失败 / 权限位失败 / 本地更新失败）**全部不可达**，用例恒红，且它们声称覆盖的回滚路径零覆盖。
2. **mock 缺角色模板（次因，与 #2540 的落地顺序耦合）**：`ActivateUser` 在 #2540 合入后会在写权限位之后 `ListRoleTemplates` + `AssignRoleTemplateToUserWithToken`。mock 只回 `{"data": []}` 时该查询报错 → 走「模板查询失败」回滚。此时第三个用例仍可能拿到 `500 + purge=1` 而**假绿**（回滚发生在模板查询处，而不是它声称的「本地更新失败」第三处回滚）——这正是 #2540 曾加、随后又被排除出 `### Files` 的那段 mock 适配。
3. **不可观测**：现有断言只有 `500/purge/soft-delete`，无法区分「走到了哪一条回滚分支」，也没有任何证据证明 `PurgeUser` 之前确实发生了 IAM 创建/绑定/权限位调用。

### 修复设计（只改测试合同与验证脚本，不改产品代码）

1. **三个夹具补员工身份**：`Role: "site_member"`（与 `CreateUser` 落库值同域：`backend/handlers/user_staff.go:354-376`；`toIAMRole("site_member")` → `STAFF`）。空 role 仍是纯顾客，保持 #2526 口径不变。
2. **mock 增补角色模板**：`/api/v1/namespaces/` 对 `*/role-templates` 返回
   `{"role_templates": [{"id": "rt-site_member", "code": "site_member"}]}`，其余路径保持原样（`{"data": []}`）。
   与 `origin/issue-2540` 上 `12cc82cec` 的实现逐字一致 → 两边同时落地时是同内容叠加（无冲突），任一侧先合入都不会让用例退回「模板查询失败」假绿。
3. **mock 增加调用计数**：`creates`（`POST /api/v1/users`）、`binds` / `unbinds`（同一 path，按 body `action` 区分）、`perms`（`*_/customer-permissions`）。用于证明每条回滚分支真的被走到。
4. **三个用例输出机器可判定 token**（`fmt.Printf`，不受 `go test -v` 缩进影响；沿用 #2526 的做法）：
   `ACTIVATE2309_CASE=<bind_failure|permission_failure|local_update_failure> HTTP=… PURGE=… SOFT=… IAM_CREATE=… IAM_BIND=… IAM_PERM=… IAM_UNBIND=…`；
   本地更新失败用例追加 `LOCAL_UPDATE_ATTEMPT=1`（回调真的被触发过），把「第三处回滚」从推断变成断言。
5. **新增 2 条用例**：
   - `TestActivateUser2309_FixtureVisibilityPrecondition`：同一路由下，空 role 行 → `404` 且 `IAM_CREATE=0`（**红条件固定**：这就是修复前夹具的形态）；`role=site_member` 行 → `500` + `PURGE=1`（绿条件）。任何一侧漂移都会让本单回归点失效。
   - `TestActivateUser2309_IAMMockServesRoleTemplates`：`services.NewIAMClient().ListRoleTemplates("ns-2555")` 返回恰好 1 条 `code=site_member`（证明第 2 项 mock 适配真的生效，而不是靠 `listErr` 假绿）。
6. **验证脚本** `scripts/verify-2555.sh`：单行子命令 `r1`…`r5`，每行首打印 provenance（`# build=… # commit=… # dirty=…`），go 命令统一经 `heavy-run.sh`，Postgres 测试库参数沿用 `scripts/verify-2526.sh` 的既有约定。
7. **不做的**：不改 `personnelUserVisible` / `ActivateUser` / `CreateUser`；不新增/翻转其它既有断言；不动 #2540 的 handler 改动。

### 语义变更（必须翻转的既有断言）

**无既有断言翻转**。三点必须随单登记：

| 行为 | 修改前 | 修改后 | 依据 |
|---|---|---|---|
| 三个 ActivateUser 夹具的 `users.role` | 空（被判纯顾客 → 404） | `site_member`（员工 → 可达 IAM） | #2526 口径本身不变，改的是夹具数据 |
| mock `/api/v1/namespaces/` 响应 | `{"data": []}`（解析失败） | 含 `role_templates[site_member]` | 让 `ListRoleTemplates` 成功，避免模板查询失败掩盖第三处回滚 |
| `TestCreateUser2309_*` 的 `role_errors` 内容 | `failed to list role templates` | `role template not found`（`merchant_staff` 不在 mock 模板里） | 仅告警字段变化；`500/purge/soft` 断言不变，R4 覆盖 |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志） |
|---|---|---|---|---|---|---|
| R1 三条回滚分支真实可达且各自可区分 | `backend/handlers/user_staff_rollback_2309_test.go` | `POST /users/:id/activate`（gin 测试路由，actor=site_admin） | `role=site_member` + `iam_sub=''`；分别注入 bind 500 / customer-permissions 500 / users UPDATE 失败 | backend（Postgres 测试库） | 三行 token：`bind_failure`（`IAM_BIND=1`、`IAM_PERM=0`、`IAM_UNBIND=0`）、`permission_failure`（`IAM_PERM=1`、`IAM_UNBIND=1`）、`local_update_failure`（`IAM_PERM=1`、`IAM_UNBIND=1`、`LOCAL_UPDATE_ATTEMPT=1`），三行均 `HTTP=500 PURGE=1 SOFT=0 IAM_CREATE=1` | `bash scripts/verify-2555.sh r1`（R1） |
| R2 mock 角色模板可被真实客户端解析 | `backend/handlers/user_staff_rollback_2309_test.go` | `services.IAMClient.ListRoleTemplates`（mock IAM `/api/v1/namespaces/*/role-templates`） | 测试直连 mock，不依赖 #2540 是否合入 | backend | `IAM_ROLE_TEMPLATE_COUNT=1`、`IAM_ROLE_TEMPLATE_CODE=site_member`（旧响应 `{"data":[]}` 会解析失败 → FAIL） | `bash scripts/verify-2555.sh r2`（R2） |
| R3 红条件固定：空 role 行仍按 #2526 判顾客 | `backend/handlers/user_staff_rollback_2309_test.go` | `POST /users/:id/activate` | 同一路由两条行：空 role（修复前夹具形态）/ `site_member` | backend | `EMPTY_ROLE_STATUS=404`、`EMPTY_ROLE_IAM_CREATE=0`、`STAFF_ROLE_STATUS=500`、`STAFF_ROLE_PURGE=1` | `bash scripts/verify-2555.sh r3`（R3） |
| R4 构建 + 定向回归（含 #2526 可见性用例与 #2375 白名单） | `backend/handlers/user_staff_rollback_2309_test.go` | `go build ./...` + `go test ./handlers -run 'TestActivateUser2309|TestCreateUser2309|TestStaffDetail2526|TestStaffActivate2526|TestUpdateUserRoleForbidden_2375'` | 修后工作树 | backend | `BUILD_RC=0`、`REGRESSION_PASS=1`、`FAIL_COUNT=0`、`EXISTING_2526_PASS≥1`、`EXISTING_2375_PASS≥1` | `bash scripts/verify-2555.sh r4`（R4） |
| R5 静态卫生（gofmt / vet） | `backend/handlers/user_staff_rollback_2309_test.go` | `gofmt -l` + `go vet ./handlers` | 修后工作树 | backend | `GOFMT_DIFF=0`、`VET_RC=0` | `bash scripts/verify-2555.sh r5`（R5） |

全量 `go test ./...` 由 `/work` 的 `pre-ready-check.sh` P5（baseline 差集）承担，产物为 `docs/evidence/2555/pre-ready-2555.json`；本表不重复全量命令。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `setupStaffRollback2309`（actor=site_admin）+ 三条 `Role:"site_member"` 行 + `iamRollbackMock2309`（含 role-templates / bind-unbind / perms 计数） | `bash scripts/verify-2555.sh r1` | `docs/evidence/2555/R1-rollback-paths.txt` | exit 0；grep 三行 `ACTIVATE2309_CASE=…`：`bind_failure … HTTP=500 PURGE=1 SOFT=0 IAM_CREATE=1 IAM_BIND=1 IAM_PERM=0 IAM_UNBIND=0`、`permission_failure … IAM_PERM=1 IAM_UNBIND=1`、`local_update_failure … IAM_PERM=1 IAM_UNBIND=1 LOCAL_UPDATE_ATTEMPT=1`；且 `--- PASS:` 覆盖 `TestActivateUser2309_BindFailurePurgesIAM`、`TestActivateUser2309_PermissionFailurePurgesIAM`、`TestActivateUser2309_LocalUpdateFailurePurgesIAM` | 任一行 `HTTP=404`（夹具又退回顾客）或 `PURGE=0`（回滚未执行）或 `LOCAL_UPDATE_ATTEMPT=0`（第三处回滚未被走到）→ FAIL |
| R2 | `iamRollbackMock2309`（`/api/v1/namespaces/*/role-templates`） | `bash scripts/verify-2555.sh r2` | `docs/evidence/2555/R2-mock-role-templates.txt` | exit 0；grep `IAM_ROLE_TEMPLATE_COUNT=1`、`IAM_ROLE_TEMPLATE_CODE=site_member` | `IAM_ROLE_TEMPLATE_COUNT=0` 或解析报错（旧 `{"data":[]}`）→ FAIL |
| R3 | 同一 DB/路由：`role=''` 行（tenant/org 命中）与 `role='site_member'` 行，前者无 `site_members` | `bash scripts/verify-2555.sh r3` | `docs/evidence/2555/R3-fixture-precondition.txt` | exit 0；grep `EMPTY_ROLE_STATUS=404`、`EMPTY_ROLE_IAM_CREATE=0`、`STAFF_ROLE_STATUS=500`、`STAFF_ROLE_PURGE=1` | `EMPTY_ROLE_STATUS=200`（#2526 顾客收窄被破坏）或 `STAFF_ROLE_STATUS=404`（红条件复发）→ FAIL |
| R4 | 修后工作树 + 既有 #2526/#2375 用例 | `bash scripts/verify-2555.sh r4` | `docs/evidence/2555/R4-regression.txt` | exit 0；grep `BUILD_RC=0`、`REGRESSION_PASS=1`、`FAIL_COUNT=0`、`EXISTING_2526_PASS=1`（或 ≥1）、`EXISTING_2375_PASS=1` | 任一既有用例 FAIL 或 `BUILD_RC≠0` → FAIL |
| R5 | 修后工作树 | `bash scripts/verify-2555.sh r5` | `docs/evidence/2555/R5-static.txt` | exit 0；grep `GOFMT_DIFF=0`、`VET_RC=0` | `GOFMT_DIFF≠0` 或 `VET_RC≠0` → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2555.sh r1` | `docs/evidence/2555/R1-rollback-paths.txt` | exit 0；grep `ACTIVATE2309_CASE=bind_failure HTTP=500 PURGE=1 SOFT=0 IAM_CREATE=1 IAM_BIND=1 IAM_PERM=0 IAM_UNBIND=0`、`ACTIVATE2309_CASE=permission_failure HTTP=500 PURGE=1 SOFT=0 IAM_CREATE=1 IAM_BIND=1 IAM_PERM=1 IAM_UNBIND=1`、`ACTIVATE2309_CASE=local_update_failure HTTP=500 PURGE=1 SOFT=0 IAM_CREATE=1 IAM_BIND=1 IAM_PERM=1 IAM_UNBIND=1 LOCAL_UPDATE_ATTEMPT=1`；`grep -c '^--- PASS: TestActivateUser2309_'` ≥3 | PASS | 任一行 `HTTP=404`、`PURGE=0`、`IAM_CREATE=0` 或 `LOCAL_UPDATE_ATTEMPT=0` → 回滚分支未被走到 → FAIL | dev |
| R2 | `bash scripts/verify-2555.sh r2` | `docs/evidence/2555/R2-mock-role-templates.txt` | exit 0；grep `IAM_ROLE_TEMPLATE_COUNT=1`、`IAM_ROLE_TEMPLATE_CODE=site_member` | PASS | `IAM_ROLE_TEMPLATE_COUNT=0`（mock 未提供模板，#2540 合入后用例会退化为模板查询回滚）→ FAIL | dev |
| R3 | `bash scripts/verify-2555.sh r3` | `docs/evidence/2555/R3-fixture-precondition.txt` | exit 0；grep `EMPTY_ROLE_STATUS=404`、`EMPTY_ROLE_IAM_CREATE=0`、`STAFF_ROLE_STATUS=500`、`STAFF_ROLE_PURGE=1` | PASS | `EMPTY_ROLE_STATUS=200`（纯顾客 by-id 被放宽）或 `STAFF_ROLE_STATUS=404`（主诉复发）→ FAIL | dev |
| R4 | `bash scripts/verify-2555.sh r4` | `docs/evidence/2555/R4-regression.txt` | exit 0；grep `BUILD_RC=0`、`REGRESSION_PASS=1`、`FAIL_COUNT=0`、`EXISTING_2526_PASS=1`、`EXISTING_2375_PASS=1` | PASS | `REGRESSION_PASS=0` 或 `FAIL_COUNT>0` → FAIL | dev |
| R5 | `bash scripts/verify-2555.sh r5` | `docs/evidence/2555/R5-static.txt` | exit 0；grep `GOFMT_DIFF=0`、`VET_RC=0` | PASS | `GOFMT_DIFF≠0` 或 `VET_RC≠0` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2555/R1-rollback-paths.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2555/R2-mock-role-templates.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2555/R3-fixture-precondition.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2555/R4-regression.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2555/R5-static.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 symptom | 触发条件 trigger | 根因 root cause | 测试盲区 test blind spot | 同类路径 same-class | 修复层 fix layer | 红绿 red-green | 预防 prevention |
|---|---|---|---|---|---|---|---|---|
| B1 | 3 个回滚用例恒 404 | 夹具 `users.role` 为空 + #2526 角色感知可见性 | 语义收窄后夹具未同步 | 空 role 形态此前是「合法员工」，无人固化 | 其它「按 tenant 命中即员工」的旧夹具 | 夹具补 `Role:"site_member"` | R3 `EMPTY_ROLE_STATUS=404`（红）→ `STAFF_ROLE_STATUS=500`（绿） | R3 永久固定红条件，语义再变即报错 |
| B2 | 用例假绿（本地更新回滚未被走到） | mock 无 `role_templates`，#2540 合入后提前在模板查询处回滚 | mock 能力落后于 handler 演进 | 只断言 `500+purge`，不区分回滚位置 | 任何「多回滚分支」用例 | mock 增补 role-templates | R1 `LOCAL_UPDATE_ATTEMPT=1` + `IAM_PERM=1` | 每条回滚分支必须有独立可观测计数 |
| B3 | 无法判断回滚是否真发生 | 仅看 HTTP 500 与 purge 计数 | 缺少 IAM 侧调用证据 | bind/unbind 共用 path，mock 不区分 | ActivateUser / CreateUser 回滚 | mock 按 body `action` 计数 | R1 `IAM_BIND`/`IAM_UNBIND` 逐例期望值 | 回滚证据必须双向（本地 purge + IAM 调用序列） |
| B4 | 「测试通过」但核心路径零覆盖 | 断言被 404 短路 | 前置可见性未纳入用例设计 | 未验证 404 与 500 的区分 | #2526 的 by-id 动作族 | 夹具补身份 + token 断言 | R1 三行 `HTTP=500` | 用例必须断言「走到了哪一步」而非最终码 |
| B5 | 语义收窄未做邻居扫描 | #2526 改可见性判定 | 只改产品代码，未 grep 全部夹具 | 无「空 role 夹具」清单 | 全部 `models.User{Role:""}` 夹具 | 本单修复 + R4 定向回归 | R4 `FAIL_COUNT=0` | 改判定后必须 `grep` 同形态夹具并逐条登记 |
| B6 | 修复可能反向放宽顾客 | 把空 role 当员工 | 用夹具迁就实现而非实现对齐语义 | 无顾客负例 | by-id 详情/保存/激活 | 不改判定，只改夹具 | R3 `EMPTY_ROLE_STATUS=404` | 负例必须常驻合同（R3） |
| B7 | 与 #2540 合并后再次假绿 | 两分支各自改同一 mock | 跨 Issue 共享测试资产缺少归属 | 无人核对两分支的 mock 形态 | 任何共享 fake IAM | 采用与 `12cc82cec` 逐字一致的实现 | R2 `IAM_ROLE_TEMPLATE_CODE=site_member` | 共享 fake 变更以「同内容叠加」为准则，并在计划登记 |
| B8 | 基线失败被误记为他单缺陷 | 全量基线差集里出现本 3 例 | 存量红用例无 owner | 无回归用例守护 | 其它存量红用例 | 本单独立修复 | R1/R4 | 存量红必须建单（本单即产物） |
| B9 | 迁移/结构无关却能拖垮门禁 | 定向 gate 失败 | 用例与产品语义脱节 | 门禁只看 exit code | 全量 gate | 本单 + P5 基线差集 | R4 + `pre-ready-2555.json` | 全量回归交 P5，定向用例负责语义 |

### Workload

- Size: S
- Basis: 1 个后端测试文件（夹具 role + mock 能力/计数 + 2 条新用例 + token 输出）+ 1 个验证脚本 + 6 份证据产物；无产品代码、无 migration、无新依赖
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/user_staff_rollback_2309_test.go`
- `scripts/verify-2555.sh`
- `docs/evidence/2555/plan.md`
- `docs/evidence/2555/evidence.json`
- `docs/evidence/2555/pre-ready-2555.json`
- `docs/evidence/2555/R1-rollback-paths.txt`
- `docs/evidence/2555/R2-mock-role-templates.txt`
- `docs/evidence/2555/R3-fixture-precondition.txt`
- `docs/evidence/2555/R4-regression.txt`
- `docs/evidence/2555/R5-static.txt`

## Input Truth & Tool Preflight

- **Source of truth（口径唯一）**：
  - 可见性 = `personnelUserVisible`（`backend/handlers/personnel.go:513-568`）+ `isCustomerUserRole`（`:473-480`）：`lower(role) ∈ {"", "user"}` = 顾客，不可见 → 404。
  - 员工角色取值域 = `CreateUser` 落库值（`backend/handlers/user_staff.go:349-376`）：`site_member` / `site_admin` / `merchant_staff` / `repair_technician`；`toIAMRole`（`backend/handlers/permission_manage.go:240-255`）。
  - 2309 契约 = 「新建 IAM 用户后回滚必须 `PurgeUser`，禁止 `DeleteUser`」（Issue #2309 + 本文件头注释）。
  - #2540 现状以 `origin/issue-2540` 为准（`12cc82cec`），**不是** develop 祖先（已用 `git merge-base --is-ancestor` 核对，rc=1）。
- **差异核对（实施前先跑，结果入 R4 artifact）**：
  - `grep -c 'Role: "site_member"' backend/handlers/user_staff_rollback_2309_test.go` → 修后 ≥3（三个回滚用例）。
  - `grep -c 'role_templates' backend/handlers/user_staff_rollback_2309_test.go` → 修后 ≥1；修前 0。
  - `grep -n 'func loadVisibleStaffUser' backend/handlers/personnel.go` → 恰好 1 个定义（本单不改）。
  - `git diff --stat origin/develop -- backend/handlers/user_staff.go backend/handlers/personnel.go` → 必须为空（本单零产品代码改动）。
- **损坏输入拒绝**：`TUNELOOP_TEST_ALLOW_SKIP` 不得设置（skip 视为 FAIL）；非 UUID id 的 404 既有用例不改；`iam_sub` 已存在的 40001 分支不在本单范围。
- **工具盲区（显式声明）**：
  - 后端用例依赖真实 Postgres（`testfixtures.SetupTestDB`），本机 analyze 阶段**未运行** `go test`/`go build`（轻节点只读）；红/绿实证由 heavy worker 在 `/work` 的 worktree 内生成。
  - `bind` / `unbind` 在 IAM 侧共用同一 path（`/api/v1/organizations/{org}/users/{user}/bind`），只能按 body `action` 区分 —— mock 计数必须解析 body，不能按 URL 后缀。
  - `LIST role-templates` 响应解析优先 `{"role_templates":[...]}`（`backend/services/iam_client.go:1771-1784`），mock 不得只回裸数组之外的旧形态 `{"data":[]}`。
  - gorm 回调注入（第三例）必须在 `t.Cleanup` 中移除，沿用 #2336 既有写法。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 修复层 = **夹具**，不改 `personnelUserVisible` / `ActivateUser` | Architect | 本计划锁定（#2526 B10「空 role = 纯顾客」是有意收窄，改实现会退化数据保护） | 若要求改实现 → 回 Architect，#2526 的 R4 负例同步翻转 |
| D2 | 夹具角色取值 `site_member`（不取 `member`/`STAFF`） | Architect | 本计划锁定（与 `CreateUser` 落库值、`toIAMRole` 映射、#2540 的 `resolveActivationRoleTemplate` 同域） | 若取 `member`，在 #2540 合入后仍回落 `site_member`，但会与 `users.role` 真实域不一致 → 不允许 |
| D3 | mock 的 role-templates 适配采用与 `12cc82cec` **逐字一致**的实现 | Architect | 本计划锁定（同内容叠加，两分支任意顺序合入均不冲突、不倒退） | 若改用裸数组/其它 code，与 #2540 形成两份实现 → 回 Architect |
| D4 | 回滚证据必须包含 IAM 调用计数（create/bind/perm/unbind）+ `LOCAL_UPDATE_ATTEMPT` | Architect | 本计划锁定（否则第三处回滚可能被模板查询失败掩盖成假绿） | 只留 `500+purge` → 审计应判证据不足 |
| D5 | 红条件由 R3 固定：空 role 行 → 404 且 `IAM_CREATE=0` | Architect | 本计划锁定（Issuer 描述的修复前形态可直接用路由复现） | 若 #2526 未来放宽顾客边界，R3 会红 → 届时回 Architect 改口径而非删断言 |
| D6 | 本单不触碰 #2540 的 handler 实现与证据 | Architect | 本计划锁定（#2555 只拥有 mock 侧） | 若 #2540 需要改同一测试文件其它段落 → 由 #2540 与本单协商顺序 |
| D7 | 不新增产品文档章节（无行为变更） | Architect | 本计划锁定（测试合同修复） | 若审计要求 api.md 记录「激活可见性」→ 属 #2526 的 `## Core Update` 范围，另开 |

## Out of Scope

- #2540 的 `ActivateUser` 功能角色模板分配实现（handler 改动）与其证据：由 #2540 承载；本单只准备 mock 能力。
- 产品代码零改动：`backend/handlers/user_staff.go`、`backend/handlers/personnel.go`、`backend/services/iam_client.go` 均不改。
- `TestCreateUser2309_*` 的夹具不改（它们经 API 建用户，`role` 由 handler 写入）；仅共用 mock 的增补能力。
- 批量导入 / 批量重置密码 / 网点调动的 IAM 回滚（#2527 T1/T2）。
- 全量 `go test ./...` 的绝对退出码门禁：由 `pre-ready-check.sh` P5 基线差集承担（本机不重复）。
- 预生产真机走查、预生产数据核对：本单为测试合同修复，无部署动作，关闭时不产生 Ops-Followup。

## Dependencies

- 无

### 关联（非硬依赖）

- `#2540`（wip）：其激活路径将新增角色模板分配，并在 `origin/issue-2540` 上已含同一段 mock 适配（`12cc82cec`）。本单提前落地同内容改动；两边合入顺序不影响结果（D3）。
- `#2526`：本单是其「语义收窄未做邻居扫描」的收敛，口径本身由该单锁定（B10）。
- `#2309` / `#2336`：本单沿用其「回滚必须 PurgeUser」契约与 GORM 回调清理纪律。
- 集成提示：`origin/issue-2540` 的 `ActivateUser` 仍按 `users.tenant_id` 过滤（未见 #2526 改动），合入 develop 时必然与 #2526 冲突，需由该单集成者以 develop 版可见性为准；本单不介入。

*Model: DeepSeek V4.1 Flash 1017*

