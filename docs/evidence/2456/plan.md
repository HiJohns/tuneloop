## Accepted Plan — #2456（PC 后台静置 1h 退出；小程序/H5 不受影响）

**分阶段 gate**：Phase A（R1–R7 后端：迁移/会话登记/续期空闲校验/活动上报/配置）必须先全绿，才允许进入 Phase B（R8–R10 PC 前端接线与构建）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 迁移 + 模型 + 启动校验登记 | `backend/database/migrations/20261123001_pc_session_idle_2456.up.sql`、`backend/models/models.go`、`backend/database/db.go` | 启动 `validateDatabaseSchema` | 迁移 up/down 成对；`pc_sessions` 入 `modelsToValidate` 与 `defaultMigrationArtifacts` | 后端 | 定向测试断言：up/down 文件同时存在；artifact 清单含 `20261123001 → pc_sessions`；`ModelsToValidateForTest()` 覆盖 `PCSession` | R1 |
| R2 PC 会话登记（仅 PC 入口） | `backend/handlers/auth.go`、`backend/services/pc_idle.go` | `/auth/callback`（无 `client_type`）、`/auth/login`、`/auth/callback?client_type=wx` | 同一账号分别走 PC 回调 / 账密登录 / H5 回调（`client_type=wx`） | 后端 | PC 两条入口各写入 1 行 `pc_sessions`（`iam_sub` + `refresh_hash`）；H5 路径 0 行 | R2 |
| R3 静置超 1h 拒绝续期 | `backend/handlers/auth.go` → `Refresh` | `POST /auth/refresh` | `pc_sessions.last_activity_at = now()-2h`，refresh hash 命中 | 后端 | HTTP 401 + code `40110` + message `登录已超时，请重新登录`；fake IAM 调用次数=0（不向 IAM 续期） | R3 |
| R4 1h 内续期保持登录 | `backend/handlers/auth.go` → `Refresh` | `POST /auth/refresh` | `last_activity_at = now()-10m` | 后端 | HTTP 200 + code `20000`；IAM 调用 1 次；`last_activity_at` 前移（`TOUCHED=1`） | R4 |
| R5 非 PC 会话不受影响（weapp/H5） | `backend/handlers/auth.go` → `Refresh` | `POST /auth/refresh` | refresh hash 无 `pc_sessions` 行（H5/小程序令牌） | 后端 | HTTP 200 + code `20000`；`IDLE_ENFORCED=0`；IAM 调用 1 次 | R5 |
| R6 活动上报接口 | `backend/handlers/auth.go` → `PCActivity`、`backend/main.go` | `POST /auth/pc-activity`（`authRequired`） | 已登录 sub 有 pc 会话且 `last_activity_at=now()-30m` | 后端 | HTTP 200；该 sub 行 `last_activity_at ≥ 请求时刻-5s`；他人 sub 行不变；无 token → 401 | R6 |
| R7 空闲阈值可配置 | `backend/services/pc_idle.go` | 环境变量 `PC_IDLE_TIMEOUT_MINUTES` | `=1`（静置 61 分钟）、`=0`（禁用）、非法值 | 后端 | `=1` → 过期被拒；`=0` → 放行（`IDLE_ENFORCED=0`）；非法/负数 → 回落 60；单位与上限（0–1440）被断言 | R7 |
| R8 前端活动/超时纯函数 | `frontend-pc/src/utils/pcSession.js`、`frontend-pc/tests/pcSession.test.mjs` | `node --test` | 节流窗口 60s、`code=40110`、其它 code | PC | `shouldSendActivity` 59s=false / 60s=true；`isIdleTimeoutPayload({code:40110})=true`、`{code:40105}=false`、`null=false` | R8 |
| R9 PC 接线（活动上报 + 超时跳转 + 文案） | `frontend-pc/src/services/api.js`、`frontend-pc/src/App.jsx`、`frontend-pc/src/pages/LogoutPage.jsx` | PC 前端运行时 | 续期返回 40110；用户交互 60s 节流 | PC | 源码结构断言：`api.js` 对 `40110` 走 `clearTokens(` + `logout_reason`=`idle_timeout`；`App.jsx` 调用 `startActivityTracking(`；`LogoutPage.jsx` 含 `idle_timeout` 文案 | R9 |
| R10 PC 构建 + lint | `frontend-pc` | `npm run build`、`npm run lint` | 修后树 | PC | build exit 0；lint exit 0（no-undef 0） | R10 |

