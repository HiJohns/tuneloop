## Accepted Plan — #2587 乐器只读子端点补 `instrument:read` 守卫（+ `photos/upload` 补 `instrument:media_upload`）

> 来源：#2586 的同类路径扫描（导出/导入族收敛时发现的只读端点缺口），已由 Architect 立单。
> 现状：这些端点只挂 `ExplicitTenantScope`，**无 cus_perm 守卫**；PC/H5 页面靠页面级权限间接隐藏，
> 但直接调 API 仍可读取本租户/本网点数据（属性字典、识别码存在性、流程照片、媒体、操作日志、定价）。
> 权威口径：`docs/spec/permissions.md` §3.2.1「查看列表/详情 = `instrument:read`（含分类/属性/标签/库存/维修记录）」、
> §3.2.3「上传媒体 = `instrument:media_upload`」——本单是**补齐实现**，不改语义（`target_semantics`）。

### 变更点

#### 1. 逐端点结论（先核调用方，再加守卫）

| 端点（`backend/main.go`） | 现状守卫 | 调用方（仓库内） | 调用方角色是否有该位 | 结论 |
|---|---|---|---|---|
| `GET /instruments/filter-options`（`:405`） | 无 | PC `InstrumentStock.jsx:84`（页 `/site/stock` 需 read）、`List.jsx:59`（页 OR 门控，但 `GET /instruments` 本身已需 read） | 是（read 是这些页面的既有前提） | 补 `instrument:read` |
| `GET /instruments/check`（`:406`） | 无 | PC `Form.jsx:685`（新增乐器页）、H5 `ReceivingInterface.jsx:85`（收货）、`StaffInstrumentForm.jsx:84`（员工建乐器） | 是（staff 角色模板均含 read） | 补 `instrument:read` |
| `GET /instruments/:id/media`（`:461`） | 无 | PC `Detail.jsx:101/116/129`（乐器详情页需 read） | 是 | 补 `instrument:read` |
| `GET /instruments/:id/activity-log`（`:464`） | 无 | PC `Detail.jsx:874`、H5 `StaffInstrumentDetail.jsx:94` | 是 | 补 `instrument:read` |
| `GET /instruments/:id/pricing`（`:465`） | 无 | 仅 `api.js:408` 定义，`Detail.jsx` 已改用 pricing-v2 → 无活跃调用方 | —（无调用方） | 补 `instrument:read` |
| `GET /instruments/:id/pricing-v2`（`:588`） | 无 | PC `Detail.jsx:76`（`api.js:549`） | 是 | 补 `instrument:read` |
| `GET /instruments/:id/photos/latest`（`:456`） | 无 | 无前端调用方（仅 `instrument_leftovers_scope_2276_test.go`） | —（无调用方） | 补 `instrument:read` |
| `POST /instruments/:id/photos/upload`（`:455`） | 无（写端点） | 无前端调用方 | —（无调用方） | 补 `instrument:media_upload`（与 `POST /instruments/:id/media` 同族） |

**角色位核对（为什么不会打断既有流程）**：`backend/services/role_templates.go` 中所有非空位图模板
（`merchant_admin` / `site_admin` / `site_member` / `merchant_staff` / `repair_technician`）均含 `instrument:read`；
`namespace_admin` / `sys_admin` / `platform_staff` 由 `middleware.isSystemLevelRole`（#1964）全量放行；
`customer` 的 `CusPermCodes` 为空 → `cus_perm = 0` → `RequireCusPerm` 既有 legacy 直通（`permissions.go:174-177`）。
因此本单**不改变任何既有角色的可访问性**，只堵住「位图存在但缺 read / 缺 media_upload」的自定义权限情形。

#### 2. 后端路由（`backend/main.go`，8 行中间件，不动 handler）

- `:405` `/instruments/filter-options`、`:406` `/instruments/check`、`:456` `/instruments/:id/photos/latest`、
  `:461` `/instruments/:id/media`、`:464` `/instruments/:id/activity-log`、`:465` `/instruments/:id/pricing`、
  `:588` `/instruments/:id/pricing-v2` → 各补 `middleware.RequireCusPerm("instrument:read")`；
- `:455` `/instruments/:id/photos/upload` → 补 `middleware.RequireCusPerm("instrument:media_upload")`；
- 中间件顺序统一为 `ExplicitTenantScope()`（若有）→ `RequireCusPerm(...)` → handler；handler 内不复制权限判定。

#### 3. 测试（红绿 + 全端点覆盖）

