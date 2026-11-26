## Accepted Plan — #2529 资产详情页取数/金额/占位图（#2528 拆出的同族缺陷）

> Tags: bug, code:go, scope:pc
> 前置（硬依赖）：#2528（`frontend-pc/src/utils/instrumentDisplay.js` 的图片取源/占位/日租/押金纯函数）
> 关联：#975（thumbnail 缺失同类）、#2519（批量设押金的详情页核对）、#2043/#2101（库存监控页历史）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 详情页用 `inventoryApi.list()` **无参**取列表第 1 页（默认 `pageSize=20`），再在内存 `find` 目标 id | `frontend-pc/src/pages/AssetDetail.jsx:21-31` |
| 因此仅第 1 页（默认排序前 20 条）的乐器能打开，其余显示「资产不存在」（预生产 219 台 → 199 台打不开） | 同上 `:45-47` |
| 取不到与网络/接口错误同表现：都是 `setAsset(null)` →「资产不存在」，无原因区分 | 同上 `:26-31` |
| 「估值」裸 `(asset.base_daily_rate || 0).toLocaleString()`，而 `base_daily_rate` 是**分** → 100 倍；列名与实际内容（标准日租金）不一致 | 同上 `:100-103`、`backend/models/cents.go:33` |
| 页头卡片不渲染任何图片 | 同上 `:74-105` |
| 单条取数入口已存在：`GET /instruments/:id` → `GetInstrumentByID`（`instrument:read`），响应含 `poster` / `images` / `media.display[].url` / `base_daily_rate`（分）/ `category_name` / `level_name` / `site_name` / `status` | `backend/main.go:407`、`backend/handlers/api.go:140-165`、`:235-300`、`frontend-pc/src/services/api.js:422` |
| 其它 PC 页占位图也指向未入库资源：`/placeholder-instrument.png`、`/placeholder.png`；`frontend-pc/public/` 只有 favicon | `frontend-pc/src/pages/InstrumentListUser.jsx:163`、`frontend-pc/src/components/IdPhotoDisplay.jsx:66` |

**根因**：详情页把「列表首页快照」当单条数据源（无法覆盖第 2 页之后），并在展示层自选未换算的金额字段与不存在的静态占位图。

### 修复设计（最小、可测、复用 #2528）

1. **取数改单条接口**（`AssetDetail.jsx`）：`instrumentsApi.get(id)`（= `GET /instruments/:id`），不再 `list()`+`find`。
2. **响应状态纯函数**（新文件 `frontend-pc/src/utils/assetDetail.js`）：
   - `assetDetailState(resp)` → `{ status: 'ok', asset }` / `{ status: 'missing' }`（`code === 40400` 或 `data` 为空）/ `{ status: 'error', message }`（请求异常/其它 `code`）；
   - `assetHeroImage(asset)`：复用 #2528 的 `instrumentImageSrc`，输入映射为 `{ thumbnail: asset.media?.display?.[0]?.url, poster, images }`（`GetInstrument` 无 `thumbnail` 字段，`media.display[0].url` 即展示图）。
3. **页面接线**（`AssetDetail.jsx`）：
   - `missing` → 明确文案「资产不存在或已删除」+ 返回按钮；`error` → 「加载失败：<message>」+ 重试按钮（可再次调用）；
   - 页头补图片：`<img src={assetHeroImage(asset)} onError={→ #2528 占位常量}>`；
   - 「估值」→「标准日租金」，渲染走 #2528 的 `formatDailyRate`（分→元，未配置 `-`）；
   - `leaseEnd` 条件块保留（`GetInstrument` 不返回该字段，维持不渲染，不属本单缺陷）。
4. **占位图收敛**（`InstrumentListUser.jsx`、`IdPhotoDisplay.jsx`）：`fallback` 改为 #2528 util 的占位常量；为避免语义错配，在 `instrumentDisplay.js` 增补通用别名导出 `IMAGE_PLACEHOLDER`（同一常量，不复制 data URI）。
5. **后端零改动**，仅补契约回归测试（`backend/handlers/asset_detail_2529_test.go`）：断言 `GET /instruments/:id` 返回页面依赖的键集合，且不存在 id 返回 `40400`（防止未来收窄字段导致页面静默空白）。
6. **不做**：不改 `GetInstrumentByID` 逻辑/字段；不新增「租约到期」字段；不动移动端/公开端。

### 语义变更（必须翻转的既有断言）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `frontend-pc/src/pages/AssetDetail.jsx`（无既有单测） | 详情数据来自 `inventoryApi.list()` 首屏 | 详情数据来自 `instrumentsApi.get(id)` | 旧路径无法覆盖第 2 页之后（本单主缺陷） |

