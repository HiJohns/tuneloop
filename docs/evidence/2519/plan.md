## Accepted Plan — #2519（Rev.5：批量设价改为「日租金 / 逾期日租金 / 押金」逐项开关 + 押金双写 + 显式设置优先 + `overrides` 入参废弃）｜Rev.6｜Rev.7

> 取代 Rev.3。Rev.3 的 D6（押金只写 `pricing.deposit`）经预生产实测证伪（见下），UI 的「价格类型 + 操作」选择器也被 Owner 判定为无意义，改为**三项各带开关**。

### 修订记录（Rev.4 → Rev.5：补齐 `overrides` 入参废弃的断言翻转登记）

来源：Work 侧执行前检查的 Investigation Packet（`pricing_overdue_2315_test.go:22` 仍锁定旧 `overrides` 语义）。

| # | 修订项 | Rev.4 | Rev.5 |
|---|---|---|---|
| 1 | `overrides` 入参 | 变更点只写三个显式字段，未说明旧 `overrides` 是「废弃」还是「兼容」 | **废弃**（不再消费）；只传 `overrides`、三字段全空 → 该条 `failed[] reason=no_fields`，定价不变 |
| 2 | 既有断言翻转登记 | 未登记 | 新增「语义变更（被翻转的既有断言）」段：`backend/handlers/pricing_overdue_2315_test.go:22` 旧期望 → 新期望 + 理由 |
| 3 | 证据行 | R1–R12 | 新增 **R13**：legacy `overrides` 拒绝路径（`UPDATED=0`、`FAILED_REASON=no_fields`、`OVERRIDES_STORED=0`、`PRICING_V2_OVERDUE=0`），并把 R6 的正向口径明确为「显式 `overdue_daily_fee`」 |
| 4 | `pricing_overrides` 机制归属 | 未说明 | 明确：本单只改批量设价入口；`pricing_overrides` 的维护入口仍是 `PUT /inventory/rent-setting/batch`（`inventory.go:492-514`），该路径与测试不动 |

不在本次修订范围：Rev.4 的 D1/D2/D6/D8 口径、三项开关 UI、`failed[]` 上报、批量状态端点。

### Owner 裁决（2026-11-25 / 2026-11-26）

| 决策 | 裁定 |
|---|---|
| 批量上架/下架 | **1A**：新增 `PUT /instruments/batch/status`（`instrument:update` + `ExplicitTenantScope`），并去掉单条 `PUT /instruments/:id/status` 的 `RequireDebugMode()` |
| 批量设价交互 | **三项并列 + 每项一个开关**：日租金 / 逾期日租金 / 押金；开关关闭的项**不提交、不改动**。废弃「价格类型 + 操作（设置为）+ 金额」三段式 |
| 押金落库口径 | **A：列 + JSONB 双写**（`instruments.deposit` + `pricing.deposit`），与单条编辑 `instrument.go:702-705` 同语义 |
| 失败上报 | 两个批量接口统一 `{updated, failed:[{id, reason}]}`，单条失败不得静默 `continue` |

### 变更点

1. **`backend/handlers/instrument.go` 新增 `BatchUpdateInstrumentStatus`**
   `POST` 体 `{ids:[], status}`；逐条校验状态白名单（复用 `UpdateInstrumentStatus` 的合法集合）；单条非法 id／跨租户／非法状态 → 进 `failed[]`（`reason` 为机器可读串），成功计数 `updated`；不改任何其它字段。

2. **`backend/main.go` 路由**
   - 新增 `PUT /instruments/batch/status`（`ExplicitTenantScope()` + `RequireCusPerm("instrument:update")`）；
   - 移除 `PUT /instruments/:id/status` 上的 `RequireDebugMode()`（保留 `ExplicitTenantScope` + `RequireCusPerm`）；
   - **不动** `PUT /orders/:id/admin-update` 的 `RequireDebugMode`（D5）。

