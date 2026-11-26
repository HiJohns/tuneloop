## Accepted Plan — #2515 丢失台账跨网点泄漏：`GET /api/instrument-loss` 按「乐器可见性」收敛（复用 #1911 口径，无 migration）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| `List` 只按 `tenant_id` 过滤，**完全不消费 JWT `oid`**；`db.WithContext(ctx)` 只自动注入 `tenant_id`（无 org/site 注入） | `backend/handlers/instrument_loss.go:388-410`；`backend/database/db.go:103-129`（`addTenantScope` 仅 `tenant_id`） |
| 路由启用 `ExplicitTenantScope()`（关闭自动租户范围）→ handler 必须自己显式隔离 | `backend/main.go:442` |
| `site_admin` / `site_member` 均持有 `instrument:read` → 网点级账号可拉到**本商户全部网点**台账（金额/责任方/描述/照片） | `backend/services/role_templates.go:49,63,69` |
| 既有的「乐器可见性」唯一权威：`applyInstrumentListScope`（#1911）——站点角色 → `site_id IN (本人 site_members 集合)`，否则 `ApplyOrgScope`（商户 = 组织+下级；平台/系统 = 不过滤） | `backend/handlers/api.go:354-370` |
| `GetVisibleOrgIDs`：`site_member` → 仅自身 org；`merchant_admin` → org + 下级站点 org；`system_admin`/`platform_staff` → 不过滤 | `backend/middleware/iam.go:659-677` |
| `instruments.site_id` = **归属站点**（`sites.id`），仅显式改归属时变；`current_site_id` = 物理位置，**每次调拨被改写** | `backend/handlers/instrument.go:748`（`site_id`）；`backend/handlers/warehouse.go:695,870`（`current_site_id`） |
| `instrument_loss_records` **无 `site_id` / `org_id` 列**（只有 `tenant_id`/`instrument_id`/`order_id`） | `backend/models/models.go:1450-1478` |
| 核心文档已规定乐器数据范围 = 「本网点/商户」 | `docs/spec/permissions.md:132`（查看列表/详情 + 数据范围） |
| 丢失/找回 3 个端点**从未在 API 文档登记** | `docs/spec/api/api.md`（`grep 丢失` 无命中） |

**根因**：台账是「乐器数据的派生物」（`docs/cases/instrument-loss.md:61`：`instrument_loss_records` 是丢失原因的唯一权威来源），却绕过了乐器可见性口径，直接以 `tenant_id` 全量返回 → 网点账号看到同商户其它网点的财务凭证。

### 归属判定口径（本单裁定，D1）

**台账可见性 = 乐器可见性**：复用 `applyInstrumentListScope`（#1911），网点级账号按 `instruments.site_id`（归属站点）过滤。

| 候选 | 结论 | 理由 |
|---|---|---|
| `instruments.site_id`（**采用**） | ✅ | 无 migration；与全仓乐器列表可见性同一权威（不会出现「乐器列表看不到、台账看得到」的错位）；台账是财务凭证，须锚定**归属**站点 |
| `instruments.current_site_id` | ❌ | 调拨会改写（`warehouse.go:695,870`）→ 历史凭证随乐器「搬家」，把 A 网点的赔偿金额泄漏给 B 网点（新增泄漏向量） |
| 新增 `site_id` 快照列 | ❌（本单） | 需 up/down + `defaultMigrationArtifacts` 登记 + 启动校验 + 预生产执行授权（AGENTS.md 迁移纪律），且本单口径已可满足需求 → 如需「丢失时刻快照」另立 Issue |
| `orders.org_id` | ❌ | 纯库存丢失 `order_id IS NULL`（`models.go:1454` 注释）→ 会把库存台账整批藏掉 |

### 变更点

**A. 代码（后端）**
1. `backend/handlers/instrument_loss.go` `List`（`:388-410`）：保留租户过滤，新增站点收敛（**不改** `loadLossInstrument`，避免与在途 #2510 冲突）：

