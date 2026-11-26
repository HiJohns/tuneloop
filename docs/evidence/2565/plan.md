## Accepted Plan — #2565 develop 全量 handlers gate 恒红：#2309 激活夹具按 #2526 口径对齐（Option A），并以守卫用例锁定语义

> **裁定摘要（Architect）**：采用 Issue 正文的 **Option A（测试语义过期）**，#2526 的可见性口径**不改**。理由见 `## Decision Lock` D1/D2：
> - #2526 已把「`lower(users.role) ∈ {"", "user"}` 且无任何成员关系 → by-id 详情/保存/激活一律 `404 40400`」写入 Accepted Plan 的**目标语义**（B10「顾客 PII by-id 暴露」），并有预生产只读核对（11 行，全部 `iam_sub` 非空、均不在人员列表内）；
> - 人员列表以 membership 锚定 → 「role 空 + 无 membership」的行**本来就不在列表里**，激活按钮不可达；因此 Option A 不构成任何 UI 功能回退；
> - Option B（放行本地 pending 空角色行）会直接推翻 #2526 的已锁定 Decision Lock，属另立语义变更，不在本单。
> 本单**不改产品代码**：改 3 处测试夹具夹具形态（`users.role` 显式设员工角色）+ 新增 1 条守卫用例（把 #2526 语义钉死，防止再次被「修红」时反向放宽）。

### 背景与根因（代码为据）

| 事实 | 位置 |
|---|---|
| 激活的可见性走唯一判定 `loadVisibleStaffUser` → `personnelUserVisible` | `backend/handlers/user_staff.go:632`（`ActivateUser`）、`backend/handlers/personnel.go:516` |
| `isCustomerUserRole` 把**空 role** 判为顾客（注释即「空值不可当员工」） | `backend/handlers/personnel.go:484-491` |
| `site_admin` 分支：membership 命中→可见；否则 `customer == true` 时直接 `return false` | `backend/handlers/personnel.go:546-549` |
| `merchant_admin` 旧口径同样要求「非顾客角色」 | `backend/handlers/personnel.go:534-535` |
| `#2309` 三条激活用例的 pending 行**只设** `TenantID/OrgID/Status`，**不设 `Role`**、也无 `site_members` | `backend/handlers/user_staff_rollback_2309_test.go:175-180`、`:198-203`、`:233-238` |
| `#2462 D1` 起 `users.role` **恒被写入**（此前为空串），合法集合 `{site_member, site_admin, repair_technician, merchant_staff}` | `backend/handlers/user_staff.go:374-381`（validRoles + `user.Role = resolvedRole`） |
| `#2526` 目标语义 + 预生产核对（空 role/`user` + 无关系 = 11 行，均 `iam_sub` 非空，均不在列表） | `#2526` Accepted Plan「语义变更」表 + 预生产只读核对段；`docs/spec/api/api.md` §10.18.6 |
| 当前红：3 例期望 500 实际 404（`{"code":40400,"message":"user not found"}`） | `#2565` 正文（`origin/develop 58cb3a54a`） |

**结论**：红是**夹具形态过期**，不是实现回归。#2309 想验证的是「IAM bind / 权限 / 本地更新失败后必须 `PurgeUser` 而非软删」，前提是先通过可见性闸门；夹具必须按 #2462 之后真实员工形态构造。

### 变更点

1. **`backend/handlers/user_staff_rollback_2309_test.go`（夹具形态）**：三条激活用例的 pending 行补 `Role: "site_member"`（`users.role` 合法员工角色），并加注释说明依据（#2462 D1 恒写 role；#2526 D1/B10 空 role = 顾客类 → 404）。**断言不改**（仍断言 500 + `Purges=1` + `softDeletes=0`）。
2. **`backend/handlers/user_staff_rollback_2309_test.go`（测试保真，同文件）**：`newIAMRollbackMock2309` 的 `/api/v1/namespaces/` 现返回 `{"data": []}` → `ListRoleTemplates` 取不到模板 → `ActivateUser` 在「角色模板缺失」回滚处就 500 返回，**第三个用例根本走不到「本地写回失败」分支**（名称与覆盖不符）。改为返回单模板数组 `[{"id":"rt-site-member","code":"site_member"}]`，并在 `TestActivateUser2309_LocalUpdateFailurePurgesIAM` 用原子计数断言注入的 users-UPDATE 回调**确实被命中**（`LOCAL_WRITEBACK_ATTEMPT>=1`）。副作用核对：同文件另 2 条 `CreateUser` 用例的角色模板分配为「失败只警告」，断言（purge/soft）不受影响。
3. **`backend/handlers/user_activate_visibility_2565_test.go`（新增）**：守卫用例 `TestActivateUser2565_EmptyRoleNoMembershipIs404` —— 同 tenant/org、`role=""`、无 `site_members`/`merchant_members`、`status=pending`、`iam_sub=""`，actor 为 site_admin → `POST /users/:id/activate` 断言 `404` + `40400` + **零 IAM 调用**（fake IAM 计数 0）：把 #2526 语义变成可执行断言，未来若有人为「修红」而放宽 `personnelUserVisible`，本用例立刻变红。
4. **`scripts/verify-2565.sh`**：4 行证据生成器（`r1`..`r4`），provenance 首行由 `git` 派生（`FIX_COMMIT` 锚定 fix commit，公式同 `scripts/verify-2526.sh`）。
5. 证据包：`docs/evidence/2565/`（`plan.md`、`R1`–`R4`、`evidence.json`、`pre-ready-2565.json`）。

