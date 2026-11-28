## Accepted Plan — #2455（v3：Owner 授权落地 + 依赖口径按 O1=A/O4 修正 + gate 行）（Owner 2026-11-27 裁定 B：重名由 IAM 判定，不做本地预检）｜Rev.5｜Rev.6（IAM_CALLS 口径统一：Create=1 / Update=0）
> **Rev.6 说明（采纳 Investigation Packet option 1）**：Rev.5 只改写了 R2 矩阵行，未同步 R4 的 Assertions/Contract（仍抄旧值 `=1`），也未清理 Bug Reflection Matrix 的「handler 预检 / R2 `IAM_CALLS=0`」残留，导致同一 `IAM_CALLS` 在计划里既有 0 又有 1。
> 本版以**矩阵行 = 语义合同**为准统一：**CreateSite 不预检 → `IAM_CALLS=1`**；**UpdateSite 沿用 #2457 本地预检 → `IAM_CALLS=0`**。D2 追加限定「不做本地预检」仅约束 CreateSite。**代码不改**。

> **v3 说明（修 Investigation Packet 的计划 Files 缺口）**：R1 把「同商户同名网点」定为非法后，既有用例 `TestSyncOrganizations_SiteTenantID` 的 fixture（在同 tenant 造两条同名 site 再断言 sync 纠正 tenant）前提失效 → `23505` 使全量套件出现新失败。按 **option 1**（最小、保留用例原意）：把 `backend/handlers/iam_proxy_test.go` 登记进 Files，并只修 fixture（改用独立 org 或先软删同 org 站点）；同时新增「被翻转的既有断言」小节。

> **v2 变更（采纳 2026-11-24 Owner 裁定）**：
> ① **O2 授权**：DDL（唯一部分索引）与存量对齐（方向 **IAM→本地**）**已获 Owner 批准**；D1/D3 由「待批准」转为 `owner_approved`；执行仍在部署侧（ops）。
> ② **O1=A / O4**：前置 #2457 先审后合并；`status:ready` 视为「已实现」可放行下游开发（不再因 ready 反复 HOLD），但**合并顺序 #2457 → #2455**，最终证据在合并后的 `origin/develop` 基线重跑。
> ③ **D4 口径澄清**：`merchants.name` 与 `sites.name` **各自**等于其对应 IAM 组织名（商户根组织 / 网点组织，两个层级）；`label` 拼接「商户名 · 网点名」仅用于跨商户同名网点的区分，不代表数据不同源。
> ④ 新增 R10（gate 读校验）。R1–R9 语义不变。

**分阶段 gate**：Phase A（R1–R5 数据完整性：索引/拦截/清单）必须先全绿，才允许进入 Phase B（R6–R9 身份上下文可辨识 + 构建回归）。
**前置依赖**：#2457（`sites.name` = IAM 组织名镜像 + UpdateSite 先写 IAM）必须先落地；本单不改 IAM 同步逻辑。

## 语义变更：被翻转的既有断言