3. **`backend/handlers/pricing.go` 重写 `BatchSetInstrumentPricing`**（单条路径改为一次 `Updates` 写多列）
   - 入参：`{items:[{id, base_daily_rate?, overdue_daily_fee?, deposit?}]}`，金额单位**元**；每项至少一个字段，否则该条进 `failed[]`（reason=`no_fields`）。
   - `base_daily_rate`（> 0）→ `base_daily_rate` 列（分）+ `pricing.daily_rent`（元）+ **删除 `pricing_overrides.daily_rent`**。
   - `overdue_daily_fee`（> 0）→ `pricing.overdue_daily_fee`（元）+ **删除 `pricing_overrides.overdue_daily_fee`**。
   - `deposit`（≥ 0，0 = 显式零押金）→ `deposit` 列（分）+ `pricing.deposit`（元）+ **删除 `pricing_overrides.deposit`**。
   - 新增 JSONB 合并助手 `mergePricingFields(raw string, fields map[string]interface{})`，`mergePricingDeposit` 保留为它的薄封装（单条路径零改动）；overrides 清理用 `pricing_overrides - ARRAY['daily_rent',...]`（JSONB 删键），空对象写回 `{}`。
   - 响应 `{updated, failed:[{id, reason}]}`；`ids` 为空或项数超限 → `40002`。
   - **旧 `overrides` 入参废弃（Rev.5）**：请求体不再声明 `overrides` 字段；若调用方仍传且三字段全空 → 该条进 `failed[]`（reason=`no_fields`），`pricing_overrides` 与定价**都不变**。`pricing_overrides` 的维护入口保持 `PUT /inventory/rent-setting/batch`（`inventory.go:492-514`），本单不改该路径。

4. **`frontend-pc/src/utils/batchPricing.js` 改为开关模型**
   - 导出 `BATCH_PRICE_FIELDS`：`[{key:'base_daily_rate',label:'日租金',unit:'元/天'},{key:'overdue_daily_fee',label:'逾期日租金',unit:'元/天'},{key:'deposit',label:'押金',unit:'元'}]`。
   - `buildBatchPricingPayload(ids, values)`：`values[key] = {enabled, value}`；**只把 `enabled` 为真的项写进 `items[]`**；全关 → `{error:'请至少开启一项'}`；`base_daily_rate`/`overdue_daily_fee` 必须 > 0，`deposit` ≥ 0；空 id → 报错。
   - `batchPricingFeedback(res, requested)`：优先读 `failed[]`，按 `updated` / `failed.length` 给出「成功 N 条，失败 M 条（原因…）」。

5. **两页 UI 收敛**（`InstrumentStock.jsx`、`admin/instrument/List.jsx`）
   删除「价格类型」「操作」两个 `Select` 与 `BATCH_PRICE_TYPE_OPTIONS`/`BATCH_PRICE_OPERATOR_OPTIONS`；改为三行「`Switch` + 标签 + `InputNumber`（关闭时 disabled）」。批量上架/下架改调 `PUT /instruments/batch/status` 并展示 `failed[]`。

6. **`docs/spec/api/api.md`**
   新增 `PUT /instruments/batch/status`（权限/状态集/`failed[]`）；`batch-pricing` 改为三字段可选 + 单位 + 「开关关闭即不改动」+ 显式设置会清理同名 `pricing_overrides` + `failed[]`。

7. **既有调用方收敛（Rev.7，审计 B1）**：`frontend-pc/src/pages/admin/instrument/Detail.jsx` 的「价格设定」仍以 `overrides.overdue_daily_fee` 调 `batch-pricing` → 新后端（D11）不消费 `overrides`，会「提示成功但未落库」。改为显式 `overdue_daily_fee`，并消费 `failed[]`。
8. **批量上/下架真实可达（Rev.7，审计 B2）**：`List.jsx` 三条批量按钮被 `{false && <>…</>}` 整块隐藏；解除隐藏，取值对齐后端白名单：**上架 → `available`、下架 → `archived`**。
9. **计划项落地（Rev.7，审计 B4）**：①抽出 `mergePricingFields` 薄封装；②`Switch` 关闭时 `InputNumber` 必须 `disabled`。
10. **契约与审计（Rev.7，审计 B3 + 观察项）**：`docs/spec/api/api.md` 非法 `status` 改为与实现一致（HTTP 200 + 逐项 `failed[]{reason:"invalid_status"}`，并补 `reason` 全集）；`middleware/audit_logger.go` 登记 `PUT /api/instruments/batch/status`（HIGH）。

### Files

