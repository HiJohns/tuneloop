## Accepted Plan — #2579 导出 xlsx 字段取值修复（价格/分类/级别接单一数据源，删除无源字段）

> 需求来源：`[Bug] 乐器导出 xlsx 的价格/分类/库存列恒为空或 0（getFieldValue 未接数据源）`。
> 根因：`backend/service/instrument.go:440-456` `getFieldValue()` 只实现了 `sn`/`level`/`stock`(硬编码 `"0"`)/`status`/`description`，其余走 `default: return ""`；而 `headerMap`（`:22-34`）仍在导出 `brand`/`model`/`monthly_rate`/`stock` 这些**模型已删除或根本不存在**的列（`backend/models/models.go:84-90` 注释「Brand 字段已删除」「Model 字段已删除」）。
> 结果：默认导出的 6 列里「品牌/日租金/月租金/押金」空白、「库存数量」恒 0。

### 变更点

#### 1. 字段集合 = 有数据源的字段（表头与选择器同源）

`backend/service/instrument.go` `headerMap`：

| 字段 | 现状 | 本单动作 |
|---|---|---|
| `sn` | 缺失（#2578 已补 `识别码`） | 保留（#2578 交付） |
| `category_name` | 表头有、取值空 | 取值 `instrument.CategoryName` |
| `level` | 表头有、取值 `instrument.Level`（deprecated） | `LevelID` 非空 → `InstrumentLevel.Caption`；否则回落 `Level`；两者皆空 → `-` |
| `daily_rate` | 表头有、取值空 | 经注入的**定价解析器**（见 2）取 `resolved_daily_rate`（分）→ 元 2 位 |
| `deposit` | 表头有、取值空 | 同上取 `deposit`（分；`null` → `-`；显式 `0` → `0.00`） |
| `overdue_daily_fee` | 缺失 | 新增表头「逾期租金」，同上取 `overdue_daily_fee`（分；`null` → `-`） |
| `status` | 表头有、取值原始码 | 保留原始码（D5；中文词表归前端 owner） |
| `description` | 已实现 | 不变 |
| `images` | 表头有、取值空 | 取 `instrument.Images`（JSON 文本） |
| `brand` / `model` / `monthly_rate` / `stock` | 表头有、模型无此数据源（`stock` 无库存概念、`monthly_rate` 定价链不产出） | **移除**表头与前端字段选择器（D3，不再导出空列/恒 0 列） |

#### 2. 金额走单一数据源（禁止导出侧自行解析 pricing JSONB）

`ExportOptions` 增 `MoneyFields func(*models.Instrument) map[string]string`（由 handler 注入）：

- `backend/handlers/instrument_import.go` 构建闭包，内部调用**列表同一个** `instrumentListPriceFields(ctx, inst, configCache)`（`backend/handlers/pricing.go:316`，其下游为 `resolveDailyRateCents` → `CalculatePricing` → `pricingResponseWithInstrument`，与 pricing-v2 / 列表 / 详情同链，#2528/#2537），把「分」转「元」字符串（2 位小数，不可解析 → `-`）；`configCache` 单次导出内复用（每租户最多 1 次配置查询）；配置读取失败按列表既有 D7 行为降级为默认模板并 `log.Printf`，**不**中断导出。
- `backend/service/instrument.go` `getFieldValue` 改为接收 `opts`（内部函数签名变更即可），金额字段只经 `opts.MoneyFields` 取值；`opts.MoneyFields == nil`（如旧调用方/单测）→ 金额列输出 `-`，不得 panic。

#### 3. 前端字段选择器去死字段

- `frontend-pc/src/pages/admin/instrument/List.jsx`：`fieldOptions` 去掉「品牌/型号/库存」，补「识别码/日租金/押金/逾期租金/级别」；默认选中改为 `sn,category_name,status`（原默认 `brand,model,category_name,stock,status` 全是死字段）。
- `frontend-pc/src/pages/InstrumentStock.jsx`：导出 `fields` 由 #2578 的 `sn,category_name,status` 扩为 `sn,category_name,level,daily_rate,deposit,overdue_daily_fee,status`（与表格列一致）。

#### 4. 证据（含红绿实证）

