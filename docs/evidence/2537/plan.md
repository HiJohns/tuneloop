## Accepted Plan — #2537 两页列口径区分：列表去价、库存监控去级别/状态、押金拆全额与差额、新增逾期租金、SN 可点进详情

> 需求来源：Owner 2026-11-26。上承 #2528（两页展示 helper 与字段来源），下接 #2538（详情页统一）。

### 变更点

1. **后端：列表补 `overdue_daily_fee`（分）**
   - `backend/handlers/pricing.go` 的 `instrumentListPriceFields` 增加 `OverdueDailyFee *int64`：从 `pricingResponseWithInstrument` 产出的 `data["overdue_daily_fee"]` 取值（`services.FormatPricingResult` 已输出分），与 `pricing-v2` **同一条计算链**（overrides → pricing JSONB 回落）。
   - `backend/handlers/api.go` 的 `GetInstruments` 增加 `instrumentMap["overdue_daily_fee"] = ...`（无法解析 → `null`，不伪造 0）。

2. **前端 helper（`frontend-pc/src/utils/instrumentDisplay.js`）拆分押金**
   - `formatDepositFull(record)`：`deposit` 分 → `¥X.XX`；`null/缺失` → `-`；显式 `0` → `¥0.00`。
   - `formatDepositReduced(record)`：`deposit_reduced` 分 → `¥X.XX`；`null/缺失` → `未配置`；显式 `0` → `¥0.00`（与详情页「差额押金」同文案）。
   - `formatOverdueDailyFee(record)`：`overdue_daily_fee` 分 → `¥X.XX`；`null/≤0` → `-`。
   - 删除 `formatDeposit`（合并式展示不再需要），同步 `frontend-pc/tests/instrumentDisplay.test.mjs`。

3. **库存监控（`frontend-pc/src/pages/InstrumentStock.jsx`）**
   - 删除「级别」「状态」两列（Owner 2026-11-26 明确两列都去）。
   - 「押金」单列 → 「全额押金」+「差额押金」两列，并在「标准日租金」后新增「逾期租金」列（三列均受 `isManagerLike` 价格可见性门控，与现有日租金/押金同策略）。
   - 删除操作列的「详情」按钮；「识别码」改为可点击链接（`<Button type="link">` 或等价可点元素），`stopPropagation` 后 `navigate('/site/stock/' + record.id)`（行点击行为不变）。

4. **乐器列表（`frontend-pc/src/pages/admin/instrument/List.jsx`）**
   - 删除「标准日租金」「押金」两列。
   - 删除操作列的「查看」按钮；「识别码」改为可点击链接 → `navigate('/instruments/detail/' + record.id)`。上移/下移/删除按钮保持不变。

5. **文档**
   - `docs/spec/api/api.md` §5.4.0（列表价格字段）：补 `overdue_daily_fee`（分，`null` = 不可解析）。
   - `docs/spec/ui/ui.md`：登记两页列清单（乐器列表 = 图片/识别码/分类/乐器分级/加入时间/网点/状态/操作；库存监控 = 图片/识别码/分类/所属网点/标准日租金/逾期租金/全额押金/差额押金/操作），并写「识别码即详情入口，不设独立查看按钮」。

### Files

- `backend/handlers/api.go`
- `backend/handlers/pricing.go`
- `backend/handlers/instrument_list_price_2537_test.go`（新增）
- `frontend-pc/src/utils/instrumentDisplay.js`
- `frontend-pc/tests/instrumentDisplay.test.mjs`
- `frontend-pc/src/pages/InstrumentStock.jsx`
- `frontend-pc/src/pages/admin/instrument/List.jsx`
- `docs/spec/api/api.md`
- `docs/spec/ui/ui.md`
- `scripts/verify-2537.sh`（新增）
- `docs/evidence/2537/plan.md`
- `docs/evidence/2537/evidence.json`
- `docs/evidence/2537/pre-ready-2537.json`
- `docs/evidence/2537/R1-list-field.txt`
- `docs/evidence/2537/R2-display-helpers.txt`
- `docs/evidence/2537/R3-columns-static.txt`
- `docs/evidence/2537/R4-sn-link-static.txt`
- `docs/evidence/2537/R5-docs.txt`
- `docs/evidence/2537/R6-build.txt`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 列表返回逾期租金 | `backend/handlers/api.go`、`pricing.go` | `GET /api/instruments` | 乐器带 `pricing.overdue_daily_fee=36`、另一台带 `pricing_overrides.overdue_daily_fee=15` | backend（测试库） | `LIST_OVERDUE=3600`（分）与 `PRICING_V2_OVERDUE=3600` 相等；override 行为 `1500`；无配置 → `null` | R1 |
| 押金拆分 helper | `frontend-pc/src/utils/instrumentDisplay.js` | `node --test` | `deposit/deposit_reduced/overdue` 的 `null`/`0`/正常值 | PC | 全额 `¥500.00`、差额 `未配置`/`¥0.00`/`¥200.00`、逾期 `-`/`¥36.00` | R2 |
| 两页列口径（静态） | `InstrumentStock.jsx`、`List.jsx` | grep/结构断言 | 修后源码 | PC | 乐器列表无 `resolved_daily_rate`/`deposit` 列定义；库存监控无 `level_name`/`status` 列定义；有「逾期租金」「全额押金」「差额押金」三列 | R3 |
| SN 即详情入口（静态） | 同上 | grep/结构断言 | 修后源码 | PC | 两页「识别码」含 `navigate`；`详情`/`查看` 按钮关键字命中数 = 0 | R4 |
| 文档同步 | `docs/spec/api/api.md`、`docs/spec/ui/ui.md` | grep | — | 文档 | 含 `overdue_daily_fee` 与两页列清单表述 | R5 |
| 构建/测试 | backend + frontend-pc | 定向测试 + `go build` + `eslint` + `npm run build` | 修后工作树 | 双端 | 全绿 | R6 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R3 / R4 | static | PC 无组件测试运行器（`frontend-pc/package.json` 只有 dev/build/lint/preview）；Owner 已就 PC 证据形态豁免（沿用 #2528 / #2526 D6）。真实渲染截图归部署侧复核 |

