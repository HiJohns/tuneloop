# Work Summary — #2538 详情页统一（`/site/stock/:id` → `/instruments/detail/:id`）+ 页头摘要 6 项

基线：`/instruments/detail/:id`（`frontend-pc/src/pages/admin/instrument/Detail.jsx`）。本单终结 #2529 的 `/site/stock/:id` 存根。

## 修改内容

| # | 改动 | 文件 |
|---|------|------|
| 1 | `/site/stock/:id` 改为兼容重定向：`LegacyStockDetailRedirect` → `<Navigate to={/instruments/detail/${id}} replace />`；`/site/stock` 列表路由不变；`AssetDetail` import/路由移除 | `frontend-pc/src/App.jsx` |
| 2 | 页头 160×160 主图（取源复用 `instrumentImageSrc`：media.display[0].url → `poster` → `images[0]` → 内置占位，`onError` 回落）+ 2×3 摘要网格（标准日租金 / 逾期租金 / 全额押金 / 差额押金 / 估值 / 加入日期） | `frontend-pc/src/pages/admin/instrument/Detail.jsx` |
| 3 | 摘要口径纯函数 `buildAssetSummary(instrument, pricingV2)`（6 项，金额一律分→元，缺失 `-`、差额空 `未配置`） | `frontend-pc/src/utils/assetSummary.js`（新增） |
| 4 | 价格策略标签对齐：标准日租→**标准日租金**、押金→**全额押金**、逾期日费→**逾期租金**；基本信息移除重复「创建时间」（值即页头加入日期）| frontend-pc/src/pages/admin/instrument/Detail.jsx |
| 5 | 删除存根三件套（frontend-pc/src/pages/AssetDetail.jsx / frontend-pc/src/utils/assetDetail.js / frontend-pc/tests/assetDetail.test.mjs）——「流转轨迹」「关联单据」占位随之消失 | 删除 |
| 6 | 文档：登记「乐器详情页（唯一）」信息架构（页头摘要 6 项 + tab 列表 + `/site/stock/:id` 兼容重定向 + 日志为唯一事件视图） | `docs/spec/ui/ui.md` |
| 7 | 证据生成器 `r1`–`r6`（首行 provenance + `GENERATED_AT_HEAD`） | `scripts/verify-2538.sh`（新增） |

**Commit**：fix `308a48c02`（Round 1）+ `bc544a810`（Round 1 审计返工）→ evidence（本提交）；provenance 锚定最新 fix commit。

## 验证层级

- 独立 Issue：`node --test`（21 tests：assetSummary 9 + instrumentDisplay 12）+ `eslint`（改动文件，0 error）+ `npm run build`（frontend-pc）+ `go build ./...`；无 migration、无后端逻辑改动。
- 真实浏览器点击 `/site/stock/<id>` 的重定向与页头渲染属部署侧复核（#2533），本单不冒充。

## Remote

remote: `origin/issue-2538`; fix=`308a48c02`（Round 1）+ `bc544a810`（Round 1 审计返工）pushed=yes; evidence 提交在其后并已 push。

## 红绿实证

- **R3（存根/占位清除）**：修复前存根三件套存在（`STUB_FILES=3`）、存在 `流转轨迹`/`关联单据` 字面量；删除后 `STUB_FILES=0`、`ASSETDETAIL_IMPORTS=0`、`TIMELINE_TAB_LABEL=0`、`DOCUMENTS_TAB_LABEL=0`。
- **R2（摘要口径）**：`node --test` 9/9 通过；关键值 `LABELS=6`、DAILY=¥100.00/天、OVERDUE=¥36.00/天、DEPOSIT=¥500.00、`REDUCED_EMPTY=未配置`、TOTAL_PRICE=¥3600.00（取 `total_price` 360000 分，非日租）、`JOIN_DATE=1月2日 11:04`（取 `created_at`）、`OVERDUE_ZERO=-`（0 不是有效逾期费）、`JOIN_DATE_UNPARSABLE=-`。
- **R1（入口）**：`NAVIGATE_REDIRECT=1`、`ROUTE_WIRED=1`、`STUB_ROUTE_REMOVED=1`、`STOCK_LIST_ROUTE_UNCHANGED=1`。

## Round 1 审计返工（Bug 1 + Findings 1/2）

