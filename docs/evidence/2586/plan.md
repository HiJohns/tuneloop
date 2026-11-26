## Accepted Plan — #2586 乐器数据进/出端点 cus_perm 守卫对齐（导出 = read，导入族 = create）｜Rev.2

> 来源：#2578 审计发现（pre-existing）。缺陷形态：`GET /api/instruments/export` 只挂
> `ExplicitTenantScope`，缺 `RequireCusPerm("instrument:read")` → 无该 cus_perm 的已登录租户用户
> 前端按钮被 `PermissionGate` 隐藏，但直接调 API 仍能导出其可见范围内全部乐器（xlsx）。
> 权威口径已存在：`docs/spec/permissions.md` §3.2.1「导出 / 批量导入 = `instrument:read` / `instrument:create`」，
> 即**代码与 spec 不一致**，本单是补齐实现，不改语义（`target_semantics` = spec 既有定义）。

### 变更点

#### 1. 守卫口径与「同类端点」结论（唯一规则，逐端点）

同一批「乐器数据进/出」端点的现状与判定（`backend/main.go:403-472`）：

| 端点 | 现状守卫 | 结论 | 本单动作 |
|---|---|---|---|
| `GET /api/instruments`（列表） | `ExplicitTenantScope` + `RequireCusPerm("instrument:read")` | 基准口径（不变） | 无 |
| `GET /api/instruments/export` | `ExplicitTenantScope` | 缺 `instrument:read` | 补 `RequireCusPerm("instrument:read")` |
| `POST /api/instruments/import` | `ExplicitTenantScope` | 缺 `instrument:create`（写入乐器 = 创建） | 补 `RequireCusPerm("instrument:create")` |
| `GET /api/instruments/import/template` | 无 | 导入流程子步骤（暴露属性列定义），随导入 → `create` | 补 `RequireCusPerm("instrument:create")` |
| `GET /api/instruments/batch-import/template` | `ExplicitTenantScope` | 同上 | 补 `RequireCusPerm("instrument:create")` |
| `POST /api/instruments/batch-import/preview` | `ExplicitTenantScope` | 导入流程解析步骤（只读解析，仅导入方使用） | 补 `RequireCusPerm("instrument:create")` |
| `POST /api/instruments/batch-import/media` | `ExplicitTenantScope` | 导入流程媒体步骤；持有 `instrument:media_upload` 的角色（merchant_admin/site_admin/site_member/merchant_staff）均同时持有 `instrument:create` | 补 `RequireCusPerm("instrument:create")` |
| `POST /api/instruments/batch-import` | `ExplicitTenantScope` | 缺 `instrument:create`（真正写入乐器） | 补 `RequireCusPerm("instrument:create")` |

**同类扫描结论（不进本单，已立单）**：

- 乐器**只读子端点**（`/instruments/filter-options`、`/instruments/check`、`/instruments/:id/photos/latest`、
  `/instruments/:id/media`、`/instruments/:id/activity-log`、`/instruments/:id/pricing`、`/instruments/:id/pricing-v2`）
  同样缺 `instrument:read`，但加守卫前必须逐端点核对 H5/小程序/员工流程调用方，否则会直接打断流程 → **#2587**。
- 非乐器域导出（`/api/admin/payments/export`、`/api/admin/audit-logs/export`）口径属独立审计议题 → Out of Scope。

#### 2. 后端路由（`backend/main.go`，只加中间件，不改 handler 逻辑）

- `:467` `authRequired.GET("/instruments/export", middleware.ExplicitTenantScope(), middleware.RequireCusPerm("instrument:read"), handlers.ExportInstruments)`
- `:466` `/instruments/import` + `:468` `/instruments/import/template` + `:469` `/instruments/batch-import/template`
  + `:470` `/instruments/batch-import` + `:471` `/instruments/batch-import/preview` + `:472` `/instruments/batch-import/media`
  → 各补 `middleware.RequireCusPerm("instrument:create")`（中间件顺序：`ExplicitTenantScope` 之后、handler 之前）。
- **不动** handler（`backend/handlers/instrument_import.go`、`backend/handlers/instrument_batch_import.go`）：
  权限只在路由层单一 owner；handler 内不再复制 cus_perm 判定（避免双份口径）。

#### 3. 测试（红绿 + 同类覆盖）

