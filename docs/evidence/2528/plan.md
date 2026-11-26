## Accepted Plan — #2528 库存监控列表显示缺陷（图片源 / 分当元 / 押金列）

> Tags: bug, code:go, api-contract, scope:pc
> 关联：#2522（批量设价验收被本缺陷阻塞核对）、#2519（批量设押金需要押金列）、#975（同类 thumbnail 缺失，本页未覆盖）、#2514（批量设价契约）、#2043/#2101（库存监控页历史）
> 前置（硬依赖）：无

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 库存监控页图片列读 `dataIndex: 'images'`（`instruments.images` 旧 JSONB），未用后端已算好的 `thumbnail` | `frontend-pc/src/pages/InstrumentStock.jsx:67-77` |
| 占位图 `/images/default-instrument.jpg` 在 `frontend-pc/public/` 不存在（仅 `favicon.ico` / `favicon.svg`）→ 回退路径本身 404（二次空图） | `frontend-pc/public/`、`InstrumentStock.jsx:74`、`admin/instrument/List.jsx:177` |
| 后端 `GetInstruments` 已返回 `thumbnail`（`instrument_media` 优先 `is_display=true`）与 `poster` | `backend/handlers/api.go:452-500`、`:531-533` |
| 「估值」列直接 `value.toLocaleString()`，而 `models.Cents.MarshalJSON` 输出**分** → 100 倍 | `InstrumentStock.jsx:131-139`、`backend/models/cents.go:33` |
| 同字段在详情页走 `formatCents`（分→元），列表是唯一漏换算处 | `frontend-pc/src/pages/admin/instrument/Detail.jsx:524`、`frontend-pc/src/utils/money.js` |
| 列表无押金列；`GetInstruments` 未返回 `deposit` / `deposit_reduced` | `backend/handlers/api.go:519-537` |
| 押金权威口径已存在：`instruments.deposit` 非 nil 即权威（含显式 0），NULL 才回落定价 | `backend/handlers/pricing.go:250-264`（`pricingResponseWithInstrument`）、`backend/handlers/instrument.go:23-39` |
| 定价日租单点解析 `resolveDailyRateCents`（`base_daily_rate>0`，否则 `pricing.daily_rent`） | `backend/handlers/public.go:252-263`，pricing-v2 亦用它（`pricing.go:305-311`） |
| `admin/instrument/List.jsx` 图片列已用 `record.thumbnail`，但占位图同样是 404 且无押金/价格列 | `frontend-pc/src/pages/admin/instrument/List.jsx:164-185` |
| 预生产实测：219 台乐器 170 台 `images` 空；3 台验收乐器 `base_daily_rate=1000`（分）、`is_display` 媒体 12 张 | #2528 正文只读 SQL |

**根因**：列表展示层自选数据源（旧 `images`、裸 `toLocaleString`、无押金字段），与会话链路的单一权威源（`thumbnail` / `Cents`→`formatCents` / `pricingResponseWithInstrument`）脱钩；占位图指向不存在的静态资源，使回退路径本身也失败。

### 修复设计（最小、可测、同源）

1. **后端 `GetInstruments`（`backend/handlers/api.go`）新增三个 additive 字段**（既有字段语义不变）：
   - `resolved_daily_rate`：`int64` 分，值 = pricing-v2 的 `base_daily_rate`（含 `pricing_overrides.daily_rent` 覆盖）；0 表示无法解析（pricing-v2 对同一乐器返回 `40004`）。
   - `deposit`：`int64` 分或 `null`，与 pricing-v2 `deposit` **同一函数**产出（`pricingResponseWithInstrument`）：`instruments.deposit` 非 nil 且显式（含显式 0）即权威，NULL 才回落商户定价（`CalculatePricing`）。
   - `deposit_reduced`：`int64` 分或 `null`（`instruments.deposit_reduced` 原值；null = 无差额档，0 = 显式零差额）。