```go
query := db.Model(&models.InstrumentLossRecord{}).Where("instrument_loss_records.tenant_id = ?", tid)
// #2515：台账可见性 = 乐器可见性（#1911 applyInstrumentListScope 口径）：
// 网点级账号按 instruments.site_id（归属站点）过滤；商户/平台保持全商户可见。
if role := middleware.GetBusinessRole(ctx); role == middleware.BusinessRoleSiteAdmin || role == middleware.BusinessRoleSiteMember {
	visible := db.Model(&models.Instrument{}).Select("id").Where("instruments.tenant_id = ?", tid)
	visible = applyInstrumentListScope(db, ctx, visible)
	query = query.Where("instrument_id IN (?)", visible)
}
```
   - `?instrument_id=` / `open=true` 过滤保持不变（与站点条件取交集；跨站点指名查询 → 空列表）；
   - 返回结构 `{list, total}` 不变（前端零改动）。

**B. 测试（`backend/handlers/instrument_loss_2515_test.go`，新文件，自带夹具）**
2. 夹具**必须**让 ID 域分离（#2508/#2510 口径）：`sites.id ≠ sites.org_id ≠ tenant_id`；`site_members` 为操作者建行；乐器 `org_id` = 所属站点 org、`site_id` = 站点 id、`current_site_id` 故意设为**另一站点**（用于证明 `current_site_id` 不参与判定）。
3. 用例：
   - `TestInstrumentLoss2515_SiteScope`：网点 A 账号 → 仅 A 的 1 条（`CROSS_SITE_LEAK=0`），并输出旧谓词复现计数（`OLD_PREDICATE_LEAK=2`）作为红证据（`RED_CONFIRMED=1`）；
   - `TestInstrumentLoss2515_MerchantAndPlatform`：`tid==oid` 商户管理员 / `NAMESPACE_ADMIN` → 均 2 条（不回归）；
   - `TestInstrumentLoss2515_MultiSiteStaff`：同一用户两条 `site_members` → 2 条（#1911 多站点语义）；
   - `TestInstrumentLoss2515_NegativeCrossSiteInstrument`：网点 A 指名 B 的 `instrument_id` → 0 条；并断言新过滤未引用 `current_site_id`（`DRIFT_SCOPE=0`）。
4. 不修改既有 `setupLossFixture` / `TestInstrumentLoss_*`（避免与 #2510 在途改动冲突）。

**C. 文档**
5. `docs/spec/api/api.md` 新增 `### 5.12 乐器丢失 / 找回 / 台账`：登记 `POST /instruments/:id/lost`、`POST /instruments/:id/restore`、`GET /instrument-loss`（参数 `instrument_id`/`open`、响应 `{list,total}`、权限 `instrument:update|read`），并写明台账可见性口径（本网点归属站点 / 商户全量 / 平台全量）。
6. `docs/cases/instrument-loss.md`：LS-04 台账条目补「可见范围」一行（网点员工=本网点归属台账；平台/商户=全商户），角色矩阵补同一句。

**D. 证据**
7. 新增 `scripts/verify-2515.sh`（`r1`–`r6` + `gate`；每条首行写 provenance；`r5` 内部跑 `cd backend && go test ./handlers -run TestInstrumentLoss -count=1 -timeout 15m`，`gate` 内部跑 `cd backend && go build ./...`）。

- Tags: bug, code:go, scope:backend

### Workload

- Size: M
- Basis: 2 个后端文件（1 处 handler + 1 个新测试文件）+ 2 个文档 + 1 个证据脚本 + 7 条证据行；无 migration、无前端改动、无外部依赖
- Confidence: high

## Core Update

| 文档路径 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | 丢失/找回/台账 3 个端点**完全未登记**（`grep 丢失` 无命中） | 新增 §5.12：3 个端点的路径/方法/权限/参数/响应 + 台账可见性口径（网点=归属站点；商户/平台=全量） | 行为变更必须有权威契约；口径不入文档会再次漂移（本 bug 的成因之一） |
| `docs/cases/instrument-loss.md` | LS-04 只写「丢失记录本身即台账：责任方/描述/金额/比例可查」，未写可见范围 | LS-04 补「可见范围」；角色矩阵补同一句 | 用例层固化「网点只看本网点台账」，避免后续再被当成「全商户台账」 |

## Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 触发条件 | 网点员工（`site_member`/`site_admin`，持 `instrument:read`）打开小程序「乐器管理 → 丢失台账」→ `GET /instrument-loss` → 返回本商户全部网点记录（金额/责任方/照片） | `frontend-mobile/src/pages/InstrumentLossManage.jsx:95`；`backend/main.go:442` |
| 2 | 根因 | 台账过滤只到 `tenant_id`，未接入既有乐器可见性口径（`applyInstrumentListScope`）；`ExplicitTenantScope` 关闭自动租户注入后，handler 需自行隔离，此处只做了一半 | `instrument_loss.go:391-396`、`api.go:354-370` |
| 3 | 为何长期未发现 | `GET /instrument-loss` **零测试**（`grep "instrument-loss" backend/handlers/*_test.go` 仅路由注册 1 处）；页面无后端契约文档；夹具长期把 `site.id` 当 org 用（ID 域同值）→ 即便写测试也看不出 | `backend/handlers/instrument_loss_test.go:68`；`SeedSiteFK`（`org_id=tenant_id`） |
| 4 | 同类扫描 | 同族可见性问题：#2510（写侧 ID 域，在途）、#2511/#2512/#2513（ID 域子任务）；本单只修 `List` 读侧 | `backend/handlers/instrument_loss.go`、#2509 家族 |
| 5 | 防复发（代码） | 台账过滤改为调用 `applyInstrumentListScope`（唯一权威），不再自写 tenant-only 查询；口径注释指向 #1911 | 本计划变更点 1 |
| 6 | 防复发（测试） | 新增 4 个用例，夹具强制 `sites.id ≠ sites.org_id`，并显式把 `current_site_id` 指向他站（证明不参与判定） | 本计划变更点 2–3 |
| 7 | 防复发（文档） | api.md 登记 3 个端点 + 可见性口径；用例 LS-04 补可见范围 | 本计划 Core Update |
| 8 | 假阴性反例 | 若过滤条件退回 tenant-only、或改用 `current_site_id`、或网点账号指名他站 `instrument_id` 仍返回数据 → 必须 FAIL | R1/R4 负例 |
| 9 | 回归面 | 商户管理员/平台/系统管理员必须仍见全量；多站点员工必须见全部所属站点（#1911 不回归） | R2/R3 |
| 10 | 未修边界 | 丢失时刻的站点快照（如业务确需「凭证永久锚定丢失时位置」）→ 需新增列 + migration，另行立项（D1 已裁定本单不做） | D1、Out of Scope |

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 网点隔离主诉 + 红证据 | `backend/handlers/instrument_loss.go`、`backend/handlers/instrument_loss_2515_test.go` | `GET /instrument-loss` | 网点 A 账号（`oid=aOrg≠tid`，`site_members:[aID]`），记录 recA/recB 分属 A/B 站 | backend（Postgres 测试库） | `SITE_A_IDS=recA`、`CROSS_SITE_LEAK=0`、`OLD_PREDICATE_LEAK=2`、`RED_CONFIRMED=1` | R1 |
| R2 商户 / 平台不回归 | `backend/handlers/instrument_loss_2515_test.go` | 同上 | `tid==oid` 商户管理员；`NAMESPACE_ADMIN` | backend | `MERCHANT_SEES=2`、`PLATFORM_SEES=2`、`MERCHANT_IDS=recA,recB` | R2 |
| R3 多站点员工（#1911）不回归 | `backend/handlers/instrument_loss_2515_test.go` | 同上 | 用户 `site_members` 含 A、B 两站 | backend | `MULTISITE_SEES=2` | R3 |
| R4 负例：跨站点指名 + 漂移口径排除 | `backend/handlers/instrument_loss.go`、`backend/handlers/instrument_loss_2515_test.go` | 同上 + `?instrument_id=<B 的乐器>` | 网点 A 账号指名 B 站乐器；夹具 `current_site_id` 指向他站 | backend | `CROSS_INSTRUMENT_ROWS=0`、`DRIFT_SCOPE=0`（新过滤不含 `current_site_id`） | R4 |
| R5 既有丢失套件回归 | 既有丢失测试套件（TestInstrumentLoss_*） | `scripts/verify-2515.sh r5` | 全部 `TestInstrumentLoss_*`（LS-03/03a/03b/05/06 结算链路） | backend | `REGRESSION_PASS=1`、`FAIL_COUNT=0` | R5 |
| R6 构建 + 同类扫描 | `scripts/verify-2515.sh` | `cd backend && go build ./...` + grep | 修后工作树 | backend | `BUILD_RC=0`、`LIST_TENANT_ONLY=0`、`LIST_USES_SHARED_SCOPE=1`、`ROLE_GATE_PARITY=1`、`CURRENT_SITE_ID_IN_LIST=0` | R6 |
| R7 pre-ready gate（四源 + 定向测试 + 构建） | `docs/evidence/2515/pre-ready-2515.json` | `pre-ready-check.sh 2515` | 修后干净树 + 最新 Accepted Plan | backend | `GATE_RESULT=PASS`、P12 evidence 完整、P13 修后提交在 `origin/issue-2515` | R7 |