- `backend/instrument_route_guard_2586_test.go`（新，`package main`）：**静态结构断言**读取 `main.go` 源码，
  断言导出路由行含 `RequireCusPerm("instrument:read")`、6 条导入族路由行含 `RequireCusPerm("instrument:create")`、
  「乐器进/出端点无守卫」计数 = 0。**修前必 FAIL（红），修后 PASS（绿）**——这是唯一能覆盖「路由表接线」的断言
  （handler 级测试不会因 main.go 缺失中间件而失败）。
- `backend/handlers/instrument_perm_2586_test.go`（新）：
  - `TestExportPerm2586`：`testfixtures.SetupTestDB` + 复用 `seedExportFixture2578`（A/B/E/D fixture），
    路由挂 `middleware.RequireCusPerm("instrument:read")` + `ExportInstruments`，`middleware.PermissionRegistry`
    临时切到 `services.NewPermissionRegistry()`（默认 `MockPermissionRegistry.GetCusPermBit` 恒为 -1 会直通，测不出守卫）。
    覆盖：无 read 位（`CusPerm = 1<<5` maintain）→ 403/`40305` 且响应体非 xlsx；有 read 位 → 200 且 SN 集合与 #2578 R1 一致；
    `cus_perm = 0`（legacy）→ 200（既有兼容语义）；`NAMESPACE_ADMIN`（`isSystemLevelRole`）→ 200（#1964 口径）。
  - `TestImportFamilyPerm2586`：6 条导入族入口各挂 `RequireCusPerm("instrument:create")`；无 create 位 → 6/6 均 403/`40305`
    （中间件先中断，handler 不执行）；有 create 位 → 6/6 均「非 403」（导入无文件 = `40002`，模板 = `200`）。

#### 4. PC 前端入口门控（避免「按钮可见 → 点进去全 403」的死路）

后端补 `instrument:create` 后，两个现有入口对**只读角色**会变成可见但不可用，必须同批加门控：

- `frontend-pc/src/pages/admin/instrument/List.jsx`（`:619-624` 「导入」按钮）：包 `<PermissionGate code="instrument:create">`
  （该文件已 import `PermissionGate`，`:627` 起已用于「新增乐器」）。
- `frontend-pc/src/App.jsx`（`:771` `/instruments/batch-import` 路由）：`ProtectedRoute` 增
  `requiredPermission={{ cusPermCodes: ['instrument:create'] }}`（`checkPermission` 单码即 `cusPermCodes.some`；
  `ProtectedRoute` 对 sys/legacy 全 0 位图 Pass-through，不改既有兼容）。
- `frontend-pc/src/pages/InstrumentStock.jsx` 的「导出 Excel」（`instrument:read`）与「批量导入」（`instrument:create`）
  gate **保持不变**（回归断言，防被顺手改坏）。

#### 5. 文档（Core Update）

见 `## Core Update`。

#### 6. 证据 harness `scripts/verify-2586.sh r1..r6`

沿用 #2581/#2578 形态：首行 provenance（`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`），
重命令经 `heavy-run.sh` 取锁，测试库 `POSTGRES_HOST/PORT=127.0.0.1:15433`（一次性 PostgreSQL，#2341 口径）。

### Files（变更文件清单）

- `backend/main.go`
- `backend/instrument_route_guard_2586_test.go`
- `backend/handlers/instrument_perm_2586_test.go`
- `frontend-pc/src/pages/admin/instrument/List.jsx`
- `frontend-pc/src/App.jsx`
- `docs/spec/api/api.md`
- `docs/spec/permissions.md`
- `scripts/verify-2586.sh`
- `docs/evidence/2586/plan.md`
- `docs/evidence/2586/R1-export-perm.txt`
- `docs/evidence/2586/R2-import-family-perm.txt`
- `docs/evidence/2586/R3-route-wiring.txt`
- `docs/evidence/2586/R4-pc-gates.txt`
- `docs/evidence/2586/R5-build-lint.txt`
- `docs/evidence/2586/R6-docs.txt`
- `docs/evidence/2586/evidence.json`
- `docs/evidence/2586/pre-ready-2586.json`

### Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §5.6.1（导入）/ §5.6.2（导出）/ §5.6.3（导入模板）只写接口、参数与错误码，**未写所需 cus_perm**，错误响应未含 403/`40305` | 三段各补一行「**权限**：`instrument:create`」（导入/模板）/「**权限**：`instrument:read`」（导出），并在错误响应补 `{"code":40305,"message":"insufficient customer permission"}`（403）；导出段补注「可见性同列表 `applyInstrumentListScope`」 | 权限口径已落实到代码，接口文档不写即与实现漂移；审计以 api.md 为口径 |
| `docs/spec/permissions.md` | §3.2.1 只有一行「导出 / 批量导入 = `instrument:read` / `instrument:create`」，未列具体端点 | §3.2.1 追加「乐器数据进/出 端点-权限对照」7 行表（列表/导出/导入/导入模板/批量导入模板/批量导入预览/批量导入/批量导入媒体），并注明「`cus_perm = 0`（legacy JWT）沿用既有 pass-through；system_admin/platform_staff 由 #1964 放行」 | 本单结论（同类端点逐条裁定）必须落到权威文档，否则下次审计仍无法判断「哪些端点该有哪个守卫」 |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 导出端点补 `instrument:read` 守卫（无权限 → 403，有权限语义同 #2578） | `backend/handlers/instrument_perm_2586_test.go` | `GET /api/instruments/export` | fixture：租户 T + siteA/siteB + 分类 P(父)→C(子)、O；A(`CVZ-104`)/B(`CVZ-105`)@siteA、E(`CVZ-106`)/D(`CVZ-107`,lost)@siteB；四类 actor：无 read 位(`1<<5`)、有 read 位、`cus_perm=0`、`NAMESPACE_ADMIN` | backend（Postgres 测试库 127.0.0.1:15433） | `EXPORT_NO_READ_PERM_STATUS=403`、`EXPORT_NO_READ_PERM_CODE=40305`、`EXPORT_NO_READ_PERM_XLSX=0`、`EXPORT_WITH_READ_PERM_STATUS=200`、`EXPORT_WITH_READ_PERM_SNS=2`、`EXPORT_ZERO_CUSPERM_STATUS=200`、`EXPORT_SYSTEM_ADMIN_STATUS=200` | R1 |
| 导入族 6 端点补 `instrument:create` 守卫（无权限 → 403） | `backend/handlers/instrument_perm_2586_test.go` | `POST /api/instruments/import`、`GET /api/instruments/import/template`、`GET /api/instruments/batch-import/template`、`POST /api/instruments/batch-import`、`POST /api/instruments/batch-import/preview`、`POST /api/instruments/batch-import/media` | 无 create 位（`CusPerm=1<<1` read only）vs 有 create 位；请求体为空（守卫路径不触达文件解析） | backend（Postgres 测试库 127.0.0.1:15433） | `IMPORT_FAMILY_ENDPOINTS=6`、`IMPORT_FAMILY_403=6`、`IMPORT_FAMILY_40305=6`、`IMPORT_FAMILY_WITH_CREATE_NOT403=6` | R2 |
| 路由表接线（`main.go` 实际注册行含守卫）+ 同类端点无残留 | `backend/instrument_route_guard_2586_test.go` + `backend/main.go` | `go test . -run TestInstrumentPermRoutes2586`（结构断言，修前红/修后绿） | 修后工作树 + `git show <fix>^:backend/main.go` 红样对照 | backend | `ROUTE_EXPORT_GUARDED=1`、`ROUTE_IMPORT_FAMILY_GUARDED=6`、`ROUTE_INSTRUMENT_IO_UNGUARDED=0`、`RED_PRE_FIX_UNGUARDED_EXPORT=1`、`RED_PRE_FIX_UNGUARDED_IMPORT=6` | R3 |
| PC 入口门控（导入按钮 + 批量导入路由），只读角色不再看到不可用入口 | `frontend-pc/src/pages/admin/instrument/List.jsx` + `frontend-pc/src/App.jsx` | `/instruments/list`、`/instruments/batch-import` | 修后源码结构断言（evidence_type: static，Owner 豁免） | PC（浏览器） | `PC_LIST_IMPORT_BUTTON_GATED=1`、`PC_LIST_IMPORT_BUTTON_UNGATED=0`、`PC_BATCH_IMPORT_ROUTE_GATED=1`、`PC_STOCK_EXPORT_GATE=1`、`PC_STOCK_BATCH_IMPORT_GATE=1` | R4 |
| 构建 / lint / 回归（后端全包编译 + 定向测试 + PC 构建） | `backend` + `frontend-pc` | `go build ./...`、`go test ./... -run '^$'`、定向 `go test ./handlers`、`eslint`、`vite build` | 修后工作树 | 双端 | `GO_BUILD_OK=1`、`GO_COMPILE_SMOKE_OK=1`、`GO_TEST_2586_OK=1`、`ESLINT_NOUNDEF=0`、`PC_BUILD_OK=1` | R5 |
| 文档同步（接口权限口径 + 端点-权限对照表） | `docs/spec/api/api.md` + `docs/spec/permissions.md` | grep 结构断言（evidence_type: static，Owner 豁免） | 修后文档 | 文档 | `API_EXPORT_PERM=1`、`API_IMPORT_PERM=1`、`API_TEMPLATE_PERM=1`、**`PERM_ENDPOINT_ROWS=8`**、**`PERM_321_TABLE_INTACT=1`**、**逐端点 8 个 token**：`PERM_HAS_LIST=1`、`PERM_HAS_EXPORT=1`、`PERM_HAS_IMPORT=1`、`PERM_HAS_BATCH=1`、`PERM_HAS_BATCH_MEDIA=1`、`PERM_HAS_BATCH_PREVIEW=1`、`PERM_HAS_BATCH_TEMPLATE=1`、`PERM_HAS_IMPORT_TEMPLATE=1` | R6 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | integration | Go 定向测试 + Postgres 测试库真实数据，解析 xlsx 响应体与状态码 |
| R2 | integration | 同上，6 端点 × 有/无 create 位 |
| R3 | static | 路由注册行结构断言（Go test 读 `main.go`）+ `git show fix^` 红样；这是唯一覆盖「中间件是否挂在生产入口」的证据形态 |
| R4 | static | PC 无 DOM 测试运行器（`frontend-pc` 无 jest/vitest/jsdom，`node --test` 仅纯模块）；按 **Owner 豁免**（沿用 #2578 D7 / #2581 R4 / #2528 D9）以源码结构断言 + 构建为证据，真实点击归部署侧复核 |
| R5 | build | go build / 全包编译冒烟 / 定向测试 / eslint / vite build |
| R6 | static | 文档 grep（Owner 豁免同上） |

