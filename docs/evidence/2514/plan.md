## Accepted Plan — #2514 批量设价路径 404：接通 `PUT /instruments/batch-pricing`（共享 payload 构建器 + 仅保留可用语义）

### 背景与现状核对（代码为据，含对 Issue 前提的更正）

| 事实 | 位置 |
|---|---|
| 后端批量设价路由只有 `PUT /instruments/batch-pricing`（`RequireRole(ADMIN, OWNER, site_admin)` + `ExplicitTenantScope`） | `backend/main.go:589` |
| `/instruments/batch/price` **不存在** → 恒 404 | `backend/main.go:403-472`（无此路由） |
| 后端请求契约：`{items:[{id, base_daily_rate(元), overrides?}]}`，`items` 为 `binding:"required"`；返回 `{code:20000, data:{updated}}` | `backend/handlers/pricing.go:196-247` |
| 前端实际发送 `{ids, price_type, amount, operator}` → 与后端契约**完全不同**（即使路径修对也会 400） | `frontend-pc/src/pages/InstrumentStock.jsx:187-192`；`frontend-pc/src/pages/admin/instrument/List.jsx:403-408` |
| **两处**相同死调用（同类扫描结果）：`/site/stock`（库存监控）与 `/instruments/list`（乐器列表） | `InstrumentStock.jsx:187`、`admin/instrument/List.jsx:403` |
| **前提更正**：两处均已判 `res.code`，404 时走 `message.error(...)` → **不存在「假成功」**；真实缺陷是「功能不可用 + 误导文案（未找到相关数据/批量操作失败）」 | `InstrumentStock.jsx:193-201`、`List.jsx:410-417` |
| UI 提供 4 种价格类型 × 4 种操作，但后端只有 `base_daily_rate`（日租金）；`weekly_rate`/`monthly_rate` **全仓仅前端 UI 存在**（后端 0 命中），increase/decrease/multiply 需读改写 | `InstrumentStock.jsx:378-397`、`List.jsx:652-676`；`grep -rni weekly_rate backend` = 0 |
| 后端单条失败 `log` 后 `continue`，仍返回 20000 + `updated` 计数 → 前端无法感知部分失败 | `backend/handlers/pricing.go:229-239` |
| 共享客户端方法已存在：`instrumentsApi.batchSetPricing` → `PUT /instruments/batch-pricing` | `frontend-pc/src/services/api.js:563` |
| `api.md` **没有** `batch-pricing` 的契约章节（仅金额单位矩阵里一句旁注） | `docs/spec/api/api.md:1126` |
| 同页还有一处独立 404：`PUT /instruments/batch/status`（`List.jsx:331`，后端只有 `PUT /instruments/:id/status` 且 `RequireDebugMode`） | → 已另立 **#2519** |

**结论**：修法 = 两页统一走 `instrumentsApi.batchSetPricing`，payload 由**共享纯函数**按后端契约生成；UI 只暴露后端真实支持的组合（日租金 + 设置为），其余选项移除并旁注说明；成功后用 `data.updated` 与请求条数比对，部分失败给出可见提示。

### 变更点

**A. 共享逻辑（唯一 owner）**
1. 新增 `frontend-pc/src/utils/batchPricing.js`：
   - `BATCH_PRICE_TYPE_OPTIONS = [{ value: 'daily_rate', label: '日租金' }]`、`BATCH_PRICE_OPERATOR_OPTIONS = [{ value: 'set', label: '设置为' }]`（UI 选项与后端能力同源，防止再次漂移）；
   - `buildBatchPricingPayload(ids, { price_type, amount })` → `{ payload } | { error }`：
     - `ids` 非数组/空 → `{error:'请先选择乐器'}`；
     - `price_type !== 'daily_rate'` → `{error:'批量设价当前仅支持「日租金」+「设置为」'}`；
     - `amount` 空/非有限数/`<= 0` → `{error:'金额必须大于 0'}`（后端对 `<=0` 会静默跳过，必须前置拦截）；
     - 合法 → `{payload:{items: ids.map(id => ({ id, base_daily_rate: Number(amount) }))}}`（**元**，后端 `ToCentsPtr` ×100）；
   - `batchPricingFeedback(res, requested)` → `{level, text}`：`updated >= requested` → `success`「已修改 N 个乐器价格」；否则 `warning`「已修改 X/N 个乐器，其余未更新（详见后端日志）」。