| 位置 | 旧断言 | 新预期 | 理由 |
|---|---|---|---|
| `backend/handlers/iam_proxy_test.go:475-500`（`TestSyncOrganizations_SiteTenantID/update_existing_site_with_wrong_tenant_id`） | 在**同一 tenant** 下造两条同名 `site`（`test-site`），断言 sync 把错租户那条的 `tenant_id` 纠正为商户 org | fixture 改为不产生「同 tenant 同名」：第二条站点使用独立 `org_id`，或先软删第一条（断言「纠正 tenant」的目标不变） | R1 的 `uq_sites_tenant_name` 明确禁止同商户同名，旧 fixture 前提非法（AGENTS.md §语义变更：必须翻转并登记） |

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 同商户网点名唯一索引 | `backend/database/migrations/20261123002_site_name_unique_2455.up.sql` | 迁移执行（测试库显式执行 up/down） | 索引不存在 → 执行 up；重复执行 up；再执行 down | 后端 | `pg_indexes` 出现 `uq_sites_tenant_name`（`(tenant_id, lower(name)) WHERE deleted_at IS NULL`）；up 幂等（2 次均成功）；down 后索引消失 | R1 |
| R2 创建网点重名由 IAM 判定（Owner 裁 B） | `backend/handlers/site.go` → `CreateSite` | `POST /api/merchant/sites` | 同商户已存在同名网点（IAM 组织名唯一性拒绝） | 后端 | HTTP 409 + code `40900` + **中文** message（映射 IAM conflict，不暴露英文原文）；`IAM_CALLS=1`（不预检）；`LOCAL_ROWS_ADDED=0`（不产生本地半态）；`ORG_COUNT_UNCHANGED=1` | R2 |
| R3 跨商户同名允许 + DB 层兜底 | `backend/database/migrations/20261123002_site_name_unique_2455.up.sql`、`backend/handlers/site.go` | 迁移索引 + `POST /api/merchant/sites` | tenant A 已有「卡丹萨乐器」，tenant B 建同名 | 后端 | tenant B 创建成功（HTTP 200）；tenant A 直接 SQL 再插同名被索引拒绝（`SAME_TENANT_DUP_REJECTED=1`）；并发竞态落入 `isUniqueViolation` → 409（非 500） | R3 |
| R4 编辑网点同商户重名拦截（先于 IAM） | `backend/handlers/site.go` → `UpdateSite` | `PUT /api/merchant/sites/:id` | 同商户另一网点已占用目标名 | 后端 | HTTP 409 + code `40900`；fake IAM `UpdateOrganizationName` 调用次数=0；本地名称不变 | R4 |
| R5 存量重复清单报告（只读） | `scripts/site-name-duplicates.sh`、`scripts/tests/test_site_name_duplicates.sh` | 脚本 fixture | 干净 fixture（3 行不同名）与重复 fixture（同商户 2 行同名） | 后端 | 干净 → `DUP_GROUPS=0`；重复 → 输出恰好 1 组、2 个 site id；脚本无写库语句（`READONLY=1`），不做任何改名 | R5 |
| R6 身份上下文标签含商户名 + 网点名 | `backend/handlers/auth.go` → `WxAccounts` | `GET /auth/wx-accounts` | IAM contexts 两项：org_a=`76d46658…`（商户 cadenza）、org_b=`a5eb38d4…`（商户 卡丹萨）；两网点本地名均为「卡丹萨乐器」 | 后端 | 每个 context 含 `merchant_name`、`site_name`、`org_name`；`label`=`商户名 · 网点名`；两项 label 互不相同（`DISTINCT=1`） | R6 |
| R7 小程序身份切换改用服务端 label | `frontend-mobile/src/pages-weapp/account-select/AccountSelect.jsx` | 小程序「切换身份」页 | contexts 与 legacy 两种返回 | 小程序 | 渲染 `ct.label` / `a.label`（服务端组合），本地不再用 `sites.name` 单源拼接；legacy 分支同样优先 `a.label` | R7 |
| R8 PC 员工身份显示「商户 · 网点」 | `frontend-pc/src/pages/UserProfile.jsx` | PC 用户详情「关联网点」 | `/users/me` 返回 `tenant_name` + `site_name` | PC | 渲染 `商户名 · 网点名`（任一缺失时回退单项，不显示 `undefined`） | R8 |
| R9 构建与定向回归 | `backend`、`frontend-pc` | `go build ./...`、站点/鉴权定向测试、PC 构建 | 修后树 | 后端 + PC | build exit 0；站点/鉴权定向测试 0 FAIL；PC 构建 exit 0 | R9 |
| R10 gate 四源一致 + 全量套件（读校验） | `docs/evidence/2455/pre-ready-2455.json`、`docs/evidence/2455/plan.md` | `/work` 以 `--full-gate-cmd` 跑一次 pre-ready（`cd backend && go test ./handlers -count=1 -timeout 15m` 与 `./services`），随后 `plan-source.sh --issue 2455` | 修后树（干净） | 后端 | gate `result=PASS` 且含 `FULL_SUITE_EXIT=0 FAIL_COUNT=0 PANIC_COUNT=0`（P5 不得为 N/A）；`PLAN_SOURCE: PASS` | R10 |

## Input Truth & Tool Preflight

