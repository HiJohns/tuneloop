# Work Summary — #2537 两页列口径区分 + 列表透出逾期租金 + 识别码即详情入口

关联：上承 #2528（两页展示 helper 与字段来源），下接 #2538（详情页统一）

## 修改内容

| # | 改动 | 文件 |
|---|------|------|
| 1 | 列表价格字段补 `overdue_daily_fee`（分）：`instrumentListPriceFields` 从 `pricingResponseWithInstrument` 同一计算链取值；`GetInstruments` 透出（不可解析 → `null`，不伪造 0） | `backend/handlers/pricing.go`、`backend/handlers/api.go` |
| 2 | 新增 R1 回归：A(pricing JSONB 36 元)/B(pricing_overrides 15 元)/C(无定价)，断言列表值与 pricing-v2 相等 | `backend/handlers/instrument_list_price_2537_test.go` |
| 3 | 押金展示拆分：`formatDeposit` → `formatDepositFull` / `formatDepositReduced` / `formatOverdueDailyFee`（全额 `null`→`-`、显式 0→¥0.00；差额 `null`→`未配置`；逾期 `null`/≤0→`-`） | `frontend-pc/src/utils/instrumentDisplay.js`、`frontend-pc/tests/instrumentDisplay.test.mjs` |
| 4 | 库存监控：删「级别」「状态」列；押金拆「全额押金」「差额押金」；新增「逾期租金」列；删「详情」按钮；识别码改为详情链接 | `frontend-pc/src/pages/InstrumentStock.jsx` |
| 5 | 乐器列表：删「标准日租金」「押金」列与「查看」按钮；识别码改为详情链接 | `frontend-pc/src/pages/admin/instrument/List.jsx` |
| 6 | 文档：api.md §5.4.0 补 `overdue_daily_fee`；ui.md 登记两页列清单与「识别码即详情入口」 | `docs/spec/api/api.md`、`docs/spec/ui/ui.md` |
| 7 | 证据生成器 `r1`–`r6`（首行 provenance + `GENERATED_AT_HEAD`） | `scripts/verify-2537.sh` |

**Commit**：fix `3f08c2dd8` → evidence（本提交）；provenance 锚定 fix commit。

## 验证层级

- 独立 Issue（无 `Parent:` 行）：受影响面定向——`go build ./...` + `go test ./handlers -run 'TestInstrumentListOverdue2537_R1|TestInstrumentListPricing2528'` + `node --test frontend-pc/tests/instrumentDisplay.test.mjs` + `eslint`（改动文件）+ `npm run build`（frontend-pc）；无 migration。
- 全量 `go test ./...` 不在本单（plan R6 未要求；父/批次由集成侧执行）。

## Remote

remote: `origin/issue-2537`; fix=`3f08c2dd8` pushed=yes; evidence 提交在其后并已 push（P13 校验）

## 红绿实证

本单以**同源等价**代替「缺陷版 FAIL」：R1 直接断言「列表值 == pricing-v2 值」，未加字段前列表无 `overdue_daily_fee`（字段缺失 → `numOrNil` 断言 `ok=false` → FAIL）；加成后 `LIST_OVERDUE_A=3600 == PRICING_V2_OVERDUE_A=3600`、override 1500、无定价 `null`。

```
LIST_OVERDUE_A=3600
PRICING_V2_OVERDUE_A=3600
LIST_OVERDUE_B=1500
LIST_OVERDUE_C=null
--- PASS: TestInstrumentListOverdue2537_R1
```

helper 侧（R2）：旧 `formatDeposit` 把差额并入同一格（`¥500.00（差额 ¥200.00）`），新口径三函数分别断言 `FULL_NULL=-` / `REDUCED_NULL=未配置` / ZERO=¥0.00 / `OVERDUE_NULL=-`（18 tests, 0 fail）。

## Accepted Plan 对照（5/5）

1. 变更点 1：后端列表补 `overdue_daily_fee`（同源、不可解析 null）→ 完成
2. 变更点 2：helper 拆 `formatDepositFull` / `formatDepositReduced` / `formatOverdueDailyFee`，删除 `formatDeposit`，测试同步 → 完成
3. 变更点 3：库存监控删级别/状态、押金拆两列、新增逾期租金、删详情按钮、识别码可点 → 完成
4. 变更点 4：乐器列表删日租/押金列与查看按钮、识别码可点 → 完成
5. 变更点 5：api.md + ui.md 同步 → 完成

## Verification Matrix（6/6）

| 变更点 | 状态 | 证据 |
|---|---|---|
| 列表返回逾期租金 | PASS | `docs/evidence/2537/R1-list-field.txt` |
| 押金拆分 helper | PASS | `docs/evidence/2537/R2-display-helpers.txt` |
| 两页列口径（静态） | PASS | `docs/evidence/2537/R3-columns-static.txt` |
| SN 即详情入口（静态） | PASS | `docs/evidence/2537/R4-sn-link-static.txt` |
| 文档同步 | PASS | `docs/evidence/2537/R5-docs.txt` |
| 构建/测试 | PASS | `docs/evidence/2537/R6-build.txt` |

## Evidence Contract