### Input Truth & Tool Preflight

- **Source of truth（目标口径）**：`docs/spec/permissions.md` §3.2.1「导出 / 批量导入 = `instrument:read` / `instrument:create`」；
  权限位定义 `backend/services/permission_registry.go:90-107`（`instrument:create`=bit 0、`instrument:read`=bit 1）。
- **现状差异核对（改前先跑）**：
  - `grep -n '"/instruments/export"\|"/instruments/import\|"/instruments/batch-import' backend/main.go` → 8 个端点中仅 `:403`（列表）带 `RequireCusPerm`；
  - `grep -rn 'RequireCusPerm' backend/middleware/permissions.go` → 语义：`cus_perm == 0` 直通（legacy JWT）、
    `isSystemLevelRole` 直通、`PermissionRegistry.GetCusPermBit` 返回 -1 直通（**测试必须切真 registry，否则永远 PASS**）；
  - `grep -rn "instruments/export\|batch-import\|instruments/import" frontend-pc/src frontend-mobile/src` →
    PC 3 个调用点（`InstrumentStock.jsx:265/463`、`List.jsx:458/480/623`、`BatchImport.jsx:11`）+ `api.js:413/423/429`；
    `/instruments/import`（legacy xlsx）**无前端调用方**，加守卫不影响既有 UI；
  - `grep -n 'batch-import' frontend-pc/src/App.jsx` → 路由 `:771` 现为裸 `ProtectedRoute`（无权限），需补门控。
- **损坏/异常输入拒绝**：`?fields=` 空 → 既有默认字段集（不变）；0 行导出 → 既有 `40006`（不变）；
  无 Authorization → 仍由 `IAMInterceptor` 401（守卫不改变 401 链路）；`cus_perm=0` 的 legacy 账号 → 直通（兼容语义保持，
  不因本单把老 token 用户全部阻断）。
- **工具盲区**：路由层中间件无法通过 handler 级测试覆盖（故 R3 用源码结构断言 + `git show fix^` 红样）；
  `frontend-pc` 无 DOM 测试运行器（故 R4 用结构断言 + Owner 豁免）；`go test ./...` 全量属 P5 baseline 差分，
  本单只做全包「编译冒烟」（`-run '^$'`），不把全量套件当合同 exit 0 行。