- `backend/handlers/instrument.go`
- `backend/handlers/pricing.go`
- `backend/main.go`
- `backend/handlers/instrument_batch_2519_test.go`（新增）
- `backend/handlers/pricing_overdue_2315_test.go`（改写：legacy `overrides` 拒绝 + 显式字段正向）
- `frontend-pc/src/utils/batchPricing.js`
- `frontend-pc/src/pages/InstrumentStock.jsx`
- `frontend-pc/src/pages/admin/instrument/List.jsx`
- `frontend-pc/src/pages/admin/instrument/Detail.jsx`
- `backend/middleware/audit_logger.go`
- `frontend-pc/tests/batchPricingToggles.test.mjs`（新增）
- `frontend-pc/tests/batchPricing.test.mjs`（改写：三段式 → 三开关模型）
- `docs/spec/api/api.md`
- `scripts/verify-2519.sh`（新增，证据 harness）
- `docs/evidence/2519/R1-batch-status.txt`
- `docs/evidence/2519/R2-cross-tenant.txt`
- `docs/evidence/2519/R3-status-route.txt`
- `docs/evidence/2519/R4-partial-fail.txt`
- `docs/evidence/2519/R5-daily-rate.txt`
- `docs/evidence/2519/R6-overdue.txt`
- `docs/evidence/2519/R7-deposit.txt`
- `docs/evidence/2519/R8-multi-field.txt`
- `docs/evidence/2519/R9-toggles.txt`
- `docs/evidence/2519/R10-frontend-wiring.txt`
- `docs/evidence/2519/R11-docs.txt`
- `docs/evidence/2519/R12-build.txt`
- `docs/evidence/2519/R13-legacy-overrides.txt`
- `docs/evidence/2519/R14-full-gate.txt`
- `docs/evidence/2519/R15-ui-status-contract.txt`
- `docs/evidence/2519/R16-detail-overdue.txt`
- `docs/evidence/2519/R17-invalid-status-contract.txt`
- `docs/evidence/2519/evidence.json`

### 语义变更（被翻转的既有断言）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `backend/handlers/pricing_overdue_2315_test.go:22`（`TestBatchSetInstrumentPricing_OverdueOverride`） | 请求 `{"items":[{"id":…, "overrides":{"overdue_daily_fee":15}}]}` → `pricing-v2.overdue_daily_fee == 1500`（分） | 同一请求 → `updated=0`、`failed[]` 含该 id 且 `reason=no_fields`、`pricing-v2.overdue_daily_fee` 保持原值（0）；正向口径改为显式 `overdue_daily_fee=15` → 1500 | `overrides` 入参废弃（D11）；#2315 的业务意图（可设置逾期费）由显式字段承接，见 R6 |
| `frontend-pc/tests/batchPricing.test.mjs`（#2514 引入，5 个 test） | 断言三段式 payload：`{price_type:'daily_rate', amount:100}` → `items[{id, base_daily_rate}]`；以及 `BATCH_PRICE_TYPE_OPTIONS` / `BATCH_PRICE_OPERATOR_OPTIONS` 的选项集合 | 改写为**三开关模型**：`values[key]={enabled,value}` → 只提交 enabled 项；`BATCH_PRICE_FIELDS` 三项；全关 → 报错；`deposit=0` 合法、日租金 0 非法。旧选项常量引用全部删除 | Rev.5 已删除三段式 API（D2：Owner 判定选择器无意义），旧用例锁定的是被废弃语义 |