| 行 | 结果 | 产物 |
|---|------|------|
| R1 | PASS → `LIST_OVERDUE_A=3600、PRICING_V2_OVERDUE_A=3600、LIST_OVERDUE_B=1500、LIST_OVERDUE_C=null、R1_OK` | `docs/evidence/2537/R1-list-field.txt` |
| R2 | PASS → TESTS_PASS=18 / TESTS_FAIL=0 / FULL_NULL=- / REDUCED_NULL=未配置 / ZERO=¥0.00 / OVERDUE_NULL=- / R2_OK | `docs/evidence/2537/R2-display-helpers.txt` |
| R3 | PASS → `LIST_HAS_RATE_COL=0、LIST_HAS_DEPOSIT_COL=0、STOCK_HAS_LEVEL_COL=0、STOCK_HAS_STATUS_COL=0、STOCK_DEPOSIT_COLS=2、STOCK_OVERDUE_COL=1、R3_OK` | `docs/evidence/2537/R3-columns-static.txt` |
| R4 | PASS → `SN_LINK_COUNT=2、DETAIL_BUTTON_COUNT=0、VIEW_BUTTON_COUNT=0、R4_OK` | `docs/evidence/2537/R4-sn-link-static.txt` |
| R5 | PASS → `DOC_OVERDUE=2、UI_LIST_COLUMNS=1、UI_SN_ENTRY=1、R5_OK` | `docs/evidence/2537/R5-docs.txt` |
| R6 | PASS → `GO_BUILD_OK=1、GO_TEST_OK=1、ESLINT_OK=1（0 error）、PC_BUILD_OK=1、R6_OK` | `docs/evidence/2537/R6-build.txt` |

机器可判定记录：`docs/evidence/2537/evidence.json`（6 行，全部 `pass=true`、`dirty=false`、build=1.0.3f08c2dd8、`plan_sha256=e115f761…`）。

### 契约三向核对

- **Plan/API**：计划变更点 1 只给 `GET /api/instruments` 增加 `overdue_daily_fee`（int64 分 / null），字段由 `instrumentListPriceFields` → `pricingResponseWithInstrument` → services.FormatPricingResult 产出，与 `GET /api/instruments/:id/pricing-v2` 的 `overdue_daily_fee` **同一计算链**（R1 断言两值相等：3600 == 3600）。
- **docs**：`docs/spec/api/api.md` §5.4.0 价格字段表已补 `overdue_daily_fee`（分，`null` = 无法解析），与 `backend/handlers/pricing.go` 的实现及 backend/services/pricing.go 的 `FormatPricingResult`（`toCents`，输出 float64 分）逐项对照，单位/空值语义一致；`docs/spec/ui/ui.md` §2.20.1 已登记两页列清单与「识别码即详情入口」。
- **handler/frontend**：frontend-pc/src/pages/InstrumentStock.jsx / frontend-pc/src/pages/admin/instrument/List.jsx 只消费 `resolved_daily_rate` / `overdue_daily_fee` / `deposit` / `deposit_reduced`（均为后端已返回字段，无新增请求参数）；展示统一经 `frontend-pc/src/utils/instrumentDisplay.js` 的 `formatDailyRate` / `formatOverdueDailyFee` / `formatDepositFull` / `formatDepositReduced`（分→元，`formatCents`），未在前端解析 `pricing` JSONB。识别码链接只走已注册路由（`/site/stock/:id`、`/instruments/detail/:id`）。

## G0 Golden Acceptance

| 行 | artifact | assertions | 负例（未被触发） |
|---|---|---|---|
| R1 | `docs/evidence/2537/R1-list-field.txt` | `LIST_OVERDUE_A=3600`／`PRICING_V2_OVERDUE_A=3600`／`LIST_OVERDUE_B=1500`／`LIST_OVERDUE_C=null` | 两值不等或字段缺失 → `R1_FAIL` |
| R2 | `docs/evidence/2537/R2-display-helpers.txt` | `TESTS_PASS>=9`（实际 18）／`REDUCED_NULL=未配置`／ZERO=¥0.00／`OVERDUE_NULL=-` | 差额显示 `-` 或 0 被吞 → `R2_FAIL` |
| R3 | `docs/evidence/2537/R3-columns-static.txt` | `LIST_HAS_RATE_COL=0`／`LIST_HAS_DEPOSIT_COL=0`／`STOCK_HAS_LEVEL_COL=0`／`STOCK_HAS_STATUS_COL=0`／`STOCK_DEPOSIT_COLS=2`／`STOCK_OVERDUE_COL=1` | 列残留/缺失 → `R3_FAIL` |
| R4 | `docs/evidence/2537/R4-sn-link-static.txt` | `SN_LINK_COUNT=2`／`DETAIL_BUTTON_COUNT=0`／`VIEW_BUTTON_COUNT=0` | 识别码不可点或按钮残留 → `R4_FAIL` |
| R5 | `docs/evidence/2537/R5-docs.txt` | `DOC_OVERDUE=2`／`UI_LIST_COLUMNS=1`／`UI_SN_ENTRY=1` | 文档缺项 → `R5_FAIL` |
| R6 | `docs/evidence/2537/R6-build.txt` | `GO_BUILD_OK=1`／`GO_TEST_OK=1`／`ESLINT_OK=1`／`PC_BUILD_OK=1` | 任一失败 → `R6_FAIL` |

## 证据说明（静态断言的边界）

- 前端证据形态按 Decision Lock D7（Owner 豁免同 #2528/#2526）：纯函数 `node --test`（R2）+ 主列表 `columns` 数组块的源码结构断言（R3/R4）。**R3/R4 按「`const columns = [` / `const baseColumns = [` 到首个 `  ]`」截取**，因此文件后段的「丢失记录台账」表（含 `状态` 列与其「详情」按钮）不会被误计；真实渲染由部署侧复核（#2536）。
- `ESLINT` 对改动文件为 0 error（10 条均为既有 warning：`no-use-before-define` / `exhaustive-deps`）。

## 未纳入本单（Out of Scope，计划已登记）

- 详情页字段补全与 `/site/stock/:id` 路由统一 → #2538。
- 列表筛选/排序/分页/导出 CSV 的列集合调整；移动端展示。
- `pricing_overrides` 管理与定价语义（#2519 / #2515）。

*Model: DeepSeek V4.1 Flash 1017*