2. **定价配置加载抽单点**（`backend/handlers/pricing.go`）：新增 `loadMerchantPricingConfigJSON(ctx, tenantID)`，把 pricing-v2 内联的「查 `merchant_pricing_configs`，NotFound → 内置默认模板」逻辑抽成函数，pricing-v2 与列表共用（pricing-v2 行为不变：非 NotFound 仍 500）。
3. **列表押金解析抽单点**（`backend/handlers/pricing.go`）：新增 `instrumentListPriceFields(...)`，内部复用 `resolveDailyRateCents`、`instrumentDepositExplicit`、`pricingResponseWithInstrument`、`CalculatePricing`，**不复制**优先级判定；商户配置按 `tenant_id` 在单次请求内缓存（一页最多 1 次查询）。
4. **PC 展示层单一 helper**（新文件 `frontend-pc/src/utils/instrumentDisplay.js`）：
   - `instrumentImageSrc(record)`：`thumbnail` → `poster` → `images[0]`（兼容 JSON 字符串）→ 内置 `data:image/svg+xml` 占位（**不新增二进制资源**，不再引用不存在的 `/images/default-instrument.jpg`）。
   - `formatDailyRate(record)`：`resolved_daily_rate`（>0）优先，回落 `base_daily_rate`，走共享 `formatCents`（分→元）；`<=0`/缺失 → `-`。
   - `formatDeposit(record)`：`deposit == null` → `-`；显式 0 → `¥0.00`；`deposit_reduced != null` 追加「（差额 ¥X）」。
5. **两列表页接线**：
   - `InstrumentStock.jsx`：图片列改用 `instrumentImageSrc` + `onError` 回落占位；「估值」列改名「标准日租金」并用 `formatDailyRate`；新增「押金」列用 `formatDeposit`；两列沿用既有管理级代理 `hasCusPerm('instrument:delete')`。
   - `admin/instrument/List.jsx`：图片列改用 helper（含 `onError`）；新增「标准日租金」「押金」两列，使用**同一代理**（`usePermission().hasCusPerm('instrument:delete')`），保持两页口径一致。
6. **不做**：不改 `images`/`thumbnail` 后端计算逻辑（已正确）；不改 `Cents` 序列化契约；不引入新权限码；不改批量设价/设押金语义。

### 语义变更（必须翻转的既有断言）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| 无（`grep` 核对 `backend/handlers/*_test.go`、`frontend-pc` 无断言列表字段集/列名） | — | 新增字段为 additive，`TestGetInstruments_Thumbnail` 等既有用例期望值不变 | 本单不改既有字段语义，只新增字段 + 改展示 |

