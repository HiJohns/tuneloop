## Accepted Plan — #2578 库存监控导出与列表筛选口径对齐（sn / 分类（含后代）/ 状态 / 可见性）

> 来源：由 #2577（库存监控补筛选）暴露的导出/列表口径分裂，立单跟踪。
> 关键事实：导出与列表**走两套语义** —— 列表 `GET /api/instruments`（`backend/handlers/api.go:395-419`：`sn` ILIKE / `category_id` 含后代 / `stock_status` / `exclude_status` + `applyInstrumentListScope`），导出 `GET /api/instruments/export`（`backend/service/instrument.go:347-372`：`category` **按分类名称**等值 / `search_text` 按 name+brand / 无后代展开 / 无网点范围）。仅靠前端补参无法对齐，需后端扩展导出过滤。

### 变更点

#### 1. 后端：导出接受与列表同一套过滤与可见性（唯一 owner 复用）

`backend/service/instrument.go`：

- `ExportOptions` 增字段：`SN string`、`CategoryIDs []string`（**已展开**的分类 ID 集合）、`ExcludeStatus []string`、`Scope func(*gorm.DB) *gorm.DB`（可见性作用域闭包，由 handler 注入，**不在 service 重写权限逻辑**）；既有 `Fields/Category/Status/SearchText` 保持不变（D2 兼容）。
- `ExportInstruments` 查询链顺序固定：`tenant_id = ?` → `Scope`（非 nil 时）→ `sn ILIKE %sn%` → `CategoryIDs` 非空时 `category_id IN ?` → `Status`/`StockStatus` 等值 → `ExcludeStatus` 非空时 `stock_status NOT IN ?` → 既有 `category`(名称)/`search_text`。
- `headerMap` 增 `"sn": "识别码"`；`getFieldValue` 增 `case "category_name": return instrument.CategoryName`（#2579 负责价格类字段，本单不接价格链）。

`backend/handlers/instrument_import.go` `ExportInstruments`：

- 解析新参数：`sn`、`category_id`（`getDescendantCategoryIDs(db, id)` 展开，空则回落等值）、`exclude_status`（csv）、并接受 `stock_status` 作为 `status` 的别名；
- 注入 `opts.Scope = func(q *gorm.DB) *gorm.DB { return applyInstrumentListScope(db, ctx, q) }`（与列表**同一函数**，D1）。

#### 2. 前端：库存监控导出透传当前筛选 + 修正客户端调用

- `frontend-pc/src/utils/instrumentStockFilters.js`（#2577 新建）扩展纯函数 `buildStockExportParams(filters, { isManagerLike })`：产出 `{ sn, category_id, status, exclude_status, fields: 'sn,category_name,status' }`，空值剔除；显式状态 ∈ `{lost,archived,sold}` 时不叠加 `exclude_status`（与 `buildStockListParams` 同规则，**复用**同一判定）。
- `frontend-pc/src/pages/InstrumentStock.jsx`：`handleExport()` 改为 `fetchBlob('/instruments/export', { params: buildStockExportParams(...) })`（`frontend-pc/src/services/api.js:227-243`：带 `Authorization: Bearer`、空参剔除、统一 `导出失败` 错误），下载名 `instruments_export_<date>.xlsx`，按钮文案「导出 Excel」（产物实为 xlsx，与乐器列表 `List.jsx:480-487` 一致）。

#### 3. 测试与证据

- `backend/handlers/instrument_export_scope_2578_test.go`（新增）：`testfixtures.SetupTestDB` + `testutil.TestActor` + `gin` 路由挂 `ExportInstruments`；用 `excelize.OpenReader` 解析响应体逐行断言（fixture 见 G0 R1）。
- `frontend-pc/tests/instrumentStockFilters.test.mjs`（#2577 新建）追加导出参数用例。
- `scripts/verify-2578.sh`（新增）：`r1`–`r5`，产物落 `docs/evidence/2578/`，首行 provenance；重命令经 `heavy-run.sh`（D7）。

#### 4. 文档

