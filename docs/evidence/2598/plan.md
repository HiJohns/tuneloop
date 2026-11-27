## Accepted Plan — #2598 [Bug] 错误三元组（status/code/message）语义错配遗留：中转建组织压平持久错误 + 微信绑定/合并 409 却提示「稍后再试」

### 背景与现状核对（代码为据）

| # | 事实 | 位置 |
|---|---|---|
| 1 | 中转建组织失败的**非冲突**分支一律 `502/50200` +「中转网点组织创建失败，请稍后再试」：上游 400（父组织非法/参数被拒）、403（权限/模板缺失）、404（命名空间/资源不存在）这些**持久**失败与网络/5xx **瞬时**失败同码同文案 | `backend/handlers/transit_site_admin.go:127` |
| 2 | `CreateOrganization` 只把 409 类型化（`ErrOrganizationNameConflict`）；403 返回 `fmt.Errorf("permission denied: %s")`、其余非 2xx 返回 `fmt.Errorf("CreateOrganization returned status %d: %s")`——**调用方无法用 `errors.Is/As` 区分状态类**，禁止字符串匹配 | `backend/services/iam_client.go:315-330` |
| 3 | `WxBindCurrentUser` 把 `WxBind` 的**任何**错误（含 IAM 不可达/超时）映射为 `409/40900` +「微信绑定失败，请稍后再试」：HTTP 状态与业务码都把瞬时故障伪装成「数据冲突」，监控/重试策略按冲突处理 | `backend/handlers/user_staff.go:2271` |
| 4 | `WxMergeCurrentUser` 同构：`WxMerge` 任意失败 → `409/40900` +「账户合并失败，请稍后再试」 | `backend/handlers/user_staff.go:2315` |
| 5 | **同批扫描新增同类点**：`MergeBind`（PC 二维码绑定 → 小程序确认合并）的 `WxMergeByTicket` 失败同样 → `409/40900` +「账户合并失败，请稍后重试」（`请稍后重试` 与 #2597 的 18 处扫描词不同，故未被那轮扫到） | `backend/handlers/wechat_bind.go:296` |
| 6 | `WxBind`/`WxMerge` 已返回**类型化** `*services.IAMAPIError{StatusCode, ErrorCode, Body}`；传输层失败返回 `fmt.Errorf("failed to call IAM ...: %w")`（非 `IAMAPIError`） | `backend/services/iam.go:29-51,593-700` |
| 7 | beaconiam 侧 `wx-bind` 4xx 实际语义（本地检出 `7bb6e31`）：`400` = `invalid or expired exchange_token` / `invalid wechat code`；`404` = `client not found` / `user not found`；`503` = `wechat login not configured`；`500` = 绑定落库失败。**不存在 409** | `~/beaconiam/internal/auth/handler.go:1892-1990` |
| 8 | beaconiam 侧 `wx-merge` 4xx 语义：`400` = 缺字段/自合并/invalid uuid/`openid is not bound to the target account`；`403` = `only the account owner can merge` / `account not active` / `target account not active`；`404` = `user not found` / `target user not found`；`500` = 事务失败 | `~/beaconiam/internal/auth/handler.go:2019-2130` |
| 9 | 本地 beaconiam 检出**不含** `/auth/wx-merge-confirm`（`merge_ticket` 版本在部署实例侧）→ 项 4 只能按**状态类**分流，不得依赖上游错误串 | `~/beaconiam`（`grep merge_ticket` = 0 命中） |
| 10 | 前端解析口径：5xx（`code >= 50000`）的中文串**不得**透传，走 `code_map` 命中否则 fallback；4xx（< 50000）含 CJK 走 `human_message` 原样透传 | `frontend-pc/src/services/errorMessages.js:120-131` |
| 11 | `frontend-pc` / `frontend-mobile` 的 `ERROR_CODE_MAP` 均**无** `50015`（也无 `50200`）；`50014` 由 #2597（`origin/issue-2597`，status:done，待集成）取用 → 本单取 `50015` | `grep -n "5001[45]\|50200" frontend-*/src/services/errorMessages.js` = 0 命中 |
| 12 | 既有断言把「任意失败」钉在 409：`TestWxMergeCurrentUser_IAMFailureMapped_2228`（fake IAM 400 → 断言 409/含「账户合并失败」）、`TestWxBindCurrentUser`（fake IAM 409 → 断言 409/40900/含「微信绑定失败」） | `backend/handlers/wx_merge_2228_test.go:136-150`、`backend/handlers/auth_test.go:1220-1232` |

**判定口径（本单锁定）**：错误三元组必须满足「状态 ≡ code 家族 ≡ 文案可行动性」：

| 失败类别 | 判据（只用 `errors.As` 取上游状态，禁止字符串匹配） | HTTP / code | 文案要求 |
|---|---|---|---|
| 瞬时 | 传输层错误（无上游状态）/ 上游 `429`、`5xx` | `502 / 50200` | 含「请稍后再试」 |
| 冲突 | 上游 `409` | `409 / 40900` | 指明冲突对象 + 可纠正动作 |
| 用户可重做 | 上游 `400` / `422` | `400 / 40002` | 指明用户动作（重新扫码/重新发起），**不含**「稍后再试」 |
| 持久配置/权限 | 上游 `401` / `403` / `404` / 其余 4xx | `500 / 50015` | 「联系平台管理员」，**不含**「稍后再试」 |

特例：中转建组织是**服务端代发起**的上游请求，其上游 `400/422` 只能是我们的请求/配置缺陷（父组织非法、参数契约不符）→ 归入「持久配置」而非「用户可重做」（`UserActionAsConfig: true`）。

### 同批扫描判定（同类缺陷全量，`grep -rn "StatusConflict" backend/handlers/*.go` + 「请稍后」交叉扫）