- **批次/单 Issue 边界**：#2586 无 `Parent`，按 #2651 口径执行「`go build ./...` + 定向测试 + 全包编译冒烟」，
  全量 `go test ./...` 由批次 gate 承担（不在本单合同内）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 导出守卫 = `instrument:read`；导入族守卫 = `instrument:create`；模板随导入 = `create` | 本计划锁定（= `docs/spec/permissions.md` §3.2.1 既有口径，`target_semantics`） |
| D2 | 守卫只加在路由层（`main.go`），handler 内不复制 `cus_perm` 判定 | 本计划锁定（单一 owner，避免双份口径漂移） |
| D3 | `POST /instruments/batch-import/media` 用 `create` 而非 `media_upload` | 本计划锁定：它是导入流程的创建子步骤；持有 `media_upload` 的角色（`role_templates.go:32-90`）均同时持有 `create`，不会削弱既有能力 |
| D4 | 「同类端点」边界 = 乐器数据**进/出**（导出、导入、导入模板、批量导入族）；乐器**只读子端点**另立 **#2587**；非乐器域导出另议 | 本计划锁定（只读子端点需逐端点核对调用方流程，误加会打断 H5/小程序流程） |
| D5 | `cus_perm = 0`（legacy JWT）保持直通、`system_admin`/`platform_staff` 保持放行 | 本计划锁定（`RequireCusPerm` 既有语义，不在本单收紧；收紧属独立安全议题） |
| D6 | PC 端同批补入口门控（`List.jsx` 导入按钮 + `/instruments/batch-import` 路由） | 本计划锁定：后端收紧后不放门控 = 「按钮可见但点进去全 403」死路（违反前端流程可达性红线） |
| D7 | PC/文档证据 = 源码结构断言 + 构建；真实点击归部署侧复核。**Owner 豁免**（UI static exemption，沿用 #2578 D7 / #2581 R4） | 本计划锁定（`frontend-pc` 无 DOM 测试运行器） |
| D8 | 重命令（`go build`/`go test`/`eslint`/`vite build`）一律经 `heavy-run.sh` 取锁执行 | 本计划锁定 |

### Out of Scope

- **#2587**：乐器只读子端点（`/instruments/filter-options`、`/instruments/check`、`/instruments/:id/photos/latest`、
  `/instruments/:id/media`、`/instruments/:id/activity-log`、`/instruments/:id/pricing`、`/instruments/:id/pricing-v2`）
  的 `instrument:read` 守卫 —— 本单不实现（需逐端点核对 H5/小程序/员工流程调用方）。
- 非乐器域导出端点（`GET /api/admin/payments/export`、`POST /api/admin/audit-logs/export`）的权限口径：
  属独立审计议题，不在本单；若判定为缺陷另立 Issue。