- **Source of truth**：
  - `sites` 表结构（`backend/models/models.go:469-490`）无 `(tenant_id, name)` 唯一约束。
  - 创建路径把 `req.Name` 同时写 IAM 组织名与本地 `sites.name`（`backend/handlers/site.go:327-341`）→ 索引必须在 IAM 调用**之前**预检，否则会产生无本地行的 IAM 孤儿组织。
  - `isUniqueViolation(err)` 已存在于 `backend/handlers/order_no.go:23`（匹配 `23505` / `duplicate key`）→ 直接复用，不新造。
  - 测试库由 GORM `AutoMigrate` 建表（`backend/handlers/testfixtures/db.go:20-80`），**不含迁移里的原生索引** → R1/R3 必须在测试内显式执行迁移 SQL（`../database/migrations/…`）并断言索引存在；这是本条目的已知工具盲区。
  - 移动端身份切换：`AccountSelect.jsx:49,58` 现在 contexts 用 `ct.label || org_name`，legacy 用 `merchant_name-site_name`（'-' 分隔）→ 口径不统一，本单收敛到服务端 `label`。
  - `/users/me` 已返回 `site_name` 与 `tenant_name`（`backend/handlers/user_staff.go:1213-1232`）→ R8 无需后端改动。
- **差异核对**：预生产 3 个网点 `sites.name` 与 IAM 组织名 3/3 不一致（#2455 补充评论）；本单**不修也不能修**存量命名，只保证「同商户不再重复 + UI 可区分」；存量对齐由 #2457 的只读报告 + Owner 授权执行。
- **损坏输入拒绝**：`tenant_id` 为空 → 拒绝建/改（400）；名字仅空白 → 400；同名判定用 `lower(name)` 并 `trim`；迁移发现存量重复 → `RAISE EXCEPTION` 并列出 tenant/name/ids（不静默跳过建索引）。
- **工具盲区**：`frontend-pc` 无测试运行器（只有 dev/build/lint/preview）→ R8 用源码结构断言 + 构建（`evidence_type: static`，Owner 豁免见 D6）；小程序 `vitest` 仅纳入 `scripts/page-smoke/**/*.test.jsx`，故 R7 同样使用静态结构断言（组合语义由 R6 后端测试保证）。
- **运行环境**：测试库 `tuneloop_test`；fake IAM 沿用 `t.Setenv("IAM_NAMESPACE","test-ns")` + `services.SetIAMInternalURLForTesting(srv.URL)`（`backend/handlers/auth_test.go:81-87`）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 唯一约束口径：`(tenant_id, lower(name)) WHERE deleted_at IS NULL` 的**唯一部分索引**（非表约束），普通 `CREATE UNIQUE INDEX`（迁移在事务内；`sites` 体量小）。**Owner 已批准 DDL（O2）** | Owner / Architect | owner_approved | 若 ops 要求 CONCURRENTLY → 需拆独立运维迁移，回 Architect |
| D2 | 冲突语义：同商户重名 = 业务冲突 `40900`（HTTP 409），绝不允许 500。**创建（CreateSite）**：判定权威 = IAM `CreateOrganization` 组织名唯一性，**不做本地预检**（`IAM_CALLS=1`，Owner 2026-11-27 裁 B）。**编辑（UpdateSite）**：沿用 #2457 已落地的「先本地同名预检、后 IAM 改名」（`IAM_CALLS=0`），本单只回归不重实现；矩阵 R4 即该语义 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 存量处理：迁移在发现同商户重复时 **fail-fast 并列出清单**（不得静默跳过）；`sites.name` ↔ IAM 组织名的存量对齐**方向 = IAM→本地，已获 Owner 批准（O2）**，执行由 ops 在预生产/生产按只读清单逐条核对（禁止静默改名） | Owner / ops | owner_approved（方向与授权） | 执行窗口与回执由 `[Ops-Followup of #2455]` 承载 |
| D4 | 身份上下文标签：`merchants.name` 与 `sites.name` **各自**等于其 IAM 组织名（商户根组织 / 网点组织两个层级，Owner 澄清）；`label` = `商户名 · 网点名` **仅用于跨商户同名网点的区分**（商户名缺失时用 IAM `org_name`，都缺失回退 org_id）；`org_name` 单独返回供 UI 展示，不改 IAM | Architect | 本计划锁定 | 若要求只显示单一名字 → 回 Architect |
| D5 | 权威源：`sites.name` 仍为 IAM 组织名镜像（#2457 D1）；本单不引入第二套命名来源 | Architect | 本计划锁定 | 阻塞实现 |
| D6 | R7/R8 为纯前端渲染接线，PC 无测试运行器、小程序 vitest 仅收 `scripts/page-smoke/**` → 采用 `evidence_type: static` 源码结构断言 + 构建，**Owner 豁免**（Architect 授予） | Architect | 本计划锁定 | 若审计要求 render → 另立前端测试基建 Issue |
| D7 | **保留** `CreateSite` 的 IAM 先行顺序（#2457）；本单负责把 IAM conflict 映射成中文 409 并保证不产生本地半态；**不新增任何本地预检**（Owner 裁 B：重名判定唯一权威 = IAM） | Architect | 本计划锁定 | 若要求本地预检 → 与 B 冲突，回 Owner |
| D7b | **唯一索引 ≠ 预检**：`uq_sites_tenant_name (tenant_id, lower(name)) WHERE deleted_at IS NULL` 作为**存储层不变量**保留（R1/D1 不变）：它不参与「是否允许创建」的判定（请求仍先发 IAM），只在 IAM 放行后兜底防并发/导入绕行；违反时把 PG `23505` 映射为同一 `409 40900 + 中文`（R3） | Architect | 本计划锁定（Rev.5：消除 D1/R1 与 D7 的自相矛盾；Owner 裁 B 只否定「预检」，不否定存储约束） | 若 Owner 要求连索引一起删 → 删 R1/Files/迁移并把 R3 的 `23505` 断言一并移除 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R7 | static | 源码结构断言（渲染服务端 `label`），Owner 豁免见 D6 |
| R8 | static | 源码结构断言（`商户 · 网点` 组合）+ PC 构建，Owner 豁免见 D6 |