## Input Truth & Tool Preflight

- **Source of truth（现状事实，均已核对源码）**：
  - `/auth/refresh` 目前直接转发 IAM 续期，无任何空闲判定：`backend/handlers/auth.go:1006-1037`。
  - IAM 刷新**只返回新 access_token（900s），不轮换 refresh_token**：`beaconiam/internal/auth/handler.go:974-997`；因此 refresh token 的 sha256 可作为稳定会话键。
  - `expires_in` 契约：`setAuthCookies` 用 `expires_in`（<=0 回退 900s），`backend/handlers/auth.go:225-250`。
  - PC 与 H5 共用 `/auth/callback`：H5 显式传 `client_type: 'wx'`（`frontend-mobile/src/App-H5.jsx:234-240`）；PC 的 `AuthCallback` 不传（`frontend-pc/src/pages/AuthCallback/index.tsx:38`）。小程序走 `/auth/wx-login`（`backend/main.go:237`）。**PC 判定必须以 `client_type` 为依据，不能以端点为依据。**
  - PC 存在 60s 后台轮询（`frontend-pc/src/components/NotificationCenter.jsx:59`）与 30s 过期检查（`frontend-pc/src/App.jsx:181`）→ “任意请求即活动”会让 idle 永不触发。
- **差异核对**：Issue 原文写“正常请求刷新 last_activity_at”，与本仓 60s 轮询事实冲突 → 本计划改为**显式用户交互活动上报**（见 Decision Lock D3），并保留后端节流。
- **损坏输入拒绝**：`refresh_token` 为空 → 40002（既有）；`pc_sessions` 无行 → 视为非 PC（不得误拒 H5/小程序）；活动上报缺 `sub` → 401。
- **工具盲区**：`frontend-pc` 无测试运行器（`package.json` 只有 dev/build/lint/preview），故 PC 运行时行为用**纯函数 node 测试（R8）+ 源码结构断言（R9）**双轨；R9 已按 `### Evidence Types` 声明 `static` 并申请 Owner 豁免。e2e 真机/浏览器走查不在本单（部署侧）。
- **运行环境**：`handlers` 包 `TestMain`（`backend/handlers/main_test.go:135`）+ 真实测试库 `tuneloop_test`；fake IAM 用 `httptest` + `services.SetIAMInternalURLForTesting`（沿用 `backend/handlers/auth_test.go:81-87` 模式）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 会话事实源：tuneloop 自建 `pc_sessions`（id/sub/refresh_hash/last_activity_at/revoked_at），不依赖 IAM 会话表 | Architect | 本计划锁定 | 阻塞实现 |
| D2 | PC 判定：`Callback` 的 `client_type ∉ {wx,wechat,mobile}` 或 `PostLogin` → PC；`wx-login`/`wx-login-select`/H5 callback → 非 PC | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 活动信号 = **用户交互显式上报**（`POST /auth/pc-activity`，前端 60s 节流 + 服务端 `last_activity_at < now()-60s` 才写）。任意请求/后台轮询不计活动（校准 Issue 原文，理由见 Input Truth） | Architect | 本计划锁定 | 若 Owner 要求“任意请求即活动”→ 需同时关闭 60s 轮询，回 Architect |
| D4 | 阈值配置：`PC_IDLE_TIMEOUT_MINUTES` 默认 60，`0` 表示禁用，越界/非法回落 60；不引入 DB 配置页（会话策略属部署配置） | Architect / ops | 本计划锁定 | 若要求后台可配 → 另立 Issue |
| D5 | 过期语义：仅 PC 会话过期；H5/小程序令牌保持 7 天现有语义不变 | Architect | 本计划锁定 | 阻塞实现 |
| D6 | 既有 PC 令牌（部署前签发、无 `pc_sessions` 行）不做追溯拒绝，下次登录起生效；不新增后台白名单 | Architect | 本计划锁定 | 若要求“立即全员强制”→ ops 另行发布说明 |
| D7 | R9 为纯前端接线，`frontend-pc` 无测试运行器 → 采用 `evidence_type: static` 源码结构断言，**Owner 豁免**（Architect 授予），不作 render 证据要求 | Architect | 本计划锁定 | 若审计要求 render → 需另立 `frontend-pc` 测试基建 Issue |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R8 | render | 纯函数 node 测试（`node --test` 断言行为，无 DOM 依赖） |
| R9 | static | 源码结构断言（`grep` 命中 + 构建），Owner 豁免见 D7；`frontend-pc` 无测试运行器 |