| # | 位置 | 判定 | 处置 |
|---|---|---|---|
| 1 | `transit_site_admin.go:127` | 持久 4xx 与瞬时压平在同一 `502/50200` | **本单修**（项 1） |
| 2 | `user_staff.go:2271` | 瞬时故障被写成 `409/40900` | **本单修**（项 2） |
| 3 | `user_staff.go:2315` | 同上 | **本单修**（项 3） |
| 4 | `wechat_bind.go:296` | 同上（#2597 扫描词外的同类点，本轮补扫发现） | **本单修**（项 4） |
| 5 | `auth.go:487`（注册流程 WxBind 失败 → 409 + 回滚 purge） | **不同类**：该 409 是注册中止信号，前端按 409 走「重新注册」路径，且与 #1644 回滚契约耦合；改动会动注册事务语义 | 保持（判定记录在案） |
| 6 | `transit_site_admin.go:387` | HTTP `502` + code `50000` 码/状态不一致（上游角色更新失败，用户不可见） | 保持（#2597 已记录为低危，本单不扩面） |
| 7 | 其余 14 处「请稍后再试」（`registration_session` / `payscore` / `user_management` / `merchant` / `site_member` / `merchant_member` / `user_staff` 切换网点族） | 真瞬时故障，文案正确 | 保持（#2597 判定） |

### 变更点

#### 1. `backend/services`：上游失败分类唯一出口

新文件 `backend/services/iam_failure.go`：

```go
// IAMFailureKind 分类 IAM 上游失败（#2598）：决定 handlers 的 HTTP 状态/业务码/文案。
type IAMFailureKind int

const (
	IAMFailureTransient  IAMFailureKind = iota // 传输层/超时/上游 429、5xx → 可重试
	IAMFailureConflict                         // 上游 409
	IAMFailureUserAction                       // 上游 400/422：请求被上游校验拒绝，用户需重做动作
	IAMFailureConfig                           // 上游 401/403/404/其余 4xx：持久配置/权限/数据不一致
)

// ClassifyIAMFailure 返回失败分类与上游 HTTP 状态码（0 = 无上游状态，即传输层失败）。
// 判定只用 errors.As（*IAMAPIError / *IAMClientHTTPError），禁止字符串匹配。
func ClassifyIAMFailure(err error) (IAMFailureKind, int)
```

- `errors.As(err, &*IAMAPIError)`（wx-bind / wx-merge 路径）与 `errors.As(err, &*IAMClientHTTPError)`（IAMClient 路径）双类型支持；`err == nil` → `(IAMFailureTransient, 0)`（调用方不得以 nil 调用，仅作防御）。
- 状态映射：`0`（未知）/`429`/`>=500` → transient；`409` → conflict；`400`,`422` → userAction；其余（`401`,`403`,`404`,其它 4xx）→ config。
- **不改变**任何既有 `IAMAPIError` 行为，只新增分类函数。

`backend/services/iam_client.go`：新增类型化 HTTP 错误（**文本逐字保持**，零断言漂移）：

```go
// IAMClientHTTPError（#2598）：IAMClient 非 2xx 响应的类型化出口。
// Error() 文本与改造前三串逐字一致（403 = "permission denied: <body>"，
// 其余 = "<Op> returned status <n>: <body>"），故历史日志/断言口径不变。
type IAMClientHTTPError struct {
	Op     string
	Status int
	Body   string
}
```

- `CreateOrganization` 与 `CreateOrganizationWithToken` 的 403 分支 → `fmt.Errorf("permission denied: %w", &IAMClientHTTPError{Op: "CreateOrganization", Status: 403, Body: string(respBody)})`；其余非 2xx 分支同理包装（`Error()` 复现原串）。409 继续走 `ErrOrganizationNameConflict` 哨兵（不改）。
- 其余 `IAMClient` 方法（`UpdateOrganization` 等）本单**不改造**（Out of Scope）。

#### 2. `backend/handlers`：统一响应出口

新文件 `backend/handlers/iam_failure.go`：

```go
const (
	// iamConfigErrorMessage 与 PC/mobile ERROR_CODE_MAP[50015] 必须逐字一致（#2497 规则下 5xx+CJK 走 code_map）。
	iamConfigErrorMessage     = "身份服务配置异常，请联系平台管理员处理"
	iamConflictFallbackMessage = "操作与当前数据状态冲突，请刷新后重试"
)

// iamFailureTexts 由调用点提供业务化文案；Config 分支固定用 iamConfigErrorMessage 防漂移。
type iamFailureTexts struct {
	Transient  string // 502/50200
	Conflict   string // 409/40900（留空则 iamConflictFallbackMessage）
	UserAction string // 400/40002
	// UserActionAsConfig=true：服务端代发起的上游请求，400/422 视为持久配置缺陷（归 500/50015）。
	UserActionAsConfig bool
}

// respondIAMFailure 写响应并记录分类/状态到日志（细节只进日志）。
func respondIAMFailure(c *gin.Context, err error, t iamFailureTexts)
```

- 固定三元组：transient → `502/50200`；conflict → `409/40900`；userAction → `400/40002`；config → `500/50015` + `iamConfigErrorMessage`。
- 调用点保留各自 `log.Printf`（含 user id/org name 等上下文），helper 只额外记录 `kind=`/`upstream_status=`（便于线上区分）。

#### 3. 四个调用点分流（文案逐字固定）

