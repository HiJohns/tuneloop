## Accepted Plan — #2454（PC 选网点后空组织 token：IAM code 绑定组织 + tuneloop 显式拒绝）

**跨仓说明**：根因在 beaconiam（授权码无组织上下文）。本单同时交付 **beaconiam 侧修复**（Issue `HiJohns/beaconiam#500`）与 **tuneloop 侧显式拒绝 + 可诊断文案**。
**分阶段 gate**：Phase A（R1–R3 beaconiam：code 绑定组织 / legacy 显式报错 / select-org 端点）全绿后，才允许进入 Phase B（R4–R7 tuneloop）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 授权码绑定组织后签发正确上下文 | `beaconiam/internal/models/authorization_code.go`、`beaconiam/internal/auth/handler.go` | `POST /api/v1/auth/token`（authorization_code grant） | 多组织账号：code 已绑定 `org=76d46658…`（海淀店） | IAM | 返回 access_token claims `oid=76d46658…`、`tid=bfce6c6d…`、`role=STAFF`、`fn_roles` 非空；refresh_token 同步带上下文 | R1 |
| R2 未绑定组织的多组织 code 显式报错 | `beaconiam/internal/auth/handler.go` → `handleAuthorizationCodeGrant` | `POST /api/v1/auth/token` | 多组织账号 + legacy code（`OrgID=nil`） | IAM | HTTP 400 + `error=org_not_selected`；**不得**返回 200 + 空上下文 token | R2 |
| R3 select-org 绑定并返回可用 code | `beaconiam/internal/auth/handler.go` → `HandleSelectOrg`、`beaconiam/ui/src/pages/SelectOrg.tsx` | `POST /api/v1/auth/select-org` | 请求 `{org_id, code}`（有效 code）；再请求已使用/不存在 code | IAM | 首次：200 + `access_token` + `code`（该 code 可交换出带该组织上下文的 token）；已使用 code：响应中给出新的有效 code 或 400 明确错误（不静默复用） | R3 |
| R4 PC 回调拒绝空组织 token | `backend/handlers/auth.go` → `Callback` | `GET/POST /api/auth/callback`（PC：`client_type` 缺失） | 交换所得 token claims 为 `oid=""`+`tid=""`+`fn_roles=null`+`sys_perm=0` | 后端 | HTTP 502 + code `50202` + message `登录上下文缺失（未选择组织），请重新登录`；**不**写 auth cookie；H5（`client_type=wx`）不受影响 | R4 |
| R5 PC 回调页显示真实原因且不落 token | `frontend-pc/src/pages/AuthCallback/index.tsx` | PC `/callback` | 后端返回 `50202` / 非 20000 | PC | 不写 `localStorage.token`；展示后端 message；2 秒后重新发起 OAuth（可重选组织） | R5 |
| R6 40104 组织上下文缺失文案 | `frontend-pc/src/services/api.js`、`frontend-pc/src/pages/LogoutPage.jsx` | dashboard 401（`code=40104`） | 空组织 token 访问 `/api/admin/dashboard/stats` | PC | `clearTokens(` 后 `logout_reason`=`org_context_missing`；LogoutPage 文案 `登录上下文缺失，请重新登录并选择组织` | R6 |
| R7 构建与回归 | `backend`、`frontend-pc`、`beaconiam` | `go build ./...`、定向测试、`npm run build` | 修后树（两仓） | 后端 + IAM | 两仓 build exit 0；定向测试 0 FAIL；PC 构建 exit 0 | R7 |

## Input Truth & Tool Preflight

- **Source of truth（根因链，已逐行核对）**：
  - beaconiam `HandleLogin` 多组织分支签发 minimal token 并生成**无组织上下文**的 `AuthorizationCode`：`beaconiam/internal/auth/handler.go:560-640`。
  - `resolveUserOrg` 在多组织时返回空 `userOrgContext`：`beaconiam/internal/auth/handler.go:299-330`。
  - `handleAuthorizationCodeGrant` 直接用 `resolveUserOrg` 结果签名：`beaconiam/internal/auth/handler.go:820-910` → 空上下文 token。
  - IAM UI 选择组织后仍用旧 code 重定向：`beaconiam/ui/src/pages/SelectOrg.tsx:35-37`。
  - tuneloop 的 PC 回调把该 token 存入 localStorage：`frontend-pc/src/pages/AuthCallback/index.tsx:43-49`；401 处理：`frontend-pc/src/services/api.js:265-285`。