- `backend/instrument_read_guard_2587_test.go`（新，`package main`）：读 `main.go` 源码的**结构断言** ——
  8 条路由行必须各含预期 `RequireCusPerm(...)`，「乐器只读/上传端点无守卫」计数 = 0。**修前必 FAIL（红），修后 PASS（绿）**
  （handler 级测试不会因 `main.go` 缺中间件而失败，故这是唯一能锁住路由接线的断言）。
- `backend/handlers/instrument_read_perm_2587_test.go`（新）：`testfixtures.SetupTestDB` + 复用
  `seedExportFixture2578` 的 instrument（A/B）与 actor；`middleware.PermissionRegistry` 临时切到
  `services.NewPermissionRegistry()`（默认 mock 恒返回 -1 会直通，测不出守卫）。
  - `TestInstrumentReadEndpointsPerm2587`：8 端点 × 无对应位（`CusPerm=1<<5`，不含 read bit 1 与 media_upload bit 15）
    → 8/8 `403` + `40305`；
  - 同用例 × 有对应位（read：`1<<1`；upload：`1<<15`）→ 8/8 「非 403 且非 5xx」（handler 正常进入：
    字典/媒体/日志/定价返回 200 或 404，`check` 返回 200，空表单上传返回 4xx 业务码）。

#### 4. 前端

无改动：8 个端点的所有仓库内调用方都在已按 `instrument:read` 门控的页面/流程内
（`/site/stock`、`/instruments/list`、`/instruments/detail`、H5 收货/员工建乐器/员工详情），
且 `instrument:media_upload` 只用于无调用方的 `photos/upload`。

#### 5. 证据 harness `scripts/verify-2587.sh r1..r4`

沿用 #2581/#2586 形态：首行 provenance（`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`），
重命令经 `heavy-run.sh` 取锁，测试库 `POSTGRES_HOST/PORT=127.0.0.1:15433`（一次性 PostgreSQL，#2341 口径）。

### Files（变更文件清单）

- `backend/main.go`
- `backend/instrument_read_guard_2587_test.go`
- `backend/handlers/instrument_read_perm_2587_test.go`
- `scripts/verify-2587.sh`
- `docs/evidence/2587/plan.md`
- `docs/evidence/2587/R1-read-endpoints-403.txt`
- `docs/evidence/2587/R2-authorized-passthrough.txt`
- `docs/evidence/2587/R3-route-wiring.txt`
- `docs/evidence/2587/R4-build-lint.txt`
- `docs/evidence/2587/evidence.json`
- `docs/evidence/2587/pre-ready-2587.json`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 8 个端点补齐守卫（7 读 + 1 上传）——无对应位一律 403 | `backend/handlers/instrument_read_perm_2587_test.go` | `GET /api/instruments/filter-options`、`GET /api/instruments/check`、`GET /api/instruments/:id/photos/latest`、`GET /api/instruments/:id/media`、`GET /api/instruments/:id/activity-log`、`GET /api/instruments/:id/pricing`、`GET /api/instruments/:id/pricing-v2`、`POST /api/instruments/:id/photos/upload` | fixture：租户 T + siteA；乐器 A(`CVZ-104`)、B(`CVZ-105`)；actor 无 read/media_upload 位（`CusPerm=1<<5`） | backend（Postgres 测试库 127.0.0.1:15433） | `READ_ENDPOINTS=8`、`READ_ENDPOINTS_403=8`、`READ_ENDPOINTS_40305=8` | R1 |
| 有对应位的合法调用方不被误拒（不改变既有角色可访问性） | `backend/handlers/instrument_read_perm_2587_test.go` | 同上 8 端点（upload 用空表单） | actor 有 read 位（`1<<1`）与 media_upload 位（`1<<15`） | backend（Postgres 测试库 127.0.0.1:15433） | `AUTHORIZED_ENDPOINTS=8`、`AUTHORIZED_NOT_403=8`、`AUTHORIZED_NOT_5XX=8` | R2 |
| 路由表接线（`main.go` 实际注册行含守卫）+ 无残留 | `backend/instrument_read_guard_2587_test.go` + `backend/main.go` | `go test . -run TestInstrumentReadRoutes2587`（结构断言，修前红/修后绿） | 修后工作树 + `git show <fix>^:backend/main.go` 红样对照 | backend | `ROUTE_READ_GUARDED=7`、`ROUTE_UPLOAD_GUARDED=1`、`ROUTE_INSTRUMENT_READ_UNGUARDED=0`、`RED_PRE_FIX_UNGUARDED=8` | R3 |
| 构建 / lint / 回归 | `backend` | `go build ./...`、`go test ./... -run '^$'`、定向 `go test ./handlers` | 修后工作树 | backend | `GO_BUILD_OK=1`、`GO_COMPILE_SMOKE_OK=1`、`GO_TEST_2587_OK=1` | R4 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | integration | Go 定向测试 + Postgres 测试库，8 端点 × 无权限 → 403/`40305` |
| R2 | integration | 同上，8 端点 × 有权限 → 非 403/非 5xx（合法调用方不被误拒） |
| R3 | static | 路由注册行结构断言（Go test 读 `main.go`）+ `git show fix^` 红样 |
| R4 | build | go build / 全包编译冒烟 / 定向测试 |