| 调用点 | 瞬时（502/50200） | 冲突（409/40900） | 用户可重做（400/40002） | 持久配置（500/50015） |
|---|---|---|---|---|
| `transit_site_admin.go`（建组织） | 「中转网点组织创建失败，请稍后再试」（**不变**） | 既有分支「同名组织已存在，请更换名称后重试」（**不变**） | — （`UserActionAsConfig=true` → 归持久配置） | `iamConfigErrorMessage` |
| `user_staff.go` `WxBindCurrentUser` | 「微信绑定失败，请稍后再试」 | 「微信绑定失败：该微信已绑定其他账户，请确认合并后重试」（保留「微信绑定失败」关键词） | 「微信授权已失效，请重新进入页面重试」 | `iamConfigErrorMessage` |
| `user_staff.go` `WxMergeCurrentUser` | 「账户合并失败，请稍后再试」 | 「账户合并失败：微信绑定关系已变更，请重新发起绑定」 | 「账户合并失败：绑定信息已失效，请重新发起绑定」 | `iamConfigErrorMessage` |
| `wechat_bind.go` `MergeBind` | 「账户合并失败，请稍后再试」 | 「账户合并失败：绑定状态已变更，请让管理员重新生成绑定二维码」（保留「账户合并失败」关键词） | 「合并凭证已失效，请让管理员重新生成绑定二维码」 | `iamConfigErrorMessage` |

- `needs_merge` 分支（`user_staff.go:2252-2274`，20000 + `data.needs_merge`）**不变**，回归断言保留。
- `WxMergeCurrentUser` 的 `userID == req.ToUserID` 400 前置校验**不变**。

#### 4. 前端：登记 `50015`（PC + mobile 逐字一致）

- `frontend-pc/src/services/errorMessages.js`：`ERROR_CODE_MAP` 增 `50015: '身份服务配置异常，请联系平台管理员处理',`
- `frontend-mobile/src/services/errorMessages.js`：同一条同文案（mobile `t()` 缺键回退原串，无需 locale 条目）。
- **不新增** `50200` 到码表（全站影响另议，#2597 D3 口径延续）→ 瞬时类在前端落到调用点 fallback（`微信绑定失败` / `创建失败，请重试`），仍不含误导性「数据冲突」。此事实写入 Out of Scope 与 ui.md 说明，不静默。

#### 5. 文档（见 Core Update）

#### 6. 测试与证据

- 新增 `backend/services/iam_failure_2598_test.go`：`ClassifyIAMFailure` 表驱动（`IAMAPIError` / `IAMClientHTTPError` / 包装链 `%w` / 传输层 / `nil`）× 状态 {0,400,401,403,404,409,422,429,500,503}；并断言 `IAMClientHTTPError.Error()` 文本与改造前逐字一致。
- 扩展 `backend/handlers/transit_site_admin_2552_test.go` 的 `transitIAMRecorder`：`createMode` 增 `rejected400` / `forbidden403` 两个分支（本次只加分支，不改既有断言）。
- 新增 `backend/handlers/transit_site_admin_2598_test.go`：复用 `transitIAMRecorder` + `transitActorRouter2552()`，断言持久类（400/403）→ `500/50015`、文案含「联系平台管理员」且**不含**「请稍后再试」、`creates` 之后无 `createdOrgs`/`sites`；瞬时类（`createMode=error` 500 与 `SetIAMInternalURLForTesting` 指向已关闭端口）→ `502/50200` + 「请稍后再试」。
- 新增 `backend/handlers/wx_failure_2598_test.go`：fake IAM 按状态矩阵驱动 `WxBindCurrentUser` / `WxMergeCurrentUser` / `MergeBind`（`MergeBind` 复用 `bindMergeTestRouter()` 与 `testfixtures.SetupTestDB`；IAM 不可达由关闭的 `httptest.Server` 注入），断言四类三元组 + 负例（持久/重做类响应**不含**「请稍后再试」；瞬时类**不得**为 409）。
- 翻转/强化既有断言（见下节）。

### 被翻转 / 被强化的既有断言（强制清单）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `backend/handlers/wx_merge_2228_test.go:136-150`（`TestWxMergeCurrentUser_IAMFailureMapped_2228`） | fake IAM 400 → `409` + 含「账户合并失败」 | `400` + `40002` + 含「账户合并失败」+ **不含**「请稍后再试」 | 上游 400 是请求被拒（用户可重做），不是数据冲突；旧断言把 409 钉成「任意失败」 |
| `backend/handlers/auth_test.go:1220-1232`（`TestWxBindCurrentUser` "bind failure surfaces 409 not swallowed"） | fake IAM 409 → `409`/`40900`/含「微信绑定失败」 | 同上 + 追加「已绑定其他账户」与「不含『请稍后再试』」 | 该分支现只在**真冲突**（上游 409）出现；断言强化到可行动性（旧断言仍成立，不得改弱） |

> 除上表外**不改**任何既有断言：`transit_site_admin_2552_test.go` 的 `DUPLICATE_NAME_409`（409/40900）、`TestWxBindCurrentUser_NeedsMergePassthrough_2228`（20000 + `needs_merge`）、`bind_merge_2445_test.go` 全量 token 均保持原值。

### Core Update