## Out of Scope

- IAM 侧会话表 / Redis idle 状态（本单在 tuneloop 本地落库）。
- refresh token 轮换与主动吊销、设备会话管理页（查看/踢出）。
- 小程序/H5 的 idle 策略调整（明确不套用，见 D5）。
- PC 前端导航/布局改造；本单只加活动上报、超时跳转与提示文案。
- 多节点并发写 `pc_sessions` 的分布式锁（单库条件更新即可满足语义）。
- 全量 `go test ./...`（由批次 gate 承担）。

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|---|---|---|---|
| R1 | 迁移文件对 `20261123001_pc_session_idle_2456.{up,down}.sql`；`db.go` 登记项 | `ModelsToValidateForTest()` / artifact 清单读取 | `docs/evidence/2456/R1-migration.txt` |
| R2 | `users` 行 sub=`22bfb6d9-…`；fake IAM 返回 `access_token`+`refresh_token`；三条入口参数 | `Callback`（pc / wx）与 `PostLogin` | `docs/evidence/2456/R2-pc-register.txt` |
| R3 | `pc_sessions(last_activity_at=now()-2h, refresh_hash=sha256("rt-idle"))` | `POST /auth/refresh` body `{"refresh_token":"rt-idle"}` | `docs/evidence/2456/R3-idle-reject.txt` |
| R4 | `pc_sessions(last_activity_at=now()-10m, refresh_hash=sha256("rt-active"))` | `POST /auth/refresh` | `docs/evidence/2456/R4-idle-allow.txt` |
| R5 | 无 `pc_sessions` 行；refresh_token=`rt-weapp`（模拟小程序/H5） | `POST /auth/refresh` | `docs/evidence/2456/R5-nonpc-unaffected.txt` |
| R6 | 两行 pc_sessions：本 sub（`now()-30m`）、他人 sub（`now()-30m`） | `POST /auth/pc-activity`（带本 sub JWT） | `docs/evidence/2456/R6-activity.txt` |
| R7 | `PC_IDLE_TIMEOUT_MINUTES=1`（last_activity=now()-61m）、`=0`（同前）、`=abc` | `services.PCSessionIdleTimeout()` + `Refresh` | `docs/evidence/2456/R7-config.txt` |
| R8 | 节流用例 `now-last=59000/60000`；payload `{code:40110}`/`{code:40105}`/`null` | `frontend-pc/src/utils/pcSession.js` | `docs/evidence/2456/R8-frontend-unit.txt` |
| R9 | 修后 `frontend-pc/src` 三文件源码 | `api.js` / `App.jsx` / `LogoutPage.jsx` 结构 | `docs/evidence/2456/R9-wiring.txt` |
| R10 | 修后 `frontend-pc`（`node_modules` 已安装） | `npm run build` + `npm run lint` | `docs/evidence/2456/R10-build.txt` |

### Assertions