- **复现证据**：Issue 正文 Bearer token claims（`oid="",tid="",role="USER",fn_roles=null`）与后端日志（`tid=bfce6c6d… oid=76d46658… role=STAFF`）互为对照，足以锁定「code 未绑定组织」，无需再跑预生产。
- **跨仓证据**：beaconiam 检查目录默认为 `$HOME/beaconiam`（可用 `BEACONIAM_DIR` 覆盖）；缺失 → **INFRA-BLOCKED**，不得伪造 R1–R3 证据。beaconiam 改动提交在其仓库，commit sha 写入 R1/R2/R3 artifact（`BEACONIAM_COMMIT=<sha>`）。
- **差异核对**：单组织路径（`len(relations)==0`）直接签发带上下文 token，不受本单影响；`wx-login` / `wx-login-select` 不走 authorization_code grant，不受影响。
- **损坏输入拒绝**：`code` 为空/过期/已使用/client_id 不匹配 → 沿用既有 400 语义；PC 回调 claims 无法解析 → 视为空上下文（拒绝）。
- **工具盲区**：`frontend-pc` 无测试运行器 → R5/R6 用源码结构断言（`evidence_type: static`，Owner 豁免见 D5）；beaconiam 迁移采用 `runXxxMigration` + `schema_migrations` 幂等模式（参考 `beaconiam/cmd/api/main.go:752-771`），**不用** AutoMigrate 自动加列。
- **运行环境**：tuneloop 测试库 `tuneloop_test` + fake IAM；beaconiam 使用其仓库测试（`beacon.db`/测试库）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 修复层：beaconiam 授权码携带组织上下文（`AuthorizationCode.OrgID`），select-org 绑定该 code；**不在** tuneloop 侧伪造上下文 | Architect | 本计划锁定 | 阻塞实现 |
| D2 | legacy 多组织 code（无 OrgID）语义：`400 org_not_selected`（显式失败），不再静默签发空上下文 token | Architect | 本计划锁定（行为变更，需在 beaconiam 发布说明注明） | 若要求兼容旧客户端 → 回 Architect |
| D3 | PC 回调对「空组织上下文 token」的判定：`tid=="" && oid=="" && len(fn_roles)==0 && sys_perm==0 && cus_perm==0`（PC 分支专用）→ `50202`，不写 cookie/localStorage | Architect | 本计划锁定 | 阻塞实现 |
| D4 | H5/小程序不受影响：`client_type ∈ {wx,wechat,mobile}` 与 `wx-login*` 路径不参与该判定 | Architect | 本计划锁定 | 阻塞实现 |
| D5 | R5/R6 为纯前端接线，`frontend-pc` 无测试运行器 → `evidence_type: static` 源码结构断言 + 构建，**Owner 豁免**（Architect 授予） | Architect | 本计划锁定 | 若审计要求 render → 另立前端测试基建 Issue |
| D6 | 真机/预生产端到端验证（选海淀店后 dashboard 200、不再弹回）属部署侧，写入 `[Ops-Followup of #2454]` 跟踪 Issue（`waitForConfirm`，无 `status:*`），不作为矩阵行 | Architect / ops | beaconiam + tuneloop 部署后执行 | 未部署 → 不阻塞本单 done，但跟踪 Issue 必须创建 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R5 | static | 源码结构断言（回调页不落 token + 展示后端 message），Owner 豁免见 D5 |
| R6 | static | 源码结构断言（40104 → org_context_missing + 文案），Owner 豁免见 D5 |

## Out of Scope

- `sites.name` 与 IAM 组织名漂移、网点名唯一约束 → #2457 / #2455。
- PC 空闲超时 → #2456。
- IAM 登录页/组织选择页视觉改造（仅改跳转 code 取值，见 R3）。
- IAM 会话表、refresh token 轮换、组织切换的审计日志。
- 预生产数据修复与部署（部署侧，见 D6）。
- 全量 `go test ./...`（由批次 gate 承担）。

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|---|---|---|---|
| R1 | 多组织用户（`user_org_relations`：org_a=海淀店/`76d46658…`、org_b=卡丹萨西铁营/`a5eb38d4…`）+ 已绑定 org_a 的 code | `POST /api/v1/auth/token`（code grant） | `docs/evidence/2454/R1-iam-code-org.txt` |
| R2 | 同上用户 + legacy code（`OrgID=nil`） | `POST /api/v1/auth/token` | `docs/evidence/2454/R2-iam-legacy.txt` |
| R3 | 有效 code + `{org_id:76d46658…}`；随后复用同一 code 再次 select-org | `POST /api/v1/auth/select-org` | `docs/evidence/2454/R3-iam-select-org.txt` |
| R4 | fake IAM 返回空上下文 token（`tid="" oid="" fn_roles=null`），PC 回调 `client_type` 缺失；对照 H5 `client_type=wx` 同 token | `AuthHandler.Callback` | `docs/evidence/2454/R4-pc-callback-guard.txt` |
| R5 | 修后 `frontend-pc/src/pages/AuthCallback/index.tsx` | PC `/callback` | `docs/evidence/2454/R5-pc-callback-ui.txt` |
| R6 | 修后 `frontend-pc/src/services/api.js`、`frontend-pc/src/pages/LogoutPage.jsx` | 401 分支 + `/logout` | `docs/evidence/2454/R6-auth-error-copy.txt` |
| R7 | 修后 tuneloop + beaconiam 工作树 | build + 定向测试 | `docs/evidence/2454/R7-build.txt` |