`[Core Update Required]`

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §14.4 错误码表止于 `50000`（`50014` 由 #2597 追加，尚未集成）；无「错误三元组」口径段 | §14.4 增行 `\| 50015 \| 身份服务配置/权限异常（上游 IAM 401/403/404 或服务端代发请求被上游拒绝；需平台管理员修数据/权限，**不可重试**）\|`；表后新增口径段「三元组一致性（#2598）」：瞬时（传输层/429/5xx）→ `502/50200` + 含「请稍后再试」；冲突（上游 409）→ `409/40900`；用户可重做（上游 400/422）→ `400/40002`；持久配置（401/403/404）→ `500/50015` + 「身份服务配置异常，请联系平台管理员处理」（与 PC/mobile `ERROR_CODE_MAP[50015]` 逐字一致） | 新增错误码 + 错误三元组口径成为跨端契约（§1.3 要求新码同步双端码表） |
| `docs/spec/ui/ui.md` | §2.6.2「绑定微信与账户合并」只写成功/合并路径，未写失败分类文案；§3.20 中转中心只写端点/列/操作（#2597 追加失败提示段落，尚未集成） | §2.6.2 增「失败提示（#2598）」小表：上游 400/422 → 「微信授权已失效，请重新进入页面重试」/「账户合并失败：绑定信息已失效，请重新发起绑定」；上游 409 → 「该微信已绑定其他账户，请确认合并后重试」；上游 401/403/404 → 「身份服务配置异常，请联系平台管理员处理」；网络/5xx → 瞬时提示（前端按 fallback 展示，`50200` 未入码表）。§3.20 增一行「建组织失败：上游配置/权限类（IAM 400/403/404）→「身份服务配置异常，请联系平台管理员处理」（500/50015）；网络/5xx → 502/50200 瞬时提示」 | 用户可见文案成为 UI 契约（两处页面均展示这些分支） |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 上游失败分类器（四类 + 状态提取 + 文本保持） | `backend/services/iam_failure.go`、`backend/services/iam_failure_2598_test.go` | `go test ./services -run TestClassifyIAMFailure2598` | 表驱动 10 个状态 + 传输层 + `nil` + `%w` 包装 + 两个错误类型 | backend（单元） | `CLASSIFIER_MISMATCH=0`、`CLASSIFIER_CASES=13`、`HTTPERR_TEXT_KEPT=1` | R1 |
| R2 中转建组织持久失败分流（400 / 403） | `backend/handlers/transit_site_admin.go`、`backend/handlers/transit_site_admin_2598_test.go` | `POST /api/admin/transit-sites` | fake IAM `organizations` 返回 400 / 403 | backend（Postgres 测试库） | `TRANSIT_REJECTED_STATUS=500`、`TRANSIT_REJECTED_CODE=50015`、`TRANSIT_REJECTED_MSG_ACTIONABLE=1`、`TRANSIT_REJECTED_MSG_NO_RETRY=0`、`TRANSIT_REJECTED_SITES_CREATED=0` | R2 |
| R3 中转建组织瞬时失败保持不变（上游 500 / 不可达） | `backend/handlers/transit_site_admin.go`、`backend/handlers/transit_site_admin_2598_test.go` | `POST /api/admin/transit-sites` | fake IAM `organizations` 返回 500；IAM URL 指向已关闭端口 | backend（Postgres 测试库） | `TRANSIT_TRANSIENT_STATUS=502`、`TRANSIT_TRANSIENT_CODE=50200`、`TRANSIT_TRANSIENT_MSG_RETRY=1`、`TRANSIT_TRANSIENT_NOT_CONFIG=1` | R3 |
| R4 微信绑定 / 合并 / 二维码合并三路径四类分流 | `backend/handlers/user_staff.go`、`backend/handlers/wechat_bind.go`、`backend/handlers/wx_failure_2598_test.go` | `POST /api/users/me/wx-bind`、`POST /api/users/me/wx-merge`、`POST /api/wechat-bind/merge` | fake IAM 400 / 403 / 404 / 409 / 500 / 不可达 | backend（Postgres 测试库） | `WXBIND_400_CODE=40002`、`WXBIND_403_CODE=50015`、`WXBIND_409_CODE=40900`、`WX_TRANSIENT_STATUS=502`、`WX_TRANSIENT_CODE=50200`、`WXMERGE_400_CODE=40002`、`MERGEBIND_400_CODE=40002`、`PERSISTENT_NO_RETRY_HINT=0`、`WXBIND_NEEDS_MERGE=1` | R4 |
| R5 既有断言翻转与强化 | `backend/handlers/wx_merge_2228_test.go`、`backend/handlers/auth_test.go` | `go test ./handlers -run 'TestWxMergeCurrentUser_IAMFailureMapped_2228\|TestWxBindCurrentUser'` | fake IAM 400（merge）/ 409（bind） | backend（Postgres 测试库） | `WXMERGE_400_STATUS=400`、`LEGACY_FLIPPED=1`、`WXBIND_409_ACTIONABLE=1`、`LEGACY_TESTS_FAIL=0` | R5 |
| R6 双端码表 `50015` 与后端常量一致性 | `frontend-pc/src/services/errorMessages.js`、`frontend-mobile/src/services/errorMessages.js`、`frontend-pc/tests/errorText-2598.test.mjs`、`backend/handlers/iam_failure.go` | `node --test` + `node scripts/check-error-message-parity-2480.mjs` | 码表与后端常量文本；`{code:50015}` 解析层 | PC（Node 纯函数） | `PC_MAP_50015=1`、`MOBILE_MAP_50015=1`、`MAP_TEXT_MATCH=1`、`CONFIG_MSG_DRIFT=0`、`PARITY=1`、`NODE_TEST_FAIL=0` | R6 |
| R7 文档同步（新码 + 三元组口径 + 页面失败文案） | `docs/spec/api/api.md`、`docs/spec/ui/ui.md` | 文档结构断言（`evidence_type: static`，Owner 豁免） | 修后文档 | 文档 | `API_CODE_50015=1`、`API_TRIPLE_RULE=1`、`UI_WXBIND_TEXTS=1`、`UI_TRANSIT_NOTE=1` | R7 |
| R8 构建 / gofmt / lint / 双端平价 | `scripts/verify-2598.sh` | `cd backend && go build ./...` + `gofmt -l` + `npx eslint` | 修后工作树 | 双端 | `GO_BUILD_OK=1`、`GOFMT_DIFF=0`、`ESLINT_NOUNDEF=0` | R8 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | unit | `go test ./services` 表驱动单测（无 DB/网络依赖） |
| R2 | integration | handler + fake IAM（`transitIAMRecorder`）+ Postgres 测试库真实建站路径，断言响应与副作用 |
| R3 | integration | 同上，瞬时类（上游 500 / 端口关闭） |
| R4 | integration | 三路径 × 六种上游状态；`MergeBind` 走 `bindMergeTestRouter()` + `testfixtures.SetupTestDB` |
| R5 | integration | 既有测试翻转载荷（同一 fake IAM 矩阵） |
| R6 | unit+static | `node --test` 纯函数 + 静态码表/常量抽取 + 跨包平价脚本。PC 无渲染测试运行器（`frontend-pc/package.json` 无 vitest/jsdom），按 **Owner 豁免**（沿用 #2597 D7 / #2580 D6 / #2480 D6） |
| R7 | static | 文档结构断言（Owner 豁免同上） |
| R8 | build | `go build ./...` + `gofmt -l` + `npx eslint` |