- R1：`MIGRATION_PAIR=ok`、`ARTIFACT_REGISTERED=pc_sessions`、`MODEL_IN_VALIDATE=yes`；负例：up 有 down 无 → FAIL。
- R2：`PC_CALLBACK_ROWS=1`、`PC_LOGIN_ROWS=1`、`H5_CALLBACK_ROWS=0`；负例：`H5_CALLBACK_ROWS=1`（误把 H5 当 PC）→ FAIL。
- R3：`HTTP=401`、`CODE=40110`、`IAM_CALLS=0`；负例：`HTTP=200` 或 `IAM_CALLS=1` → FAIL。
- R4：`HTTP=200`、`CODE=20000`、`IAM_CALLS=1`、`TOUCHED=1`；负例：`TOUCHED=0` → FAIL。
- R5：`HTTP=200`、`CODE=20000`、`IDLE_ENFORCED=0`、`IAM_CALLS=1`；负例：`CODE=40110`（误拒小程序/H5）→ FAIL。
- R6：`HTTP=200`、`OWNER_TOUCHED=1`、`OTHER_TOUCHED=0`、`NO_TOKEN_HTTP=401`；负例：`OTHER_TOUCHED=1` → FAIL。
- R7：`MINUTES_1_REJECT=1`、`ZERO_DISABLED=1`、`INVALID_FALLBACK=60`、`CLAMP_MAX=1440`；负例：`=0` 仍拒绝 → FAIL。
- R8：`node --test` exit 0，`--- pass` 覆盖 5 条断言；负例：节流 59s 却发送 → FAIL。
- R9：静态结构断言命中 `40110`+`clearTokens(`+`idle_timeout`+`startActivityTracking(`+`pc-activity`；负例：`api.js` 无 `40110` → FAIL。
- R10：`BUILD_EXIT=0`、`LINT_EXIT=0`、`NO_UNDEF=0`；负例：任一非 0 → FAIL。
- 通用 provenance：每个 artifact 首行 `# build=1.0.<fix>` `# commit=<sha>` `# dirty=false`。
- 负例（跨行）：任何 artifact 出现 `idle enforced for weapp`/`H5 blocked`/`任意请求即活动` 语义 → FAIL。

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2456/R1-migration.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2456/R2-pc-register.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2456/R3-idle-reject.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2456/R4-idle-allow.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2456/R5-nonpc-unaffected.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2456/R6-activity.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2456/R7-config.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2456/R8-frontend-unit.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2456/R9-wiring.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2456/R10-build.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2456.sh r1` | `docs/evidence/2456/R1-migration.txt` | exit 0；grep `MIGRATION_PAIR=ok`、`ARTIFACT_REGISTERED=pc_sessions`、`MODEL_IN_VALIDATE=yes` | PASS | 缺 down 文件或未登记 artifact → FAIL | dev |
| R2 | `bash scripts/verify-2456.sh r2` | `docs/evidence/2456/R2-pc-register.txt` | exit 0；grep `PC_CALLBACK_ROWS=1`、`PC_LOGIN_ROWS=1`、`H5_CALLBACK_ROWS=0` | PASS | `H5_CALLBACK_ROWS=1` → FAIL | dev |
| R3 | `bash scripts/verify-2456.sh r3` | `docs/evidence/2456/R3-idle-reject.txt` | exit 0；grep `CODE=40110`、`IAM_CALLS=0`、`HTTP=401` | PASS | `HTTP=200` 或 `IAM_CALLS=1` → FAIL | dev |
| R4 | `bash scripts/verify-2456.sh r4` | `docs/evidence/2456/R4-idle-allow.txt` | exit 0；grep `CODE=20000`、`IAM_CALLS=1`、`TOUCHED=1` | PASS | `TOUCHED=0` → FAIL | dev |
| R5 | `bash scripts/verify-2456.sh r5` | `docs/evidence/2456/R5-nonpc-unaffected.txt` | exit 0；grep `CODE=20000`、`IDLE_ENFORCED=0` | PASS | `CODE=40110` → FAIL | dev |
| R6 | `bash scripts/verify-2456.sh r6` | `docs/evidence/2456/R6-activity.txt` | exit 0；grep `OWNER_TOUCHED=1`、`OTHER_TOUCHED=0`、`NO_TOKEN_HTTP=401` | PASS | `OTHER_TOUCHED=1` → FAIL | dev |
| R7 | `bash scripts/verify-2456.sh r7` | `docs/evidence/2456/R7-config.txt` | exit 0；grep `MINUTES_1_REJECT=1`、`ZERO_DISABLED=1`、`INVALID_FALLBACK=60` | PASS | `ZERO_DISABLED=0` → FAIL | dev |
| R8 | `bash scripts/verify-2456.sh r8` | `docs/evidence/2456/R8-frontend-unit.txt` | exit 0；grep `# pass 5`、`# fail 0` | PASS | 任一 fail 或断言数 <5 → FAIL | dev |
| R9 | `bash scripts/verify-2456.sh r9` | `docs/evidence/2456/R9-wiring.txt` | exit 0；grep `WIRING_40110=1`、`WIRING_ACTIVITY=1`、`WIRING_REASON=1` | PASS | 任一 `=0` → FAIL | dev |
| R10 | `bash scripts/verify-2456.sh r10` | `docs/evidence/2456/R10-build.txt` | exit 0；grep `BUILD_EXIT=0`、`LINT_EXIT=0` | PASS | 任一非 0 → FAIL | dev |