> 结论：**没有需要翻转的既有断言**；R4 回归行固定既有 `TestGetInstruments_Thumbnail` / `TestGetInstruments_DefaultGroupedOrder` / `TestRentSetting_*` / `TestPricingResponseDeposit_*` 全部 PASS。

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志） |
|---|---|---|---|---|---|---|
| R1 列表新增分字段与空值边界 | `backend/handlers/api.go`、`backend/handlers/pricing.go` | `GET /instruments` | F1 显式押金 / F3 显式零押金 / F4 无配置 | backend | `RESOLVED=1000`、`DEPOSIT=50000`、`ZERO_DEPOSIT=0`（存在但为 0）、`NO_CONFIG_DEPOSIT=null`、`NO_CONFIG_RESOLVED=0` | R1 |
| R2 列表 ↔ pricing-v2 同源 | `backend/handlers/pricing.go`、`backend/handlers/api.go` | `GET /instruments` + `GET /instruments/:id/pricing-v2` | F1 / F2 / F3 / F5（覆盖 override） | backend | `PARITY_OK=4`、`PARITY_MISMATCH=0`、`F2_RESOLVED=1000`、`F2_DEPOSIT=3000`、`F5_RESOLVED=2000` | R2 |
| R3 PC 展示层纯函数 + 三页接线（源码结构断言，evidence_type: static） | `frontend-pc/src/utils/instrumentDisplay.js`、`frontend-pc/src/pages/InstrumentStock.jsx`、`frontend-pc/src/pages/admin/instrument/List.jsx` | `node --test frontend-pc/tests/instrumentDisplay.test.mjs` + 源码结构断言 | 空 `images` / 仅 `poster` / 仅 `thumbnail` / 分 1000 / 押金 null / 押金 0 / 差额 0 | PC | `NODE_TEST_RC=0`、`TESTS_FAIL=0`、`STOCK_WIRED=1`、`LIST_WIRED=1`、`LEGACY_ASSET_REF=0`、`LEGACY_RAW_CENTS=0`、`MANAGER_GATE_BOTH=1` | R3 |
| R4 构建 / 回归 / pre-ready gate | 全部改动文件 | `go build ./...` + 定向 `go test ./handlers` + `npx eslint` + `npm run build` + `pre-ready-check.sh` | 修后工作树（fix commit 之后） | backend + PC | `BUILD_RC=0`、`TEST_FAIL_COUNT=0`、`ESLINT_NO_UNDEF=0`、`PC_BUILD_RC=0`、`GATE_RESULT=PASS`、`PLAN_SOURCE=PASS` | R4 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R3 | static | PC 无组件测试运行器（`frontend-pc/package.json` 仅 dev/build/lint/preview）→ 纯函数 `node --test` + 源码结构断言；Owner 豁免见 Decision Lock D5（沿用 #2480/#2527 惯例） |

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | F1 `base_daily_rate=1000` 分 + `deposit=50000` 分；F3 `deposit=0` 且 `pricing={"deposit":0}`；F4 `base_daily_rate=NULL`、`pricing={}`、`deposit=NULL` | `bash scripts/verify-2528.sh r1` | `docs/evidence/2528/R1-list-fields.txt` | `TOTAL=3`、`F1_RESOLVED=1000`、`F1_DEPOSIT=50000`、`F3_DEPOSIT=0`、`F3_DEPOSIT_PRESENT=1`、`F4_RESOLVED=0`、`F4_DEPOSIT=null` | `F1_DEPOSIT=1000`（分当元/取错源）→ FAIL；`F3_DEPOSIT=null`（显式 0 被当未配置）→ FAIL |
| R2 | F1 / F2（`base=NULL`、`pricing={"daily_rent":10,"deposit":30}`）/ F3 / F5（`pricing_overrides={"daily_rent":20}`） | `bash scripts/verify-2528.sh r2` | `docs/evidence/2528/R2-parity.txt` | `PARITY_OK=4`、`PARITY_MISMATCH=0`、`F2_RESOLVED=1000`、`F2_DEPOSIT=3000`、`F5_RESOLVED=2000` | `PARITY_MISMATCH>0`（列表与详情押金/日租不一致）→ FAIL |
| R3 | `instrumentDisplay` 12 条 fixture + 两页源码接线 | `bash scripts/verify-2528.sh r3` | `docs/evidence/2528/R3-pc-display.txt` | `NODE_TEST_RC=0`、`TESTS_FAIL=0`、`TESTS_PASS=12`、`STOCK_WIRED=1`、`LIST_WIRED=1`、`LEGACY_ASSET_REF=0`、`LEGACY_RAW_CENTS=0`、`MANAGER_GATE_BOTH=1` | `LEGACY_ASSET_REF>0`（仍引用 404 占位）或 `LEGACY_RAW_CENTS>0`（仍裸 `toLocaleString` 渲染分）→ FAIL |
| R4 | 修后工作树（fix commit 之后） | `bash scripts/verify-2528.sh r4` | `docs/evidence/2528/R4-gate.txt` | `BUILD_RC=0`、`TEST_FAIL_COUNT=0`、`ESLINT_NO_UNDEF=0`、`PC_BUILD_RC=0`、`GATE_RESULT=PASS`、`PLAN_SOURCE=PASS` | 任一 `*_RC=1` 或 `GATE_RESULT!=PASS` → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2528.sh r1` | `docs/evidence/2528/R1-list-fields.txt` | exit 0；grep `TOTAL=3`、`F1_RESOLVED=1000`、`F1_DEPOSIT=50000`、`F3_DEPOSIT=0`、`F3_DEPOSIT_PRESENT=1`、`F4_RESOLVED=0`、`F4_DEPOSIT=null` | PASS | `F1_DEPOSIT=1000` → 分当元 → FAIL；`F3_DEPOSIT=null` → 显式 0 丢失 → FAIL | dev |
| R2 | `bash scripts/verify-2528.sh r2` | `docs/evidence/2528/R2-parity.txt` | exit 0；grep `PARITY_OK=4`、`PARITY_MISMATCH=0`、`F2_RESOLVED=1000`、`F2_DEPOSIT=3000`、`F5_RESOLVED=2000` | PASS | `PARITY_MISMATCH=1` → 列表与 pricing-v2 不同源 → FAIL | dev |
| R3 | `bash scripts/verify-2528.sh r3` | `docs/evidence/2528/R3-pc-display.txt` | exit 0；grep `NODE_TEST_RC=0`、`TESTS_FAIL=0`、`TESTS_PASS=12`、`STOCK_WIRED=1`、`LIST_WIRED=1`、`LEGACY_ASSET_REF=0`、`LEGACY_RAW_CENTS=0`、`MANAGER_GATE_BOTH=1` | PASS | `LEGACY_ASSET_REF=1`（仍指向 404 占位）或 `LEGACY_RAW_CENTS=1`（仍裸渲染分）→ FAIL | dev |
| R4 | `bash scripts/verify-2528.sh r4` | `docs/evidence/2528/R4-gate.txt` | exit 0；grep `BUILD_RC=0`、`TEST_FAIL_COUNT=0`、`ESLINT_NO_UNDEF=0`、`PC_BUILD_RC=0`、`GATE_RESULT=PASS`、`PLAN_SOURCE=PASS` | PASS | `BUILD_RC=1` 或 `GATE_RESULT=FAIL` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2528/R1-list-fields.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2528/R2-parity.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2528/R3-pc-display.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2528/R4-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 symptom | 触发条件 trigger | 根因 root cause | 测试盲区 test blind spot | 同类路径 same-class | 修复层 fix layer | 红绿 red-green | 预防 prevention |
|---|---|---|---|---|---|---|---|---|
| B1 | 图片列绝大多数空白 | 乐器 `instruments.images` 为空但 `instrument_media` 有 display 图 | 列表自选旧字段 `images`，忽略已返回的 `thumbnail` | 无「thumbnail 优先」断言 | `admin/instrument/List.jsx`、`InstrumentListUser.jsx` | PC 展示层单一取源 helper | R3（thumbnail/poster/images 三档 fixture） | 取源优先级进纯函数单测 |
| B2 | 占位图也是空白 | 乐器无任何图片 | 占位路径 `/images/default-instrument.jpg` 不存在（404） | 无资源存在性断言 | List.jsx 同一路径 | helper 内置 data-URI 占位 + `onError` | R3 `LEGACY_ASSET_REF=0` | 禁止引用未入库静态资源（源码结构断言） |
| B3 | 日租显示放大 100 倍（10 元 → ¥1,000） | `base_daily_rate` 为分 | 裸 `toLocaleString()` 未除 100 | 无金额单位断言 | 其他读 `Cents` 字段的列表 | 统一走 `formatCents` | R3 分 1000 → `¥10.00`；`LEGACY_RAW_CENTS=0` | 金额展示必须经 `money.js`（结构断言） |
| B4 | 列名「估值」与内容（标准日租金）不一致 | 用户按列名理解为资产估值 | 命名沿用历史错误语义 | 无列名断言 | ui.md §2.20.1 同词 | 列名 + 文档同步 | R3 `STOCK_WIRED=1`（列名）+ Core Update | UI 文档与列名同源 |
| B5 | 无法核对批量设押金结果 | 列表无押金字段/列 | 后端未返回 `deposit`，前端无列 | 无押金列断言 | — | 后端 additive 字段 + 两页列 | R1 + R3 | #2519 验收前置字段进计划 |
| B6 | 列表押金与详情不一致（显式 0/NULL 回落） | 乐器未配置押金但商户定价有押金 | 若前端/后端各自实现优先级会分裂 | 无同源断言 | `GetRentSetting`（#2444 已有同类结论） | 复用 `pricingResponseWithInstrument` | R2 `PARITY_MISMATCH=0` | 同源对比测试固定进 R2 |
| B7 | 只修库存监控，乐器列表仍错 | 用户切到 `/instruments/list` | 单页补丁思维 | 无跨页断言 | `admin/instrument/List.jsx` | 同一 helper 两页接线 | R3 `LIST_WIRED=1` | 邻居检查进 Bug Matrix |
| B8 | 后端新增字段无人消费（契约漂移） | 只改后端 | 前后端不同步 | 无消费方断言 | 历史「新增响应字段」改动 | 同单落地两页消费 | R3 `*_WIRED=1` | 新字段必须同单接线 |
| B9 | 文档仍写「估值」，下次又按旧语义实现 | 只改代码 | 契约文档未同步 | 无文档断言 | api.md §5.4.0 / ui.md §2.20.1 | Core Update | Core Update 章节 + R4 | 文档同步列入 Files |

### Workload

- Size: M
- Basis: 2 个后端实现文件 + 1 个后端测试 + 1 个 PC 纯函数 + 1 个 PC 测试 + 2 个 PC 页面 + 1 个证据脚本 + 2 份契约文档；API 仅 additive 字段，无 migration、无新依赖；4 条证据行
- Confidence: high（单点解析器 `resolveDailyRateCents` / `pricingResponseWithInstrument` / `instrumentDepositExplicit` 均已存在，测试夹具 `testfixtures` + `node --test` 均有先例）

### Files（变更文件清单）

- `backend/handlers/api.go`
- `backend/handlers/pricing.go`
- `backend/handlers/instrument_list_pricing_2528_test.go`
- `frontend-pc/src/utils/instrumentDisplay.js`
- `frontend-pc/tests/instrumentDisplay.test.mjs`
- `frontend-pc/src/pages/InstrumentStock.jsx`
- `frontend-pc/src/pages/admin/instrument/List.jsx`
- `scripts/verify-2528.sh`
- `docs/spec/api/api.md`
- `docs/spec/ui/ui.md`
- `docs/evidence/2528/plan.md`
- `docs/evidence/2528/evidence.json`
- `docs/evidence/2528/pre-ready-2528.json`
- `docs/evidence/2528/R1-list-fields.txt`
- `docs/evidence/2528/R2-parity.txt`
- `docs/evidence/2528/R3-pc-display.txt`
- `docs/evidence/2528/R4-gate.txt`

## Input Truth & Tool Preflight

- **Source of truth**：
  - 图片 = `instrument_media`（`is_display=true` 优先）→ 后端 `thumbnail`（`api.go:452-500`）；PC 取源顺序 `thumbnail` → `poster` → `images[0]` → 内置占位。
  - 金额 = DB 分，API 分（`models.Cents.MarshalJSON`），展示走 `frontend-pc/src/utils/money.js`。
  - 日租 = `resolveDailyRateCents`（`base_daily_rate>0`，否则 `pricing.daily_rent`），再经 `CalculatePricing` 应用 `pricing_overrides.daily_rent` 得生效值（pricing-v2 同口径）。
  - 押金 = `pricingResponseWithInstrument`（显式 `instruments.deposit` 即权威，含 0；NULL 回落定价）。
- **差异核对（实施前跑，结果入 R4 artifact）**：
  - `grep -rn "default-instrument.jpg" frontend-pc/src` → 修复后必须为 0。
  - `grep -rn "base_daily_rate.*toLocaleString\|toLocaleString.*base_daily_rate" frontend-pc/src` → 修复后必须为 0。
  - `grep -c "instrumentDepositExplicit" backend/handlers/*.go` → 修复后新增调用点必须复用该函数（不得复制阈值判定）。
- **损坏输入拒绝**：`images` 为非 JSON 字符串/对象 → `instrumentImageSrc` 回落占位且不抛错；`base_daily_rate` 缺失/0/负数 → 显示 `-`（不显示 `¥0.00`）；`deposit` 缺失 → `-`（不伪造 0）；`pricing` JSON 非法 → `CalculatePricing` 既有容错路径（不新增 panic）。
- **工具盲区（显式声明）**：
  - 后端测试依赖真实 Postgres 测试库（`testfixtures.SetupTestDB` / `testutil.SetDB`）；`TUNELOOP_TEST_ALLOW_SKIP` 不得设置，skip 视为 FAIL。
  - PC 无组件测试运行器 → R3 用纯函数 `node --test` + 源码结构断言（D5 豁免）；浏览器视觉效果以本地 `npm run build` + 人工核对列为 Ops/Owner 回评，不冒充渲染证据。
  - 占位图为 `data:image/svg+xml`（文本内联），不新增二进制资源，规避「禁止提交二进制」红线。
- **影响面预检**：`GetInstruments` 仅新增键，`base_daily_rate` / `images` / `thumbnail` / `pricing` 原值不变 → 其他消费方（`RentSetting.jsx` 走 `/inventory/rent-setting`、`Form.jsx` 走 `GET /instruments/:id`）不受影响；`pricing-v2` 抽出配置加载函数后行为不变（R4 回归固定）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 图片取源 = `thumbnail` → `poster` → `images[0]` → 内置 data-URI 占位；不新增二进制资源、不再引用 `/images/default-instrument.jpg` | Architect | 本计划锁定 | 若 Owner 要求真实占位图文件 → 另立（涉及二进制入库与 `.gitignore` 边界） |
| D2 | 「估值」→「标准日租金」；取值 `resolved_daily_rate`（pricing-v2 生效值）优先、回落 `base_daily_rate`；`<=0` 显示 `-` | Architect | 本计划锁定 | 若 Owner 坚持展示配置值（`base_daily_rate`）而非生效值 → 去掉 `resolved_daily_rate`，改 R2 断言 |
| D3 | 押金列 = `deposit` + （`deposit_reduced` 非空时）差额；显式 0 → `¥0.00`；null → `-` | Architect | 本计划锁定 | 若要求分两列（押金/差额押金）→ 回 Architect（表格宽度） |
| D4 | 两页价格列可见性沿用既有管理级代理 `hasCusPerm('instrument:delete')`，不新增权限码 | Architect | 本计划锁定（与 #2043 现有实现一致） | 若要求独立权限码（如 `instrument:pricing_read`）→ 回 Architect + IAM 侧变更 |
| D5 | R3 UI 证据形态 = 纯函数 `node --test` + 源码结构断言（`evidence_type: static`），PC 无组件测试运行器 | Owner | Owner 豁免（沿用 #2480/#2527 惯例） | 若审计要求真实渲染证据 → 另立 PC 前端测试基建 Issue |
| D6 | 列表押金/日租与 pricing-v2 同源由 `pricingResponseWithInstrument` 单一函数保证；配置加载抽 `loadMerchantPricingConfigJSON` 共用 | Architect | 本计划锁定 | 若不抽函数（列表自写一套）→ 触发 #2406/#2444 同类回归，回 Architect |
| D7 | 列表定价配置读取遇非 NotFound 错误 → 记 log 并回落内置默认模板（列表降级），不整页 500 | Architect | 本计划锁定 | 若要求与 pricing-v2 一致 500 → 回 Architect（会让整页不可浏览） |

## Out of Scope

- **资产详情页（`/site/stock/:id`，`AssetDetail.jsx`）的数据源缺陷**：该页仍用 `inventoryApi.list()` 无参取第 1 页后在内存查找，19/20 以上乐器显示「资产不存在」，且其「估值」同样裸渲染分 → 已另立 **#2529**（本单不改该页）。
- 其它 PC 页占位图 404（`InstrumentListUser.jsx` 的 `/placeholder-instrument.png`、`IdPhotoDisplay.jsx` 的 `/placeholder.png`）→ 归 #2529，本单只保证库存监控/乐器列表不再引用失效路径。
- 移动端 / 公开端（`public.go` / `frontend-mobile`）图片与价格显示。
- 批量设价、批量设押金语义（#2519 / #2514）。
- 存量数据回填、DB 结构变更（本单无 migration）。
- `GetInstruments` 既有 N+1（site/category 逐行查询）性能优化。
- 预生产真机/浏览器截图回评（PC 本地 build + Owner 回评，归 Ops-Followup）。

## Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` §5.4.0 | 返回字段写 `images`/`thumbnail`、`base_daily_rate`（称「前端『估值』语义」），无押金字段 | ① 明确「标准日租金」= `resolved_daily_rate`（pricing-v2 生效值，分）优先、`base_daily_rate`（分，原始配置）回落；② 新增 `deposit`（分或 null，显式优先、NULL 回落定价，与 pricing-v2 同源）与 `deposit_reduced`（分或 null）；③ 标注图片取源 `thumbnail` 优先、`images` 仅兼容回落 | 契约不写就会再次出现「页面自选字段 + 自行换算」；#2519 验收也依赖押金字段口径 |
| `docs/spec/ui/ui.md` §2.20.1 | 列表列为「… 估值（按权限显示）…」，无押金列、无图片取源说明 | ① 「估值」→「标准日租金」（分→元展示，未配置显示 `-`）；② 新增「押金」列（含差额档写法）；③ 图片列写取源顺序与内置占位（禁止引用未入库静态资源） | UI 文档是列名/可见性权威，否则下次仍按「估值」实现 |

## Dependencies

无

### 关联（非硬依赖）

- `#2529`：资产详情页（`/site/stock/:id`）数据源 + 金额显示 + 占位图缺陷（本单拆出的同族缺陷，复用本单 helper）。
- `#2522`：其验收（批量设价后核对）被本缺陷阻塞，本单修完后由 Ops 回评。
- `#2519`：批量设押金能力缺口，本单提供核对所需的押金字段/列。
- `#975`：同类 `thumbnail` 缺失修复，本单覆盖其未覆盖的库存监控页。
- `#2514`：批量设价写入 `base_daily_rate`（分），本单展示口径与其一致。
- `#2406` / `#2444`：押金权威口径（显式 0 / NULL 回落）已有裁定，本单只复用，不改语义。

*Model: DeepSeek V4.1 Flash 1017*