## Out of Scope

- 存量 3 条命名对齐的**实际执行**（方向已获 Owner 授权 = IAM→本地；由 ops 按只读清单执行并回执，本单不执行、不静默改名）。
- IAM 组织改名 API 与 `UpdateSite` 同步顺序（#2457）。
- PC 选择组织页（IAM `SelectOrg` 前端，展示 `organizations.name`，属 beaconiam 仓库）。
- PC 员工管理列表（`StaffManagement.jsx`）网点列：列表按当前租户隔离，不会出现跨商户同名网点，故不改。
- 网点 `/api/sites` 列表增加商户名/组织名字段（无跨商户混排场景）。
- 跨商户网点名唯一（明确**允许**同名，见 R3）。
- 全量 `go test ./...`（由批次 gate 承担）。

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|---|---|---|---|
| R1 | 测试库 `sites`（无 `uq_sites_tenant_name`）→ 执行 `20261123002_site_name_unique_2455.up.sql` 两次 → 执行 down | 迁移 SQL + `pg_indexes` 查询 | `docs/evidence/2455/R1-index.txt` |
| R2 | tenant=`bfce6c6d…`；已存在 site name=`卡丹萨乐器`；请求 `{"name":"卡丹萨乐器"}`（另一请求 `{"name":" 卡丹萨乐器 "}`） | `SiteHandler.CreateSite`（fake IAM 返回 conflict） | `docs/evidence/2455/R2-create-conflict.txt` |
| R3 | tenant A 有 `卡丹萨乐器`；tenant B 无同名 | `CreateSite` + 直接 SQL 插入 | `docs/evidence/2455/R3-cross-tenant.txt` |
| R4 | 同商户两个 site（`卡丹萨乐器`、`北京中转站`）；把后者改名为 `卡丹萨乐器` | `SiteHandler.UpdateSite` | `docs/evidence/2455/R4-update-conflict.txt` |
| R5 | 干净 fixture（3 行不同名）/ 重复 fixture（同商户 2 行 `卡丹萨乐器`） | `scripts/site-name-duplicates.sh --report`（fixture DSN） | `docs/evidence/2455/R5-duplicates.txt` |
| R6 | IAM contexts：`{org_id:76d46658…,name:海淀店}`、`{org_id:a5eb38d4…,name:卡丹萨西铁营}`；本地 sites 两行同名 `卡丹萨乐器`，tenant 分别为 `bfce6c6d…`（merchants.name=`cadenza`）与 `bd6bfa4b…`（merchants.name=`卡丹萨`） | `GET /auth/wx-accounts`（fake IAM） | `docs/evidence/2455/R6-context-label.txt` |
| R7 | 修后 `frontend-mobile/src/pages-weapp/account-select/AccountSelect.jsx` | 小程序「切换身份」渲染分支 | `docs/evidence/2455/R7-mobile-label.txt` |
| R8 | 修后 `frontend-pc/src/pages/UserProfile.jsx` | PC 用户详情「关联网点」 | `docs/evidence/2455/R8-pc-profile.txt` |
| R9 | 修后 backend + frontend-pc | `go build ./...` + 定向测试 + `npm run build` | `docs/evidence/2455/R9-build.txt` |
| R10 | 修后干净树 + `--full-gate-cmd` 产出的 gate 报告 | 读校验（`plan-source.sh` + gate 解析） | `docs/evidence/2455/R10-gate.txt` |

