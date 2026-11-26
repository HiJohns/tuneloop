## Accepted Plan — #2577 库存监控页补搜索/筛选（识别码 / 分类 / 状态）

> 需求来源：Owner 2026-11-26（复验 1.0.102 时的可用性缺口）。
> 结论：后端 `GET /api/instruments` **已支持全部所需过滤参数**（`backend/handlers/api.go:395-419`：`sn` ILIKE / `category_id` 含后代 / `level_id` / `stock_status` / `exclude_status`），本单**不改后端生产代码**，只改前端 + 文档，并补一个**只读**后端回归测试用于真实数据证据（证明页面将要发送的参数组合在服务端确实成立）。

### 变更点

#### 1. 新增纯函数模块（筛选语义唯一 owner）

`frontend-pc/src/utils/instrumentStockFilters.js`（新增），导出：

| 导出 | 语义 |
|---|---|
| `readStockFilters(searchParams)` | 从 URL 读 `{ sn, categoryId, status }`；`status` 优先，`stock_status` 作为**兼容读别名**（乐器列表写的是 `stock_status`） |
| `buildStockListParams({ filters, page, pageSize, isManagerLike })` | 生成列表请求参数：`page`/`pageSize`/`sn`/`category_id`/`stock_status`；非管理级追加 `exclude_status=lost,archived,sold`，**显式状态优先**：所选状态 ∈ {lost,archived,sold} 时**不**追加 `exclude_status`（否则恒定 0 行） |
| `buildStockFilterQuery(filters)` | 生成写回 URL 的查询对象：`status`/`sn`/`category_id`，空值剔除（**参数名用 `status`**：Dashboard 深链与导出接口都只认 `status`） |
| `applyFilterPatch({ filters, page, selectedRowKeys }, patch)` | 筛选变化的唯一状态迁移：合并 patch → **页码回 1** → **清空 `selectedRowKeys`**（批量设价/删除只作用于当前可见选中行） |
| `stockStatusLabel(value)` | 本页状态词表（见 D4） |
| `stockStatusOptions({ statuses, isManagerLike, current })` | 状态下拉选项：非管理级剔除 `lost/archived/sold`；`current`（URL 带入）即使不在选项集合中也保留，避免「下拉显示空白但列表已过滤」 |

纯函数不得依赖 DOM / React / `window`（便于 `node --test` 与跨端复用）。

#### 2. 页面接线（`frontend-pc/src/pages/InstrumentStock.jsx`）

- **URL 即筛选唯一来源**（D1）：`filters = readStockFilters(searchParams)`；`page`/`pageSize` 仍为本地 state（D6）。
- 工具栏（仅 `view === 'stock'` 渲染，接在现有「导出 CSV / 批量导入 / 批量设价 / 批量删除」同一 `Space` 内，按 cusPerm 门控保持不变）新增：
  - `Input.Search` 搜索识别码（`onSearch` 触发、`allowClear` 清空即触发，D3）；
  - 分类 `Select allowClear`，选项 = `instrumentsApi.getFilterOptions()` 的 `categories`（与乐器列表同源，D2）；
  - 状态 `Select allowClear`，选项 = `stockStatusOptions(...)`；
  - 「清空筛选」按钮：清空全部筛选 + 回第 1 页 + 清空选择。
- 控件变化一律走 `applyFilterPatch` → `setPage` / `setSelectedRowKeys` / `setSearchParams(..., { replace: true })`；**只有** URL 变化触发取数（`useEffect` 依赖 `searchParams.toString()` + `page` + `pageSize` + `isManagerLike`），不在控件回调里重复请求。
- 取数：`inventoryApi.list(buildStockListParams(...))`；`setTotal(response.data.total)` 照旧（分类/状态/识别码过滤后 `total` 随服务端变化）。
- **移除整页 early return**（现 `if (loading) return <Spin/>`，行 379-385）：改为 `Table loading={loading}`（D7）。否则每次筛选都卸载工具栏 → 输入框失焦、控件闪烁。
- 移除旧「当前筛选: 状态 = X」蓝条（行 460-467），由工具栏控件承接；`?status=` 深链仍生效（URL → 控件回显）。
- 删除已失效的 `statusColors` 常量（#2537 删状态列后已无引用）。
- 导出 `handleExport()` 的 `status` 改为从 `filters.status` 取值（参数名仍 `status`，导出仍按状态，遗留项见 Out of Scope）。

#### 3. 后端回归测试（只读，不改生产代码）