## Input Truth & Tool Preflight

- **Source of truth（口径唯一）**：
  - 可见性口径：`backend/handlers/api.go:354-370`（`applyInstrumentListScope`，#1911）+ `backend/middleware/iam.go:659-677`（`GetVisibleOrgIDs` 层级）；
  - 核心文档：`docs/spec/permissions.md:132`（乐器数据范围 = 本网点/商户）；
  - 业务口径：`docs/cases/instrument-loss.md:61`（台账是丢失原因的唯一权威来源）+ `:93`（网点员工可登记/找回）；
  - ID 域：`docs/cases/cases.md:289`（`sites.org_id` = 网点 IAM 组织）与 `models.go:99`（`current_site_id` = `sites.id`）。
- **差异核对（只读，已于 /analyze 执行）**：
  - `grep -n "GetOrgID\|BusinessRole" backend/handlers/instrument_loss.go` → List 中 0 命中（确认未消费 oid）；
  - `grep -n "addTenantScope" -A 25 backend/database/db.go` → 仅注入 `tenant_id`，无 org/site 注入（确认泄漏真实存在）；
  - `grep -rn "current_site_id" backend/handlers/warehouse.go` → `:695,:870` 调拨改写（漂移证据）；
  - `grep -c "instrument-loss" backend/handlers/instrument_loss_test.go` → 1（仅路由注册，List 无测试）；
  - dev 库数据核查（只读）：`instruments` 1 行、`instrument_loss_records` 0 行，且该行 `org_id/site_id/current_site_id` 全 NULL → **dev 无真实样本**，结论必须由夹具证明（写入 Input Truth，不据此推断生产分布）。
- **损坏输入拒绝**：
  - `tid == ""` → 保持 `403/40300`（现状）；
  - 网点账号无 `site_members` 行 → `applyInstrumentListScope` 回退 `org_id IN (自身 org)`（fail-closed，不放大可见范围）；
  - `site_id IS NULL` 的历史乐器的台账对网点级账号**不可见**（fail-closed，可接受；商户/平台仍可见）；
  - 子查询返回空集 → `IN (空)` → 空列表（不得退化为无过滤）。
- **工具盲区声明**：
  - `go test ./handlers` 依赖 Postgres 测试库（`docs/ops/test-db-lifecycle.md` 模板库 + 按节点 run 库），本单只跑**定向**用例；全量套件由 pre-ready P5 baseline 差分承担。
  - dev 库无丢失记录样本 → 生产数据分布（`instruments.site_id IS NULL` 占比、跨站点丢失实际条数）**本单无法核实**，需在 Ops 回执中抽样（见 Out of Scope）。
  - R6 的 grep 为文本级，无法识别动态拼接的角色判定；以 `ROLE_GATE_PARITY=1`（两处同一角色集合）作为最小防漂移断言。