### Input Truth & Tool Preflight

- **Source of truth**：
  - 列表价格字段口径 = `pricingResponseWithInstrument`（`backend/handlers/pricing.go:268-284`）+ `services.FormatPricingResult`（`services/pricing.go:303-326`，金额均为**分**，含 `overdue_daily_fee`）。
  - 「差额押金」= `instruments.deposit_reduced`（`docs/spec/database/database.md:195`；`docs/spec/api/api.md:1390`）。
  - #2528 已定：`deposit` `null` → `-`、显式 `0` → `¥0.00`（`docs/evidence/2528/plan.md` D3）。
- **差异核对（实施前先跑）**：
  - `grep -n "formatDeposit(" frontend-pc/src frontend-pc/tests` → 修后应只剩 `formatDepositFull/formatDepositReduced`；
  - `grep -n "overdue_daily_fee" backend/handlers/api.go backend/handlers/pricing.go` → 列表与 pricing-v2 各出现一次取值；
  - `grep -n "level_name\|status" frontend-pc/src/pages/InstrumentStock.jsx` → 列定义中 0 命中（筛选/批量仍可用）。
- **损坏输入拒绝**：`pricing` JSONB 非法 → `CalculatePricing` 回落 0 → 列表 `overdue_daily_fee=0` → 前端显示 `-`（不得抛错、不得整页失败）。
- **工具盲区**：静态 grep 只能证明「列定义不存在/存在」，不能证明运行时渲染；真实渲染由部署侧截图复核（#2536 一并看）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 库存监控列 = 图片/识别码/分类/所属网点/标准日租金/逾期租金/全额押金/差额押金/操作（**无**级别、**无**状态） | 本计划锁定（Owner 2026-11-26 明确） |
| D2 | 乐器列表列 = 图片/识别码/分类/乐器分级/加入时间/网点/状态/操作（**无**日租金、**无**押金） | 本计划锁定 |
| D3 | 押金拆两列（覆盖 #2528 D3 的「合并在押金列」） | 本计划锁定 |
| D4 | 逾期租金列数据源 = 列表新字段 `overdue_daily_fee`（分），与 pricing-v2 同链；禁止前端解析 `pricing` JSONB | 本计划锁定 |
| D5 | 差额押金空值显示 `未配置`（与详情页一致），全额押金空值 `-` | 本计划锁定 |
| D6 | 「识别码」即详情入口：库存监控 → `/site/stock/:id`，乐器列表 → `/instruments/detail/:id`；两页不再有独立查看/详情按钮 | 本计划锁定；#2538 会把这一个入口统一到同一页面 |
| D7 | 前端证据 = 纯函数 `node --test` + 静态接线（Owner 豁免同 #2528/#2526 D6） | 本计划锁定 |

### Out of Scope