- `docs/spec/api/api.md`：导出接口补 `sn` / `category_id`（含后代）/ `stock_status`(=`status`)/`exclude_status` / `fields`（新增 `sn`→识别码、`category_name`）与可见性说明（同列表 `applyInstrumentListScope`）。
- `docs/spec/ui/ui.md` §2.20.1：库存监控导出 = 跟随当前筛选（识别码/分类/状态/可见性），产物 `.xlsx`，按钮「导出 Excel」。

### Files（变更文件清单）

- `backend/service/instrument.go`
- `backend/handlers/instrument_import.go`
- `backend/handlers/instrument_export_scope_2578_test.go`
- `frontend-pc/src/utils/instrumentStockFilters.js`
- `frontend-pc/tests/instrumentStockFilters.test.mjs`
- `frontend-pc/src/pages/InstrumentStock.jsx`
- `docs/spec/api/api.md`
- `docs/spec/ui/ui.md`
- `scripts/verify-2578.sh`
- `docs/evidence/2578/plan.md`
- `docs/evidence/2578/R1-export-parity.txt`
- `docs/evidence/2578/R2-export-params.txt`
- `docs/evidence/2578/R3-export-wiring.txt`
- `docs/evidence/2578/R4-docs.txt`
- `docs/evidence/2578/R5-build.txt`
- `docs/evidence/2578/evidence.json`
- `docs/evidence/2578/pre-ready-2578.json`

### Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | 导出接口（`docs/spec/api/api.md` §5.4.x / 导入导出章节）只列 `category`/`status`/`search_text`/`fields`，`fields` 取值未含 `sn`/`category_name` 的说明 | 补 `sn`（模糊）、`category_id`（含后代）、`stock_status` 别名、`exclude_status`（csv，`NOT IN`）、可见性同列表（`applyInstrumentListScope`）、`fields` 有效值含 `sn`（识别码）/`category_name`（分类名称） | 导出契约变更后不同步即与实现漂移，审计会以 api.md 为口径判 REJECT |
| `docs/spec/ui/ui.md` | §2.20.1 库存监控只写「工具栏含 导出/批量导入/批量设价/批量删除」 | 补「导出 Excel 跟随当前筛选（识别码/分类/状态/可见性），产物 `.xlsx`」 | 导出行为变更后原文案过期 |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 导出过滤与可见性 = 列表同一套（识别码 / 后代分类 / 排除状态 / 网点范围） | `backend/handlers/instrument_export_scope_2578_test.go` + `backend/service/instrument.go` | `GET /api/instruments/export` | fixture：租户 T；siteA/siteB；分类 P(父)→C(子)、O(其他)；A(`CVZ-104`,available,C,siteA) / B(`CVZ-105`,available,C,siteA) / E(`CVZ-106`,available,O,siteB) / D(`CVZ-107`,lost,O,siteB)；用户 u 为 siteA 的 site_admin | backend（Postgres 测试库 127.0.0.1:15433） | `EXPORT_SN_FILTER=1`（`?sn=CVZ-104`）、`EXPORT_CATEGORY_DESC=2`（`?category_id=P` → A+B）、`EXPORT_EXCLUDE=3`（租户级 `?exclude_status=lost,archived,sold` → A+B+E）、`EXPORT_SCOPE_SITE=2`（site_admin 只见 A+B）、`EXPORT_HEADER_SN=1`（首行含「识别码」）、`EXPORT_ROWS_EQ_LIST=1` | R1 |
| 导出参数纯函数（筛选透传 + 显式状态优先 + 字段清单） | `frontend-pc/src/utils/instrumentStockFilters.js` | `node --test` | 空筛选；`sn=CVZ-104`；`category_id=c1`；`status=available`；非管理级 `status=lost`；管理级 `status=lost` | PC（Node 纯函数） | `EXPORT_SN=CVZ-104`；`EXPORT_FIELDS=sn,category_name,status`；`EXPORT_EXCLUDE_NONMGR=lost,archived,sold`；`EXPORT_EXCLUDE_WITH_STATUS=`（显式 lost 时不叠加）；`TESTS_FAIL=0` | R2 |
| 页面导出接线（fetchBlob / 透传筛选 / xlsx 命名 / 无裸 fetch） | `frontend-pc/src/pages/InstrumentStock.jsx` | `/site/stock` | 修后源码（结构断言，evidence_type: static） | PC | `EXPORT_USES_FETCHBLOB=1`、`EXPORT_RAW_FETCH=0`、`EXPORT_XLSX_NAME=1`、`EXPORT_PASSES_FILTERS=1`、`EXPORT_BUTTON_LABEL=1` | R3 |
| 文档同步（导出参数字段 / 页面导出行为） | `docs/spec/api/api.md` + `docs/spec/ui/ui.md` | grep 结构断言 | 修后文档（evidence_type: static） | 文档 | `API_EXPORT_PARAMS=1`、`API_EXPORT_FIELDS_SN=1`、`UI_EXPORT_XLSX=1`、`UI_EXPORT_FOLLOW_FILTER=1` | R4 |
| 构建 / lint / 回归 | `frontend-pc` + `backend` | `eslint` + `vite build` + `node --test` + `go build` + 定向 `go test` | 修后工作树 | 双端 | `ESLINT_OK=1`、`PC_BUILD_OK=1`、`PC_TESTS_OK=1`、`GO_BUILD_OK=1`、`GO_EXPORT_TEST_OK=1` | R5 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | integration | Go 定向测试 + Postgres 测试库真实数据，逐单元格解析 xlsx |
| R2 | unit | `node --test` 纯函数单测（无 DOM） |
| R3 | static | PC 无组件测试运行器；按 **Owner 豁免**（沿用 #2577 D9 / #2528 / #2526 D6）以源码结构断言 + 纯函数单测 + 构建为 PC 证据，真实点击归部署侧复核 |
| R4 | static | 文档 grep（Owner 豁免同上） |
| R5 | build | eslint / vite build / node --test / go build / 定向 go test |