2. 新增 `frontend-pc/tests/batchPricing.test.mjs`（`node --test`）：合法映射、空 ids、非法 price_type、缺失/0/NaN 金额、`updated==requested`、`updated<requested`、`res=null`。

**B. 两页接线（同业务域复用同一实现）**
3. `frontend-pc/src/pages/InstrumentStock.jsx`：
   - `handleBatchPrice` 改用 `buildBatchPricingPayload` + `instrumentsApi.batchSetPricing`（删除自写 `api.put('/instruments/batch/price')`）；
   - `error` 分支 `message.error(result.error)` 并 `return`；成功分支用 `batchPricingFeedback(res, selectedRowKeys.length)` 选择 `message.success` / `message.warning`；仍刷新 `loadData()`；
   - 模态「价格类型」「操作」下拉的 options 改为 `BATCH_PRICE_TYPE_OPTIONS` / `BATCH_PRICE_OPERATOR_OPTIONS`，并在表单上方加一行说明「当前仅支持按日租金批量设置为固定金额」。
4. `frontend-pc/src/pages/admin/instrument/List.jsx`：同一套改动（`handleBatchPriceSubmit` + 模态 options + 说明行），复用同一实例，禁止复制映射逻辑。

**C. 文档**
5. `docs/spec/api/api.md` 新增 `### 5.6.2 批量设价`（置于 5.6.1 阶梯定价方案 V2 之后）：`PUT /api/instruments/batch-pricing`，权限 `ADMIN/OWNER/site_admin`，请求 `{items:[{id, base_daily_rate(元), overrides?}]}`，响应 `{code, data:{updated}}`，并注明「`items` 逐条失败当前仅记日志、以 `updated` 反映；周/月租与增减乘不支持（见 #2519）」。

**D. 证据**
6. 新增 `scripts/verify-2514.sh`（`r1`–`r6` + `gate`，每条首行写 provenance）。

- Tags: bug, scope:pc, scope:frontend

### Workload

- Size: M
- Basis: 3 个前端代码/测试文件（2 页 + 1 共享件 + 1 测试）+ 1 个文档 + 1 个证据脚本 + 7 条证据行；无后端改动、无 migration
- Confidence: high

## Core Update

| 文档路径 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | `PUT /instruments/batch-pricing` 无契约章节（仅 `:1126` 一句金额单位旁注） | 新增 §5.6.2：路径/权限/请求体（元）/响应 `{updated}`/已知边界（逐条失败只回计数；周月租与增减乘不支持） | 本轮 bug 的根因之一就是「无契约 → 前端自造 payload」；契约必须可查 |

## Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 触发条件 | 库存监控（`/site/stock`）或乐器列表（`/instruments/list`）选行 → 「批量设价」→ 填类型/操作/金额 → 提交 → `PUT /instruments/batch/price` → 404 → 页面提示「未找到相关数据」 | `InstrumentStock.jsx:184-205`、`List.jsx:399-426` |
| 2 | 根因（双重） | ① 路径自造（`batch/price` ≠ `batch-pricing`）；② payload 自造（`{ids,price_type,amount,operator}` ≠ `{items:[{id,base_daily_rate}]}`）→ 即便改路径仍 400 | `main.go:589`、`pricing.go:196-208` |
| 3 | 为何未被发现 | 无 API 契约章节可对照；两页各自实现同一功能（无双页一致性测试）；路径与 payload 均无自动化校验 | `api.md`（无章节）、两处独立实现 |
| 4 | 同类扫描（本单交付） | `batch/price` × 2 处（本单修）；`batch/status` × 1 处（后端无端点，#2519）；`DELETE /instruments/batch`（存在 ✓）；`batch-pricing` 正确使用 2 处（`Detail.jsx:494`、`RentSetting.jsx:164`） | R4 artifact |
| 5 | 防复发（前端） | 选项与 payload 同源于 `batchPricing.js`；两页复用同一实现（禁止再各自拼 payload） | 变更点 1–4 |
| 6 | 防复发（文档） | api.md §5.6.2 固化契约与边界 | 变更点 5 |
| 7 | 回归/负例 | 合法用例 payload 逐字等于后端契约；非法类型/0 金额必须被前置拦截（不产生请求）；`updated<requested` 必须给 warning 而非 success | R1/R2/R3 负例 |
| 8 | 假阴性反例 | 若残留 `batch/price`、或 payload 仍为 `{ids,...}`、或 `updated<requested` 仍报 success → 断言 FAIL | R2/R3/R4 |
| 9 | 影响面 | `api.js` 已有 `batchSetPricing`，无新增客户端方法；`pricing.go` 不改（逐条失败上报归 #2519） | `api.js:563` |
| 10 | 未修边界 | `PUT /instruments/batch/status` 404、周/月租与增减乘语义、后端逐条失败上报 → **#2519** | #2519 |

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 共享 payload/反馈纯函数 | `frontend-pc/src/utils/batchPricing.js`、`frontend-pc/tests/batchPricing.test.mjs` | `node --test` | 合法（daily_rate+set+100）；空 ids；`weekly_rate`；金额 0/NaN/空；`updated=3/3`、`updated=1/3`、`res=null` | Node 20 | `PAYLOAD_ITEMS=2`、`ITEM_KEYS=id,base_daily_rate`、`UNSUPPORTED_REJECTED=1`、`ZERO_REJECTED=1`、`FEEDBACK_WARN=1`（静态结构断言 + Owner 豁免 D5） | R1 |
| R2 库存监控页接线（页面结构） | `frontend-pc/src/pages/InstrumentStock.jsx` | `/site/stock` 页面组件 | 修后源码结构 | PC | `DEAD_PATH=0`、`SHARED_BUILDER=1`、`USES_SHARED_API=1`、`OPTIONS_FROM_SHARED=1`、`PARTIAL_WARN=1` | R2 |
| R3 乐器列表页接线（页面结构） | `frontend-pc/src/pages/admin/instrument/List.jsx` | `/instruments/list` 页面组件 | 修后源码结构 | PC | `DEAD_PATH=0`、`SHARED_BUILDER=1`、`USES_SHARED_API=1`、`OPTIONS_FROM_SHARED=1`、`PARTIAL_WARN=1` | R3 |
| R4 同类路径扫描 | `scripts/verify-2514.sh` | 文本比对（前端写调用 ↔ 后端注册表） | 修后全仓 `frontend-pc/src` | 前端 + 后端路由表 | `BATCH_PRICE_CALLS=0`；`RESIDUAL_OUTSIDE_ALLOWLIST=0`（残留仅 `/instruments/batch/status` → #2519） | R4 |
| R5 文档契约 | `docs/spec/api/api.md` | §5.6.2 | 修后文本 | 文档 | `DOC_SECTION=1`、`DOC_ITEMS_REQUIRED=1`、`DOC_UNIT_YUAN=1`、`DOC_BOUNDARY=1` | R5 |
| R6 全量构建 + ESLint | `frontend-pc`（vite build / eslint） | `cd frontend-pc` | 修后干净树 | PC | `BUILD_EXIT=0`、`ESLINT_NO_UNDEF=0` | R6 |
| R7 pre-ready gate（四源 + 全量构建） | `docs/evidence/2514/pre-ready-2514.json` | `pre-ready-check.sh 2514` | 修后树 + 最新 Accepted Plan | PC | `GATE_RESULT=PASS`、P12 完整、P13 在 `origin/issue-2514` | R7 |

## Input Truth & Tool Preflight

- **Source of truth（契约唯一）**：`backend/main.go:589`（路径 + 权限）与 `backend/handlers/pricing.go:196-247`（请求/响应结构、`items` required、`base_daily_rate` 为元 → `ToCentsPtr`）；金额单位口径见 `docs/spec/api/api.md:1126`。
- **差异核对（只读，已于 /analyze 执行）**：
  - `grep -n '"/instruments' backend/main.go` → `batch-pricing` 唯一批量设价路由；
  - `grep -rn "api\\.\\(put\\|post\\|delete\\)('/instruments" frontend-pc/src` → `batch/price` × 2、`batch/status` × 1、`batch`（delete）✓；
  - `grep -rni "weekly_rate\\|monthly_rate" backend frontend-pc/src` → 仅前端 2 页 UI（确认后端无该语义）；
  - `grep -n "overrides" backend/services/pricing.go` → 仅 `daily_rent` / `deposit` 两个键被消费；
  - `grep -n "batchSetPricing" frontend-pc/src` → `api.js:563` 已存在（本单复用，不新增方法）。