- 该用例**改写**而非删除：保留「同一个 #2315 业务意图（可设置逾期费）仍有正向断言」，并新增 legacy 入参拒绝断言，避免覆盖丢失。
- 其余既有 `batch-pricing` 用例（`instrument_leftovers_scope_2276_test.go:77,80`）只断言租户隔离与 `updated` 计数，新响应 `{updated, failed[]}` 仍满足，**无需翻转**。
- 实现若仍消费 `overrides` → R13 FAIL（禁止「静默保留」造成双路径）。

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 批量状态成功 | `backend/handlers/instrument.go` | `PUT /instruments/batch/status` | 3 条合法状态 | 后端 | `code=20000`；`updated=3`；`failed=[]`；DB `stock_status` 全变 | R1 |
| 批量状态跨租户/权限 | 同上 | 同上 | 他租户 id / 无 `instrument:update` | 后端 | 他租户项进 `failed[]`（`not_found_or_forbidden`）且未改写；无权限整体 403 | R2 |
| 单条状态生产可用 | `backend/main.go` | `PUT /instruments/:id/status` | 非 debug 环境 | 后端 | 中间件链**不含** `RequireDebugMode`；请求 `20000` | R3 |
| 批量状态部分失败 | `backend/handlers/instrument.go` | 同上 | 混合合法/非法 id | 后端 | `failed[]` 每项含 `id`+`reason`；成功项已落库 | R4 |
| 批量日租金 | `backend/handlers/pricing.go` | `PUT /instruments/batch-pricing` | `base_daily_rate=50` | 后端 | 列=5000 分；`pricing.daily_rent=50`；`pricing_overrides` 无 `daily_rent`；pricing-v2 日租=5000 分 | R5 |
| 批量逾期日租金 | 同上 | 同上 | `overdue_daily_fee=120` | 后端 | `pricing.overdue_daily_fee=120`；`pricing_overrides` 无 `overdue_daily_fee`；pricing-v2 逾期=120 元 | R6 |
| 批量押金（含显式 0） | 同上 | 同上 | `deposit=100` / `deposit=0` | 后端 | 列=10000 分（或 0）；`pricing.deposit=100`（或 0）；`pricing_overrides` 无 `deposit`；pricing-v2 押金与下单档位一致 | R7 |
| 三字段同开 + 部分失败 | 同上 | 同上 | 一条三字段全开 + 一条不存在 id | 后端 | 三项同时生效；`updated=1`、`failed[]` 含不存在 id | R8 |
| legacy `overrides` 拒绝（语义翻转） | 同上 | 同上 | 只传 `overrides`、三字段全空 | 后端 | `updated=0`、`failed[]` 含该 id 且 `reason=no_fields`、`pricing_overrides` 与定价均不变 | R13 |
| 开关模型（前端逻辑） | `frontend-pc/src/utils/batchPricing.js` | `node --test` | 单项开/多项开/全关/越界 | PC | 只提交开启项；全关报错；`deposit=0` 合法、日租金 0 非法 | R9 |
| 前端接线收敛（静态） | `InstrumentStock.jsx`、`admin/instrument/List.jsx` | grep | — | PC | 命中 `price_type`/`operator` 选择器 = 0；调用 `batch/status` = 1；两页均只用 `BATCH_PRICE_FIELDS` | R10 |
| 文档同步 | `docs/spec/api/api.md` | grep | — | 文档 | 含 `batch/status`、三字段、`failed` 表述 | R11 |
| 构建与测试 | `backend` + `frontend-pc` | 定向测试 + `go build` + `npm run build` + eslint | — | 双端 | 全绿 | R12 |
| 全量后端 gate（Rev.7，审计 B5） | `backend` | `go test ./...` | 修后工作树 | 后端 | `GO_TEST_FULL_OK=1`、`FAIL_COUNT=0`、含 `TestInstrumentsLeftovers_*` | R14 |
| 批量上/下架真实入口（Rev.7，审计 B2） | `frontend-pc/src/pages/admin/instrument/List.jsx` | 抓取按钮提交的 `status` 字面值 → 后端 | 修后树 | PC + 后端 | `UI_STATUS_VALUES=available,archived`、`UI_HIDDEN_BLOCK=0`、`BATCH_UPDATED=N`、`BATCH_FAILED=0` | R15 |
| 详情页逾期日费（Rev.7，审计 B1） | `frontend-pc/src/pages/admin/instrument/Detail.jsx` | 改「逾期日费」→ `PUT /instruments/batch-pricing` | 显式 `overdue_daily_fee` | PC + 后端 | `DETAIL_SENDS_OVERRIDES=0`、`PRICING_OVERDUE_UPDATED=1`、`FAILED_CONSUMED=1` | R16 |
| 非法 status 契约（Rev.7，审计 B3） | `docs/spec/api/api.md` + `backend/handlers/instrument.go` | 提交非法状态 | 修后树 | 文档 + 后端 | `DOC_FAILED_REASONS=5`、`API_INVALID_STATUS_IN_FAILED=1` | R17 |

### Input Truth & Tool Preflight

