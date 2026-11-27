## Accepted Plan — #2538 详情页统一 + 页头摘要（标准日租金 / 逾期租金 / 全额押金 / 差额押金 / 估值 / 加入日期）

> 需求来源：Owner 2026-11-26。基线：`/instruments/detail/:id`（`frontend-pc/src/pages/admin/instrument/Detail.jsx`，已含价格策略页与日志 tab）。本单**终结** `/site/stock/:id` 存根（#2529 的临时实现）。

### 变更点

1. **入口统一（路由）**
   - `frontend-pc/src/App.jsx`：`/site/stock/:id` 改为重定向组件 → `<Navigate to={\`/instruments/detail/${id}\`} replace />`（旧链接、书签、库存监控行点击都仍然可用）。
   - 删除存根与其专用实现：`frontend-pc/src/pages/AssetDetail.jsx`、`frontend-pc/src/utils/assetDetail.js`、`frontend-pc/tests/assetDetail.test.mjs`（`assetDetailState`/`assetHeroImage` 一并移除；`instrumentDisplay.js` 的取图 helper 复用为页头图取源）。

2. **页头摘要区（`frontend-pc/src/pages/admin/instrument/Detail.jsx`）**
   - 页头左侧加 160×160 主图：取源顺序 `media.display[0].url → poster → images[0] → 内置占位`（复用 `instrumentImageSrc`，禁止第二份实现）；`onError` 回落占位。
   - 页头右侧加摘要网格（2 行 × 3 列），标签与口径：
     | 摘要项 | 数据源 | 空值 |
     |---|---|---|
     | 标准日租金 | `pricingV2.base_daily_rate`（分）→ `¥X.XX/天` | `-` |
     | 逾期租金 | `pricingV2.overdue_daily_fee`（分）→ `¥X.XX/天` | `-` |
     | 全额押金 | `pricingV2.deposit`（分，显式 0 有效） | `-` |
     | 差额押金 | `pricingV2.deposit_reduced`（分） | `未配置` |
     | 估值 | `instrument.total_price`（分） | `-` |
     | 加入日期 | `instrument.created_at`（`formatBeijingDateTimeShort`） | `-` |
   - 摘要口径抽成纯函数 `buildAssetSummary(instrument, pricingV2)`（放 `frontend-pc/src/utils/assetSummary.js`，返回 `[{key,label,value}]`），页头只做渲染，便于 `node --test` 覆盖。
   - 价格策略页内标签对齐：`押金` → **全额押金**、`逾期日费` → **逾期租金**、`标准日租` → **标准日租金**（值与数据源不变）。
   - 基本信息页移除重复的「创建时间」行（值已在页头摘要「加入日期」；「更新时间」保留）。

3. **日志取代流转轨迹**
   - 现有「日志」tab（`ActivityLogTab`：`GET /instruments/:id/activity-log` 订单会话事件 + `/admin/audit-logs` 操作记录）即为 Owner 要求的「日志」，**不新增接口、不新增时间线实现**。
   - 全站不再出现「流转轨迹」「关联单据」文案（存根删除后自然消失，需静态断言守住）。

4. **文档**
   - `docs/spec/ui/ui.md`：登记「乐器详情页（唯一）」信息架构：页头摘要 6 项 + tab 列表（基本信息 / 价格策略 / 多媒体 / 动态属性 / 当前租赁 / 日志 / 促销覆盖），并写明 `/site/stock/:id` 为兼容重定向。

### Files