- **损坏输入拒绝**：`ids` 空/非数组 → 前置拒绝（不发请求）；`price_type` 非 `daily_rate` → 拒绝（即使后端收到也会静默忽略）；`amount` 空/NaN/≤0 → 拒绝（后端 `>0` 才写，`0` 会静默跳过 → 会产生「显示成功但没改」）；`res` 为 `null`/无 `data.updated` → 按 `0` 处理并给 warning。
- **工具盲区声明**：
  - PC **无组件测试运行器**（`frontend-pc/package.json` 仅 dev/build/lint/preview）→ R1 用 `node --test` 覆盖纯函数，R2/R3 用**静态结构断言**并在 Decision Lock D5 记录 Owner 豁免（真实点击验证需浏览器，见 Out of Scope）。
  - R4 为文本级扫描，无法解析动态拼接路径；覆盖范围为 `frontend-pc/src` 的 `/instruments` 前缀字面量调用。
  - 后端 `BatchSetInstrumentPricing` 的逐条失败只在服务端日志可见 → 前端只能依 `updated` 计数判断部分失败（上报能力缺口见 #2519）。
- **证据时序**：fix commit 之后重跑 `bash scripts/verify-2514.sh r1` … `r6`；`evidence.json` 的 `build/commit/dirty` 由脚本从 `git rev-parse` 派生，禁止手改。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 本单只接通后端**已有语义**（日租金 + 设置为）；周/月租、押金、增加/减少/乘以**不在本单实现**，UI 移除并旁注 | Architect | 本计划锁定（代码为据：后端无 `weekly_rate`/`monthly_rate` 字段、无读改写能力） | 若 Owner 要求全语义 → 回 Architect，实现在 **#2519** 完成后再放开 UI |
| D2 | 两页必须复用同一共享实现 `batchPricing.js`；禁止各页自拼 payload / 自维护选项列表 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | `DELETE /instruments/batch`、`GET /instruments/export` 已存在，**不改**；`PUT /instruments/batch/status` 404 → #2519 | Architect | 本计划锁定 | 若要求本单一起修 → 需新增后端端点（回 Architect 拆分） |
| D4 | 后端逐条失败上报（`failed[]`）**不在本单**：前端以 `updated` 计数给出 warning | Architect | 本计划锁定 | 若审计要求后端上报 → 转 #2519 |
| D5 | R2/R3 证据形态：静态结构断言 + **Owner 豁免**（PC 无组件测试 runner，沿用 #2469/#2480 惯例） | Architect | Owner 豁免 | 若要求 render/点击 → 另立前端测试基建 Issue（Out of Scope） |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R2 | static | 库存监控页接线结构断言（Owner 豁免见 D5） |
| R3 | static | 乐器列表页接线结构断言（同上） |
| R4 | static+scan | 路径文本比对（可失败），非渲染证据 |

## Out of Scope

- `PUT /instruments/batch/status`（404）→ **#2519**（已创建）。
- 批量设价的周/月租、押金、increase/decrease/multiply 语义与 `pricing_overrides` 键契约 → **#2519**（D1）。
- 后端 `BatchSetInstrumentPricing` 的逐条失败上报（`failed[]`）与错误码 → **#2519**（D4）。
- 浏览器真实点击/截图验证（两页批量设价端到端）→ 前端测试基建另立 Issue；本单以纯函数测试 + 结构断言为准（D5）。
- `frontend-mobile`（H5/小程序）批量设价：该端无此功能，不涉及。
- 其它 `/instruments` 写调用（`DELETE /instruments/batch`、`GET /instruments/export`）已确认后端存在，不改。
- `pricing.go` 任何改动（含 `overrides` 键扩展）。

## G0 Golden Acceptance