`backend/handlers/instrument_stock_filters_2577_test.go`（新增）：用 `testfixtures.SetupTestDB` + `testutil.TestActor` + `gin` 路由挂 `GetInstruments`，断言页面即将发送的参数组合在真实数据上成立：

| fixture | 期望 |
|---|---|
| 分类 P（父）→ 子分类 C；A(`sn=CVZ-104`,`available`,C) / B(`sn=CVZ-105`,`available`,C) / E(`sn=CVZ-106`,`available`,其他分类) / D(`sn=CVZ-107`,`lost`,其他分类) | `?sn=CVZ-104` → 1 行；`?category_id=P` → 2 行（**后代展开**）；`?stock_status=available` → 3 行；`?exclude_status=lost,archived,sold` → 3 行（D 被排除）；每次 `data.total == len(data.list)` |

#### 4. 文档同步（Core Update）

`docs/spec/ui/ui.md` §2.20.1：在「库存监控」条目补筛选能力与 URL 参数口径（`?status=` 兼容读、`sn`、`category_id`）、状态词表、`exclude_status` 可见性、筛选后清空选择；`docs/spec/api/api.md` §5.4.0 已完整登记 `sn`/`category_id`/`stock_status`/`exclude_status`（本单仅断言其未漂移，不改后端契约）。

#### 5. 证据 harness

`scripts/verify-2577.sh`（新增）：`r1`–`r5` 五个子命令，每个产出 `docs/evidence/2577/R*.txt`，首行 provenance `# build=1.0.<短码> # commit=<sha> # dirty=<bool>`；重命令经 `~/.config/opencode/scripts/heavy-run.sh`（D9）。

### Files（变更文件清单）

- `frontend-pc/src/utils/instrumentStockFilters.js`
- `frontend-pc/tests/instrumentStockFilters.test.mjs`
- `frontend-pc/src/pages/InstrumentStock.jsx`
- `backend/handlers/instrument_stock_filters_2577_test.go`
- `docs/spec/ui/ui.md`
- `scripts/verify-2577.sh`
- `docs/evidence/2577/plan.md`
- `docs/evidence/2577/R1-filter-unit.txt`
- `docs/evidence/2577/R2-api-params.txt`
- `docs/evidence/2577/R3-page-wiring.txt`
- `docs/evidence/2577/R4-docs.txt`
- `docs/evidence/2577/R5-build.txt`
- `docs/evidence/2577/evidence.json`
- `docs/evidence/2577/pre-ready-2577.json`

### Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/ui/ui.md` | §2.20.1「库存监控」条目只写「工具栏含 导出/批量导入/批量设价/批量删除」与「透传 `?status=`」 | 补：工具栏筛选清单（识别码搜索 / 分类 / 状态 / 清空筛选）、URL 参数口径（`?status=` 兼容读、写回 `status`、`sn`、`category_id`；`stock_status` 仅兼容读）、状态词表（待租/在租/维修中/已预订/配送中/已丢失/已下架/已售出）、非管理级 `exclude_status=lost,archived,sold`、筛选变化回第 1 页并清空行选择 | 页面能力扩充后原文案过期，若不同步，后续审计会以 ui.md 为「设计缺失」依据判 REJECT |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 筛选语义纯函数（请求参数 / URL 回写 / 筛选变化重置 / 状态词表） | `frontend-pc/src/utils/instrumentStockFilters.js` | `node --test` | 空筛选；`sn=CVZ-104`；`category_id=c1`；`status=available`；非管理级 `status=lost`；第 3 页 + 已选 2 行时改筛选 | PC（Node 纯函数，不依赖 DOM） | `PARAMS_SN=CVZ-104`；`PARAMS_CATEGORY=c1`；`PARAMS_STATUS=available`；`PARAMS_EXCLUDE=`（非管理级显式 lost 时不叠加排除）；`FILTER_PATCH_PAGE=1`；`FILTER_PATCH_SELECTED=0`；`URL_STATUS=status=available`；`LABEL_AVAILABLE=待租`；`OPTIONS_NONMGR=5`（剔除 3 项） | R1 |
| 列表参数在真实数据上的过滤结果（识别码 / 后代分类 / 状态 / 排除状态） | `backend/handlers/instrument_stock_filters_2577_test.go` | `GET /api/instruments` | fixture：A(`CVZ-104`,available,C) / B(`CVZ-105`,available,C) / E(`CVZ-106`,available,其他) / D(`CVZ-107`,lost,其他)；P 为 C 的父分类 | backend（Postgres 测试库 127.0.0.1:15433） | `SN_HITS=1`、`CATEGORY_HITS=2`（父分类含后代）、`STATUS_HITS=3`、`EXCLUDE_HITS=3`（D 被排除）、`TOTAL_MATCH=1`（每次 `total == len(list)`） | R2 |
| 页面控件接线（搜索 / 分类 / 状态 / 清空 / 回第 1 页 / 清空行选择 / 表格 loading） | `frontend-pc/src/pages/InstrumentStock.jsx` | `/site/stock` | 修后源码（结构性断言，evidence_type: static） | PC | `SN_SEARCH_INPUT=1`、`CATEGORY_SELECT=1`、`STATUS_SELECT=1`、`CLEAR_BUTTON=1`、`TABLE_LOADING=1`、`EARLY_SPIN_RETURN=0`、`FILTER_RESET_WIRED=1`、`SELECTION_CLEAR_WIRED=1`、`STOCK_COL_COUNT=8`（#2537 列集合无回归） | R3 |
| 文档同步（ui.md 筛选清单与词表；api.md 参数未漂移） | `docs/spec/ui/ui.md` | grep 结构断言 | 修后文档（evidence_type: static） | 文档 | `UI_STOCK_FILTERS=1`、`UI_STATUS_VOCAB=1`、`UI_STOCK_URL_PARAMS=1`、`API_FILTER_PARAMS_DOC=1` | R4 |
| 构建 / lint / 回归 | `frontend-pc` + `backend` | `eslint` + `vite build` + `node --test` + `go build` | 修后工作树 | 双端 | `ESLINT_OK=1`、`PC_BUILD_OK=1`、`PC_TESTS_OK=1`、`GO_BUILD_OK=1` | R5 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | unit | `node --test` 纯函数单测（无 DOM 依赖，确定性） |
| R2 | integration | Go 定向测试 + Postgres 测试库真实数据（`testfixtures.SetupTestDB`） |
| R3 | static | PC 无组件测试运行器（`frontend-pc/package.json` 仅 dev/build/lint/preview），按 **Owner 豁免**（沿用 #2528 / #2526 D6）以源码结构断言 + 纯函数单测 + 构建作为 PC 证据；真实渲染截图归部署侧复核 |
| R4 | static | 文档 grep 结构断言（Owner 豁免同上） |
| R5 | build | eslint / vite build / node --test / go build |

### Input Truth & Tool Preflight

- **Source of truth**：
  - 列表过滤参数与语义 = `backend/handlers/api.go:395-419`（`sn` ILIKE、`category_id` 经 `getDescendantCategoryIDs` 展开、`stock_status` 等值、`exclude_status` 为 `NOT IN`(csv)、`page`/`pageSize` 见 `clampPageSize`）。
  - 分类/状态选项来源 = `GET /api/instruments/filter-options`（`backend/handlers/api.go:641-694`，含 `categories`/`levels`/`statuses`/`sites`，与 `applyInstrumentListScope` 同可见性）。
  - 页面既有可见性口径 = `isManagerLike`（`hasCusPerm('instrument:delete')`）→ 非管理级 `exclude_status=lost,archived,sold`（`frontend-pc/src/pages/InstrumentStock.jsx:48-60`，#2140）。
  - 列集合与词表基线 = `docs/spec/ui/ui.md` §2.20.1（#2537 D1/D2）+ 页面提示行「待租 / 在租 / 维修中 / 已丢失」。
  - URL 深链契约 = `frontend-pc/src/pages/Dashboard.jsx:157/196/234`（只发 `?status=<code>`）与 `docs/spec/ui/ui.md:1570`（「透传 `?status=`」）。
- **实施前差异核对（先跑再改）**：
  - `grep -n "statusParam\|stock_status\|exclude_status" frontend-pc/src/pages/InstrumentStock.jsx` → 修后应收敛到 `readStockFilters` / `buildStockListParams` 两处；
  - `grep -n "if (loading) return" frontend-pc/src/pages/InstrumentStock.jsx` → 修后 0 命中；
  - `grep -n "statusColors" frontend-pc/src/pages/InstrumentStock.jsx` → 修后 0 命中；
  - `grep -n "sn ILIKE\|category_id IN\|exclude_status" backend/handlers/api.go` → 参数名必须与 `buildStockListParams` 完全一致（`sn`/`category_id`/`stock_status`/`exclude_status`）。