- `backend/handlers/instrument_export_fields_2579_test.go`（新增）：`testfixtures.SetupTestDB` 造 3 条乐器（A：`pricing.daily_rate` 有值、`deposit` NULL → 回落配置比例；B：`pricing_overrides.daily_rent` 覆盖 + `deposit=0` 显式；C：无定价），逐单元格断言值 + 表头断言（不得含品牌/月租金/库存数量）。
- **红**：实现前先跑 `bash scripts/verify-2579.sh r1 --expect-red`，产出 `docs/evidence/2579/R2-red.txt`（断言失败 → 证明缺陷存在）；**绿**：实现后 `r1` 通过。`scripts/verify-2579.sh r2` 校验红文件存在且行内确有空白列证据（`RED_BLANK_CELLS>=1`）。
- `scripts/verify-2579.sh`（新增）：`r1`–`r5`，产物落 `docs/evidence/2579/`，首行 provenance；重命令经 `heavy-run.sh`（D8）。

### Files（变更文件清单）

- `backend/service/instrument.go`
- `backend/handlers/instrument_import.go`
- `backend/handlers/instrument_export_fields_2579_test.go`
- `frontend-pc/src/pages/admin/instrument/List.jsx`
- `frontend-pc/src/pages/InstrumentStock.jsx`
- `docs/spec/api/api.md`
- `docs/spec/ui/ui.md`
- `scripts/verify-2579.sh`
- `docs/evidence/2579/plan.md`
- `docs/evidence/2579/R1-field-values.txt`
- `docs/evidence/2579/R2-red.txt`
- `docs/evidence/2579/R2-red-green.txt`
- `docs/evidence/2579/R3-fields-static.txt`
- `docs/evidence/2579/R4-docs.txt`
- `docs/evidence/2579/R5-build.txt`
- `docs/evidence/2579/evidence.json`
- `docs/evidence/2579/pre-ready-2579.json`

### Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | 导出接口 `fields` 可用值未完整登记，且仍暗示存在 `monthly_rate`/`brand`/`stock` | 登记有效 `fields` 集合（`sn`/`category_name`/`level`/`daily_rate`/`deposit`/`overdue_daily_fee`/`status`/`description`/`images`）、金额单位（分→元 2 位，`null` → `-`）、已移除字段（`brand`/`model`/`monthly_rate`/`stock`）与原因 | 导出字段契约变更；不同步即与实现漂移 |
| `docs/spec/ui/ui.md` | §2.20.1 未说明导出字段集合与金额展示 | 补：两页导出的字段集合（库存监控跟随表格列；乐器列表字段可选）与「金额元两位、缺失显示 `-`」 | 用户可见的导出内容变更 |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 价格/分类/级别取值接单一数据源（分→元；null → `-`） | `backend/handlers/instrument_export_fields_2579_test.go` + `backend/service/instrument.go` | `GET /api/instruments/export?fields=sn,category_name,level,daily_rate,deposit,overdue_daily_fee,status` | fixture：A（`pricing.daily_rate=100`、`deposit` NULL、`level_id`→专业）/ B（`pricing_overrides.daily_rent=150`、`deposit=0`）/ C（无 `pricing`） | backend（Postgres 测试库 127.0.0.1:15433） | `MONEY_DAILY_A=100.00`、`MONEY_DAILY_B=150.00`、`MONEY_DEPOSIT_B=0.00`、`MONEY_DAILY_C=-`、`LEVEL_CAPTION_A=专业`、`CATEGORY_NAME_A=大提琴`、`STATUS_A=available` | R1 |
| 红绿实证（缺陷存在 → 修复生效） | `backend/service/instrument.go` | `bash scripts/verify-2579.sh r1 --expect-red`（实现前）/ `r1`（实现后） | 同一断言集，前/后两次运行 | backend | `RED_CAPTURED=1`、`RED_BLANK_CELLS>=1`、`GREEN_OK=1` | R2 |
| 死字段移除 + 单一链复用（结构断言） | `backend/service/instrument.go` + `frontend-pc/src/pages/admin/instrument/List.jsx` + `frontend-pc/src/pages/InstrumentStock.jsx` | grep / 结构断言 | 修后源码（evidence_type: static） | 双端 | `DEAD_FIELDS_IN_HEADER=0`（brand/model/monthly_rate/stock）、`DEAD_FIELDS_IN_PC_OPTIONS=0`、`SINGLE_CHAIN_REUSE=1`（导出 handler 调 `instrumentListPriceFields`）、`DECLARED_FIELDS=9` | R3 |
| 文档同步（fields 集合 / 金额口径 / 已移除字段） | `docs/spec/api/api.md` + `docs/spec/ui/ui.md` | grep 结构断言 | 修后文档（evidence_type: static） | 文档 | `API_EXPORT_FIELDS=1`、`API_REMOVED_FIELDS=1`、`API_MONEY_UNIT=1`、`UI_EXPORT_FIELDS=1` | R4 |
| 构建 / lint / 回归 | `frontend-pc` + `backend` | `eslint` + `vite build` + `go build` + 定向 `go test` | 修后工作树 | 双端 | `ESLINT_OK=1`、`PC_BUILD_OK=1`、`GO_BUILD_OK=1`、`GO_EXPORT_TEST_OK=1`、`LEGACY_EXPORT_TEST_OK=1`（`TestExportInstruments` sqlmock 用例不回归） | R5 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | integration | Go 定向测试 + Postgres 测试库，逐单元格解析 xlsx |
| R2 | integration | 同一断言集的红/绿两次运行（红文件由实现前采集） |
| R3 | static | 源码结构断言；**Owner 豁免**（沿用 #2577 D9 / #2528 / #2526 D6） |
| R4 | static | 文档 grep（Owner 豁免同上） |
| R5 | build | eslint / vite build / go build / 定向 go test（含既有导出单测） |