| 审计项 | 矩阵行 | 修复 | 修复后证据 |
|---|---|---|---|
| **Bug 1**：`逾期租金` 的 0 值口径与列表 `formatOverdueDailyFee`、本页价格 tab 不一致（页头渲染 0 元/天 vs 别处 `-`）；「缺失→`-`」分支在真实数据下不可达 | R2 | `frontend-pc/src/utils/assetSummary.js` 逾期项改用正数判定（`null`/缺失/`≤0` → `-`），与 #2537 `formatOverdueDailyFee` 同语义；旧断言 `0 → ¥0.00/天` **翻转为** `-`，并新增负例「`≤0` 不得渲染 0 元金额」 | `docs/evidence/2538/R2-summary-helpers.txt`：`OVERDUE_ZERO=-`、`OVERDUE_MISSING=-`、`TESTS_PASS=9` |
| Finding 1：`r3` 用 `grep -F "'流转轨迹'"`（带引号）→ `title="流转轨迹"` / JSX 文本形态会漏检 | R3 | 改为不带引号的字面量搜索，显式排除注释行与无关的工单页「工单流转轨迹」并单独计数 | `docs/evidence/2538/R3-stub-removed.txt`：`TIMELINE_RAW_HITS=2`、`WORKORDER_TIMELINE_HITS=1`、`TIMELINE_TAB_LABEL=0` |
| Finding 2：`created_at` 不可解析时回吐原始字符串，与计划「损坏输入 → `-`」不符 | R2 | `formatJoinDate()` 校验可解析性，不可解析回落 `-`；断言由 `'not-a-date'` 翻转为 `'-'` | `docs/evidence/2538/R2-summary-helpers.txt`：`JOIN_DATE_UNPARSABLE=-` |
| Finding 3：（非缺陷）统一后不再有「加载失败 + 重试」三态，属计划 D5 授权行为变更 | — | 不修改，维持计划授权；由 #2533 复核时与 Owner 确认（或另立 UX Issue） | 计划 D5 |

**红绿实证（Bug 1 / Finding 2）**：把 pre-fix `frontend-pc/src/utils/assetSummary.js`（`git show 308a48c02:…`）+ 修复后的测试放入隔离目录运行 → `# pass 6 / # fail 3`（0 元/天 vs `-`、`not-a-date` vs `-`）；修复后 `# pass 9 / # fail 0`。

**红绿实证（Finding 1）**：合成样例 `title="流转轨迹"` → 旧探针 `OLD_QUOTED=0`（漏检），新探针 `NEW_PIPELINE=1`（命中）。

**审计原命令复跑**：`bash scripts/verify-2538.sh r2` / `r3` / `r6` → 全部 `R*_OK`（修复前 r2 在旧语义下 `OVERDUE` 断言为空、`JOIN_DATE_UNPARSABLE` 不存在；修复后见上述 artifact）；`pre-ready-check.sh 2538` → `overall=PASS`（报告 `docs/evidence/2538/pre-ready-2538.json`）。

**范围声明**：仅修复审计意见所列项（逾期 0 值口径、r3 探针、created_at 兜底），未引入新功能；`frontend-pc/src/pages/admin/instrument/Detail.jsx` 价格 tab 与摘要在本轮保持一致（`¥{overdueDailyFee || '-'}` ↔ `≤0 → '-'`）。

## Accepted Plan 对照（7/7）

1. 变更点 1：路由重定向 + 删除存根与其专用 helper → 完成
2. 变更点 2：页头主图 + 摘要网格（6 项，标签/口径/空值按表）+ 摘要纯函数 → 完成
3. 变更点 2：价格策略标签对齐（标准日租金/全额押金/逾期租金）→ 完成
4. 变更点 2：基本信息移除重复「创建时间」（更新时间保留）→ 完成
5. 变更点 3：日志 tab 沿用（不新增接口/时间线），不再出现「流转轨迹」→ 完成
6. 变更点 3：「关联单据」占位彻底移除（随存根删除 + R3 静态断言守住）→ 完成
7. 变更点 4：`docs/spec/ui/ui.md` 登记唯一详情页信息架构 → 完成

## Verification Matrix（6/6）

| 变更点 | 状态 | 证据 |
|---|---|---|
| 旧入口重定向 | PASS | `docs/evidence/2538/R1-route-redirect.txt` |
| 页头摘要字段 | PASS | `docs/evidence/2538/R2-summary-helpers.txt` |
| 存根与占位文案清除 | PASS | `docs/evidence/2538/R3-stub-removed.txt` |
| 日志 tab 存在 | PASS | `docs/evidence/2538/R4-log-tab.txt` |
| 文档同步 | PASS | `docs/evidence/2538/R5-docs.txt` |
| 构建/测试 | PASS | `docs/evidence/2538/R6-build.txt` |

## Evidence Contract

| 行 | 结果 | 产物 |
|---|------|------|
| R1 | PASS → `NAVIGATE_REDIRECT=1、ROUTE_WIRED=1、REDIRECT_TARGET=/instruments/detail/、STOCK_LIST_ROUTE_UNCHANGED=1、STUB_ROUTE_REMOVED=1、R1_OK` | `docs/evidence/2538/R1-route-redirect.txt` |
| R2 | PASS → `TESTS_PASS=9、LABELS=6、OVERDUE_ZERO=-、OVERDUE_MISSING=-、JOIN_DATE_UNPARSABLE=-、REDUCED_EMPTY=未配置、TOTAL_PRICE_CENTS=1、JOIN_DATE_FROM_CREATED_AT=1、R2_OK` | `docs/evidence/2538/R2-summary-helpers.txt` |
| R3 | PASS → `STUB_FILES=0、ASSETDETAIL_IMPORTS=0、TIMELINE_TAB_LABEL=0、DOCUMENTS_TAB_LABEL=0、R3_OK` | `docs/evidence/2538/R3-stub-removed.txt` |
| R4 | PASS → `LOG_TAB=1、ACTIVITY_LOG_CALL=1、HERO_IMAGE=1、SUMMARY_RENDERED=1、CREATE_TIME_ROW=0、R4_OK` | `docs/evidence/2538/R4-log-tab.txt` |
| R5 | PASS → `DOC_SUMMARY=1、DOC_JOIN_DATE=1、DOC_REDIRECT=1、DOC_LOGS=1、R5_OK` | `docs/evidence/2538/R5-docs.txt` |
| R6 | PASS → `NODE_TEST_OK=1、ESLINT_OK=1、PC_BUILD_OK=1、GO_BUILD_OK=1、R6_OK` | `docs/evidence/2538/R6-build.txt` |