### Input Truth & Tool Preflight

- **Source of truth**：
  - 列表语义（导出的目标口径）= `backend/handlers/api.go:372-419`（分页/`sn`/`category_id` 后代/`stock_status`/`exclude_status`/`applyInstrumentListScope`）。
  - 可见性唯一实现 = `applyInstrumentListScope`（`backend/handlers/api.go:354-370`，site_admin/site_member → `resolveOperatorSiteMemberships`；其余 → `middleware.ApplyOrgScope`）。
  - 后代展开唯一实现 = `getDescendantCategoryIDs`（`backend/handlers/public.go:657-665`）。
  - 现有导出实现 = `backend/handlers/instrument_import.go:137-205` + `backend/service/instrument.go:347-372`（字段/表头 `:22-34`、`:440-456`）。
  - 客户端下载正确姿势 = `fetchBlob`（`frontend-pc/src/services/api.js:227-243`，带 Bearer + 空参剔除）；对照调用方 `frontend-pc/src/pages/admin/instrument/List.jsx:474-490`。
- **实施前差异核对（先跑再改）**：
  - `grep -n "ExportOptions{\|ExportInstruments(" backend -r` → 调用点收敛为 handler 1 处 + service 1 处（改动后签名/字段兼容）；
  - `grep -n "fetch(" frontend-pc/src/pages/InstrumentStock.jsx` → 修后仅剩 0 处裸 `fetch`（导出走 `fetchBlob`）；
  - `grep -n "\"sn\":\|category_name" backend/service/instrument.go` → 修后 `headerMap` 含 `sn`、`getFieldValue` 含 `category_name`；
  - 既有测试 `backend/handlers/instrument_import_test.go:139 TestExportInstruments`（sqlmock 期望 `SELECT * FROM "instruments"`）必须仍通过 —— 新增 `Where` 只影响 SQL 文本，仍需回归该用例。