- `RequireCusPerm` 的 legacy 直通（`cus_perm = 0`）与 system 角色放行语义收紧（D5）。
- 导入 handler 内部行为（模板内容、属性解析、媒体匹配、性能日志）与 `instrument_import.go`/`instrument_batch_import.go` 改动。
- 移动端（H5 / 微信小程序）任何改动（无相关调用方）。
- 用户权限模板同步（`role_templates.go` 不变：既有权模板已含 `create`/`read`）。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 租户 T + siteA/siteB + P(父)→C(子)、O + A(`CVZ-104`,available,C,siteA)/B(`CVZ-105`,available,C,siteA)/E(`CVZ-106`,available,O,siteB)/D(`CVZ-107`,lost,O,siteB)；actor：无 read 位（`1<<5`）、有 read 位（`1<<1`）、`cus_perm=0`、`NAMESPACE_ADMIN` | `GET /api/instruments/export?fields=sn,category_name,status`（`TestExportPerm2586`，经 `verify-2586.sh r1`） | `docs/evidence/2586/R1-export-perm.txt` | `EXPORT_NO_READ_PERM_STATUS=403`、`EXPORT_NO_READ_PERM_CODE=40305`、`EXPORT_NO_READ_PERM_XLSX=0`、`EXPORT_WITH_READ_PERM_STATUS=200`、`EXPORT_WITH_READ_PERM_SNS=2`、`EXPORT_ZERO_CUSPERM_STATUS=200`、`EXPORT_SYSTEM_ADMIN_STATUS=200` | 无 read 位返回 200、403 body 含 xlsx、有 read 位 SN 集合 ≠ 2（#2578 语义回退）、legacy/system 被误拒 → 任一即 FAIL |
| R2 | actor：无 create 位（`1<<1`）、有 create 位（`1<<0`）；6 条导入族端点，空请求体 | `POST /api/instruments/import`、`GET /api/instruments/import/template`、`GET /api/instruments/batch-import/template`、`POST /api/instruments/batch-import`、`POST /api/instruments/batch-import/preview`、`POST /api/instruments/batch-import/media`（`TestImportFamilyPerm2586`，经 `verify-2586.sh r2`） | `docs/evidence/2586/R2-import-family-perm.txt` | `IMPORT_FAMILY_ENDPOINTS=6`、`IMPORT_FAMILY_403=6`、`IMPORT_FAMILY_40305=6`、`IMPORT_FAMILY_WITH_CREATE_NOT403=6` | 任一端点缺守卫（无 create 位仍 200/400）、或带 create 位被误拒 403 → FAIL |
| R3 | 修后 `backend/main.go` + `git show <fix>^:backend/main.go` 红样 | `go test . -run TestInstrumentPermRoutes2586`（经 `verify-2586.sh r3`） | `docs/evidence/2586/R3-route-wiring.txt` | `ROUTE_EXPORT_GUARDED=1`、`ROUTE_IMPORT_FAMILY_GUARDED=6`、`ROUTE_INSTRUMENT_IO_UNGUARDED=0`、`RED_PRE_FIX_UNGUARDED_EXPORT=1`、`RED_PRE_FIX_UNGUARDED_IMPORT=6` | 修后仍有乐器进/出路由无守卫、或修前红样为 0（说明红绿失效）→ FAIL |
| R4 | 修后 `List.jsx` / `App.jsx` / `InstrumentStock.jsx` 源码 | `bash scripts/verify-2586.sh r4` | `docs/evidence/2586/R4-pc-gates.txt` | `PC_LIST_IMPORT_BUTTON_GATED=1`、`PC_LIST_IMPORT_BUTTON_UNGATED=0`、`PC_BATCH_IMPORT_ROUTE_GATED=1`、`PC_STOCK_EXPORT_GATE=1`、`PC_STOCK_BATCH_IMPORT_GATE=1` | 导入按钮仍裸挂、路由仍无 `requiredPermission`、库存监控既有 gate 被删 → 任一即 FAIL |
| R5 | 修后工作树 | `bash scripts/verify-2586.sh r5` | `docs/evidence/2586/R5-build-lint.txt` | `GO_BUILD_OK=1`、`GO_COMPILE_SMOKE_OK=1`、`GO_TEST_2586_OK=1`、`ESLINT_NOUNDEF=0`、`PC_BUILD_OK=1` | 任一编译/lint `no-undef`/构建失败 → FAIL |
| R6 | 修后 `docs/spec/api/api.md` + `docs/spec/permissions.md` | `bash scripts/verify-2586.sh r6` | `docs/evidence/2586/R6-docs.txt` | `API_EXPORT_PERM=1`、`API_IMPORT_PERM=1`、`API_TEMPLATE_PERM=1`、**`PERM_ENDPOINT_ROWS=8`**、**`PERM_321_TABLE_INTACT=1`**、**逐端点 8 个 token**：`PERM_HAS_LIST=1`、`PERM_HAS_EXPORT=1`、`PERM_HAS_IMPORT=1`、`PERM_HAS_BATCH=1`、`PERM_HAS_BATCH_MEDIA=1`、`PERM_HAS_BATCH_PREVIEW=1`、`PERM_HAS_BATCH_TEMPLATE=1`、`PERM_HAS_IMPORT_TEMPLATE=1` | 文档缺任一端点/权限口径、对照表行数 ≠ 8 / 缺任一端点行 / 321 表被破坏 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2586.sh r1` | `docs/evidence/2586/R1-export-perm.txt` | exit 0 且含 `GO_TEST_EXIT=0` `EXPORT_NO_READ_PERM_STATUS=403` `EXPORT_NO_READ_PERM_CODE=40305` `EXPORT_NO_READ_PERM_XLSX=0` `EXPORT_WITH_READ_PERM_STATUS=200` `EXPORT_WITH_READ_PERM_SNS=2` `EXPORT_ZERO_CUSPERM_STATUS=200` `EXPORT_SYSTEM_ADMIN_STATUS=200` | PASS（8 项 token 全为期望值） | 无 read 位返回 200 xlsx、403 后仍写 xlsx、有 read 位行数 ≠ 2、legacy/system 被误拒 → FAIL | dev |
| R2 | `bash scripts/verify-2586.sh r2` | `docs/evidence/2586/R2-import-family-perm.txt` | exit 0 且含 `GO_TEST_EXIT=0` `IMPORT_FAMILY_ENDPOINTS=6` `IMPORT_FAMILY_403=6` `IMPORT_FAMILY_40305=6` `IMPORT_FAMILY_WITH_CREATE_NOT403=6` | PASS（5 项 token 全为期望值） | 任一端点无 create 位仍非 403、或有 create 位被误拒 → FAIL | dev |
| R3 | `bash scripts/verify-2586.sh r3` | `docs/evidence/2586/R3-route-wiring.txt` | exit 0 且含 `GO_TEST_EXIT=0` `ROUTE_EXPORT_GUARDED=1` `ROUTE_IMPORT_FAMILY_GUARDED=6` `ROUTE_INSTRUMENT_IO_UNGUARDED=0` `RED_PRE_FIX_UNGUARDED_EXPORT=1` `RED_PRE_FIX_UNGUARDED_IMPORT=6` | PASS（6 项 token 全为期望值） | 无守卫残留、红样为 0（红绿失效）→ FAIL | dev |
| R4 | `bash scripts/verify-2586.sh r4` | `docs/evidence/2586/R4-pc-gates.txt` | exit 0 且含 `PC_LIST_IMPORT_BUTTON_GATED=1` `PC_LIST_IMPORT_BUTTON_UNGATED=0` `PC_BATCH_IMPORT_ROUTE_GATED=1` `PC_STOCK_EXPORT_GATE=1` `PC_STOCK_BATCH_IMPORT_GATE=1` | PASS（5 项 token 全为期望值） | 按钮/路由未门控、既有 gate 被删 → FAIL | dev |
| R5 | `bash scripts/verify-2586.sh r5` | `docs/evidence/2586/R5-build-lint.txt` | exit 0 且含 `GO_BUILD_OK=1` `GO_COMPILE_SMOKE_OK=1` `GO_TEST_2586_OK=1` `ESLINT_NOUNDEF=0` `PC_BUILD_OK=1` | PASS（5 项 token 全为期望值） | 编译失败、eslint `no-undef` > 0、vite build 失败 → FAIL | dev |
| R6 | `bash scripts/verify-2586.sh r6` | `docs/evidence/2586/R6-docs.txt` | exit 0 且含 `API_EXPORT_PERM=1` `API_IMPORT_PERM=1` `API_TEMPLATE_PERM=1` `PERM_ENDPOINT_ROWS=8` | PASS（4 项 token 全为期望值） | 文档未同步、对照表行数 ≠ 8 或 `PERM_321_TABLE_INTACT=0` → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2586.sh`（`r1`–`r6`）；每个文本 artifact 首行：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，另起一行 `GENERATED_AT_HEAD=<40 位 sha>`；证据须在 fix commit **之后**生成。
- 逐条清单（全部位于 `docs/evidence/2586/`）：
  - `docs/evidence/2586/R1-export-perm.txt`
  - `docs/evidence/2586/R2-import-family-perm.txt`
  - `docs/evidence/2586/R3-route-wiring.txt`
  - `docs/evidence/2586/R4-pc-gates.txt`
  - `docs/evidence/2586/R5-build-lint.txt`
  - `docs/evidence/2586/R6-docs.txt`
- test DB：`POSTGRES_HOST=127.0.0.1` / `POSTGRES_PORT=15433`（一次性 PostgreSQL，#2341 口径）。
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2586 --summary-file tmp/pre-ready-2586-summary.md --changed-files tmp/pre-ready-2586-changed.txt --evidence-file docs/evidence/2586/evidence.json --report docs/evidence/2586/pre-ready-2586.json`；`evidence.json` 的 `build` 字段须匹配 evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Dependencies

无

### Workload

- Size: M
- Basis: 后端 1 个路由文件（8 个端点中间件）+ 2 个测试文件（1 结构断言 + 2 集成用例）+ PC 2 个文件门控（各 1-3 行）+ 2 份 core 文档 + 1 个 6 行证据 harness；无 migration、无外部依赖、6 条证据行
- Confidence: high

*Model: deepseek/deepseek-flash*