- **损坏输入拒绝**：`?status=<未知值>` → 下拉保留原值展示、请求 `stock_status=<原值>`（0 行，不清空其它筛选，不抛错）；`?category_id=<非法 uuid>` 由后端既有行为决定（两页同源，本单不新增静默吞错）；`filter-options` 请求失败 → 下拉为空但列表仍可用（不阻塞主流程）。
- **工具盲区**：静态 grep 只能证明「控件/接线存在」，不能证明运行时渲染与交互（PC 无组件测试运行器）→ 真实渲染/点击归部署侧真机复核；Go 定向测试依赖 Postgres 测试库（`docs/ops/test-db-lifecycle.md`），不可用时视为基础设施阻塞而非 PASS。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 筛选状态唯一来源 = URL query；写回参数名用 `status`（`stock_status` 仅兼容读） | 本计划锁定（保住 Dashboard 深链与导出接口的 `status` 契约） |
| D2 | 分类筛选 = `filter-options` 扁平列表（与乐器列表同源）+ 服务端后代展开；不做层级树控件 | 本计划锁定 |
| D3 | 识别码触发方式 = `Input.Search` 回车 / 点搜索 / 清空（不做输入即搜防抖），与乐器列表一致 | 本计划锁定 |
| D4 | 本页状态词表：`available=待租`、`rented=在租`、`maintenance=维修中`、`reserved=已预订`、`shipping=配送中`、`lost=已丢失`、`archived=已下架`、`sold=已售出`，未知值原样 | 本计划锁定；**不**改乐器列表既有文案（未请求的可见变更，另议） |
| D5 | 显式状态优先：所选状态 ∈ `{lost,archived,sold}`（仅管理级可达）时不再追加 `exclude_status` | 本计划锁定 |
| D6 | `page`/`pageSize` 仍为本地 state（不入 URL）；筛选变化回第 1 页 | 本计划锁定（本页无面包屑回跳需求） |
| D7 | 首屏 loading 改为 `Table loading`，移除整页 early return | 本计划锁定（否则每次筛选卸载工具栏，输入框失焦） |
| D8 | 导出 CSV 保持「仅按状态」，口径对齐立 #2578 | 见 `## Out of Scope` |
| D9 | PC 证据 = 纯函数 `node --test` + 源码结构断言 + 构建；真实渲染截图归部署侧。**Owner 豁免**（沿用 #2528 / #2526 D6） | 本计划锁定 |
| D10 | 重命令（vite build / go test / eslint）一律经 `heavy-run.sh` 取锁执行 | 本计划锁定 |

### Out of Scope