### Input Truth & Tool Preflight

- **Source of truth**：`docs/spec/permissions.md` §3.2.1（`instrument:read` 含列表/详情/分类/属性/库存/维修记录）、
  §3.2.3（`instrument:media_upload` = 上传图片/视频）；权限位 `backend/services/permission_registry.go:90-107`
  （`instrument:read`=bit 1、`instrument:media_upload`=bit 15）。
- **改前差异核对（先跑再改）**：
  - `grep -n '"/instruments/filter-options"\|"/instruments/check"\|"/instruments/:id/media"\|"/instruments/:id/activity-log"\|"/instruments/:id/pricing"\|"/instruments/:id/pricing-v2"\|"/instruments/:id/photos' backend/main.go`
    → 8 行现状均**无** `RequireCusPerm`；
  - `grep -rn 'photos/latest\|photos/upload' frontend-pc/src frontend-mobile/src` → 0 调用方（守卫无 UI 影响）；
  - `grep -rn 'filter-options\|instruments/check\|activity-log\|/pricing-v2' frontend-pc/src frontend-mobile/src`
    → 调用方页面/流程清单（见变更点 §1 表）；
  - `sed -n '20,95p' backend/services/role_templates.go` → 非空位图模板均含 `instrument:read`；`customer` 无位图。
- **损坏/异常输入拒绝**：无 Authorization → 仍 `IAMInterceptor` 401（链路不变）；`cus_perm = 0` → 直通（D3）；
  空闲表单上传 → handler 既有 4xx 业务码（不新增静默吞错）。
- **工具盲区**：路由中间件无法用 handler 级测试覆盖 → R3 用源码结构断言 + `git show fix^` 红样；
  `go test ./...` 全量属 P5 baseline 差分，本单只做全包**编译冒烟**（`-run '^$'`）。
- **批次/单 Issue 边界**：#2587 无 `Parent`，按 #2651 口径执行「`go build ./...` + 定向测试 + 全包编译冒烟」。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 7 个只读端点 = `instrument:read`；`photos/upload` = `instrument:media_upload` | 本计划锁定（= `docs/spec/permissions.md` 既有口径，`target_semantics`） |
| D2 | 守卫只加路由层（`main.go`），handler 内不复制判定 | 本计划锁定（单一 owner） |
| D3 | `cus_perm = 0`（legacy）直通、system/platform 角色放行保持现状 | 本计划锁定（`RequireCusPerm` 既有语义；收紧属独立安全议题，不在本单） |
| D4 | 不因本次收紧改动任何前端门控 | 本计划锁定：8 端点的仓库内调用方全部位于既有 read/media 门控页面或流程内，无「可见但 403」新入口 |
| D5 | 与 #2586 的边界：本单只做只读/上传子端点，不触碰导出/导入族 | 本计划锁定（#2586 已 accepted，改 `backend/main.go` 同一文件但**不同行**；`docs` 无交叉修改，可并行合并） |
| D6 | 重命令一律经 `heavy-run.sh` 取锁执行 | 本计划锁定 |

### Out of Scope