### Assertions

- R1：`INDEX_FOUND=1`、`INDEX_DEFINITION=(tenant_id, lower(name))`、`UP_IDEMPOTENT=1`、`DOWN_DROPS=1`；负例：缺 `WHERE deleted_at IS NULL` → FAIL。
- R2：`HTTP=409`、`CODE=40900`、`MSG_ZH=1`、`IAM_CALLS=1`（由 IAM 判定）、`LOCAL_ROWS_ADDED=0`、`TRIMMED_CASE_HTTP=409`；负例：`HTTP=500`、message 为英文、或本地残留半态行 → FAIL。
- R3：`CROSS_TENANT_HTTP=200`、`SAME_TENANT_DUP_REJECTED=1`、`RACE_MAPPED=409`；负例：`SAME_TENANT_DUP_REJECTED=0` 或 `RACE_MAPPED=500` → FAIL。
- R4：`HTTP=409`、`CODE=40900`、`MSG_ZH=1`、`IAM_CALLS=0`（#2457 的本地同名预检先于 IAM）、`DB_NAME=北京中转站`（未变）；负例：`DB_NAME=卡丹萨乐器` → FAIL；**`IAM_CALLS=1`（预检被绕过、请求放行到 IAM）→ FAIL**。
- R5：`DUP_GROUPS=0`（干净）、`FIXTURE_DUP_GROUPS=1`、`FIXTURE_IDS=2`、`READONLY=1`；负例：`UPDATE`/`ALTER` 语句出现 → FAIL。
- R6：`LABEL_A=cadenza · 卡丹萨乐器`、`LABEL_B=卡丹萨 · 卡丹萨乐器`、`ORG_NAME_B=卡丹萨西铁营`、`DISTINCT=1`；负例：两项 label 相同 → FAIL。
- R7：`MOBILE_USES_SERVER_LABEL=1`、`LEGACY_USES_SERVER_LABEL=1`；负例：出现 `site_name`-only 拼接 → FAIL。
- R8：`PC_SHOWS_MERCHANT_SITE=1`、`FALLBACK_SINGLE=1`；负例：直接渲染 `undefined` 或只用 `site_name` → FAIL。
- R9：`BUILD_EXIT=0`、`TESTS_FAIL=0`、`PC_BUILD_EXIT=0`；负例：任一非 0 → FAIL。
- R10：`FULL_SUITE_EXIT=0`、`FAIL_COUNT=0`、`PANIC_COUNT=0`、`PLAN_SOURCE=PASS`、`P5!=NA`；负例：`P5=NA` 或 sha 不一致 → FAIL。
- 通用 provenance：每个 artifact 首行 `# build=1.0.<fix>` `# commit=<sha>` `# dirty=false`。
- 负例（跨行）：任何 artifact 出现 `已改名`/`renamed existing site`/`跳过唯一索引` 语义 → FAIL。

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2455/R1-index.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2455/R2-create-conflict.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2455/R3-cross-tenant.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2455/R4-update-conflict.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2455/R5-duplicates.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2455/R6-context-label.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2455/R7-mobile-label.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2455/R8-pc-profile.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2455/R9-build.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2455/R10-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| symptom | trigger | root cause | test blind spot | same-class paths | fix layer | red-green | prevention |
|---|---|---|---|---|---|---|---|
| 两个员工上下文都显示「卡丹萨乐器」，无法区分 | 移动端切换身份 / PC 员工身份 | 显示只用 `sites.name`，未带商户/组织名 | 无跨商户同名 fixture | 组织选择、员工列表、用户详情「关联网点」 | 后端 label 组合 + 前端渲染 | R6 先写同名 fixture（旧实现 label 相同 → FAIL），再实现（PASS） | 身份上下文统一 `label` 口径（D4） |
| 同商户可写入两个同名网点 | 网点创建（**本地不预检**，IAM 判定） | `sites` 无唯一约束 | 无重复名用例 | 批量导入网点、代建网点 | DB 唯一索引（`23505` 兜底）+ IAM 组织名唯一性 | R1/R2/R3 红绿（R2 `IAM_CALLS=1`） | 索引 + 409 语义写进 api.md |
| 重名时先建 IAM 组织再失败 → 孤儿组织 | 创建同名网点 | 无预检、IAM 先行（Owner 裁 B） | 无 IAM 调用计数断言 | 商户创建、平台代建 | IAM 组织名唯一性 + 本地 `LOCAL_ROWS_ADDED=0`（不落半态） | R2 `IAM_CALLS=1` + `LOCAL_ROWS_ADDED=0` 红绿 | 重名判定权威 = IAM；编辑改名沿用 #2457 的本地预检 |
| 重名返回 500 | 未映射 `23505` | 缺错误映射 | 无并发/竞态用例 | 其他唯一索引（order_no 已复用） | handler | R3 `RACE_MAPPED=409` | 复用 `isUniqueViolation` |
| 迁移在脏数据上静默跳过约束 | 预生产存在同商户重复 | 迁移未 fail-fast | 无脏数据迁移用例 | 其他唯一索引迁移 | migration | R1 负例断言 | 迁移遇重复 → 异常 + 清单 |
| legacy 分支与 contexts 分支显示口径不一致 | 老 IAM 返回无 contexts | 两套拼接逻辑 | 无 legacy fixture | 账号选择/邀请页 | 后端 label + 前端优先 label | R6/R7 红绿 | 服务端口径唯一 |
| PC 员工身份缺商户名 | `/users/me` 有 `tenant_name` 但未渲染 | 前端只用 `site_name` | 无静态断言 | 移动端 Profile 身份徽标 | PC 渲染 | R8 红绿 | 静态结构断言纳入矩阵 |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2455.sh r1` | `docs/evidence/2455/R1-index.txt` | exit 0；grep `INDEX_FOUND=1`、`UP_IDEMPOTENT=1`、`DOWN_DROPS=1` | PASS | 索引定义缺 `deleted_at IS NULL` → FAIL | dev |
| R2 | `bash scripts/verify-2455.sh r2` | `docs/evidence/2455/R2-create-conflict.txt` | exit 0；grep `CODE=40900`、`MSG_ZH=1`、`IAM_CALLS=1`、`LOCAL_ROWS_ADDED=0`、`TRIMMED_CASE_HTTP=409` | PASS | `HTTP=500` / 英文 message / 本地半态残留 → FAIL | dev |
| R3 | `bash scripts/verify-2455.sh r3` | `docs/evidence/2455/R3-cross-tenant.txt` | exit 0；grep `CROSS_TENANT_HTTP=200`、`SAME_TENANT_DUP_REJECTED=1`、`RACE_MAPPED=409` | PASS | `SAME_TENANT_DUP_REJECTED=0` → FAIL | dev |
| R4 | `bash scripts/verify-2455.sh r4` | `docs/evidence/2455/R4-update-conflict.txt` | exit 0；grep `CODE=40900`、`MSG_ZH=1`、`IAM_CALLS=0`、`DB_NAME=北京中转站` | PASS | `DB_NAME=卡丹萨乐器` 或 `IAM_CALLS=1`（预检被绕过）→ FAIL | dev |
| R5 | `bash scripts/verify-2455.sh r5` | `docs/evidence/2455/R5-duplicates.txt` | exit 0；grep `DUP_GROUPS=0`、`FIXTURE_DUP_GROUPS=1`、`READONLY=1` | PASS | `READONLY=0` → FAIL | dev |
| R6 | `bash scripts/verify-2455.sh r6` | `docs/evidence/2455/R6-context-label.txt` | exit 0；grep `DISTINCT=1`、`LABEL_B=卡丹萨 · 卡丹萨乐器`、`ORG_NAME_B=卡丹萨西铁营` | PASS | `DISTINCT=0` → FAIL | dev |
| R7 | `bash scripts/verify-2455.sh r7` | `docs/evidence/2455/R7-mobile-label.txt` | exit 0；grep `MOBILE_USES_SERVER_LABEL=1`、`LEGACY_USES_SERVER_LABEL=1` | PASS | 任一 `=0` → FAIL | dev |
| R8 | `bash scripts/verify-2455.sh r8` | `docs/evidence/2455/R8-pc-profile.txt` | exit 0；grep `PC_SHOWS_MERCHANT_SITE=1`、`FALLBACK_SINGLE=1` | PASS | 只用 `site_name` → FAIL | dev |
| R9 | `bash scripts/verify-2455.sh r9` | `docs/evidence/2455/R9-build.txt` | exit 0；grep `BUILD_EXIT=0`、`TESTS_FAIL=0`、`PC_BUILD_EXIT=0` | PASS | 任一非 0 → FAIL | dev |
| R10 | `bash scripts/verify-2455.sh r10` | `docs/evidence/2455/R10-gate.txt` | exit 0；grep `FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | PASS | `P5=NA` 或 sha 不一致 → FAIL | dev |