> 后端和 PC 均无既有断言引用旧取数路径（`grep "inventoryApi.list()"` 修复后应为 0），R3 回归行固定既有 `TestGetInstruments_Thumbnail` 等用例不变。

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志） |
|---|---|---|---|---|---|---|
| R1 详情接口字段契约 + 不存在 id | `backend/handlers/asset_detail_2529_test.go` | `GET /instruments/:id` | 存在（有 poster/images/media/deposit）与不存在 id | backend | `STATUS=200`、`CODE=20000`、`HAS_KEYS=1`（`base_daily_rate`/`poster`/`images`/`media.display[0].url`/`category_name`/`level_name`/`site_name`/`status`）、`NOT_FOUND_CODE=40400` | R1 |
| R2 详情取数状态 + 页头图片纯函数 + 三页占位接线（源码结构断言，evidence_type: static） | `frontend-pc/src/utils/assetDetail.js`、`frontend-pc/src/pages/AssetDetail.jsx`、`frontend-pc/src/pages/InstrumentListUser.jsx`、`frontend-pc/src/components/IdPhotoDisplay.jsx` | `node --test frontend-pc/tests/assetDetail.test.mjs` + 源码结构断言 | 20000 / 40400 / 异常 / 空 data；media 首图 / poster / images / 全空；两处旧占位路径 | PC | `NODE_TEST_RC=0`、`TESTS_FAIL=0`、`ASSET_DETAIL_USES_GET=1`、`LIST_STILL_USES_LIST_SCAN=0`、`LEGACY_PLACEHOLDER_REF=0`、`FALLBACK_WIRED=2` | R2 |
| R3 构建 / 回归 / pre-ready gate | 全部改动文件 | `go build ./...` + 定向 `go test ./handlers` + `npx eslint` + `npm run build` + `pre-ready-check.sh` | 修后工作树（fix commit 之后） | backend + PC | `BUILD_RC=0`、`TEST_FAIL_COUNT=0`、`ESLINT_NO_UNDEF=0`、`PC_BUILD_RC=0`、`GATE_RESULT=PASS`、`PLAN_SOURCE=PASS` | R3 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R2 | static | PC 无组件测试运行器（`frontend-pc/package.json` 仅 dev/build/lint/preview）→ 纯函数 `node --test` + 源码结构断言；Owner 豁免见 Decision Lock D4（沿用 #2480/#2527 惯例） |

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 1 台乐器（`base_daily_rate=1000` 分、`poster`、`images`、1 张 `is_display` media）；1 个不存在的 UUID | `bash scripts/verify-2529.sh r1` | `docs/evidence/2529/R1-detail-contract.txt` | `STATUS=200`、`CODE=20000`、`HAS_BASE_DAILY_RATE=1`、`HAS_MEDIA_DISPLAY=1`、`HAS_POSTER=1`、`NOT_FOUND_CODE=40400` | `HAS_MEDIA_DISPLAY=0`（页面无图）→ FAIL；`NOT_FOUND_CODE=20000`（不存在也 200）→ FAIL |
| R2 | `assetDetailState` 4 类响应 + `assetHeroImage` 4 档来源 + 两处旧占位 | `bash scripts/verify-2529.sh r2` | `docs/evidence/2529/R2-pc-detail.txt` | `NODE_TEST_RC=0`、`TESTS_FAIL=0`、`ASSET_DETAIL_USES_GET=1`、`LIST_STILL_USES_LIST_SCAN=0`、`LEGACY_PLACEHOLDER_REF=0`、`FALLBACK_WIRED=2` | `LIST_STILL_USES_LIST_SCAN=1`（仍按首页列表查找）→ FAIL；`LEGACY_PLACEHOLDER_REF>0`（仍 404 占位）→ FAIL |
| R3 | 修后工作树（fix commit 之后） | `bash scripts/verify-2529.sh r3` | `docs/evidence/2529/R3-gate.txt` | `BUILD_RC=0`、`TEST_FAIL_COUNT=0`、`ESLINT_NO_UNDEF=0`、`PC_BUILD_RC=0`、`GATE_RESULT=PASS`、`PLAN_SOURCE=PASS` | 任一 `*_RC=1` 或 `GATE_RESULT!=PASS` → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2529.sh r1` | `docs/evidence/2529/R1-detail-contract.txt` | exit 0；grep `STATUS=200`、`CODE=20000`、`HAS_BASE_DAILY_RATE=1`、`HAS_MEDIA_DISPLAY=1`、`HAS_POSTER=1`、`NOT_FOUND_CODE=40400` | PASS | `HAS_MEDIA_DISPLAY=0` → 页面无图 → FAIL | dev |
| R2 | `bash scripts/verify-2529.sh r2` | `docs/evidence/2529/R2-pc-detail.txt` | exit 0；grep `NODE_TEST_RC=0`、`TESTS_FAIL=0`、`ASSET_DETAIL_USES_GET=1`、`LIST_STILL_USES_LIST_SCAN=0`、`LEGACY_PLACEHOLDER_REF=0`、`FALLBACK_WIRED=2` | PASS | `LIST_STILL_USES_LIST_SCAN=1` → 详情仍按首页列表查 → FAIL | dev |
| R3 | `bash scripts/verify-2529.sh r3` | `docs/evidence/2529/R3-gate.txt` | exit 0；grep `BUILD_RC=0`、`TEST_FAIL_COUNT=0`、`ESLINT_NO_UNDEF=0`、`PC_BUILD_RC=0`、`GATE_RESULT=PASS`、`PLAN_SOURCE=PASS` | PASS | `BUILD_RC=1` 或 `PLAN_SOURCE=FAIL` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2529/R1-detail-contract.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2529/R2-pc-detail.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2529/R3-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 symptom | 触发条件 trigger | 根因 root cause | 测试盲区 test blind spot | 同类路径 same-class | 修复层 fix layer | 红绿 red-green | 预防 prevention |
|---|---|---|---|---|---|---|---|---|
| B1 | 第 2 页之后的乐器详情页显示「资产不存在」 | 列表页 >20 条 | 详情用 `list()` 首屏内存查找 | 无「任意 id 可打开」断言 | `MerchantManagement`（同页列表内 find，未报障） | 取数改 `GET /instruments/:id` | R1 `NOT_FOUND_CODE=40400` + R2 接线 | 详情页禁止 list-scan（结构断言 `LIST_STILL_USES_LIST_SCAN=0`） |
| B2 | 网络失败与「不存在」同表现 | 接口 500/断网 | `catch` 统一 `setAsset(null)` | 无状态区分断言 | 其他 list-scan 详情页 | 纯函数三态 + 页面分支 | R2 `assetDetailState` 4 类 fixture | 状态纯函数进单测 |
| B3 | 日租放大 100 倍 | `base_daily_rate` 为分 | 裸 `toLocaleString` | 无金额单位断言 | #2528 两列表页 | 复用 #2528 `formatDailyRate` | R2 纯函数（#2528 R3 已覆盖 分→元） | 金额展示统一 helper |
| B4 | 列名「估值」语义错 | 用户按估值理解 | 历史命名 | 无列名断言 | #2528 两页 | 列名 + ui.md 同步 | R2 接线断言 + Core Update | UI 文档同源 |
| B5 | 页头无图 | 乐器有展示图 | 页面未渲染图片 | 无图片断言 | #2528 两列表页 | `assetHeroImage` 复用取源 | R2 `assetHeroImage` 4 档 | 取源单一实现 |
| B6 | 占位图 404（两处旧路径） | 图片缺失 | 引用未入库资源 | 无资源断言 | `IdPhotoDisplay` / `InstrumentListUser` | 共用占位常量 | R2 `LEGACY_PLACEHOLDER_REF=0` | 禁止引用未入库静态资源 |
| B7 | 后端收窄字段后页面静默空白（未来回归） | `GetInstrument` 字段被删 | 前后端字段契约无守护 | 无键集合断言 | 其他详情页 | handler 契约测试 | R1 `HAS_*` 键断言 | 契约测试固定页面依赖键 |
| B8 | 只修当前页，同类详情页仍坏 | 其他 list-scan 页面 | 单点修补 | 无邻居扫描 | `grep "\.list()"` 结果 | 邻居扫描结论落 Input Truth | R2 结构断言 | 扫描清单进计划 |

### Workload

- Size: M
- Basis: 1 个后端契约测试 + 1 个 PC 纯函数 + 1 个测试 + 3 个 PC 文件 + 1 个证据脚本 + 1 份 UI 文档；无 migration、无后端逻辑改动；3 条证据行
- Confidence: high（`GET /instruments/:id` 与 #2528 helper 均已存在，测试夹具与 `node --test` 有先例）

### Files（变更文件清单）

- `backend/handlers/asset_detail_2529_test.go`
- `frontend-pc/src/utils/assetDetail.js`
- `frontend-pc/tests/assetDetail.test.mjs`
- `frontend-pc/src/pages/AssetDetail.jsx`
- `frontend-pc/src/pages/InstrumentListUser.jsx`
- `frontend-pc/src/components/IdPhotoDisplay.jsx`
- `frontend-pc/src/utils/instrumentDisplay.js`
- `scripts/verify-2529.sh`
- `docs/spec/ui/ui.md`
- `docs/evidence/2529/plan.md`
- `docs/evidence/2529/evidence.json`
- `docs/evidence/2529/pre-ready-2529.json`
- `docs/evidence/2529/R1-detail-contract.txt`
- `docs/evidence/2529/R2-pc-detail.txt`
- `docs/evidence/2529/R3-gate.txt`

## Input Truth & Tool Preflight

- **Source of truth**：单条乐器 = `GET /instruments/:id`（`GetInstrumentByID`，`instrument:read`）；图片 = `media.display[0].url` → `poster` → `images[0]` → #2528 占位常量；金额 = DB 分、API 分、展示走 `formatCents`；日租展示复用 #2528 `formatDailyRate`（`resolved_daily_rate` 缺失时回落 `base_daily_rate`）。
- **差异核对（实施前跑，结果入 R3 artifact）**：
  - `grep -n "inventoryApi.list()" frontend-pc/src/pages/AssetDetail.jsx` → 修复后必须为 0；
  - `grep -rn "placeholder-instrument.png\|/placeholder.png" frontend-pc/src` → 修复后必须为 0；
  - `grep -c "INSTRUMENT_IMAGE_PLACEHOLDER\|IMAGE_PLACEHOLDER" frontend-pc/src` → ≥3（两处 fallback + AssetDetail onError）。
- **邻居扫描结论**：`grep -rn "\.list()" frontend-pc/src/pages` 后逐一核对——`AssetDetail` 为唯一「详情按列表首页查找」的确定缺陷；`MerchantManagement.jsx:46/55` 为同页列表内查找（未报障，本单不改）。
- **损坏输入拒绝**：`:id` 不存在或非 UUID → `40400`（页面显式「资产不存在或已删除」，`GetInstrumentByID` 无独立 UUID 校验）；请求异常 → 「加载失败」+ 重试；`media` 缺失/空数组 → 占位图（不抛错）。
- **工具盲区（显式声明）**：PC 无组件测试运行器 → R2 纯函数 + 源码结构断言（D4 豁免）；`GetInstrumentByID` 依赖真实 Postgres 测试库（`testfixtures`），skip 视为 FAIL；#2528 helper 必须先落地（依赖）。
- **影响面预检**：仅 3 个 PC 页面换数据源/占位，无后端逻辑改动；`AssetDetail` 字段消费全部存在于 `GetInstrument` 响应（R1 守护）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 详情取数改 `GET /instruments/:id`，禁止 list-scan | Architect | 本计划锁定 | 若要求保留列表缓存以免闪烁 → 回 Architect（仍需服务端兜底单条取数） |
| D2 | 三态展示：`ok` / `missing`（40400）/ `error`（可重试） | Architect | 本计划锁定 | 若要求统一「资产不存在」→ 回 Architect（无法区分故障） |
| D3 | 「估值」→「标准日租金」，展示复用 #2528 `formatDailyRate` | Architect | 本计划锁定（依赖 #2528 D2） | 若要求展示 `pricing-v2` 生效值 → 追加 `instrumentsApi.getInstrumentPricingV2` 调用（另立） |
| D4 | R2 UI 证据形态 = 纯函数 `node --test` + 源码结构断言（`evidence_type: static`） | Owner | Owner 豁免（沿用 #2480/#2527 惯例） | 若审计要求真实渲染证据 → 另立 PC 前端测试基建 Issue |
| D5 | 占位图统一走 #2528 util 常量（增补通用别名 `IMAGE_PLACEHOLDER`），不新增二进制资源 | Architect | 本计划锁定 | 若要求真实图片文件 → 另立（二进制入库边界） |
| D6 | `GetInstrumentByID` 不改逻辑；仅加 handler 契约测试守护页面依赖键 | Architect | 本计划锁定 | 若发现字段缺失需补 → 回 Architect（属 API 契约变更） |

## Out of Scope

- 「租约到期（`leaseEnd`）」字段来源（`GetInstrument` 不返回，条件块维持不渲染；需要时另立）。
- 移动端 / 公开端详情页（`public.go` / `frontend-mobile`）。
- 后端 `GetInstrumentByID` 的 N+1（site/category/property 查询）性能优化。
- `MerchantManagement.jsx` 同页列表查找模式（未报障，仅扫描记录）。
- 预生产浏览器截图回评（PC 本地 build + Owner 回评，归 Ops-Followup）。

## Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/ui/ui.md` §2.20.1 | 「详情/编辑：统一「资产卡片」组件（两旧详情路径收敛，兼容重定向）」一句，未写取数与字段口径 | 明确资产详情页 `GET /instruments/:id` 单条取数、三态（ok/missing/error）、「标准日租金」分→元展示、页头图片取源（`media.display[0].url` → `poster` → `images[0]` → 内置占位） | 文档不写「详情不得用列表首页查找」，同类缺陷会再次出现 |

## Dependencies

- #2528

### 关联（非硬依赖）

- `#975`：同类 `thumbnail` 缺失历史修复。
- `#2519`：批量设押金的详情页核对依赖本单可打开详情。
- `#2043` / `#2101`：库存监控页与资产卡片历史。

*Model: DeepSeek V4.1 Flash 1017*