- **Source of truth**：
  - `backend/main.go:453`（单条 status 挂 `RequireDebugMode`）、`backend/main.go:589`（`batch-pricing` 既有路由）；
  - `backend/handlers/pricing.go:191-250`（当前 `BatchSetInstrumentPricing` 只写 `base_daily_rate`/`overrides`）；
  - `backend/handlers/instrument.go:702-705`（单条编辑：押金列 + JSONB 双写）；
  - `backend/handlers/instrument.go:23-39`（`instrumentDepositExplicit`：列 > 0 即权威）、`backend/handlers/user_rental.go:354-371`（`resolveDepositTier` 以列为准）；
  - `backend/services/pricing.go:203-256`（`pricing_overrides` 优先于 JSONB 回落：`daily_rent` 改写有效基准价、`overdue_daily_fee` 覆盖回落值）。
- **预生产实测（只读，2026-11-25）**：`instruments` 219 行中 `deposit>0` **215** 行、`pricing_overrides` 非空 **24** 行（键分布：`overdue_daily_fee` 24 / `deposit` 15 / `shipping_fee` 14 / `daily_rent` 14）；`pricing` JSONB 含 `daily_rent`、`overdue_daily_fee` 各 182 行。
  ⇒ 若「设置为」只写 JSONB，则 24 行被 overrides 屏蔽、215 行押金被列屏蔽（Rev.3 D6 已证伪，故改 A + 清理同名 override）。
- **差异核对**：`grep -rn "batch/status\|weekly_rate\|monthly_rate\|price_type\|BATCH_PRICE_OPERATOR_OPTIONS" frontend-pc/src backend/main.go` 前后对比，确认「调用存在但无路由」「选项存在但后端不支持」两类缺口同时归零。
- **损坏输入拒绝**：`ids` 空 → `40002`；`status` 不在白名单 → 该条 `failed[]`；`base_daily_rate` ≤ 0 → 该条 `failed[]`；`overdue_daily_fee` ≤ 0 → 该条 `failed[]`；`deposit` < 0 → 该条 `failed[]`；三字段全空 → `no_fields`。
- **工具盲区**：路由中间件链需「读 `main.go` + handler 单测」双证，静态 grep 不能证明运行时无 `RequireDebugMode`；JSONB 删键需实测 `pricing_overrides` 结果（不能只看 SQL 文本）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 批量状态 | 采纳 1A：新增端点 + 去掉单条 `RequireDebugMode` |
| D2 | 批量设价交互 | 三项并列 + 逐项开关，开关关闭项不提交、不改动；废弃「价格类型/操作」选择器 |
| D3 | 失败上报 | 两接口统一 `{updated, failed:[{id, reason}]}`，禁止静默 `continue` |
| D4 | 周/月租与运算语义 | **Out of Scope**（模型无字段；increase/decrease/multiply 另议） |
| D5 | DebugMode 边界 | 仅移除 `instruments/:id/status`；`orders/:id/admin-update` 保持不变 |
| D6 | 押金落库口径 | **列 + JSONB 双写**（`deposit` 分列 + `pricing.deposit` 元），与单条编辑同语义；0 为显式零押金 |
| D7 | 逾期日租金落库 | 仅 `pricing.overdue_daily_fee`（元），无独立列；0 不可表达「免收」（本单要求 > 0，见 Out of Scope） |
| D8 | 显式设置优先 | 「设置为」必须生效：写入权威位置的同时删除 `pricing_overrides` 中的同名键，禁止被遗留 override 屏蔽 |
| D9 | 逾期费与日租金关系 | 不自动联动（用户可分别设置）；批量改日租金不再隐式改逾期费 |
| D10 | 前端证据口径 | 前端只做「纯逻辑测试（`node --test`）+ 静态接线核对」（`evidence_type: static`）；交互渲染不在本单证据范围 |
| D11 | `overrides` 入参 | **废弃**：批量设价只认三个显式字段；legacy `overrides` 不消费，落 `failed[]`（`no_fields`）。`pricing_overrides` 机制保留，入口为 `PUT /inventory/rent-setting/batch` |
| D12 | 前端状态取值契约：上架 = `available`、下架 = `archived`（不引入 `active/inactive`） | 本计划锁定（Rev.7，采纳审计 B2 方案 a） |
| D13 | 非法 `status` 响应形态以逐项 `failed[]{invalid_status}` + HTTP 200 为准，`api.md` 随实现改 | 本计划锁定（Rev.7，采纳审计 B3） |

### Out of Scope