- **证据时序**：fix commit 之后重跑 `scripts/verify-2515.sh r1` … `r6`；`evidence.json` 的 `build/commit/dirty` 由脚本从 `git rev-parse` 派生，禁止手改。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 归属判定口径 = **`instruments.site_id`（归属站点）+ `applyInstrumentListScope`**；明确排除 `current_site_id`（调拨漂移）与 `orders.org_id`（纯库存丢失为空） | Architect | 本计划锁定（依据见「归属判定口径」表 + `permissions.md:132`） | 若 Owner 要求「丢失时刻快照」→ 回 Architect 另立 migration Issue；本单不得私自加列 |
| D2 | 商户/平台/系统管理员**保持全商户可见**（不回归）；网点级判定以 `business_role ∈ {site_admin, site_member}` 为准（与 `applyInstrumentListScope` 同源） | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 本单**不改** `loadLossInstrument`（写侧），避免与在途 #2510（同文件 `:47`）冲突；两单合并顺序无关 | Architect | 本计划锁定 | 若 #2510 先合入并改动 List，则 /work 先 rebase 再按本计划口径执行 |
| D4 | 不新增列/不写 migration（`instrument_loss_records.site_id` 快照） | Architect | 本计划锁定 | 需 Owner 决策 + 迁移纪律（AGENTS.md） |
| D5 | 台账对 `site_id IS NULL` 的历史乐器对网点账号不可见（fail-closed） | Architect | 本计划锁定 | 若 Owner 要求可见 → 需数据回填（迁移）+ 另立 Issue |
| D6 | 生产数据抽样（`site_id IS NULL` 占比、跨站记录数）**不在本单**，由部署侧回执承担 | Architect | 本计划锁定（dev 无样本，见 Input Truth） | 审计若要求实测 → 转 Ops Follow-up |

## Out of Scope

- 写侧 `loadLossInstrument` 的 ID 域纠偏 → **#2510**（`status:ready`，同文件不同函数；本单不改其区域）。
- `warehouse` / `user_rental` / `repair_service` 的 ID 域子任务 → #2511 / #2512 / #2513（父 #2509）。
- 新增 `instrument_loss_records.site_id` 快照列（migration + 回填）→ 未立项，需 Owner 决策（D1/D4）。
- 生产数据分布抽样与「历史 `site_id IS NULL` 台账是否补录」→ 部署侧回执（D6）。
- `GET /instrument-loss` 的 `?status=` 注释漂移（注释写 `status`，实现只有 `open`）→ 文案/参数统一另议（本单不改契约）。
- 分页化（当前 `Limit(200)`，`total=len(list)`）→ 不在本单。
- 前端（H5/小程序/PC）任何改动：页面不改、不新增参数。
- `applyInstrumentListScope` 本身的重构/单测补齐 → 本单只调用，不改共享件（避免无测试覆盖的共享路径改动）。

## G0 Golden Acceptance