- 详情页字段补全与两页详情路由统一 → **#2538**。
- 列表筛选、排序、分页、导出 CSV 的列集合调整。
- 移动端（H5 / 小程序）任何展示改动。
- `pricing_overrides` 管理与定价语义（#2519 / #2515 范围）。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 乐器 A：`pricing.overdue_daily_fee=36`、无 overrides；乐器 B：`pricing_overrides.overdue_daily_fee=15`；乐器 C：无定价 | `GET /api/instruments` + `GET /api/instruments/:id/pricing-v2` | `docs/evidence/2537/R1-list-field.txt` | `LIST_OVERDUE_A=3600`、`PRICING_V2_OVERDUE_A=3600`、`LIST_OVERDUE_B=1500`、`LIST_OVERDUE_C=null` | 列表值 ≠ pricing-v2 值，或缺失 → FAIL |
| R2 | 四类 fixture（正常/0/null/缺失）× 三个 helper | `node --test frontend-pc/tests/instrumentDisplay.test.mjs` | `docs/evidence/2537/R2-display-helpers.txt` | `TESTS_PASS>=9`、`FULL_NULL=-`、`REDUCED_NULL=未配置`、`ZERO=¥0.00`、`OVERDUE_NULL=-` | 差额空值显示为 `-` 或全额 0 被吞 → FAIL |
| R3 | 修后两页源码 | `bash scripts/verify-2537.sh r3` | `docs/evidence/2537/R3-columns-static.txt` | `LIST_HAS_RATE_COL=0`、`LIST_HAS_DEPOSIT_COL=0`、`STOCK_HAS_LEVEL_COL=0`、`STOCK_HAS_STATUS_COL=0`、`STOCK_DEPOSIT_COLS=2`、`STOCK_OVERDUE_COL=1` | 任一残留或缺失 → FAIL |
| R4 | 修后两页源码 | `bash scripts/verify-2537.sh r4` | `docs/evidence/2537/R4-sn-link-static.txt` | `SN_LINK_COUNT=2`、`DETAIL_BUTTON_COUNT=0`、`VIEW_BUTTON_COUNT=0` | 仍有独立查看/详情按钮，或 SN 不可点 → FAIL |
| R5 | 两份文档 | `bash scripts/verify-2537.sh r5` | `docs/evidence/2537/R5-docs.txt` | `DOC_OVERDUE=1`、`UI_LIST_COLUMNS=1`、`UI_SN_ENTRY=1` | 缺项 → FAIL |
| R6 | 修后工作树 | `bash scripts/verify-2537.sh r6` | `docs/evidence/2537/R6-build.txt` | `GO_TEST_OK=1`、`GO_BUILD_OK=1`、`ESLINT_OK=1`、`PC_BUILD_OK=1` | 任一失败 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2537.sh r1` | `docs/evidence/2537/R1-list-field.txt` | exit 0；含 `LIST_OVERDUE_A=3600`、`PRICING_V2_OVERDUE_A=3600`、`LIST_OVERDUE_B=1500` | PASS | 两值不等 → FAIL | dev |
| R2 | `bash scripts/verify-2537.sh r2` | `docs/evidence/2537/R2-display-helpers.txt` | exit 0；含 `TESTS_PASS>=9`、`REDUCED_NULL=未配置` | PASS | 断言失败 → FAIL | dev |
| R3 | `bash scripts/verify-2537.sh r3` | `docs/evidence/2537/R3-columns-static.txt` | exit 0；含 `STOCK_DEPOSIT_COLS=2`、`STOCK_OVERDUE_COL=1`、`LIST_HAS_RATE_COL=0` | PASS | 列残留/缺失 → FAIL | dev |
| R4 | `bash scripts/verify-2537.sh r4` | `docs/evidence/2537/R4-sn-link-static.txt` | exit 0；含 `SN_LINK_COUNT=2`、`DETAIL_BUTTON_COUNT=0` | PASS | 按钮残留 → FAIL | dev |
| R5 | `bash scripts/verify-2537.sh r5` | `docs/evidence/2537/R5-docs.txt` | exit 0；含 `DOC_OVERDUE=1`、`UI_SN_ENTRY=1` | PASS | 缺项 → FAIL | dev |
| R6 | `bash scripts/verify-2537.sh r6` | `docs/evidence/2537/R6-build.txt` | exit 0；含 `GO_BUILD_OK=1`、`PC_BUILD_OK=1`、`ESLINT_OK=1` | PASS | 任一失败 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2537.sh`（`r1`–`r6`），每个文本 artifact 首行：`# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`；另起一行 `GENERATED_AT_HEAD=<40 位 sha>`。
- 逐条清单（全部位于 `docs/evidence/2537/`）：
  - `docs/evidence/2537/R1-list-field.txt`
  - `docs/evidence/2537/R2-display-helpers.txt`
  - `docs/evidence/2537/R3-columns-static.txt`
  - `docs/evidence/2537/R4-sn-link-static.txt`
  - `docs/evidence/2537/R5-docs.txt`
  - `docs/evidence/2537/R6-build.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2537 --summary-file … --changed-files … --evidence-file docs/evidence/2537/evidence.json --report docs/evidence/2537/pre-ready-2537.json`；`evidence.json` 的 build 字段须匹配 evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Dependencies

- #2528

### Workload

- Size: S（后端 1 处字段透出 + 前端 2 页列调整 + helper 拆分 + 2 份文档 + 1 个 harness）
- Confidence: high