- 导出/导入族端点（`/instruments/export`、`/instruments/import`、批量导入族）→ 已由 **#2586** 处理。
- `cus_perm = 0` legacy 直通与 system 角色放行的语义收紧（D3）。
- 非乐器域端点（订单/支付/审计日志/人员）的权限口径审计。
- 移动端（H5/小程序）与 PC 的前端改动（无新入口需要门控，D4）。
- `instruments/levels`（静态枚举，无租户数据）、`instruments/filter-options` 的字典内容裁剪（数据范围口径，非权限位）。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 租户 T + siteA；乐器 A(`CVZ-104`)/B(`CVZ-105`)；actor `CusPerm=1<<5`（仅 maintain） | 8 端点直调（`TestInstrumentReadEndpointsPerm2587`，经 `verify-2587.sh r1`） | `docs/evidence/2587/R1-read-endpoints-403.txt` | `READ_ENDPOINTS=8`、`READ_ENDPOINTS_403=8`、`READ_ENDPOINTS_40305=8` | 任一端点返回 200 数据、或 403 但 code ≠ 40305 → FAIL |
| R2 | 同上 fixture；actor `CusPerm=1<<1`（read）+`1<<15`（media_upload） | 同上 8 端点（upload 空表单）（`verify-2587.sh r2`） | `docs/evidence/2587/R2-authorized-passthrough.txt` | `AUTHORIZED_ENDPOINTS=8`、`AUTHORIZED_NOT_403=8`、`AUTHORIZED_NOT_5XX=8` | 合法角色被误拒 403、或 handler 500（说明守卫接线顺序错误/token 上下文丢失）→ FAIL |
| R3 | 修后 `backend/main.go` + `git show <fix>^:backend/main.go` 红样 | `go test . -run TestInstrumentReadRoutes2587`（经 `verify-2587.sh r3`） | `docs/evidence/2587/R3-route-wiring.txt` | `ROUTE_READ_GUARDED=7`、`ROUTE_UPLOAD_GUARDED=1`、`ROUTE_INSTRUMENT_READ_UNGUARDED=0`、`RED_PRE_FIX_UNGUARDED=8` | 修后仍有残留无守卫端点、或修前红样 ≠ 8（红绿失效）→ FAIL |
| R4 | 修后工作树 | `bash scripts/verify-2587.sh r4` | `docs/evidence/2587/R4-build-lint.txt` | `GO_BUILD_OK=1`、`GO_COMPILE_SMOKE_OK=1`、`GO_TEST_2587_OK=1` | 任一编译/定向测试失败 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2587.sh r1` | `docs/evidence/2587/R1-read-endpoints-403.txt` | exit 0 且含 `GO_TEST_EXIT=0` `READ_ENDPOINTS=8` `READ_ENDPOINTS_403=8` `READ_ENDPOINTS_40305=8` | PASS（4 项 token 全为期望值） | 任一端点无守卫返回数据、或 403 code ≠ 40305 → FAIL | dev |
| R2 | `bash scripts/verify-2587.sh r2` | `docs/evidence/2587/R2-authorized-passthrough.txt` | exit 0 且含 `GO_TEST_EXIT=0` `AUTHORIZED_ENDPOINTS=8` `AUTHORIZED_NOT_403=8` `AUTHORIZED_NOT_5XX=8` | PASS（4 项 token 全为期望值） | 合法角色被 403、或出现 5xx → FAIL | dev |
| R3 | `bash scripts/verify-2587.sh r3` | `docs/evidence/2587/R3-route-wiring.txt` | exit 0 且含 `GO_TEST_EXIT=0` `ROUTE_READ_GUARDED=7` `ROUTE_UPLOAD_GUARDED=1` `ROUTE_INSTRUMENT_READ_UNGUARDED=0` `RED_PRE_FIX_UNGUARDED=8` | PASS（5 项 token 全为期望值） | 无守卫残留、红样 ≠ 8 → FAIL | dev |
| R4 | `bash scripts/verify-2587.sh r4` | `docs/evidence/2587/R4-build-lint.txt` | exit 0 且含 `GO_BUILD_OK=1` `GO_COMPILE_SMOKE_OK=1` `GO_TEST_2587_OK=1` | PASS（3 项 token 全为期望值） | 编译失败、定向测试失败 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2587.sh`（`r1`–`r4`）；每个文本 artifact 首行：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，另起一行 `GENERATED_AT_HEAD=<40 位 sha>`；证据须在 fix commit **之后**生成。
- 逐条清单（全部位于 `docs/evidence/2587/`）：
  - `docs/evidence/2587/R1-read-endpoints-403.txt`
  - `docs/evidence/2587/R2-authorized-passthrough.txt`
  - `docs/evidence/2587/R3-route-wiring.txt`
  - `docs/evidence/2587/R4-build-lint.txt`
- test DB：`POSTGRES_HOST=127.0.0.1` / `POSTGRES_PORT=15433`（一次性 PostgreSQL，#2341 口径）。
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2587 --summary-file tmp/pre-ready-2587-summary.md --changed-files tmp/pre-ready-2587-changed.txt --evidence-file docs/evidence/2587/evidence.json --report docs/evidence/2587/pre-ready-2587.json`；`evidence.json` 的 `build` 字段须匹配 evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Dependencies

无

### Workload

- Size: S
- Basis: 后端 1 个路由文件（8 行中间件）+ 2 个测试文件（1 结构断言 + 2 集成用例覆盖 8 端点）+ 1 个 4 行证据 harness；无前端改动、无 core 文档改动、无 migration
- Confidence: high

*Model: deepseek/deepseek-flash*