### Assertions

- R1：`OID=76d46658…`、`TID=bfce6c6d…`、`ROLE=STAFF`、`FN_ROLES_NONEMPTY=1`、`REFRESH_CTX=ok`；负例：`OID=`（空）或 `ROLE=USER` → FAIL。
- R2：`HTTP=400`、`ERROR=org_not_selected`、`ACCESS_TOKEN_EMPTY=1`；负例：`HTTP=200` 且 `OID=`（空）→ FAIL。
- R3：`RESP_CODE_PRESENT=1`、`CODE_EXCHANGE_OID=76d46658…`、`REUSE_HTTP=400`（或新 code 非空）；负例：`RESP_CODE_PRESENT=0` → FAIL。
- R4：`PC_HTTP=502`、`CODE=50202`、`COOKIE_SET=0`、`H5_HTTP=200`；负例：`PC_HTTP=200`（写入空组织 token）→ FAIL。
- R5：静态断言命中 `data.code !== 20000`、`message.error(data.message`、无 `localStorage.setItem('token'` 落在失败分支；负例：失败分支仍写 token → FAIL。
- R6：静态断言命中 `40104`、`org_context_missing`、`登录上下文缺失，请重新登录并选择组织`；负例：缺任一 → FAIL。
- R7：`TUNELOOP_BUILD=0`、`BEACONIAM_BUILD=0`、`TESTS_FAIL=0`、`PC_BUILD=0`；负例：任一非 0 → FAIL。
- 通用 provenance：每个 artifact 首行 `# build=1.0.<fix>` `# commit=<sha>` `# dirty=false`；R1–R3 另含 `BEACONIAM_COMMIT=<sha>`。
- 负例（跨行）：任何 artifact 出现 `silently issue empty context`/`跳过组织绑定`/`fallback to empty ctx` 语义 → FAIL。

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2454/R1-iam-code-org.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2454/R2-iam-legacy.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2454/R3-iam-select-org.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2454/R4-pc-callback-guard.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2454/R5-pc-callback-ui.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2454/R6-auth-error-copy.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2454/R7-build.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| symptom | trigger | root cause | test blind spot | same-class paths | fix layer | red-green | prevention |
|---|---|---|---|---|---|---|---|
| 选网点后首页渲染一半被弹回 IAM 登录 | PC 登录多组织账号 | code 未绑定所选组织 | 无多组织 code grant 用例 | wx 多账号选择、组织切换 | IAM（authorization code） | R1 先写多组织用例（旧实现 oid 空 → FAIL），再实现（PASS） | code 携带组织上下文 + R4 显式拒绝 |
| token claims oid/tid 为空但 role=USER | 多组织 + resolveUserOrg 返回空 | 空上下文被当作合法上下文签发 | 无 claims 断言 | refresh grant 的历史默认上下文 | IAM | R1/R2 红绿 | 禁止静默签发空上下文 |
| 一次性错误表现为「登录环路」而非报错 | 前端把空 token 当成功 | 前端只判 `code===20000` | 无失败分支断言 | 其他 OAuth 回调（H5 callback） | tuneloop handler + 前端 | R4/R5 红绿 | 回调必须校验上下文完整性 |
| 401 后无有效诊断信息 | `handleAuthError` 统一 auth_failed | 未区分 40104 | 无 40104 用例 | 40105/40106/40107 已有语义分支 | PC 前端 | R6 红绿 | 语义码 → 专属文案 |
| 部署后仍复现 | 只修 beaconiam 未重启/未清旧 code | 旧 code 10 分钟内仍可用 | 无 legacy 用例 | 其他一次性 code 流程 | IAM + ops | R2 负例 | legacy code 显式报错而非空上下文 |
| H5 被误拒 | 用 `client_type` 缺失判断 PC 过宽 | 分支条件错误 | 无 H5 对照用例 | wx-login 路径 | tuneloop handler | R4 `H5_HTTP=200` 断言 | 明确 PC/H5 判定（D4） |
| 两仓版本错配（IAM 未升级） | 先发布 tuneloop | 依赖方向不清 | 无版本断言 | 其他跨仓契约 | 发布顺序 | R1 artifact 含 `BEACONIAM_COMMIT` | 部署顺序写入 Ops Follow-up |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2454.sh r1` | `docs/evidence/2454/R1-iam-code-org.txt` | exit 0；grep `ROLE=STAFF`、`FN_ROLES_NONEMPTY=1`、`OID=76d46658` | PASS | `OID=`（空）或 `ROLE=USER` → FAIL | dev |
| R2 | `bash scripts/verify-2454.sh r2` | `docs/evidence/2454/R2-iam-legacy.txt` | exit 0；grep `HTTP=400`、`ERROR=org_not_selected` | PASS | `HTTP=200` → FAIL | dev |
| R3 | `bash scripts/verify-2454.sh r3` | `docs/evidence/2454/R3-iam-select-org.txt` | exit 0；grep `RESP_CODE_PRESENT=1`、`CODE_EXCHANGE_OID=76d46658` | PASS | `RESP_CODE_PRESENT=0` → FAIL | dev |
| R4 | `bash scripts/verify-2454.sh r4` | `docs/evidence/2454/R4-pc-callback-guard.txt` | exit 0；grep `CODE=50202`、`COOKIE_SET=0`、`H5_HTTP=200` | PASS | `PC_HTTP=200` → FAIL | dev |
| R5 | `bash scripts/verify-2454.sh r5` | `docs/evidence/2454/R5-pc-callback-ui.txt` | exit 0；grep `FAILURE_BRANCH_NO_TOKEN=1`、`SHOWS_SERVER_MESSAGE=1` | PASS | 任一 `=0` → FAIL | dev |
| R6 | `bash scripts/verify-2454.sh r6` | `docs/evidence/2454/R6-auth-error-copy.txt` | exit 0；grep `ORG_CTX_REASON=1`、`LOGOUT_COPY=1` | PASS | 任一 `=0` → FAIL | dev |
| R7 | `bash scripts/verify-2454.sh r7` | `docs/evidence/2454/R7-build.txt` | exit 0；grep `TUNELOOP_BUILD=0`、`BEACONIAM_BUILD=0`、`PC_BUILD=0` | PASS | 任一非 0 → FAIL | dev |

### Workload

- Size: L
- Basis: beaconiam 3 文件（model/handler/migration 内联 + UI 1 行）+ 其测试；tuneloop 2 源文件 + 2 测试 + PC 2 文件；1 验证脚本；7 条证据行；跨仓验证（`$HOME/beaconiam`）
- Confidence: medium（跨仓构建与测试需 beaconiam 检查目录；前端两行为静态证据）

### Files（变更文件清单）

- `beaconiam/internal/models/authorization_code.go`
- `beaconiam/internal/auth/handler.go`
- `beaconiam/cmd/api/main.go`
- `beaconiam/ui/src/pages/SelectOrg.tsx`
- `beaconiam/internal/auth/handler_select_org_test.go`
- `backend/handlers/auth.go`
- `backend/handlers/pc_callback_guard_2454_test.go`
- `frontend-pc/src/pages/AuthCallback/index.tsx`
- `frontend-pc/src/services/api.js`
- `frontend-pc/src/pages/LogoutPage.jsx`
- `scripts/verify-2454.sh`
- `docs/spec/api/api.md`
- `docs/topics/iam/iam-notes.md`
- `docs/evidence/2454/plan.md`
- `docs/evidence/2454/R1-iam-code-org.txt`
- `docs/evidence/2454/R2-iam-legacy.txt`
- `docs/evidence/2454/R3-iam-select-org.txt`
- `docs/evidence/2454/R4-pc-callback-guard.txt`
- `docs/evidence/2454/R5-pc-callback-ui.txt`
- `docs/evidence/2454/R6-auth-error-copy.txt`
- `docs/evidence/2454/R7-build.txt`
- `docs/evidence/2454/evidence.json`

## Dependencies

- 前置 Issue：`HiJohns/beaconiam#500`（授权码绑定组织）——本单 R1–R3 即该 Issue 的实现验收，二者同一批次交付。
- 部署顺序（Ops Follow-up）：beaconiam 先、tuneloop 后；否则 R4 的 50202 会在旧 IAM 上持续触发（可诊断但不可登录）。
- 关联（不阻塞）：#2455 / #2457（组织名权威源）、#2456（PC idle）。

*Model: DeepSeek V4.1 Flash 1017*