### Input Truth & Tool Preflight

- **Source of truth**：
  - 失败分类唯一出口 = 新增 `services.ClassifyIAMFailure`（本单）；上游状态唯一来源 = `*services.IAMAPIError.StatusCode`（`iam.go:29-51`）与新增 `*services.IAMClientHTTPError.Status`（`iam_client.go`）；
  - 用户文案唯一出口 = `frontend-pc`/`frontend-mobile` 的 `ERROR_CODE_MAP`（`50015` 文案与 `iamConfigErrorMessage` 逐字一致）；
  - 上游 4xx 语义 = beaconiam 本地检出 `~/beaconiam/internal/auth/handler.go:1892-1990 / 2019-2130`（项 4 的 `wx-merge-confirm` 不在本地检出，只按状态类分流）；
  - 既有断言 = `auth_test.go:1220-1232`、`wx_merge_2228_test.go:136-150`（翻转清单见上）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `grep -n "50015" backend frontend-pc/src frontend-mobile/src docs/spec` → 修前 **0 命中**（本单新增；`50014` 属 #2597）；
  - `grep -n "ClassifyIAMFailure\|IAMClientHTTPError" backend` → 修前 **0 命中**；
  - `grep -c "respondIAMFailure" backend/handlers/*.go` → 修前 **0**，修后 **4 处调用 + 1 处定义**；
  - `grep -rn "请稍后再试" backend/handlers | wc -l` → 修前 **18**，修后仍 **18**（瞬时类文案保留，本轮不改行数；判定只用文案/token 断言，不用行数当证据）。
- **损坏输入拒绝规则**：
  - `err == nil` 传入 `respondIAMFailure` → 视为编程错误，落 transient 分支并记录 `upstream_status=0`（不得 panic、不得 200）；
  - fake IAM 返回非 JSON / 空 body → `IAMAPIError.StatusCode` 仍可用于分类（不得因解析失败改判为 config）；
  - `IAMAPIError` 被 `%w` 包裹 → 必须仍能 `errors.As` 命中（否则分类退化为 transient，用例覆盖）；
  - 前端 `result` 非对象（字符串/null）→ 解析器给 fallback，不抛二次异常（既有行为，回归断言）。
- **工具盲区**：
  - fake IAM 只证明我方分类与响应，不证明 beaconiam 侧错误码演化；上游状态类变化（如新增 422）由 `ClassifyIAMFailure` 表驱动覆盖，不需改 handler；
  - `MergeBind` 的 `wx-merge-confirm` 上游语义未在本地检出中核对（只能状态类分流）→ 记录为 RISK 已由「状态类判据」消解，不依赖具体串；
  - PC 无渲染测试运行器（见 R6 evidence_type）；真机/浏览器回执归 `[Ops-Followup of #2598]`（`waitForConfirm`，无 `status:*`，收尾时创建）。
- **契约影响**：新增错误码 `50015` + 两条文档 + 跨端码表 → Work Summary 必须含 `### 契约三向核对`（pre-ready P7）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 分类器 = `services.ClassifyIAMFailure(err) (kind, upstreamStatus)`，`kind ∈ {transient, conflict, userAction, config}`；判据 `0/429/5xx→transient`、`409→conflict`、`400/422→userAction`、其余 4xx→config；只用 `errors.As` | 本计划锁定 |
| D2 | 新码 `50015`（`50014` 已被 #2597 预留），HTTP `500`；文案恒为「身份服务配置异常，请联系平台管理员处理」，后端常量与 PC/mobile `ERROR_CODE_MAP[50015]` **逐字一致** | 本计划锁定 |
| D3 | 瞬时类固定 `502/50200`，**不新增** `50200` 到码表（全站影响另议，#2597 D3 延续）；前端瞬时提示走调用点 fallback，不显示「数据冲突」 | 本计划锁定 |
| D4 | 中转建组织：冲突走既有 `ErrOrganizationNameConflict` 分支（409/40900，文案不变），其余 `UserActionAsConfig=true` → 上游 400/403/404 统一 `500/50015`；瞬时文案保持「中转网点组织创建失败，请稍后再试」 | 本计划锁定 |
| D5 | `WxBind`/`WxMerge`/`MergeBind`：上游 409 → 409/40900（可行动）；上游 400/422 → 400/40002（可行动）；上游 401/403/404 → 500/50015；传输层/429/5xx → 502/50200。`needs_merge`（20000）分支与自合并 400 前置校验不变 | 本计划锁定 |
| D6 | 项 4（`wechat_bind.go:296` `MergeBind`）纳入本单：同批扫描发现的同类点，且未在 #2597 的 18 处扫描词内；`Bind.jsx` 的 `merge_error` 静态文案**不改**（仍显示「合并凭证可能已过期…」），后端分类只影响监控/重试语义 | 本计划锁定 |
| D7 | 前端证据形态 = 纯函数 `node --test` + 静态码表/常量抽取 + 跨包平价 + 构建；真实渲染/点击归部署侧复验。**Owner 豁免**（沿用 #2597 D7 / #2580 D6 / #2480 D6） | Owner 已豁免（同族先例） |
| D8 | 重命令（`go test`/`go build`/`eslint`/`node --test`）经 `heavy-run.sh` 取锁；合同行禁止全量 `go test ./...`（P5 baseline 差分承担） | 本计划锁定 |
| D9 | 并行改动交叉点（**非阻塞**）：#2597（`origin/issue-2597`，status:done，待集成）同时改 `backend/handlers/transit_site_admin.go`（本单项 1 上方相邻块）与 `docs/spec/api/api.md` §14.4 表尾。两单语义不冲突；集成时若产生文本冲突，**双方内容必须都保留**（不得丢弃 `50014` 或 `50015`、不得丢弃任一失败分支） | 本计划锁定 |
| D10 | 不在本单扩面：`auth.go:487` 注册流程 409、`transit_site_admin.go:387` 码/状态不一致、`getNamespaceID` 零命名空间仍按瞬时处理（#2597 已记录的低危/既有语义） | 本计划锁定 |