- **后端生产代码改动**：`GetInstruments` 已支持全部所需参数，本单不改（仅新增只读回归测试）。
- **乐器列表页筛选与状态文案统一**（已具备；词表跨页统一未请求，另议）。
- **导出 CSV 口径对齐**：属 #2578（`导出` 与列表两套语义：导出 `category` 按名称、`search_text` 按 name/brand、无后代展开与网点范围），本单**不包含**该实现。
- 移动端（H5 / 小程序）任何改动。
- 筛选状态持久化到用户偏好（Issue 已列）。
- URL 承载 `page`/`pageSize` 与浏览器前进/后退恢复页码（D6）。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 纯函数输入：空筛选 / `sn=CVZ-104` / `category_id=c1` / `status=available` / 非管理级 `status=lost` / `{page:3, selectedRowKeys:['a','b']}` + patch | `node --test frontend-pc/tests/instrumentStockFilters.test.mjs`（经 `scripts/verify-2577.sh r1`） | `docs/evidence/2577/R1-filter-unit.txt` | `TESTS_FAIL=0`、`TESTS_PASS>=12`、`PARAMS_SN=CVZ-104`、`PARAMS_EXCLUDE=`、`FILTER_PATCH_PAGE=1`、`FILTER_PATCH_SELECTED=0`、`URL_STATUS=status=available`、`LABEL_AVAILABLE=待租`、`OPTIONS_NONMGR=5` | 显式 lost 时仍叠加 `exclude_status` → FAIL；改筛选后页码/选择未清空 → FAIL |
| R2 | P(父) → C(子)；A(`CVZ-104`,available,C)、B(`CVZ-105`,available,C)、E(`CVZ-106`,available,其他分类)、D(`CVZ-107`,lost,其他分类) | `GET /api/instruments`（`TestInstrumentStockFilters2577`） | `docs/evidence/2577/R2-api-params.txt` | `SN_HITS=1`、`CATEGORY_HITS=2`、`STATUS_HITS=3`、`EXCLUDE_HITS=3`、`TOTAL_MATCH=1` | `?category_id=P` 未展开后代（=1）或 `exclude_status` 未排除 D（=4）→ FAIL |
| R3 | 修后 `InstrumentStock.jsx` 源码 | `bash scripts/verify-2577.sh r3` | `docs/evidence/2577/R3-page-wiring.txt` | `SN_SEARCH_INPUT=1`、`CATEGORY_SELECT=1`、`STATUS_SELECT=1`、`CLEAR_BUTTON=1`、`TABLE_LOADING=1`、`EARLY_SPIN_RETURN=0`、`FILTER_RESET_WIRED=1`、`SELECTION_CLEAR_WIRED=1`、`STOCK_COL_COUNT=8` | 工具栏缺失、整页 Spin 残留、筛选未清空选择、列集合回退（≠8）→ 任一即 FAIL |
| R4 | 修后 `docs/spec/ui/ui.md` + `docs/spec/api/api.md` | `bash scripts/verify-2577.sh r4` | `docs/evidence/2577/R4-docs.txt` | `UI_STOCK_FILTERS=1`、`UI_STATUS_VOCAB=1`、`UI_STOCK_URL_PARAMS=1`、`API_FILTER_PARAMS_DOC=1` | ui.md 未登记筛选/词表，或 api.md 参数被删 → FAIL |
| R5 | 修后工作树 | `bash scripts/verify-2577.sh r5` | `docs/evidence/2577/R5-build.txt` | `ESLINT_OK=1`、`PC_BUILD_OK=1`、`PC_TESTS_OK=1`、`GO_BUILD_OK=1` | 任一 eslint/build/test 失败 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2577.sh r1` | `docs/evidence/2577/R1-filter-unit.txt` | exit 0；含 `TESTS_FAIL=0`、`FILTER_PATCH_PAGE=1`、`FILTER_PATCH_SELECTED=0`、`PARAMS_EXCLUDE=`、`LABEL_AVAILABLE=待租` | PASS | 断言不符或测试失败 → FAIL | dev |
| R2 | `bash scripts/verify-2577.sh r2` | `docs/evidence/2577/R2-api-params.txt` | exit 0；含 `SN_HITS=1`、`CATEGORY_HITS=2`、`EXCLUDE_HITS=3`、`TOTAL_MATCH=1` | PASS | 后代未展开或排除失效 → FAIL | dev |
| R3 | `bash scripts/verify-2577.sh r3` | `docs/evidence/2577/R3-page-wiring.txt` | exit 0；含 `SN_SEARCH_INPUT=1`、`EARLY_SPIN_RETURN=0`、`SELECTION_CLEAR_WIRED=1`、`STOCK_COL_COUNT=8` | PASS | 控件缺失或整页 Spin 残留 → FAIL | dev |
| R4 | `bash scripts/verify-2577.sh r4` | `docs/evidence/2577/R4-docs.txt` | exit 0；含 `UI_STOCK_FILTERS=1`、`UI_STATUS_VOCAB=1`、`API_FILTER_PARAMS_DOC=1` | PASS | 文档未同步 → FAIL | dev |
| R5 | `bash scripts/verify-2577.sh r5` | `docs/evidence/2577/R5-build.txt` | exit 0；含 `ESLINT_OK=1`、`PC_BUILD_OK=1`、`GO_BUILD_OK=1` | PASS | 任一 lint/build 失败 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2577.sh`（`r1`–`r5`）；每个文本 artifact 首行：`# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`，并另起一行 `GENERATED_AT_HEAD=<40 位 sha>`；证据必须在 fix commit **之后**生成。
- 逐条清单（全部位于 `docs/evidence/2577/`）：
  - `docs/evidence/2577/R1-filter-unit.txt`
  - `docs/evidence/2577/R2-api-params.txt`
  - `docs/evidence/2577/R3-page-wiring.txt`
  - `docs/evidence/2577/R4-docs.txt`
  - `docs/evidence/2577/R5-build.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2577 --summary-file tmp/pre-ready-2577-summary.md --changed-files tmp/pre-ready-2577-changed.txt --evidence-file docs/evidence/2577/evidence.json --report docs/evidence/2577/pre-ready-2577.json`；`evidence.json` 的 `build` 字段须匹配 evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Dependencies

无

### Workload

- Size: S
- Basis: 1 个新纯函数模块 + 1 个页面改造 + 1 个前端单测文件 + 1 个只读 Go 回归测试 + 1 份文档 + 1 个 5 行证据 harness（无 migration、无外部依赖）
- Confidence: high

*Model: deepseek/deepseek-flash*