- **损坏输入拒绝**：`?category_id=<非法 uuid>` → 由列表同款既有行为决定（两页同源，不新增静默吞错）；`?exclude_status=`（空）→ 不追加条件；0 行结果 → 保持既有 400 `40006`（不改为 200 空文件，D5）。
- **工具盲区**：xlsx 逐单元格断言依赖 `fields` 参数显式包含 `sn`/`category_name`；未指定 `fields` 时导出列仍是旧默认集（本单不改默认集，价格类在 #2579）。PC 无渲染测试运行器 → 真实下载/打开文件由部署侧复核。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 导出过滤/可见性复用列表实现：`applyInstrumentListScope` 以闭包注入 service，`category_id` 由 handler 用 `getDescendantCategoryIDs` 展开 | 本计划锁定（禁止在 service 复制权限或分类树逻辑） |
| D2 | 旧导出参数 `category`(名称)/`search_text`(name+brand)/`fields` 保留兼容，不删除 | 本计划锁定（避免打断既有调用方） |
| D3 | 库存监控导出字段固定 `sn,category_name,status`（识别码 = 行标识，验收按 `sn` 集合比对）；价格类字段另立 #2579 | 本计划锁定 |
| D4 | 客户端统一 `fetchBlob`；产物命名 `.xlsx`、按钮文案「导出 Excel」（与乐器列表一致，且与真实产物类型一致） | 本计划锁定 |
| D5 | 0 行导出保持既有 400 `40006`，不改为 200 空文件 | 本计划锁定 |
| D6 | `status` 与 `stock_status` 双读（列表页写 `stock_status`、Dashboard/存货页写 `status`） | 本计划锁定 |
| D7 | PC 证据 = 纯函数 `node --test` + 源码结构断言 + 构建；真实渲染/下载归部署侧。**Owner 豁免**（沿用 #2577 D9 / #2528 / #2526 D6） | 本计划锁定 |
| D8 | 重命令（vite build / go test / eslint）一律经 `heavy-run.sh` 取锁执行 | 本计划锁定 |

### Out of Scope