- `frontend-pc/src/App.jsx`
- `frontend-pc/src/pages/admin/instrument/Detail.jsx`
- `frontend-pc/src/utils/assetSummary.js`（新增）
- `frontend-pc/tests/assetSummary.test.mjs`（新增）
- `frontend-pc/src/pages/AssetDetail.jsx`（删除）
- `frontend-pc/src/utils/assetDetail.js`（删除）
- `frontend-pc/tests/assetDetail.test.mjs`（删除）
- `docs/spec/ui/ui.md`
- `scripts/verify-2538.sh`（新增）
- `docs/evidence/2538/plan.md`
- `docs/evidence/2538/evidence.json`
- `docs/evidence/2538/pre-ready-2538.json`
- `docs/evidence/2538/R1-route-redirect.txt`
- `docs/evidence/2538/R2-summary-helpers.txt`
- `docs/evidence/2538/R3-stub-removed.txt`
- `docs/evidence/2538/R4-log-tab.txt`
- `docs/evidence/2538/R5-docs.txt`
- `docs/evidence/2538/R6-build.txt`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 旧入口重定向 | `frontend-pc/src/App.jsx` | `/site/stock/:id` | 任意乐器 id | PC | 路由声明含 `Navigate` 且目标为 `/instruments/detail/`；`/site/stock`（列表）不变 | R1 |
| 页头摘要字段 | `frontend-pc/src/utils/assetSummary.js` | `node --test` | pricingV2 缺字段 / 显式 0 / 正常 | PC | 6 项标签齐全；`差额押金` 空 → `未配置`；金额按分换算；`估值` 取 `total_price`；`加入日期` 取 `created_at` | R2 |
| 存根与占位文案清除 | 全仓 `frontend-pc/src` | grep | 修后源码 | PC | `AssetDetail.jsx`/`assetDetail.js` 不存在；`流转轨迹`/`关联单据` 命中 0；`assetDetail` import 命中 0 | R3 |
| 日志 tab 存在 | `admin/instrument/Detail.jsx` | 静态结构断言 | 修后源码 | PC | `label: '日志'` 与 `ActivityLogTab` 均在；`getActivityLog` 调用存在 | R4 |
| 文档同步 | `docs/spec/ui/ui.md` | grep | — | 文档 | 含「页头摘要」「加入日期」「/site/stock/:id 重定向」表述 | R5 |
| 构建/测试 | frontend-pc + backend | `node --test` + `eslint` + `npm run build` + `go build` | 修后工作树 | 双端 | 全绿 | R6 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 / R3 / R4 | static | PC 无组件测试运行器（`package.json` 仅 dev/build/lint/preview）；Owner 已就 PC 证据形态豁免（沿用 #2528 / #2526 / #2537 D6/D7）。真实渲染截图归部署侧复核（#2533 复验对象改为统一后的详情页） |
| R2 | unit | 纯函数 `node --test`（无 DOM） |

### Input Truth & Tool Preflight

- **Source of truth**：
  - 金额口径：`services.FormatPricingResult` 输出**分**（`backend/services/pricing.go:303-326`，含 `overdue_daily_fee`、`deposit`）；前端一律经 `formatCents`（`frontend-pc/src/utils/money.js:14-16`）。
  - 差额押金语义：`instruments.deposit_reduced`（`docs/spec/database/database.md:195`、`docs/spec/api/api.md:1390`），空值口径沿用详情页既有 `未配置`。
  - 估值 = `instruments.total_price`（分），`GET /instruments/:id` 已返回（`backend/handlers/api.go:124`）。
  - 日志数据源 = `GET /instruments/:id/activity-log`（`backend/handlers/activity_log.go:50-180`）+ `/admin/audit-logs`。
- **差异核对（实施前先跑）**：
  - `grep -rn "AssetDetail\|assetDetail" frontend-pc/src` → 修后应为 0（含 App.jsx import）；
  - `grep -rn "流转轨迹\|关联单据" frontend-pc/src` → 修后应为 0；
  - `grep -n "total_price" frontend-pc/src/pages/admin/instrument/Detail.jsx` → 修后应命中 1 处（摘要取值）。
- **损坏输入拒绝**：`pricingV2` 请求失败 → 摘要价格项显 `-`（页面不得白屏）；`total_price` 为 `null` → `-`；`created_at` 缺失 → `-`。
- **工具盲区**：静态断言不能证明真实渲染与重定向生效；由部署侧在预生产点击 `/site/stock/<id>` 验证（#2533）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | **只保留一个详情页**：`/site/stock/:id` 重定向到 `/instruments/detail/:id`；不再维护两份详情实现 | 本计划锁定（Owner 2026-11-26「信息不足」根因即双实现） |
| D2 | 页头摘要 6 项：标准日租金 / 逾期租金 / 全额押金 / 差额押金 / 估值 / 加入日期 | 本计划锁定（Owner 明确列出的 5 项 + 估值） |
| D3 | 「关联单据」整体移除（功能不存在，不保留占位） | Owner 2026-11-26 明确 |
| D4 | 「流转轨迹」由现有「日志」tab 取代（时间线 + 真实事件），不新增后端接口 | 本计划锁定 |
| D5 | 删除 `AssetDetail.jsx` / `assetDetail.js` / `assetDetail.test.mjs`（#2529 存根专用实现被取代） | 本计划锁定；#2529 历史证据不删 |
| D6 | 前端证据 = 纯函数 `node --test` + 静态结构断言（Owner 豁免沿用 #2528/#2526） | 本计划锁定 |

### Out of Scope