### Out of Scope

- `auth.go:487`（注册流程 `WxBind` 失败 → 409 + purge 回滚）：与 #1644 注册回滚契约耦合，属另一类判定（同批扫描表第 5 行记录在案）。
- `transit_site_admin.go:387`（HTTP 502 + code 50000 不一致，用户不可见）与 `getNamespaceID` 零命名空间的分类细化。
- 其余 14 处「请稍后再试」（真瞬时故障，文案正确，`#2597` 已判定保持）。
- `50200` 等 5xxxx 码的全站码表收敛（`ERROR_CODE_MAP` 增补 `50200`/`50202`）→ #2489 族。
- 全站 `r.message`/`e.message` 直出清理、其它页面错误层接线 → #2489 族。
- `backend/services/iam_client.go` 其余方法的类型化改造（`UpdateOrganization`/成员相关等）。
- `Bind.jsx` 的 `merge_error` 文案改造（保持静态提示）。
- H5 / 微信小程序页面结构改动（本单只动 PC+mobile 的码表与后端分类）。
- 全量 `go test ./...` 作为合同行（由 pre-ready P5 baseline 差分承担）。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `&IAMAPIError{StatusCode: n}` / `&IAMClientHTTPError{Status: n}`（n ∈ 0,400,401,403,404,409,422,429,500,503）、`fmt.Errorf("ctx: %w", …)`、纯传输层 error | `TestClassifyIAMFailure2598` | `docs/evidence/2598/R1-classifier.txt` | `CLASSIFIER_MISMATCH=0`、`CLASSIFIER_CASES=13`、`HTTPERR_TEXT_KEPT=1`（403 → `permission denied: ` 前缀；其余 → `returned status <n>:`） | 任一状态被分到错误类别、包装链失配、`IAMClientHTTPError.Error()` 文本漂移 → FAIL |
| R2 | 租户 T（`SeedTenantFK`）；fake IAM `createMode=rejected400` 与 `forbidden403`，`primaries=1` | `POST /api/admin/transit-sites`（`TestTransitSiteCreate2598_PersistentFailure`） | `docs/evidence/2598/R2-transit-persistent.txt` | `TRANSIT_REJECTED_STATUS=500`、`TRANSIT_REJECTED_CODE=50015`、`TRANSIT_REJECTED_MSG_ACTIONABLE=1`、`TRANSIT_REJECTED_MSG_NO_RETRY=0`、`TRANSIT_REJECTED_SITES_CREATED=0` | 返回 502/50200、文案含「请稍后再试」、建出站点、缺「联系平台管理员」 → FAIL |
| R3 | 同上；fake IAM `createMode=error`（500）与 IAM URL 指向已关闭端口 | `POST /api/admin/transit-sites`（`TestTransitSiteCreate2598_TransientFailure`） | `docs/evidence/2598/R3-transit-transient.txt` | `TRANSIT_TRANSIENT_STATUS=502`、`TRANSIT_TRANSIENT_CODE=50200`、`TRANSIT_TRANSIENT_MSG_RETRY=1`、`TRANSIT_TRANSIENT_NOT_CONFIG=1` | 返回 50015/4xx、文案缺「请稍后再试」、建出站点 → FAIL |
| R4 | fake IAM 状态矩阵（400/403/404/409/500 + 关闭端口）；`MergeBind` 用 `bindMergeTestRouter()` + `needs_merge` 绑定态 | `POST /api/users/me/wx-bind`、`POST /api/users/me/wx-merge`、`POST /api/wechat-bind/merge`（`TestWxFailure2598_*`） | `docs/evidence/2598/R4-wx-paths.txt` | `WXBIND_400_CODE=40002`、`WXBIND_403_CODE=50015`、`WXBIND_409_CODE=40900`、`WX_TRANSIENT_STATUS=502`、`WX_TRANSIENT_CODE=50200`、`WXMERGE_400_CODE=40002`、`MERGEBIND_400_CODE=40002`、`PERSISTENT_NO_RETRY_HINT=0`、`WXBIND_NEEDS_MERGE=1` | 瞬时类返回 409、持久类含「请稍后再试」、`needs_merge` 不再返回 20000、自合并 400 断言失败 → FAIL |
| R5 | 既有 fixtures（`wxMergeMockServer2228`、`TestWxBindCurrentUser` mock） | `go test ./handlers -run 'TestWxMergeCurrentUser_IAMFailureMapped_2228\|TestWxBindCurrentUser'` | `docs/evidence/2598/R5-legacy-flip.txt` | `WXMERGE_400_STATUS=400`、`LEGACY_FLIPPED=1`、`WXBIND_409_ACTIONABLE=1`、`LEGACY_TESTS_FAIL=0` | merge 400 仍为 409、bind 409 文案不可行动、既有测试失败 → FAIL |
| R6 | 码表条目 + `iamConfigErrorMessage` 常量；`{code:50015}`；`{code:50015,message:'IAM returned status: 403...'}` | `node --test frontend-pc/tests/errorText-2598.test.mjs` + `node scripts/check-error-message-parity-2480.mjs` | `docs/evidence/2598/R6-frontend-map.txt` | `PC_MAP_50015=1`、`MOBILE_MAP_50015=1`、`MAP_TEXT_MATCH=1`、`CONFIG_MSG_DRIFT=0`、`PARITY=1`、`NODE_TEST_FAIL=0` | 任一端缺 `50015`、双端文案不一致、后端常量与码表不一致、ASCII 内部串外溢 → FAIL |
| R7 | 修后 `docs/spec/api/api.md` + `docs/spec/ui/ui.md` | `bash scripts/verify-2598.sh r7` | `docs/evidence/2598/R7-docs.txt` | `API_CODE_50015=1`、`API_TRIPLE_RULE=1`、`UI_WXBIND_TEXTS=1`、`UI_TRANSIT_NOTE=1` | 文档缺 `50015` 行、缺三元组口径、缺页面失败文案 → FAIL |
| R8 | 修后工作树 | `bash scripts/verify-2598.sh r8` | `docs/evidence/2598/R8-build-lint.txt` | `GO_BUILD_OK=1`、`GOFMT_DIFF=0`、`ESLINT_NOUNDEF=0` | 构建失败、gofmt 差异、eslint `no-undef` → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2598.sh r1 > docs/evidence/2598/R1-classifier.txt` | `docs/evidence/2598/R1-classifier.txt` | exit 0 且含 `CLASSIFIER_MISMATCH=0` `CLASSIFIER_CASES=13` `HTTPERR_TEXT_KEPT=1` | PASS（三项 token 全为期望值） | 任一状态分类错误、包装链失配、错误文本漂移 → FAIL | dev |
| R2 | `bash scripts/verify-2598.sh r2 > docs/evidence/2598/R2-transit-persistent.txt` | `docs/evidence/2598/R2-transit-persistent.txt` | exit 0 且含 `TRANSIT_REJECTED_STATUS=500` `TRANSIT_REJECTED_CODE=50015` `TRANSIT_REJECTED_MSG_ACTIONABLE=1` `TRANSIT_REJECTED_MSG_NO_RETRY=0` `TRANSIT_REJECTED_SITES_CREATED=0` | PASS（五项 token 全为期望值） | 502/50200、含「请稍后再试」、建出站点 → FAIL | dev |
| R3 | `bash scripts/verify-2598.sh r3 > docs/evidence/2598/R3-transit-transient.txt` | `docs/evidence/2598/R3-transit-transient.txt` | exit 0 且含 `TRANSIT_TRANSIENT_STATUS=502` `TRANSIT_TRANSIENT_CODE=50200` `TRANSIT_TRANSIENT_MSG_RETRY=1` `TRANSIT_TRANSIENT_NOT_CONFIG=1` | PASS（四项 token 全为期望值） | 50015/4xx、缺「请稍后再试」 → FAIL | dev |
| R4 | `bash scripts/verify-2598.sh r4 > docs/evidence/2598/R4-wx-paths.txt` | `docs/evidence/2598/R4-wx-paths.txt` | exit 0 且含 `WXBIND_400_CODE=40002` `WXBIND_403_CODE=50015` `WXBIND_409_CODE=40900` `WX_TRANSIENT_STATUS=502` `WX_TRANSIENT_CODE=50200` `WXMERGE_400_CODE=40002` `MERGEBIND_400_CODE=40002` `PERSISTENT_NO_RETRY_HINT=0` `WXBIND_NEEDS_MERGE=1` | PASS（九项 token 全为期望值） | 瞬时返 409、持久类含「稍后再试」、`needs_merge` 回归 → FAIL | dev |
| R5 | `bash scripts/verify-2598.sh r5 > docs/evidence/2598/R5-legacy-flip.txt` | `docs/evidence/2598/R5-legacy-flip.txt` | exit 0 且含 `WXMERGE_400_STATUS=400` `LEGACY_FLIPPED=1` `WXBIND_409_ACTIONABLE=1` `LEGACY_TESTS_FAIL=0` | PASS（四项 token 全为期望值） | merge 400 仍 409、文案不可行动、既有测试失败 → FAIL | dev |
| R6 | `bash scripts/verify-2598.sh r6 > docs/evidence/2598/R6-frontend-map.txt` | `docs/evidence/2598/R6-frontend-map.txt` | exit 0 且含 `PC_MAP_50015=1` `MOBILE_MAP_50015=1` `MAP_TEXT_MATCH=1` `CONFIG_MSG_DRIFT=0` `PARITY=1` `NODE_TEST_FAIL=0` | PASS（六项 token 全为期望值） | 缺码表条目、双端不一致、常量漂移、平价失败 → FAIL | dev |
| R7 | `bash scripts/verify-2598.sh r7 > docs/evidence/2598/R7-docs.txt` | `docs/evidence/2598/R7-docs.txt` | exit 0 且含 `API_CODE_50015=1` `API_TRIPLE_RULE=1` `UI_WXBIND_TEXTS=1` `UI_TRANSIT_NOTE=1` | PASS（四项 token 全为期望值） | 文档缺码/缺口径/缺文案 → FAIL | dev |
| R8 | `bash scripts/verify-2598.sh r8 > docs/evidence/2598/R8-build-lint.txt` | `docs/evidence/2598/R8-build-lint.txt` | exit 0 且含 `GO_BUILD_OK=1` `GOFMT_DIFF=0` `ESLINT_NOUNDEF=0` | PASS（三项 token 全为期望值） | 构建失败/gofmt 差异/eslint no-undef → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2598.sh`（`r1`–`r8`）；每个文本 artifact 首行：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，另起一行 `GENERATED_AT_HEAD=<40 位 sha>`；证据须在 fix commit **之后**生成（`FIX_COMMIT` 可覆盖锚点）。
- 逐条清单（全部位于 `docs/evidence/2598/`）：
  - `docs/evidence/2598/R1-classifier.txt`
  - `docs/evidence/2598/R2-transit-persistent.txt`
  - `docs/evidence/2598/R3-transit-transient.txt`
  - `docs/evidence/2598/R4-wx-paths.txt`
  - `docs/evidence/2598/R5-legacy-flip.txt`
  - `docs/evidence/2598/R6-frontend-map.txt`
  - `docs/evidence/2598/R7-docs.txt`
  - `docs/evidence/2598/R8-build-lint.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2598 --summary-file tmp/pre-ready-2598-summary.md --changed-files tmp/pre-ready-2598-changed.txt --evidence-file docs/evidence/2598/evidence.json --report docs/evidence/2598/pre-ready-2598.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Files（变更文件清单）

- `backend/services/iam_failure.go`
- `backend/services/iam_client.go`
- `backend/services/iam_failure_2598_test.go`
- `backend/handlers/iam_failure.go`
- `backend/handlers/transit_site_admin.go`
- `backend/handlers/user_staff.go`
- `backend/handlers/wechat_bind.go`
- `backend/handlers/transit_site_admin_2552_test.go`
- `backend/handlers/transit_site_admin_2598_test.go`
- `backend/handlers/wx_failure_2598_test.go`
- `backend/handlers/auth_test.go`
- `backend/handlers/wx_merge_2228_test.go`
- `frontend-pc/src/services/errorMessages.js`
- `frontend-mobile/src/services/errorMessages.js`
- `frontend-pc/tests/errorText-2598.test.mjs`
- `scripts/verify-2598.sh`
- `docs/spec/api/api.md`
- `docs/spec/ui/ui.md`
- `docs/evidence/2598/plan.md`
- `docs/evidence/2598/R1-classifier.txt`
- `docs/evidence/2598/R2-transit-persistent.txt`
- `docs/evidence/2598/R3-transit-transient.txt`
- `docs/evidence/2598/R4-wx-paths.txt`
- `docs/evidence/2598/R5-legacy-flip.txt`
- `docs/evidence/2598/R6-frontend-map.txt`
- `docs/evidence/2598/R7-docs.txt`
- `docs/evidence/2598/R8-build-lint.txt`
- `docs/evidence/2598/evidence.json`
- `docs/evidence/2598/pre-ready-2598.json`

### Dependencies

- 无

### Workload

- Size: L
- Basis: 后端 5 个实现文件（2 新增）、4 个测试文件（3 新增、2 处既有断言翻转/强化 + 1 处测试 harness 扩展）、前端 3 文件（双端码表 + 新单测）、2 份核心文档、1 个 8 行证据 harness；新增错误码 1 个（API 契约）、无 migration、无跨仓依赖
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 平台管理员创建中转网点失败只看到「请稍后再试」而实际必须改配置；员工/顾客绑微信或合并账户失败时前端按「数据冲突」处理（可能提示刷新/重试），运维侧看 409 也无法区分 IAM 抖动与真冲突 | #2598 正文、#2597 扫描表第 2–4 行 |
| 2 | 触发条件（trigger） | 上游 IAM 返回 4xx（父组织非法/权限模板缺失/命名空间不存在/授权码失效）或 IAM 不可达/超时/5xx | `iam_client.go:315-330`、beaconiam `handler.go:1892-2130` |
| 3 | 根因（root cause） | 失败分类缺失：`IAMClient` 不用类型化错误暴露上游状态，handler 只能把「所有非冲突失败」压成一个码（502 或 409），于是「持久」与「瞬时」在提示层不可分 | `transit_site_admin.go:127`、`user_staff.go:2271/2315`、`wechat_bind.go:296` |
| 4 | 测试盲点（test blind spot） | 既有测试只断言「失败不静默」（409/502 出现即可），**从未断言**状态类与文案可行动性；`IAMClient` 的错误串无类型断言 | `wx_merge_2228_test.go:136-150`、`auth_test.go:1220-1232` |
| 5 | 同类扫描（same-class paths） | `StatusConflict` + 「请稍后」交叉扫描命中共 3 处同类（`user_staff.go:2271/2315`、`wechat_bind.go:296`）；`transit_site_admin.go:127` 属「持久压平为瞬时」；`auth.go:487` 判为不同类（注册中止信号） | 本计划「同批扫描判定」表 |
| 6 | 修复层（fix layer） | 修在「services 类型化错误 + 分类器 → handlers 统一响应出口 → 四个调用点 → 双端码表/文案」四层；不碰业务成功路径与 `needs_merge` 状态机 | 变更点 1–4 |
| 7 | 红绿实证（red-green） | 红：修前 R2 期望 `50015` 实际 `50200`；R4 期望 `WX_TRANSIENT_STATUS=502` 实际 `409`；R5 期望 merge 400 → `400` 实际 `409`；R6 期望 `PC_MAP_50015=1` 实际 `0`。绿：修后全 token 成立；Work Summary 需贴修前 red 片段 | R2/R4/R5/R6 |
| 8 | 防复发（prevention） | 分类器表驱动覆盖全部状态类；负例断言「瞬时类不得 409」「持久/重做类不含『稍后再试』」；码表与后端常量漂移断言；禁止字符串匹配（只用 `errors.As`） | R1/R2/R3/R4/R6 |
| 9 | 影响面（blast radius） | 只改**失败分支**：成功路径、`needs_merge`（20000）、自合并 400、`transit_site_admin` 名冲突 409/40900、建站成功契约、PC 二维码绑定轮询均不变 | R4 的 `WXBIND_NEEDS_MERGE=1`、`transit_site_admin_2552_test.go` 其余 token |
| 10 | 未修边界（deferred） | `auth.go:487` 注册流程 409（不同类）、`transit_site_admin.go:387` 码/状态不一致、`getNamespaceID` 零命名空间分类、`50200` 全站码表（#2489 族）；真机回执 → `[Ops-Followup of #2598]`（`waitForConfirm`，无 `status:*`） | Out of Scope、D10 |

*Model: deepseek/deepseek-flash*