| 行 ID | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） | assertions（含负例） |
|---|---|---|---|---|
| R1 | `ids=['i1','i2'],{price_type:'daily_rate',amount:100}`；`ids=[]`；`price_type:'weekly_rate'`；`amount:0/'':NaN`；`res={data:{updated:2}}`、`{data:{updated:1}}`、`null` | `frontend-pc/src/utils/batchPricing.js` 经 `node --test` | `docs/evidence/2514/R1-batch-pricing-fn.txt` | `payload.items.length=2`、首项键集合 `{id,base_daily_rate}`、`base_daily_rate=100`（元）；空 ids/非法类型/0/NaN 均返回 `error` 且无 payload；`updated==2` → success、`updated==1` → warning；负例：`amount:0` 生成 payload → FAIL |
| R2 | 修后 `InstrumentStock.jsx` 源码 | `/site/stock` 页面组件 | `docs/evidence/2514/R2-stock-page-static.txt` | `DEAD_PATH=0`（无 `batch/price`）、`SHARED_BUILDER=1`、`USES_SHARED_API=1`（`instrumentsApi.batchSetPricing`）、`OPTIONS_FROM_SHARED=1`、`PARTIAL_WARN=1`；负例：残留 `api.put('/instruments/batch/price'` → FAIL |
| R3 | 修后 `admin/instrument/List.jsx` 源码 | `/instruments/list` 页面组件 | `docs/evidence/2514/R3-list-page-static.txt` | 同 R2 五个标记；负例：自拼 `items:` 而未用 `buildBatchPricingPayload` → FAIL |
| R4 | 修后 `frontend-pc/src` 全树 `/instruments` 写调用 + 后端注册表 | 文本比对脚本 | `docs/evidence/2514/R4-path-scan.txt` | `BATCH_PRICE_CALLS=0`、`BATCH_PRICING_CALLS>=1`、`RESIDUAL_OUTSIDE_ALLOWLIST=0`（残留 = `/instruments/batch/status`）；负例：出现新的 `/instruments` 写调用不在 allowlist → FAIL |
| R5 | 修后 `api.md` §5.6.2 | 文档正文 | `docs/evidence/2514/R5-docs-sync.txt` | `DOC_SECTION=1`、`DOC_ITEMS_REQUIRED=1`、`DOC_UNIT_YUAN=1`、`DOC_BOUNDARY=1`（含 #2519 引用）；负例：章节缺失 → FAIL |
| R6 | 修后干净树 | `cd frontend-pc` | `docs/evidence/2514/R6-build-lint.txt` | `BUILD_EXIT=0`、`ESLINT_NO_UNDEF=0`；负例：构建非 0 → FAIL |
| R7 | 修后树 + 最新 Accepted Plan + evidence.json | `pre-ready-check.sh 2514` | `docs/evidence/2514/pre-ready-2514.json` | `GATE_RESULT=PASS`、每行 `pass=true`、`dirty=false`、P13 在 `origin/issue-2514`；负例：任一 P 失败/evidence 缺行 → 不得 ready |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2514.sh r1 > docs/evidence/2514/R1-batch-pricing-fn.txt` | `docs/evidence/2514/R1-batch-pricing-fn.txt` | exit 0；`grep -q '^R1_OK$'` 且 `grep -q 'UNSUPPORTED_REJECTED=1'` | `node --test` 全通过（`# fail 0`）、`ITEM_KEYS=id,base_daily_rate`、`FEEDBACK_WARN=1` | `amount:0` 仍生成 payload、或 `updated<requested` 报 success → `R1_FAIL` 且非 0 退出 | dev |
| R2 | `bash scripts/verify-2514.sh r2 > docs/evidence/2514/R2-stock-page-static.txt` | `docs/evidence/2514/R2-stock-page-static.txt` | exit 0；`grep -q '^R2_OK$'` 且 `grep -q 'DEAD_PATH=0'` | 5 个标记全为期望值（`SHARED_BUILDER/USES_SHARED_API/OPTIONS_FROM_SHARED/PARTIAL_WARN=1`） | 残留 `batch/price` 或未用共享构建器 → `R2_FAIL` 且非 0 退出 | dev |
| R3 | `bash scripts/verify-2514.sh r3 > docs/evidence/2514/R3-list-page-static.txt` | `docs/evidence/2514/R3-list-page-static.txt` | exit 0；`grep -q '^R3_OK$'` 且 `grep -q 'DEAD_PATH=0'` | 同 R2 五个标记 | 自拼 payload 或残留死路径 → `R3_FAIL` 且非 0 退出 | dev |
| R4 | `bash scripts/verify-2514.sh r4 > docs/evidence/2514/R4-path-scan.txt` | `docs/evidence/2514/R4-path-scan.txt` | exit 0；`grep -q '^R4_OK$'` 且 `grep -q 'BATCH_PRICE_CALLS=0'` | `RESIDUAL_OUTSIDE_ALLOWLIST=0`，残留清单打印且仅含 `/instruments/batch/status` | 新出现未声明的 `/instruments` 写调用 → 打印并非 0 退出 | dev |
| R5 | `bash scripts/verify-2514.sh r5 > docs/evidence/2514/R5-docs-sync.txt` | `docs/evidence/2514/R5-docs-sync.txt` | exit 0；`grep -q '^R5_OK$'` 且 `grep -q 'DOC_SECTION=1'` | `DOC_ITEMS_REQUIRED=1`、`DOC_UNIT_YUAN=1`、`DOC_BOUNDARY=1` | §5.6.2 缺失或未写 `items`/元单位 → `R5_FAIL` 且非 0 退出 | dev |
| R6 | `bash scripts/verify-2514.sh r6 > docs/evidence/2514/R6-build-lint.txt` | `docs/evidence/2514/R6-build-lint.txt` | exit 0；`grep -q '^R6_OK$'` 且 `grep -q 'BUILD_EXIT=0'` | `ESLINT_NO_UNDEF=0` | 构建非 0 或 `no-undef` > 0 → `R6_FAIL` 且非 0 退出 | dev |
| R7 | `~/.config/opencode/scripts/pre-ready-check.sh 2514 --summary-file docs/evidence/2514/summary.md --changed-files docs/evidence/2514/changed-files.txt --evidence-file docs/evidence/2514/evidence.json --full-gate-cmd "bash scripts/verify-2514.sh gate" --report docs/evidence/2514/pre-ready-2514.json` | `docs/evidence/2514/pre-ready-2514.json` | exit 0；`GATE_RESULT=PASS`，`jq -e '.result == "PASS"'`，evidence 每行 `pass=true` | `dirty=false`、P13 提交在 `origin/issue-2514` | 任一 P 项失败 / evidence 缺行 / `dirty=true` → gate 非 0，禁止 ready | dev |