**红绿设计（R1）**：`verify-2565.sh r1` 先临时把 3 个用例文件恢复为 `origin/develop` 版（`git checkout origin/develop -- <file>`），跑同一选择器记录 `PRE_FIX_FAIL=3`，再 `git checkout HEAD -- <file>` 复原并复跑记录 `POST_FIX_PASS=3`；结束时两份都还原（异常路径 `trap` 复原，禁止留下脏树）。**不复用 `git stash`、不新建 worktree**（避免与 worktree 生命周期规则冲突）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 #2309 三例转绿（红→绿实证 + 第三例落到本地写回分支） | `backend/handlers/user_staff_rollback_2309_test.go` | `go test ./handlers -run 'TestActivateUser2309' -count=1 -timeout 10m` | 夹具 `role=site_member`，pending，无 membership；IAM bind / 权限 / 本地更新分别失败 | backend（测试库 + fake IAM） | `PRE_FIX_FAIL=3`（origin/develop 版夹具）→ `POST_FIX_PASS=3`、`POST_FIX_FAIL=0`；三例 `Purges=1`、`softDeletes=0`；第三例 `LOCAL_WRITEBACK_ATTEMPT>=1` | R1 |
| R2 语义守卫（空角色 + 无关系 → 404） | `backend/handlers/user_activate_visibility_2565_test.go` | `POST /api/users/:id/activate` | 同 tenant/org、`role=""`、无 membership、`iam_sub=""`，actor=site_admin | backend | `GUARD_STATUS=404`、`GUARD_CODE=40400`、`GUARD_IAM_CALLS=0`（未触达 IAM 即拒绝） | R2 |
| R3 口径未改写 + 邻居扫描 | `backend/handlers/user_staff_rollback_2309_test.go`、`scripts/verify-2565.sh` | `bash scripts/verify-2565.sh r3` | 修后源码（可见性判定文件只读核对，不改） | backend | `PERSONNEL_PREDICATE_DIFF=0`（不改 #2526 判定）、`EMPTY_ROLE_ACTIVATE_FIXTURES=0`（全仓无「激活夹具未设 role」残留）、`FIXTURE_ROLE_LEGAL=3` | R3 |
| R4 handlers 全量回归 + 构建 | `backend`（全包） | `bash scripts/verify-2565.sh r4` | 修后工作树 | backend | `HANDLERS_FAIL_COUNT=0`、`HANDLERS_NEW_FAILURES=0`（与 `origin/develop` 基线同集对比）、`BUILD_OK=1` | R4 |

## Input Truth & Tool Preflight

- **Source of truth**：
  - 可见性唯一口径 = `personnelUserVisible`（`backend/handlers/personnel.go:516`）+ `docs/spec/api/api.md` §10.18.6；
  - `users.role` 合法集合 = `backend/handlers/user_staff.go:374-381`（`site_member`/`site_admin`/`repair_technician`/`merchant_staff`）；
  - 失败补偿语义 = `#2309`（IAM 新建后任何失败必须 `PurgeUser`，不得 `DeleteUser`）。
- **差异核对（实施前先跑，只读）**：
  - `grep -rn "Status:\s*\"pending\"" backend/handlers/*_test.go | grep -i act` → 确认仅本单 3 处激活夹具需补 `Role`；
  - `git log --oneline -1 44f9368c7` + `git merge-base --is-ancestor 44f9368c7 origin/develop` → #2526 已在 develop（红的前提成立）；
  - Issue 正文建议的 `Role: "staff"` **非法**（不在 validRoles）→ 统一用 `site_member`（见 D2），不得写 `staff`。