### Input Truth & Tool Preflight

- **Source of truth（金额口径唯一链）**：`instrumentListPriceFields`（`backend/handlers/pricing.go:316-345`）→ `resolveDailyRateCents` → `services.CalculatePricing`（`backend/services/pricing.go:175`）→ `pricingResponseWithInstrument`（`backend/handlers/pricing.go:256-266`，`instruments.deposit` 非 nil 即权威）。列表 `/api/instruments` 与 pricing-v2 均走该链（#2528 D6 / #2537 D4）。
- **模型事实**：`models.Instrument` 无 `Brand`/`Model`/库存数量字段（`backend/models/models.go:84-121`）；`Pricing`/`PricingOverrides` 为 JSONB，**禁止**导出侧自行解析（D1）。
- **实施前差异核对**：
  - `grep -n "case \"" backend/service/instrument.go` → 修后 `getFieldValue` 覆盖 `sn/category_name/level/daily_rate/deposit/overdue_daily_fee/status/description/images`；
  - `grep -n "brand\|monthly_rate\|\"stock\"" backend/service/instrument.go frontend-pc/src/pages/admin/instrument/List.jsx` → 修后 0 命中（表头与字段选择器）；
  - `grep -n "instrumentListPriceFields" backend/handlers/instrument_import.go` → 修后 1 处（闭包注入）；
  - 既有 `backend/handlers/instrument_import_test.go:139 TestExportInstruments` 必须仍通过（R5 断言）。
- **损坏输入拒绝**：`pricing`/`pricing_overrides` 非法 JSON → `resolveDailyRateCents`/`CalculatePricing` 回落 0 → 金额列 `-`（不 panic、不整表失败）；`level_id` 指向不存在的 level（关联缺失）→ 回落 `Level` 字符串或 `-`；`opts.MoneyFields == nil` → 金额列 `-`。
- **工具盲区**：xlsx 单元格断言只覆盖 `fields` 显式列出的字段；未显式传 `fields` 时使用默认集（本单把默认集改为 `sn,category_name,status`，价格列需显式选择）。PC 无渲染测试运行器 → 真实下载/打开文件由部署侧复核。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 金额口径唯一来源 = `instrumentListPriceFields`（handler 闭包注入 `MoneyFields`）；导出侧**禁止**解析 `pricing`/`pricing_overrides` JSONB | 本计划锁定 |
| D2 | 金额展示 = 分 → 元，2 位小数；不可解析 → `-`；显式 `0` → `0.00` | 本计划锁定 |
| D3 | 无数据源字段（`brand`/`model`/`monthly_rate`/`stock`）从 `headerMap` 与两页字段选择器移除，不导出空列/恒 0 列 | 本计划锁定 |
| D4 | `level` = `InstrumentLevel.Caption`（`LevelID` 非空）→ 回落 `Level` → `-` | 本计划锁定 |
| D5 | `status` 列维持原始码（中文词表归前端 owner，#2577 D4），本单不改 | 本计划锁定 |
| D6 | 乐器列表默认导出字段改为 `sn,category_name,status`（原默认全为死字段） | 本计划锁定 |
| D7 | PC 证据 = 结构断言 + 构建；真实渲染/下载归部署侧。**Owner 豁免**（沿用 #2577 D9 / #2528 / #2526 D6） | 本计划锁定 |
| D8 | 重命令（vite build / go test / eslint）一律经 `heavy-run.sh` 取锁执行 | 本计划锁定 |

### Out of Scope