### Files（变更文件清单）

| 类别 | 路径 |
|---|---|
| 共享构建器（新增） | `frontend-pc/src/utils/batchPricing.js` |
| 单测（新增） | `frontend-pc/tests/batchPricing.test.mjs` |
| 库存监控页 | `frontend-pc/src/pages/InstrumentStock.jsx` |
| 乐器列表页 | `frontend-pc/src/pages/admin/instrument/List.jsx` |
| 核心文档 | `docs/spec/api/api.md` |
| 证据生成器（新增） | `scripts/verify-2514.sh` |
| 证据（计划） | `docs/evidence/2514/plan.md` |
| 证据（摘要） | `docs/evidence/2514/summary.md` |
| 证据（清单） | `docs/evidence/2514/changed-files.txt` |
| 证据（合同汇总） | `docs/evidence/2514/evidence.json` |
| 证据 R1 | `docs/evidence/2514/R1-batch-pricing-fn.txt` |
| 证据 R2 | `docs/evidence/2514/R2-stock-page-static.txt` |
| 证据 R3 | `docs/evidence/2514/R3-list-page-static.txt` |
| 证据 R4 | `docs/evidence/2514/R4-path-scan.txt` |
| 证据 R5 | `docs/evidence/2514/R5-docs-sync.txt` |
| 证据 R6 | `docs/evidence/2514/R6-build-lint.txt` |
| Gate 报告 | `docs/evidence/2514/pre-ready-2514.json` |

## Artifact Provenance

- 生成器：`scripts/verify-2514.sh`（`r1`–`r6` + `gate`），每个文本 artifact 首行必须是
  `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`（锚定本单 fix commit；证据收尾提交不改变锚点）。
- 逐条清单（全部位于 `docs/evidence/2514/`）：
  - `docs/evidence/2514/R1-batch-pricing-fn.txt`
  - `docs/evidence/2514/R2-stock-page-static.txt`
  - `docs/evidence/2514/R3-list-page-static.txt`
  - `docs/evidence/2514/R4-path-scan.txt`
  - `docs/evidence/2514/R5-docs-sync.txt`
  - `docs/evidence/2514/R6-build-lint.txt`
  - `docs/evidence/2514/pre-ready-2514.json`（gate 报告，JSON 自带 `.provenance.{build,commit,dirty}`）
- `docs/evidence/2514/evidence.json` 字段口径：`build` 形如 `1.0.<7-40 位短码>`，
  `commit` 为 40 位 sha，`dirty:false`；evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$
- 证据重生成纪律：fix commit 之后统一重跑 `bash scripts/verify-2514.sh r1` … `r6`，禁止只改 `build/commit` 冒充新证据。

*Model: DeepSeek V4.1 Flash 1017*