- **损坏输入拒绝**：夹具若只补 `Role` 而不补注释依据 → 审计侧按「无 provenance 的夹具变更」退回；守卫用例若断言 200/40001 → 语义被反向，直接 FAIL。
- **工具盲区**：`go test ./handlers -run` 只证明本单目标用例；全量 handlers 可能受 Postgres 夹具/顺序影响 → R4 必须做基线差分（`origin/develop` 同机跑一次），不得只报「本地绿」。
- **测试库**：沿用 `docs/ops/test-db-lifecycle.md`（`POSTGRES_HOST/PORT` + `TUNELOOP_TEST_DB` 可覆盖），与 `scripts/verify-2526.sh` 同一组默认值。

## Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 采用 **Option A**（夹具语义过期）：`#2526` 可见性口径与 `isCustomerUserRole` **不改**；`personnel.go` 本单零改动 | 本计划锁定（依据 #2526 Accepted Plan 目标语义 B10 + 预生产 11 行核对 + 列表 membership 锚定 ⇒ 无 UI 回退） |
| D2 | 夹具角色用 `site_member`（合法员工角色），**不用** Issue 正文示例的 `"staff"`（不在 `users.role` 合法集合，会让夹具不代表真实形态） | 本计划锁定 |
| D3 | 必须新增**守卫用例**（空 role + 无 membership → 404 且零 IAM 调用）；仅改夹具视为不完整（会把语义漂移风险留给下一位「修红」的人） | 本计划锁定 |
| D4 | 断言不改：#2309 仍断言 `500` + `Purges=1` + `softDeletes=0`；不得为提高通过率放宽期望值 | 本计划锁定 |
| D5 | 不改产品代码、不改 `docs/spec/api/api.md`（语义未变；#2526 已写清 §10.18.6），因此无 `[Core Update Required]` | 本计划锁定 |
| D6 | 测试保真：`#2309` 第三例必须真正覆盖「本地写回失败」分支（fake IAM 返回 `site_member` 模板 + 断言 users-UPDATE 回调命中），不得只靠改名或注释 | 本计划锁定（否则该文件仍存在「名称 ≠ 覆盖」的假绿） |

## Out of Scope

- **Option B**（在 `personnelUserVisible` 放行「本地 pending 空角色行」）：与 #2526 已锁定语义冲突，需 Owner 另立语义变更 Issue（须同步 §10.18.6、翻转 #2526 R4 负例）。
- 生产/预生产 `users.role` 空值回填（`#2462 D1` 之后的存量行）：只读扫描 + Owner 授权后另立 Ops 单。
- 其它 `users.role` 空值相关的 UI 展示/筛选（人员列表角色列）：沿用 #2462。
- 全量 `go test ./...`（其它包）与前端构建：本单只改 handlers 包测试，回归归 R4 的 `./handlers` 全量 + `go build ./...`。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 三条 #2309 激活用例（pending + `role=site_member` + 无 membership） | `bash scripts/verify-2565.sh r1` | `docs/evidence/2565/R1-2309-green.txt` | `PRE_FIX_FAIL=3`、`POST_FIX_PASS=3`、`POST_FIX_FAIL=0`、`PURGE_OK=3`、`SOFT_DELETE=0`、`LOCAL_WRITEBACK_ATTEMPT>=1` | `PRE_FIX_FAIL=0`（夹具未复刻缺陷，红例不成立）或 `POST_FIX_FAIL>0` 或 `LOCAL_WRITEBACK_ATTEMPT=0`（第三例未走到本地写回）→ FAIL |
| R2 | 同 tenant/org + `role=""` + 无 membership + `iam_sub=""` | `bash scripts/verify-2565.sh r2` | `docs/evidence/2565/R2-guard-404.txt` | `GUARD_STATUS=404`、`GUARD_CODE=40400`、`GUARD_IAM_CALLS=0` | `GUARD_STATUS=200`（语义被放宽）或 `GUARD_IAM_CALLS>0`（越权触达 IAM）→ FAIL |
| R3 | 修后源码 + 全仓夹具扫描 | `bash scripts/verify-2565.sh r3` | `docs/evidence/2565/R3-neighbor.txt` | `PERSONNEL_PREDICATE_DIFF=0`、`EMPTY_ROLE_ACTIVATE_FIXTURES=0`、`FIXTURE_ROLE_LEGAL=3` | 出现 `Role: "staff"` 或残留空角色激活夹具 → FAIL |
| R4 | 修后 `backend`（含 Postgres 测试库）+ `origin/develop` 基线 | `bash scripts/verify-2565.sh r4` | `docs/evidence/2565/R4-suite-build.txt` | `HANDLERS_FAIL_COUNT=0`、`HANDLERS_NEW_FAILURES=0`、`BUILD_OK=1` | 仍红 ≥1 或出现新失败 → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2565.sh r1` | `docs/evidence/2565/R1-2309-green.txt` | exit 0；含 `PRE_FIX_FAIL=3`、`POST_FIX_PASS=3`、`POST_FIX_FAIL=0`、`LOCAL_WRITEBACK_ATTEMPT>=1` | PASS | 红例不成立或修后仍红或第三例未触达本地写回 → FAIL | dev |
| R2 | `bash scripts/verify-2565.sh r2` | `docs/evidence/2565/R2-guard-404.txt` | exit 0；含 `GUARD_STATUS=404`、`GUARD_CODE=40400`、`GUARD_IAM_CALLS=0` | PASS | 返回 200 或触达 IAM → FAIL | dev |
| R3 | `bash scripts/verify-2565.sh r3` | `docs/evidence/2565/R3-neighbor.txt` | exit 0；含 `PERSONNEL_PREDICATE_DIFF=0`、`EMPTY_ROLE_ACTIVATE_FIXTURES=0` | PASS | 判定被改写或残留空角色夹具 → FAIL | dev |
| R4 | `bash scripts/verify-2565.sh r4` | `docs/evidence/2565/R4-suite-build.txt` | exit 0；含 `HANDLERS_FAIL_COUNT=0`、`BUILD_OK=1` | PASS | 仍有失败或构建失败 → FAIL | dev |

## Artifact Provenance

- R1–R4 均由 `scripts/verify-2565.sh` 生成，首行 provenance：`# build=1.0.<fix 短码> # commit=<fix 40 位 sha> # dirty=false`（公式同 `scripts/verify-2526.sh`，锚定 fix commit 而非证据提交）。
  - `docs/evidence/2565/R1-2309-green.txt`
  - `docs/evidence/2565/R2-guard-404.txt`
  - `docs/evidence/2565/R3-neighbor.txt`
  - `docs/evidence/2565/R4-suite-build.txt`