### Workload

- Size: L
- Basis: 1 迁移对(2 文件) + 2 后端源文件 + 2 后端测试 + 1 报告脚本 + 1 脚本测试 + 1 verify 脚本 + 2 前端文件 + 2 文档 + **10 条证据行（R1–R10）**；两阶段 gate（数据完整性 → 身份展示）
- Confidence: medium（测试库不含迁移原生索引，R1/R3 需在测试内执行迁移 SQL；前端两行为静态证据）

### Files（变更文件清单）

- `backend/database/migrations/20261123002_site_name_unique_2455.up.sql`
- `backend/database/migrations/20261123002_site_name_unique_2455.down.sql`
- `backend/handlers/site.go`
- `backend/handlers/auth.go`
- `backend/handlers/site_name_unique_2455_test.go`
- `backend/handlers/iam_proxy_test.go`
- `backend/handlers/wx_accounts_context_label_2455_test.go`
- `frontend-mobile/src/pages-weapp/account-select/AccountSelect.jsx`
- `frontend-pc/src/pages/UserProfile.jsx`
- `scripts/site-name-duplicates.sh`
- `scripts/tests/test_site_name_duplicates.sh`
- `scripts/verify-2455.sh`
- `docs/spec/api/api.md`
- `docs/spec/ui/ui.md`
- `docs/evidence/2455/plan.md`
- `docs/evidence/2455/R1-index.txt`
- `docs/evidence/2455/R2-create-conflict.txt`
- `docs/evidence/2455/R3-cross-tenant.txt`
- `docs/evidence/2455/R4-update-conflict.txt`
- `docs/evidence/2455/R5-duplicates.txt`
- `docs/evidence/2455/R6-context-label.txt`
- `docs/evidence/2455/R7-mobile-label.txt`
- `docs/evidence/2455/R8-pc-profile.txt`
- `docs/evidence/2455/R9-build.txt`
- `docs/evidence/2455/R10-gate.txt`
- `docs/evidence/2455/pre-ready-2455.json`
- `docs/evidence/2455/plan.md`
- `docs/evidence/2455/evidence.json`

## Dependencies

- #2457

### 依赖与关联说明（非硬依赖）

- **前置 #2457（Owner O1=A / O4）**：`sites.name` 镜像语义与 `UpdateSite`「先 IAM 后本地」顺序由 #2457 落地；`status:ready` **视为已实现、可放行下游开发**（Owner O4，不再因 ready 触发 holdon↔unhold 活锁），但**合并顺序 #2457 → #2455**，最终证据在合并后的 `origin/develop` 基线重跑。落地校验：`git merge-base --is-ancestor origin/issue-2457 origin/develop` 且 `grep -q 'func.*UpdateOrganizationName' backend/services/iam_client.go` 且 `UpdateSite` 内存在 IAM 更新调用。
- **Phase A / Phase B**：Phase A（数据完整性 R1–R5）可独立验收；Phase B（R6–R9）依赖 #2457 的存量对齐结论（D5）。
- **关联 #2454**（登录上下文缺陷，不阻塞）。
- **部署侧**：存量对齐执行与回执由 `[Ops-Followup of #2455]`（`waitForConfirm`）承载。

*Model: DeepSeek V4.1 Flash 1017*