- 新增「关联单据」真实功能（租约/维保/财务数据源）→ 需要独立设计，另立 Issue。
- 日志接口的过滤/分页/导出增强（沿用现值 50 条上限）。
- 编辑能力与权限模型调整（页内既有编辑入口行为不变）。
- 列表页列口径（#2537）与移动端详情页。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 修后 `App.jsx` 路由表 | `bash scripts/verify-2538.sh r1` | `docs/evidence/2538/R1-route-redirect.txt` | `NAVIGATE_REDIRECT=1`、`REDIRECT_TARGET=/instruments/detail/`、`STOCK_LIST_ROUTE_UNCHANGED=1` | 仍渲染存根组件，或无重定向 → FAIL |
| R2 | `buildAssetSummary`：完整 / 缺 pricingV2 / 显式 0 押金 / 缺 created_at | `bash scripts/verify-2538.sh r2` | `docs/evidence/2538/R2-summary-helpers.txt` | `TESTS_PASS>=8`、`LABELS=6`、`REDUCED_EMPTY=未配置`、`TOTAL_PRICE_CENTS=1`、`JOIN_DATE_FROM_CREATED_AT=1` | 标签缺失/金额未换算/差额显示 `-` → FAIL |
| R3 | 修后 `frontend-pc/src` | `bash scripts/verify-2538.sh r3` | `docs/evidence/2538/R3-stub-removed.txt` | `STUB_FILES=0`、`ASSETDETAIL_IMPORTS=0`、`TIMELINE_TAB_LABEL=0`、`DOCUMENTS_TAB_LABEL=0` | 任一残留 → FAIL |
| R4 | 修后详情页源码 | `bash scripts/verify-2538.sh r4` | `docs/evidence/2538/R4-log-tab.txt` | `LOG_TAB=1`、`ACTIVITY_LOG_CALL=1`、`HERO_IMAGE=1` | 日志 tab 缺失或主图未接 → FAIL |
| R5 | `docs/spec/ui/ui.md` | `bash scripts/verify-2538.sh r5` | `docs/evidence/2538/R5-docs.txt` | `DOC_SUMMARY=1`、`DOC_JOIN_DATE=1`、`DOC_REDIRECT=1` | 缺项 → FAIL |
| R6 | 修后工作树 | `bash scripts/verify-2538.sh r6` | `docs/evidence/2538/R6-build.txt` | `NODE_TEST_OK=1`、`ESLINT_OK=1`、`PC_BUILD_OK=1`、`GO_BUILD_OK=1` | 任一失败 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2538.sh r1` | `docs/evidence/2538/R1-route-redirect.txt` | exit 0；含 `NAVIGATE_REDIRECT=1`、`REDIRECT_TARGET=/instruments/detail/` | PASS | 无重定向 → FAIL | dev |
| R2 | `bash scripts/verify-2538.sh r2` | `docs/evidence/2538/R2-summary-helpers.txt` | exit 0；含 `LABELS=6`、`REDUCED_EMPTY=未配置` | PASS | 标签缺失 → FAIL | dev |
| R3 | `bash scripts/verify-2538.sh r3` | `docs/evidence/2538/R3-stub-removed.txt` | exit 0；含 `STUB_FILES=0`、`TIMELINE_TAB_LABEL=0` | PASS | 残留存根/占位 → FAIL | dev |
| R4 | `bash scripts/verify-2538.sh r4` | `docs/evidence/2538/R4-log-tab.txt` | exit 0；含 `LOG_TAB=1`、`HERO_IMAGE=1` | PASS | 日志缺失 → FAIL | dev |
| R5 | `bash scripts/verify-2538.sh r5` | `docs/evidence/2538/R5-docs.txt` | exit 0；含 `DOC_REDIRECT=1` | PASS | 缺文档 → FAIL | dev |
| R6 | `bash scripts/verify-2538.sh r6` | `docs/evidence/2538/R6-build.txt` | exit 0；含 `PC_BUILD_OK=1`、`GO_BUILD_OK=1` | PASS | 任一失败 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2538.sh`（`r1`–`r6`），每个文本 artifact 首行：`# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`；另起一行 `GENERATED_AT_HEAD=<40 位 sha>`。
- 逐条清单（全部位于 `docs/evidence/2538/`）：
  - `docs/evidence/2538/R1-route-redirect.txt`
  - `docs/evidence/2538/R2-summary-helpers.txt`
  - `docs/evidence/2538/R3-stub-removed.txt`
  - `docs/evidence/2538/R4-log-tab.txt`
  - `docs/evidence/2538/R5-docs.txt`
  - `docs/evidence/2538/R6-build.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2538 --summary-file … --changed-files … --evidence-file docs/evidence/2538/evidence.json --report docs/evidence/2538/pre-ready-2538.json`；`evidence.json` 的 build 字段须匹配 evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Dependencies

- #2529
- #2537

### Workload

- Size: M（路由重定向 + 详情页页头摘要 + 1 个纯函数与测试 + 删除 3 个存根文件 + 1 份文档 + 1 个 harness）
- Confidence: high