| 行 ID | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） | assertions（含负例） |
|---|---|---|---|---|
| R1 | tenant T；site A(`id=aID,org_id=aOrg`)、site B(`id=bID,org_id=bOrg`)；user UA(`site_members:[aID]`)；乐器 instA(`site_id=aID,org_id=aOrg,current_site_id=bID`)、instB(`site_id=bID,org_id=bOrg`)；记录 recA/recB 各 1 条 | `GET /instrument-loss`（actor UA，`oid=aOrg≠tid`） | `docs/evidence/2515/R1-site-scope.txt` | `SITE_A_IDS=[recA]`、`CROSS_SITE_LEAK=0`、`OLD_PREDICATE_LEAK=2`、`RED_CONFIRMED=1`；负例：返回含 recB → FAIL |
| R2 | 同上数据；actor 商户管理员（`oid=tid=T`）/ `NAMESPACE_ADMIN` | 同上 | `docs/evidence/2515/R2-merchant-wide.txt` | `MERCHANT_SEES=2`、`PLATFORM_SEES=2`；负例：商户账号只见 1 条（过度收敛）→ FAIL |
| R3 | 同上数据；actor `site_members:[aID,bID]` | 同上 | `docs/evidence/2515/R3-multisite.txt` | `MULTISITE_SEES=2`；负例：只见 1 条 → FAIL |
| R4 | 同上；actor UA 请求 `?instrument_id=instB` | 同上 + query | `docs/evidence/2515/R4-negative.txt` | `CROSS_INSTRUMENT_ROWS=0`、`DRIFT_SCOPE=0`、`SAME_SITE_ROWS=1`；负例：跨站返回 1 条 → FAIL |
| R5 | 既有 `TestInstrumentLoss_*`（LS-03/03a/03b/05/06） | `scripts/verify-2515.sh r5` | `docs/evidence/2515/R5-regression.txt` | `REGRESSION_PASS=1`、`FAIL_COUNT=0`、`ok` 行存在；负例：任一既有用例 FAIL → 脚本非 0 |
| R6 | 修后工作树 | `cd backend` 构建 + grep 扫描 | `docs/evidence/2515/R6-build-scan.txt` | `BUILD_RC=0`、`LIST_TENANT_ONLY=0`、`LIST_USES_SHARED_SCOPE=1`、`ROLE_GATE_PARITY=1`、`CURRENT_SITE_ID_IN_LIST=0`；负例：List 仍有 tenant-only 查询或引用 `current_site_id` → FAIL |
| R7 | 修后干净树 + 最新 Accepted Plan + evidence.json | `pre-ready-check.sh 2515` | `docs/evidence/2515/pre-ready-2515.json` | `GATE_RESULT=PASS`、每行 `pass=true`、`dirty=false`、P13 在 `origin/issue-2515`；负例：任一 P 失败/evidence 缺行 → 不得 ready |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2515.sh r1 > docs/evidence/2515/R1-site-scope.txt` | `docs/evidence/2515/R1-site-scope.txt` | exit 0；`grep -q '^R1_OK$'` 且 `grep -q 'CROSS_SITE_LEAK=0'` | `SITE_A_IDS=[recA]`、`OLD_PREDICATE_LEAK=2`、`RED_CONFIRMED=1` | 返回含 recB（泄漏未修）→ `R1_FAIL` 且非 0 退出 | dev |
| R2 | `bash scripts/verify-2515.sh r2 > docs/evidence/2515/R2-merchant-wide.txt` | `docs/evidence/2515/R2-merchant-wide.txt` | exit 0；`grep -q '^R2_OK$'` 且 `grep -q 'MERCHANT_SEES=2'` | `PLATFORM_SEES=2` 且商户/平台均见 recA+recB | 商户账号看到 1 条（过度收敛）→ `R2_FAIL` 且非 0 退出 | dev |
| R3 | `bash scripts/verify-2515.sh r3 > docs/evidence/2515/R3-multisite.txt` | `docs/evidence/2515/R3-multisite.txt` | exit 0；`grep -q '^R3_OK$'` 且 `grep -q 'MULTISITE_SEES=2'` | 多站点员工见全部所属站点记录 | 只见 1 条（#1911 语义退化）→ `R3_FAIL` 且非 0 退出 | dev |
| R4 | `bash scripts/verify-2515.sh r4 > docs/evidence/2515/R4-negative.txt` | `docs/evidence/2515/R4-negative.txt` | exit 0；`grep -q '^R4_OK$'` 且 `grep -q 'CROSS_INSTRUMENT_ROWS=0'` | `DRIFT_SCOPE=0`、`SAME_SITE_ROWS=1` | 跨站指名返回 >0 条，或新过滤引用 `current_site_id` → `R4_FAIL` 且非 0 退出 | dev |
| R5 | `bash scripts/verify-2515.sh r5 > docs/evidence/2515/R5-regression.txt` | `docs/evidence/2515/R5-regression.txt` | exit 0；`grep -q '^R5_OK$'` 且 `grep -q 'FAIL_COUNT=0'` | `REGRESSION_PASS=1`、既有结算用例全绿 | 任一既有用例 FAIL → `R5_FAIL` 且非 0 退出 | dev |
| R6 | `bash scripts/verify-2515.sh r6 > docs/evidence/2515/R6-build-scan.txt` | `docs/evidence/2515/R6-build-scan.txt` | exit 0；`grep -q '^R6_OK$'` 且 `grep -q 'BUILD_RC=0'` | `LIST_USES_SHARED_SCOPE=1`、`ROLE_GATE_PARITY=1`、`CURRENT_SITE_ID_IN_LIST=0`、`LIST_TENANT_ONLY=0` | 构建失败或 List 仍为 tenant-only → `R6_FAIL` 且非 0 退出 | dev |
| R7 | `~/.config/opencode/scripts/pre-ready-check.sh 2515 --summary-file docs/evidence/2515/summary.md --changed-files docs/evidence/2515/changed-files.txt --evidence-file docs/evidence/2515/evidence.json --affected-cmd "bash scripts/verify-2515.sh r5" --full-gate-cmd "bash scripts/verify-2515.sh gate" --report docs/evidence/2515/pre-ready-2515.json` | `docs/evidence/2515/pre-ready-2515.json` | exit 0；`GATE_RESULT=PASS`，`jq -e '.result == "PASS"'`，evidence 每行 `pass=true` | `dirty=false`、P13 提交在 `origin/issue-2515`、P12 无缺行 | 任一 P 项失败 / evidence 缺行 / `dirty=true` → gate 非 0，禁止 ready | dev |

### Files（变更文件清单）

| 类别 | 路径 |
|---|---|
| Handler 修复 | `backend/handlers/instrument_loss.go` |
| 新增测试 | `backend/handlers/instrument_loss_2515_test.go` |
| 证据生成器（新增） | `scripts/verify-2515.sh` |
| 核心文档 | `docs/spec/api/api.md` |
| 用例文档 | `docs/cases/instrument-loss.md` |
| 证据（计划） | `docs/evidence/2515/plan.md` |
| 证据（摘要） | `docs/evidence/2515/summary.md` |
| 证据（清单） | `docs/evidence/2515/changed-files.txt` |
| 证据（合同汇总） | `docs/evidence/2515/evidence.json` |
| 证据 R1 | `docs/evidence/2515/R1-site-scope.txt` |
| 证据 R2 | `docs/evidence/2515/R2-merchant-wide.txt` |
| 证据 R3 | `docs/evidence/2515/R3-multisite.txt` |
| 证据 R4 | `docs/evidence/2515/R4-negative.txt` |
| 证据 R5 | `docs/evidence/2515/R5-regression.txt` |
| 证据 R6 | `docs/evidence/2515/R6-build-scan.txt` |
| Gate 报告 | `docs/evidence/2515/pre-ready-2515.json` |

## Artifact Provenance

- 生成器：`scripts/verify-2515.sh`（r1–r6），每个文本 artifact 首行必须是
  `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`（锚定本单 fix commit；证据收尾提交不改变锚点）。
- 逐条清单（全部位于 `docs/evidence/2515/`）：
  - `docs/evidence/2515/R1-site-scope.txt`（红绿 + 网点隔离）
  - `docs/evidence/2515/R2-merchant-wide.txt`（商户/平台不回归）
  - `docs/evidence/2515/R3-multisite.txt`（多站点员工）
  - `docs/evidence/2515/R4-negative.txt`（跨站指名 + 漂移口径排除）
  - `docs/evidence/2515/R5-regression.txt`（既有丢失套件）
  - `docs/evidence/2515/R6-build-scan.txt`（构建 + 同类扫描）
  - `docs/evidence/2515/pre-ready-2515.json`（gate 报告，JSON 自带 `.provenance.{build,commit,dirty}`）
- `docs/evidence/2515/evidence.json` 字段口径：`build` 形如 `1.0.<7-40 位短码>`，
  `commit` 为 40 位 sha，`dirty:false`；evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$
- 证据重生成纪律：fix commit 之后统一重跑 `bash scripts/verify-2515.sh r1` … `r6`，禁止只改 `build/commit` 冒充新证据。

*Model: DeepSeek V4.1 Flash 1017*