- 台账：`docs/evidence/2565/evidence.json`；`evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$`（每行一条记录，`dirty=false`）。
- 计划快照（P17）：`docs/evidence/2565/plan.md`（`scripts/plan-source.sh --issue 2565 --fresh` 生成，随证据一起提交）。
- gate（/work 执行）：`~/.config/opencode/scripts/pre-ready-check.sh 2565 --summary-file … --changed-files … --affected-cmd "cd backend && go test ./handlers -run 'TestActivateUser2309|TestActivateUser2565' -count=1 -timeout 10m" --full-gate-cmd "cd backend && go test ./handlers -count=1 -timeout 20m" --evidence-file docs/evidence/2565/evidence.json --report docs/evidence/2565/pre-ready-2565.json`；报告 `build` 须匹配 `schema: ^1\.0\.[0-9a-f]{7,40}$`。

## Dependencies

- #2526
- #2309

### 依赖说明

- **#2526（已合入 `origin/develop`，`44f9368c7`）**：提供可见性唯一口径；本单只对齐夹具 + 加守卫，不改其判定。
- **#2309（已合入）**：提供三条回滚补偿用例与 fake IAM；本单只补 `Role`，断言保持。
- **对下游**：本单是 `#2470` 的 R8 全量 gate（`FULL_SUITE_EXIT=0`）的前置；修完 handlers 全量应回绿。

### Workload

- Size: S
- Basis: 2 个后端测试文件（3 处夹具 + fake IAM 模板 1 处 + 本地写回回调断言 + 1 个守卫用例）+ 1 个证据脚本 + 4 条证据行；无产品代码、无 migration、无文档、无前端
- Confidence: high（根因已由 handler code 出口与 #2526 语义闭环，无需运行时探索）

### Files（变更文件清单）

- `backend/handlers/user_staff_rollback_2309_test.go`
- `backend/handlers/user_activate_visibility_2565_test.go`（新增）
- `scripts/verify-2565.sh`（新增）
- `docs/evidence/2565/plan.md`
- `docs/evidence/2565/R1-2309-green.txt`
- `docs/evidence/2565/R2-guard-404.txt`
- `docs/evidence/2565/R3-neighbor.txt`
- `docs/evidence/2565/R4-suite-build.txt`
- `docs/evidence/2565/evidence.json`
- `docs/evidence/2565/pre-ready-2565.json`

*Model: DeepSeek V4.1 Flash 1017*