- 导出过滤/可见性对齐（`sn`/`category_id`/`exclude_status`/网点范围）→ #2578（本单在其之上补字段取值，`headerMap sn` 复用其交付）。
- 导出状态列的中文词表（前端 owner，见 #2577 D4）。
- 导出格式从 xlsx 改为 CSV。
- 乐器列表/库存监控导出**新增字段选择器**（库存监控仍固定字段集）。
- 移动端（H5 / 小程序）任何改动。
- `pricing` JSONB 存储结构整治、月租金产品语义定义（当前定价链无月租金概念，需产品决策）。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | A：`pricing={"daily_rate":100}`、`deposit=NULL`、`level_id→专业`、`category_name=大提琴`、`stock_status=available`；B：`pricing_overrides={"daily_rent":150}`、`deposit=0`；C：无 `pricing` | `GET /api/instruments/export?fields=sn,category_name,level,daily_rate,deposit,overdue_daily_fee,status`（`TestExportFieldValues2579`） | `docs/evidence/2579/R1-field-values.txt` | `MONEY_DAILY_A=100.00`、`MONEY_DAILY_B=150.00`、`MONEY_DEPOSIT_B=0.00`、`MONEY_DAILY_C=-`、`LEVEL_CAPTION_A=专业`、`CATEGORY_NAME_A=大提琴`、`STATUS_A=available` | 任一金额列为空/`0.00` 冒充缺失（C 应为 `-`）、级别取到 deprecated 字符串而非 caption → FAIL |
| R2 | 与 R1 同一断言集 | 实现前 `bash scripts/verify-2579.sh r1 --expect-red`；实现后 `bash scripts/verify-2579.sh r1` | `docs/evidence/2579/R2-red-green.txt`（红证：`docs/evidence/2579/R2-red.txt`） | `RED_CAPTURED=1`、`RED_BLANK_CELLS>=1`、`GREEN_OK=1` | 未采集红证（只有绿证）→ FAIL；红证与绿证断言集不一致 → FAIL |
| R3 | 修后源码（service + 两个 PC 页面） | `bash scripts/verify-2579.sh r3` | `docs/evidence/2579/R3-fields-static.txt` | `DEAD_FIELDS_IN_HEADER=0`、`DEAD_FIELDS_IN_PC_OPTIONS=0`、`SINGLE_CHAIN_REUSE=1`、`DECLARED_FIELDS=9` | 死字段残留（仍导出空列）或导出侧自解析 pricing JSONB → FAIL |
| R4 | 修后 `docs/spec/api/api.md` + `docs/spec/ui/ui.md` | `bash scripts/verify-2579.sh r4` | `docs/evidence/2579/R4-docs.txt` | `API_EXPORT_FIELDS=1`、`API_REMOVED_FIELDS=1`、`API_MONEY_UNIT=1`、`UI_EXPORT_FIELDS=1` | 文档仍列 `monthly_rate`/`brand`/`stock` 或未写金额单位 → FAIL |
| R5 | 修后工作树 | `bash scripts/verify-2579.sh r5` | `docs/evidence/2579/R5-build.txt` | `ESLINT_OK=1`、`PC_BUILD_OK=1`、`GO_BUILD_OK=1`、`GO_EXPORT_TEST_OK=1`、`LEGACY_EXPORT_TEST_OK=1` | 任一 lint/build/test 失败，或既有 `TestExportInstruments` 回归 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2579.sh r1` | `docs/evidence/2579/R1-field-values.txt` | exit 0；含 `MONEY_DAILY_A=100.00`、`MONEY_DAILY_B=150.00`、`MONEY_DAILY_C=-`、`LEVEL_CAPTION_A=专业` | PASS | 金额空白/以 0 冒充缺失 → FAIL | dev |
| R2 | `bash scripts/verify-2579.sh r2` | `docs/evidence/2579/R2-red-green.txt` | exit 0；含 `RED_CAPTURED=1`、`RED_BLANK_CELLS>=1`、`GREEN_OK=1` | PASS | 无红证或红绿断言集不一致 → FAIL | dev |
| R3 | `bash scripts/verify-2579.sh r3` | `docs/evidence/2579/R3-fields-static.txt` | exit 0；含 `DEAD_FIELDS_IN_HEADER=0`、`DEAD_FIELDS_IN_PC_OPTIONS=0`、`SINGLE_CHAIN_REUSE=1` | PASS | 死字段残留/导出侧自解析定价 → FAIL | dev |
| R4 | `bash scripts/verify-2579.sh r4` | `docs/evidence/2579/R4-docs.txt` | exit 0；含 `API_EXPORT_FIELDS=1`、`API_REMOVED_FIELDS=1`、`API_MONEY_UNIT=1` | PASS | 文档未同步 → FAIL | dev |
| R5 | `bash scripts/verify-2579.sh r5` | `docs/evidence/2579/R5-build.txt` | exit 0；含 `ESLINT_OK=1`、`PC_BUILD_OK=1`、`GO_BUILD_OK=1`、`LEGACY_EXPORT_TEST_OK=1` | PASS | 任一 lint/build/test 失败 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2579.sh`（`r1`–`r5`）；每个文本 artifact 首行：`# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`，另起一行 `GENERATED_AT_HEAD=<40 位 sha>`；`R1/R3/R4/R5` 必须在 fix commit **之后**生成；`docs/evidence/2579/R2-red.txt` 为 **fix 前**采集（其 provenance 记录 fix 前 commit，同样带 `# dirty=false`），`R2-red-green.txt` 在 fix 后生成并引用红证。
- 逐条清单（全部位于 `docs/evidence/2579/`）：
  - `docs/evidence/2579/R1-field-values.txt`
  - `docs/evidence/2579/R2-red-green.txt`
  - `docs/evidence/2579/R2-red.txt`
  - `docs/evidence/2579/R3-fields-static.txt`
  - `docs/evidence/2579/R4-docs.txt`
  - `docs/evidence/2579/R5-build.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2579 --summary-file tmp/pre-ready-2579-summary.md --changed-files tmp/pre-ready-2579-changed.txt --evidence-file docs/evidence/2579/evidence.json --report docs/evidence/2579/pre-ready-2579.json`；`evidence.json` 的 `build` 字段须匹配 evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Bug Reflection Matrix