机器可判定记录：`docs/evidence/2538/evidence.json`（6 行，全部 `pass=true`、`dirty=false`、build=1.0.bc544a810、`plan_sha256=3fce3af0…`）。

### 契约三向核对

- **Plan/API**：本单不新增/不改后端端点，只消费既有 `GET /api/instruments/:id`（`total_price`、`created_at`、`poster`、`images`、media.display）与 `GET /api/instruments/:id/pricing-v2`（`base_daily_rate` / `overdue_daily_fee` / `deposit` / `deposit_reduced`，均为**分**）；摘要金额统一经 `frontend-pc/src/utils/money.js` 的 `formatCents`，未内联 `/100`。
- **docs**：`docs/spec/ui/ui.md` §2.20.1 已登记唯一详情页（页头摘要 6 项、tab 列表、`/site/stock/:id` 兼容重定向、日志为唯一事件视图、基本信息不重复创建时间），与 frontend-pc/src/pages/admin/instrument/Detail.jsx / frontend-pc/src/utils/assetSummary.js 的实现逐项一致。
- **handler/frontend**：路由 `/site/stock/:id` → `/instruments/detail/:id` 都是 frontend-pc/src/App.jsx 已注册的前端路由（重定向不触达后端）；主图取源复用 `instrumentImageSrc`（与列表同源，禁止第二份实现）。

## G0 Golden Acceptance

| 行 | artifact | assertions | 负例（未被触发） |
|---|---|---|---|
| R1 | `docs/evidence/2538/R1-route-redirect.txt` | `NAVIGATE_REDIRECT=1`／`ROUTE_WIRED=1`／`REDIRECT_TARGET=/instruments/detail/`／`STOCK_LIST_ROUTE_UNCHANGED=1`／`STUB_ROUTE_REMOVED=1` | 无重定向或仍渲染存根 → `R1_FAIL` |
| R2 | `docs/evidence/2538/R2-summary-helpers.txt` | `LABELS=6`／`REDUCED_EMPTY=未配置`／TOTAL_PRICE=¥3600.00／`JOIN_DATE_FROM_CREATED_AT=1`／`OVERDUE_ZERO=-`／`JOIN_DATE_UNPARSABLE=-`／`TESTS_PASS=9` | 标签缺失/估值取错源/差额显示 `-`／逾期 0 值渲染 0 元/天 → `R2_FAIL` |
| R3 | `docs/evidence/2538/R3-stub-removed.txt` | `STUB_FILES=0`／`ASSETDETAIL_IMPORTS=0`／`TIMELINE_TAB_LABEL=0`／`DOCUMENTS_TAB_LABEL=0` | 任一残留 → `R3_FAIL` |
| R4 | `docs/evidence/2538/R4-log-tab.txt` | `LOG_TAB=1`／`ACTIVITY_LOG_CALL=1`／`HERO_IMAGE=1`／`CREATE_TIME_ROW=0` | 日志缺失/主图未接/创建时间重复 → `R4_FAIL` |
| R5 | `docs/evidence/2538/R5-docs.txt` | `DOC_SUMMARY=1`／`DOC_JOIN_DATE=1`／`DOC_REDIRECT=1`／`DOC_LOGS=1` | 文档缺项 → `R5_FAIL` |
| R6 | `docs/evidence/2538/R6-build.txt` | `NODE_TEST_OK=1`／`ESLINT_OK=1`／`PC_BUILD_OK=1`／`GO_BUILD_OK=1` | 任一失败 → `R6_FAIL` |

## 证据说明（静态/单测边界）

- 前端证据形态按 Decision Lock D6（Owner 豁免沿用 #2528/#2526）：纯函数 `node --test`（R2）+ 源码结构断言（R1/R3/R4）。静态断言只证明「源码里存在/不存在」，**不证明**浏览器实际渲染；真实渲染与重定向点击归部署侧复核（#2533 复验对象改为统一后的详情页）。
- `eslint` 对改动文件 0 error（13 条 warning 均为既有 `no-use-before-define` / `exhaustive-deps`）。

## 未纳入本单（Out of Scope，计划已登记）

- 「关联单据」真实功能（租约/维保/财务数据源）→ 需独立设计。
- 日志接口过滤/分页/导出增强；编辑能力与权限模型调整。
- 列表页列口径（#2537，已 ready）与移动端详情页。

*Model: DeepSeek V4.1 Flash 1017*