### Workload

- Size: L
- Basis: 1 新表迁移(2 文件) + 模型/db.go 登记(2) + 服务(1) + handler/route(2) + 3 个后端测试 + PC 前端 3 文件 + 1 前端测试 + 2 文档 + verify 脚本；10 条证据行；分两阶段 gate（后端语义 → 前端接线）
- Confidence: medium（`frontend-pc` 无测试运行器，前端证据为纯函数 + 静态结构；真机走查属部署侧）

### Files（变更文件清单）

- `backend/database/migrations/20261123001_pc_session_idle_2456.up.sql`
- `backend/database/migrations/20261123001_pc_session_idle_2456.down.sql`
- `backend/models/models.go`
- `backend/database/db.go`
- `backend/services/pc_idle.go`
- `backend/handlers/auth.go`
- `backend/main.go`
- `backend/handlers/pc_idle_2456_test.go`
- `backend/services/pc_idle_2456_test.go`
- `frontend-pc/src/utils/pcSession.js`
- `frontend-pc/src/services/api.js`
- `frontend-pc/src/App.jsx`
- `frontend-pc/src/pages/LogoutPage.jsx`
- `frontend-pc/tests/pcSession.test.mjs`
- `docs/spec/api/api.md`
- `docs/ops/account-lifecycle.md`
- `scripts/verify-2456.sh`
- `docs/evidence/2456/plan.md`
- `docs/evidence/2456/R1-migration.txt`
- `docs/evidence/2456/R2-pc-register.txt`
- `docs/evidence/2456/R3-idle-reject.txt`
- `docs/evidence/2456/R4-idle-allow.txt`
- `docs/evidence/2456/R5-nonpc-unaffected.txt`
- `docs/evidence/2456/R6-activity.txt`
- `docs/evidence/2456/R7-config.txt`
- `docs/evidence/2456/R8-frontend-unit.txt`
- `docs/evidence/2456/R9-wiring.txt`
- `docs/evidence/2456/R10-build.txt`
- `docs/evidence/2456/evidence.json`

## Dependencies

- 无前置 Issue。
- 关联（不阻塞）：#2454（PC 选网点后空组织 token）——登录态缺陷独立修复；本单不改 IAM/OAuth 交互。
- 迁移纪律：新增表必须同批登记 `defaultMigrationArtifacts` 与 `modelsToValidate`（AGENTS.md），R1 已覆盖。

*Model: DeepSeek V4.1 Flash 1017*