- **价格 / 品牌 / 库存数量列取值修复**（`getFieldValue` 现状空或 0，需接 pricing-v2 链）→ **#2579**，本单不包含其实现。
- 导出默认字段集变更（保持 `brand,level,daily_rate,monthly_rate,deposit,stock_status` 兼容；价格列修好后再评估）。
- 乐器列表页导出是否跟随其筛选（产品决策，既有行为不变）。
- 导出格式改为真 CSV（当前产物为 xlsx；本单只把命名/文案与真实类型对齐）。
- 移动端（H5 / 小程序）任何改动。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 租户 T；siteA/siteB；P(父)→C(子)、O；A(`CVZ-104`,available,C,siteA)、B(`CVZ-105`,available,C,siteA)、E(`CVZ-106`,available,O,siteB)、D(`CVZ-107`,lost,O,siteB)；u=siteA site_admin（`site_members` + `users.iam_sub`） | `GET /api/instruments/export?fields=sn,category_name,status…`（`TestExportScope2578`） | `docs/evidence/2578/R1-export-parity.txt` | `EXPORT_SN_FILTER=1`、`EXPORT_CATEGORY_DESC=2`、`EXPORT_EXCLUDE=3`、`EXPORT_SCOPE_SITE=2`、`EXPORT_HEADER_SN=1`、`EXPORT_ROWS_EQ_LIST=1` | 分类未展开后代（=1）、排除失效（=4）、site_admin 导出跨网点（含 siteB）、首行无「识别码」→ 任一即 FAIL |
| R2 | 纯函数输入：空筛选 / `sn` / `category_id` / `status=available` / 非管理级 `status=lost` / 管理级 `status=lost` | `node --test frontend-pc/tests/instrumentStockFilters.test.mjs`（经 `verify-2578.sh r2`） | `docs/evidence/2578/R2-export-params.txt` | `TESTS_FAIL=0`、`TESTS_PASS>=4`、`EXPORT_FIELDS=sn,category_name,status`、`EXPORT_EXCLUDE_WITH_STATUS=`、`EXPORT_EXCLUDE_NONMGR=lost,archived,sold` | 显式 lost 时叠加 `exclude_status`，或 `fields` 缺 `sn` → FAIL |
| R3 | 修后 `InstrumentStock.jsx` 源码 | `bash scripts/verify-2578.sh r3` | `docs/evidence/2578/R3-export-wiring.txt` | `EXPORT_USES_FETCHBLOB=1`、`EXPORT_RAW_FETCH=0`、`EXPORT_XLSX_NAME=1`、`EXPORT_PASSES_FILTERS=1`、`EXPORT_BUTTON_LABEL=1` | 仍用裸 `fetch`、文件名 `.csv`、未透传筛选 → 任一即 FAIL |
| R4 | 修后 `docs/spec/api/api.md` + `docs/spec/ui/ui.md` | `bash scripts/verify-2578.sh r4` | `docs/evidence/2578/R4-docs.txt` | `API_EXPORT_PARAMS=1`、`API_EXPORT_FIELDS_SN=1`、`UI_EXPORT_XLSX=1`、`UI_EXPORT_FOLLOW_FILTER=1` | 文档缺任一参数/行为 → FAIL |
| R5 | 修后工作树 | `bash scripts/verify-2578.sh r5` | `docs/evidence/2578/R5-build.txt` | `ESLINT_OK=1`、`PC_BUILD_OK=1`、`PC_TESTS_OK=1`、`GO_BUILD_OK=1`、`GO_EXPORT_TEST_OK=1` | 任一 lint/build/test 失败 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2578.sh r1` | `docs/evidence/2578/R1-export-parity.txt` | exit 0；含 `EXPORT_SN_FILTER=1`、`EXPORT_CATEGORY_DESC=2`、`EXPORT_SCOPE_SITE=2`、`EXPORT_ROWS_EQ_LIST=1` | PASS | 未展开后代/未限定网点/行数≠列表 → FAIL | dev |
| R2 | `bash scripts/verify-2578.sh r2` | `docs/evidence/2578/R2-export-params.txt` | exit 0；含 `TESTS_FAIL=0`、`EXPORT_FIELDS=sn,category_name,status`、`EXPORT_EXCLUDE_WITH_STATUS=` | PASS | 显式状态叠加排除或字段缺 sn → FAIL | dev |
| R3 | `bash scripts/verify-2578.sh r3` | `docs/evidence/2578/R3-export-wiring.txt` | exit 0；含 `EXPORT_USES_FETCHBLOB=1`、`EXPORT_RAW_FETCH=0`、`EXPORT_XLSX_NAME=1` | PASS | 裸 fetch / `.csv` 命名 / 未透传筛选 → FAIL | dev |
| R4 | `bash scripts/verify-2578.sh r4` | `docs/evidence/2578/R4-docs.txt` | exit 0；含 `API_EXPORT_PARAMS=1`、`API_EXPORT_FIELDS_SN=1`、`UI_EXPORT_FOLLOW_FILTER=1` | PASS | 文档未同步 → FAIL | dev |
| R5 | `bash scripts/verify-2578.sh r5` | `docs/evidence/2578/R5-build.txt` | exit 0；含 `ESLINT_OK=1`、`PC_BUILD_OK=1`、`GO_BUILD_OK=1`、`GO_EXPORT_TEST_OK=1` | PASS | 任一 lint/build/test 失败 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2578.sh`（`r1`–`r5`）；每个文本 artifact 首行：`# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`，另起一行 `GENERATED_AT_HEAD=<40 位 sha>`；证据须在 fix commit **之后**生成。
- 逐条清单（全部位于 `docs/evidence/2578/`）：
  - `docs/evidence/2578/R1-export-parity.txt`
  - `docs/evidence/2578/R2-export-params.txt`
  - `docs/evidence/2578/R3-export-wiring.txt`
  - `docs/evidence/2578/R4-docs.txt`
  - `docs/evidence/2578/R5-build.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2578 --summary-file tmp/pre-ready-2578-summary.md --changed-files tmp/pre-ready-2578-changed.txt --evidence-file docs/evidence/2578/evidence.json --report docs/evidence/2578/pre-ready-2578.json`；`evidence.json` 的 `build` 字段须匹配 evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Dependencies

- #2577

### Workload

- Size: M
- Basis: 后端 2 文件（service + handler）+ 1 个集成测试 + 前端 2 文件复用/扩展 + 2 份文档 + 1 个 5 行证据 harness；无 migration、无外部依赖
- Confidence: medium

*Model: deepseek/deepseek-flash*