- 周租金/月租金（模型无字段，另立设计）；
- increase/decrease/multiply 运算语义（需取值/取整口径，另议）；
- 「逾期费=0 表示免收」的语义扩展（当前 `CalculatePricing` 把 0 当未配置，另立 Issue）；
- `pricing_overrides` 的管理界面（本单只删同名键，不新增编辑入口）；
- 批量删除/批量改类别/批量导入（`instrument_batch_import.go` 既有路径不动）；
- 库存监控列表的显示字段（已由 #2528 交付）。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 3 个本租户乐器，`stock_status=available` → 批量下架 | `PUT /instruments/batch/status` | `docs/evidence/2519/R1-batch-status.txt` | `STATUS=20000`、`UPDATED=3`、`FAILED=0`、`DB_CHANGED=3` | 任一未落库 → FAIL |
| R2 | 含 1 个他租户 id + 无权限请求各一次 | 同上 | `docs/evidence/2519/R2-cross-tenant.txt` | `CROSS_TENANT_BLOCKED=1`、`NO_PERM_STATUS=403`、`DB_UNCHANGED=1` | 他租户被改 → FAIL |
| R3 | 非 debug 环境 | `PUT /instruments/:id/status` | `docs/evidence/2519/R3-status-route.txt` | `MIDDLEWARE_HAS_DEBUG=0`、`STATUS=20000` | 仍含 DebugMode → FAIL |
| R4 | 混合合法/非法 id | `PUT /instruments/batch/status` | `docs/evidence/2519/R4-partial-fail.txt` | `FAILED>=1`、`REASON_PRESENT=1`、成功项 `DB_CHANGED=1` | 静默成功 → FAIL |
| R5 | 乐器带 `pricing_overrides.daily_rent` | `PUT /instruments/batch-pricing` | `docs/evidence/2519/R5-daily-rate.txt` | `BASE_DAILY_RATE=5000`、`PRICING_DAILY_RENT=50`、`OVERRIDE_DAILY_RENT_REMOVED=1`、`PRICING_V2_RATE=5000` | override 残留 → FAIL |
| R6 | 乐器带 `pricing_overrides.overdue_daily_fee` | 同上 | `docs/evidence/2519/R6-overdue.txt` | `PRICING_OVERDUE=120`、`OVERRIDE_OVERDUE_REMOVED=1`、`PRICING_V2_OVERDUE=120` | 未生效 → FAIL |
| R7 | `deposit=100`；另一台 `deposit=0` | 同上 | `docs/evidence/2519/R7-deposit.txt` | `DEPOSIT_COL=10000`、`PRICING_DEPOSIT=100`、`OVERRIDE_DEPOSIT_REMOVED=1`、`ZERO_CASE_COL=0`、`ZERO_CASE_TIER=zero` | 只写 JSONB（列不变）→ FAIL |
| R8 | 一条三字段全开 + 一条不存在 id | 同上 | `docs/evidence/2519/R8-multi-field.txt` | `UPDATED=1`、`FAILED=1`、`BASE=5000`、`OVERDUE=120`、`DEPOSIT=10000` | 任一项未生效 → FAIL |
| R13 | 只传 `overrides`（legacy） | `PUT /instruments/batch-pricing` | `docs/evidence/2519/R13-legacy-overrides.txt` | `UPDATED=0`、`FAILED=1`、`FAILED_REASON=no_fields`、`OVERRIDES_STORED=0`、`PRICING_V2_OVERDUE=0` | 仍消费 `overrides`（定价变化或 `updated=1`）→ FAIL |
| R14 | 修后 `backend` | `bash scripts/verify-2519.sh r14` | `docs/evidence/2519/R14-full-gate.txt` | `GO_TEST_FULL_OK=1`、`FAIL_COUNT=0`、含 `TestInstrumentsLeftovers` | 任一失败 → FAIL |
| R15 | `List.jsx` 批量按钮（解除隐藏后） | `bash scripts/verify-2519.sh r15` | `docs/evidence/2519/R15-ui-status-contract.txt` | `UI_STATUS_VALUES=available,archived`、`UI_HIDDEN_BLOCK=0`、`BATCH_FAILED=0` | 仍隐藏或提交 `active/inactive` → FAIL |
| R16 | 详情页价格面板「逾期日费」 | `bash scripts/verify-2519.sh r16` | `docs/evidence/2519/R16-detail-overdue.txt` | `DETAIL_SENDS_OVERRIDES=0`、`PRICING_OVERDUE_UPDATED=1`、`FAILED_CONSUMED=1` | 仍传 overrides / 未消费 failed[] → FAIL |
| R17 | `api.md` + 非法状态请求 | `bash scripts/verify-2519.sh r17` | `docs/evidence/2519/R17-invalid-status-contract.txt` | `DOC_FAILED_REASONS=5`、`API_INVALID_STATUS_IN_FAILED=1` | 文档与实现不一致 → FAIL |
| R9 | 开关模型纯函数 | `node --test frontend-pc/tests/batchPricingToggles.test.mjs` | `docs/evidence/2519/R9-toggles.txt` | `TESTS_PASS>=6`、`LEGACY_TEST_REWRITTEN=1`、`ONLY_ENABLED_SUBMITTED=1`、`ALL_OFF_ERROR=1`、`DEPOSIT_ZERO_OK=1` | 全关可提交 / 提交关闭项 → FAIL |
| R10 | 修后前端两文件 | grep | `docs/evidence/2519/R10-frontend-wiring.txt` | `LEGACY_SELECTORS=0`、`BATCH_STATUS_CALL=1`、`FIELDS_CONST=3` | 仍含选择器或手写 payload → FAIL |
| R11 | `docs/spec/api/api.md` | grep | `docs/evidence/2519/R11-docs.txt` | `DOC_TOKENS=4`（`batch/status`、`deposit`、`overdue_daily_fee`、`failed`） | 缺任一 → FAIL |
| R12 | HEAD 源码 | `bash scripts/verify-2519.sh r12` | `docs/evidence/2519/R12-build.txt` | `GO_TEST_OK=1`、`BUILD_OK=1`、`NPM_BUILD_OK=1`、`ESLINT_OK=1` | 任一失败 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2519.sh r1` | docs/evidence/2519/R1-batch-status.txt | exit 0；含 `UPDATED=3`、`FAILED=0` | PASS | 未落库 → FAIL | dev |
| R2 | `bash scripts/verify-2519.sh r2` | docs/evidence/2519/R2-cross-tenant.txt | exit 0；含 `CROSS_TENANT_BLOCKED=1`、`NO_PERM_STATUS=403` | PASS | 跨租户被改 → FAIL | dev |
| R3 | `bash scripts/verify-2519.sh r3` | docs/evidence/2519/R3-status-route.txt | exit 0；含 `MIDDLEWARE_HAS_DEBUG=0` | PASS | 仍含 DebugMode → FAIL | dev |
| R4 | `bash scripts/verify-2519.sh r4` | docs/evidence/2519/R4-partial-fail.txt | exit 0；含 `FAILED>=1`、`REASON_PRESENT=1` | PASS | 静默成功 → FAIL | dev |
| R5 | `bash scripts/verify-2519.sh r5` | docs/evidence/2519/R5-daily-rate.txt | exit 0；含 `PRICING_V2_RATE=5000`、`OVERRIDE_DAILY_RENT_REMOVED=1` | PASS | override 残留 → FAIL | dev |
| R6 | `bash scripts/verify-2519.sh r6` | docs/evidence/2519/R6-overdue.txt | exit 0；含 `PRICING_V2_OVERDUE=120` | PASS | 未生效 → FAIL | dev |
| R7 | `bash scripts/verify-2519.sh r7` | docs/evidence/2519/R7-deposit.txt | exit 0；含 `DEPOSIT_COL=10000`、`ZERO_CASE_TIER=zero` | PASS | 只写 JSONB → FAIL | dev |
| R8 | `bash scripts/verify-2519.sh r8` | docs/evidence/2519/R8-multi-field.txt | exit 0；含 `UPDATED=1`、`FAILED=1` | PASS | 项未生效 → FAIL | dev |
| R13 | `bash scripts/verify-2519.sh r13` | docs/evidence/2519/R13-legacy-overrides.txt | exit 0；含 `FAILED_REASON=no_fields`、`PRICING_V2_OVERDUE=0` | PASS | 仍消费 overrides → FAIL | dev |
| R14 | `bash scripts/verify-2519.sh r14` | docs/evidence/2519/R14-full-gate.txt | exit 0；含 `GO_TEST_FULL_OK=1`、`FAIL_COUNT=0` | PASS | 任一失败 → FAIL | dev |
| R15 | `bash scripts/verify-2519.sh r15` | docs/evidence/2519/R15-ui-status-contract.txt | exit 0；含 `UI_STATUS_VALUES=available,archived`、`UI_HIDDEN_BLOCK=0` | PASS | 隐藏块残留 / 值不符 → FAIL | dev |
| R16 | `bash scripts/verify-2519.sh r16` | docs/evidence/2519/R16-detail-overdue.txt | exit 0；含 `DETAIL_SENDS_OVERRIDES=0`、`PRICING_OVERDUE_UPDATED=1` | PASS | 仍传 overrides → FAIL | dev |
| R17 | `bash scripts/verify-2519.sh r17` | docs/evidence/2519/R17-invalid-status-contract.txt | exit 0；含 `DOC_FAILED_REASONS=5`、`API_INVALID_STATUS_IN_FAILED=1` | PASS | 不一致 → FAIL | dev |
| R9 | `bash scripts/verify-2519.sh r9` | docs/evidence/2519/R9-toggles.txt | exit 0；含 `ALL_OFF_ERROR=1`、`DEPOSIT_ZERO_OK=1`、`LEGACY_TEST_REWRITTEN=1` | PASS | 全关可提交，或旧三段式用例仍在断言废弃 API → FAIL | dev |
| R10 | `bash scripts/verify-2519.sh r10` | docs/evidence/2519/R10-frontend-wiring.txt | exit 0；含 `LEGACY_SELECTORS=0`、`BATCH_STATUS_CALL=1` | PASS | 残留选择器 → FAIL | dev |
| R11 | `bash scripts/verify-2519.sh r11` | docs/evidence/2519/R11-docs.txt | exit 0；含 `DOC_TOKENS=4` | PASS | 缺项 → FAIL | dev |
| R12 | `bash scripts/verify-2519.sh r12` | docs/evidence/2519/R12-build.txt | exit 0；含 `GO_TEST_OK=1`、`NPM_BUILD_OK=1` | PASS | 任一失败 → FAIL | dev |

### Artifact Provenance

- R1–R12（脚本生成，首行 provenance）：写 `# build=1.0.<短码>`、`# commit=<40 位 sha>`、`# dirty=false`。
  - docs/evidence/2519/R1-batch-status.txt
  - docs/evidence/2519/R2-cross-tenant.txt
  - docs/evidence/2519/R3-status-route.txt
  - docs/evidence/2519/R4-partial-fail.txt
  - docs/evidence/2519/R5-daily-rate.txt
  - docs/evidence/2519/R6-overdue.txt
  - docs/evidence/2519/R7-deposit.txt
  - docs/evidence/2519/R8-multi-field.txt
  - docs/evidence/2519/R9-toggles.txt
  - docs/evidence/2519/R10-frontend-wiring.txt
  - docs/evidence/2519/R11-docs.txt
  - docs/evidence/2519/R12-build.txt
  - docs/evidence/2519/R13-legacy-overrides.txt
  - docs/evidence/2519/R14-full-gate.txt
  - docs/evidence/2519/R15-ui-status-contract.txt
  - docs/evidence/2519/R16-detail-overdue.txt
  - docs/evidence/2519/R17-invalid-status-contract.txt
- gate（由 /work 执行）：`~/.config/opencode/scripts/pre-ready-check.sh 2519 --summary-file … --changed-files … --evidence-file docs/evidence/2519/evidence.json --report docs/evidence/2519/pre-ready-2519.json`；`evidence.json` 的 build 字段须匹配 evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$。
- JSONB 断言必须在**真实数据库**读回（Go 测试用真实 DB/测试库均可），不得只看请求前的输入值。

### Dependencies

- #2514
- #2528

### 依赖状态（非声明）

- #2514 已合并（D7 满足）：批量设价的调用链已接通。
- #2528 已合并：列表已返回 `resolved_daily_rate` / `deposit` / `deposit_reduced`（分），本单前端可直接复用展示，无需再改列表列。

### 关联说明（非声明）

- #2522（预生产批量设价端到端核验）：本单上线后需一并复验「押金列 / 逾期费」。
- #2043 / #2101 / #2212：批量设价 UI 的引入与回滚历史，本单只做交互与语义重写。

### Workload

- Size: M