| 症状 | 触发 | 根因 | 测试盲区 | 同类路径 | 修复层 | 红绿 | 预防 |
|---|---|---|---|---|---|---|---|
| 导出「日租金/月租金/押金」列空白 | 任意导出 | `getFieldValue` 未实现，`default` 返回 `""` | 既有 `TestExportInstruments` 只断言状态码，不看单元格 | 详情/列表同字段（均已接链） | 后端 service + handler 注入 | R2 | 单元格级断言进 R1 |
| 导出「品牌/型号」列空白 | 任意导出 | 模型字段已删除，表头未清理 | 表头集合无断言 | 移动端/PC 展示已去品牌 | 后端 `headerMap` + PC 字段选择器 | R3 | `DEAD_FIELDS=0` 结构断言 |
| 导出「库存数量」恒 0 | 任意导出 | 无库存概念，硬编码 `"0"` | 无值断言 | 无 | 后端 `headerMap` 移除 | R3 | 同上 |
| 导出「分类名称」空白 | 任意导出 | `category_name` 未在 switch 中 | 无值断言 | 列表用 `category_name`（已实现） | 后端 `getFieldValue` | R1 | R1 断言分类名 |
| 导出「级别」显示 deprecated 码 | 有 `LevelID` 的乐器 | 只读 `instrument.Level` | 无断言 | 列表用 `level_name` | 后端 `getFieldValue` | R1 | `LEVEL_CAPTION` 断言 |
| 导出金额与列表/详情不一致 | 有 `pricing_overrides` 的乐器 | 若导出侧自行解析 JSONB 会漂移 | 无跨源一致性断言 | 列表/pricing-v2（同链） | 注入 `instrumentListPriceFields` | R1 | `SINGLE_CHAIN_REUSE=1` |
| 默认导出字段全为死字段 | 乐器列表直接导出 | `selectedExportFields` 默认 `brand,model,category_name,stock,status` | 无默认集断言 | 库存监控固定字段集 | PC 默认集改为 `sn,category_name,status` | 用 R3 | `DEAD_FIELDS_IN_PC_OPTIONS=0` |
| 缺失定价被渲染为 `0`/空白混淆 | 无 `pricing` 的乐器 | 语义未区分「无值」与「0 元」 | 无负例 | 列表 `-`（#2528 D3） | 后端格式化 `-` | R1 负例 | `MONEY_DAILY_C=-` 断言 |

### Dependencies

- #2578

### Workload

- Size: M
- Basis: 后端 2 文件（service 字段实现 + handler 闭包注入）+ 1 个集成测试 + 2 个 PC 页面字段集 + 2 份文档 + 1 个 5 行证据 harness；无 migration、无外部依赖
- Confidence: medium

*Model: deepseek/deepseek-flash*

