## Accepted Plan — #2614 [Bug] 后端 IAM 失败三元组残留：iam_proxy.UpdateIAMUser 与 user_staff.UpdateCurrentUser

### 背景与现状核对（代码为据）

基线：`origin/issue-2601` @ `775872014`（#2601 已 `status:done`，其 fix/evidence 只在 `origin/issue-2601`，**未集成 `origin/develop`**，见 D1）。行号为该基线。

| # | 事实 | 位置 |
|---|---|---|
| 1 | `UpdateIAMUser`（`PUT /api/iam/users/:id`）把 IAM 任一失败压成 `500/50000` + `"Failed to update user in IAM: " + err.Error()`：上游 401/403/404（权限模板缺失、client/namespace 配错、账户不存在）这类**持久**错误一律显示「服务器开小差了，请稍后再试」→ 管理员被引导重试一件永远失败的事；同时把上游原始串拼进响应 | `backend/handlers/iam_proxy.go:601-608` |
| 2 | `UpdateCurrentUser`（`PUT /api/users/me`）把 IAM 任一失败压成 `409/40900` + `msg = err.Error()`（仅 `strings.ToLower(msg)` 含 `username already exists` 时换中文）：瞬时故障（IAM 不可达/超时/5xx）被表达成**数据冲突**，`code_map[40900]` =「数据冲突，请刷新后重试」与真实原因无关 | `backend/handlers/user_staff.go:1687-1695` |
| 3 | 统一出口已存在（#2598，唯一 owner）：`services.ClassifyIAMFailure`（只用 `errors.As`，支持 `*IAMAPIError` / `*IAMClientHTTPError`）+ `handlers.respondIAMFailure`（transient→`502/50200`、conflict→`409/40900`、userAction→`400/40002`、config→`500/50015` + `iamConfigErrorMessage`）；`Conflict` 留空时用 `iamConflictFallbackMessage` | `backend/services/iam_failure.go`、`backend/handlers/iam_failure.go` |
| 4 | `IAMClient.UpdateUser` 的非 2xx 已**类型化**（403/404/其余 → `*IAMClientHTTPError{Op,Status,Body}`），`errors.As` 可取上游状态 | `backend/services/iam_client.go:1295-1323`（#2601） |
| 5 | **既有冲突原因解析已存在**（#2290）：`iamConflictField(err)` 从 `{"error": ...}` 解析出 `username` / `phone` / `email`，供 `respondIAMCreateConflict` 使用（同包 `handlers`，`iam_proxy.go` 可直接调用，禁止另写一份解析） | `backend/handlers/user_staff.go:2383-2406` |
| 6 | beaconiam `PUT /api/v1/users/{id}` 的真实失败语义：`400 invalid request format`、`401 unauthorized`、`403 insufficient permissions` / `no namespace access` / `user not in scope`、`404 user not found`、`409 {"error":"username already exists"|"email already exists"|"phone already exists"}`、`500 failed to fetch user` | `/home/coder/beaconiam/internal/handler/user_handler.go:425-508` |
| 7 | `IAMClient.doRequest` 收到上游 401 会**清 token 并重试一次**（第二次仍 401 才返回） | `backend/services/iam_client.go:240-248` |
| 8 | 既有断言只覆盖成功路径与「409 中止本地写」，**从未**断言这两条失败分支的三元组；`grep -rn "Failed to update user in IAM" backend frontend-pc frontend-mobile docs` = 1 命中（仅实现行），无测试/文档依赖旧串 | `backend/handlers/iam_proxy_update_2249_test.go`、`backend/handlers/update_current_user_test.go:93` |
| 9 | 双端码表已登记 `50015`（#2598），`resolveErrorMessage` 层序 = message_map → 5xx 走 code_map → human_message → fallback；`50200` **未入码表**（#2598 明确口径）→ 瞬时类落调用点 fallback，仍不含「数据冲突」 | `frontend-pc/src/services/errorMessages.js:40`、`frontend-mobile/src/services/errorMessages.js:41` |
| 10 | 用户面消费者：weapp `EditProfile.jsx:154` 与移动端 `Profile.jsx:106` 都用 `resolveErrorMessage(result, '保存失败')`（CJK 文案透传）；PC `StaffEdit.jsx:94` 只在「邮箱变更」时调 `updateIAMUser` 且 catch 里落固定 warning（见 Out of Scope） | `frontend-mobile/src/pages-weapp/profile/edit/EditProfile.jsx`、`frontend-pc/src/pages/StaffEdit.jsx` |

**三元组口径（沿用 #2598，本单只扩适用端点，不新增错误码）**

| 失败类别 | 判据（只用 `errors.As` 取上游状态） | HTTP / code | 文案要求 |
|---|---|---|---|
| 瞬时 | 传输层失败（无上游状态）/ 上游 `429`、`5xx` | `502 / 50200` | 含「请稍后再试」 |
| 冲突 | 上游 `409` | `409 / 40900` | 指明冲突对象 + 可纠正动作 |
| 用户可重做 | 上游 `400` / `422` | `400 / 40002` | 指明需修改的输入，**不含**「稍后再试」 |
| 持久配置/权限 | 上游 `401` / `403` / `404` / 其余 4xx | `500 / 50015` | `iamConfigErrorMessage`（「身份服务配置异常，请联系平台管理员处理」），**不含**「稍后再试」 |

### 同批扫描判定（`IAMClient.UpdateUser` 的全部非测试调用点）

| # | 位置 | 判定 | 处置 |
|---|---|---|---|
| 1 | `backend/handlers/iam_proxy.go:601` `UpdateIAMUser` | `500/50000` + 原始串，持久失败被表达成可重试 | **本单修**（R1） |
| 2 | `backend/handlers/user_staff.go:1687` `UpdateCurrentUser` | `409/40900` + 原始串，瞬时故障被表达成冲突 | **本单修**（R2） |
| 3 | `backend/handlers/wechat_bind.go:373` `Unbind` | 已由 #2601 改为 `respondIAMFailure` 四类分流 | 保持（仅回归，不重做） |
| 4 | `backend/handlers/registration_helpers.go:81` `activateReservedUser` | 微信支付回调链（`wechatpay_callback.go:370` → `completeRegistrationFromSession`）：错误落 `registration_sessions.status/error` 供人工处理并回滚事务，**不产生用户响应三元组** | 保持（Out of Scope 记录理由） |
| 5 | `UpdateUserWithToken` 及 `IAMClient` 其余方法（`BindUserToOrganization` / `UpdateUserRoleInOrg` / `DeleteUser` …） | 不同方法族，尚未类型化；其调用点的 message 直出属 #2489 族与全站错误码收敛 | 保持（Out of Scope） |
| 6 | `user_staff.go` 其余 IAM 调用（`CreateUser` / `ListUsers` 等，40010/50000 分支） | 非 `UpdateUser` 路径，语义与判据不同（创建冲突已由 #2290 用 40910/40911/40913/40914 收敛） | 保持（Out of Scope） |

### 变更点

#### 1. 新增 `backend/handlers/iam_user_conflict.go` — 409 冲突文案细化（复用既有解析）

```go
package handlers

// #2614：IAM `PUT /api/v1/users/{id}` 的 409 → 冲突对象中文文案。
//
// 分类唯一 owner 仍是 #2598 的 services.ClassifyIAMFailure + respondIAMFailure；
// 本函数只做「冲突对象细化」，不参与 transient/userAction/config 判定。
// 判据：先用 errors.As 命中 *services.IAMClientHTTPError 且 Status == 409
// （禁止对整个 err.Error() 做子串匹配），再用既有的 iamConflictField(#2290)
// 取冲突字段；未知原因返回 ""，由调用点/统一出口的冲突文案兜底。
func iamUserUpdateConflictText(err error) string {
	var httpErr *services.IAMClientHTTPError
	if !errors.As(err, &httpErr) || httpErr == nil || httpErr.Status != http.StatusConflict {
		return ""
	}
	switch iamConflictField(err) {
	case "username":
		return "用户名已被占用，请换一个"
	case "email":
		return "邮箱已被其他账户使用，请更换后重试"
	case "phone":
		return "手机号已被其他账户使用，请更换后重试"
	default:
		return ""
	}
}
```

- 复用 `iamConflictField`（#2290）与 `*services.IAMClientHTTPError`（#2601）：**不新增** JSON body 解析、**不修改** `iam_failure.go` / `iam_failure.go` 的分类器语义。
- 「用户名已被占用，请换一个」与现状逐字一致（`update_current_user_2446_test.go:137` 与 `docs/spec/api/api.md:590` 均依赖该串）。

#### 2. `backend/handlers/iam_proxy.go:601-608` — `UpdateIAMUser` 接入统一出口

```go
if err := iamClient.UpdateUser(userID, iamReq); err != nil {
	log.Printf("[UpdateIAMUser] IAM UpdateUser failed for %s: %v", userID, err)
	conflictText := iamUserUpdateConflictText(err)
	if conflictText == "" {
		// 本端点只提交 name/email/phone/password，上游 409 只可能是邮箱/手机号占用。
		conflictText = "保存用户信息失败：邮箱或手机号已被其他账户占用，请修改后重试"
	}
	respondIAMFailure(c, err, iamFailureTexts{
		Transient:  "保存用户信息失败，请稍后再试",
		Conflict:   conflictText,
		UserAction: "保存用户信息失败：提交的内容格式不正确，请修改后重试",
	})
	return
}
```

- 删除 `500/50000` 与 `err.Error()` 拼接（诊断只进日志，`UPDATEIAM_RAW_LEAK=0`）。
- 本地缓存同步仍只在 IAM 成功之后（顺序不动，`UPDATEIAM_LOCAL_CACHE_KEPT=1`）。

#### 3. `backend/handlers/user_staff.go:1687-1695` — `UpdateCurrentUser` 接入统一出口

```go
if err := iamClient.UpdateUser(userID, iamReq); err != nil {
	log.Printf("[UpdateCurrentUser] IAM UpdateUser failed for %s: %v", userID, err)
	conflictText := iamUserUpdateConflictText(err)
	if conflictText == "" {
		conflictText = "保存失败：资料与现有账户冲突，请检查用户名/邮箱/手机号后重试"
	}
	respondIAMFailure(c, err, iamFailureTexts{
		Transient:  "保存失败，请稍后再试",
		Conflict:   conflictText,
		UserAction: "保存失败：资料格式不正确，请修改后重试",
	})
	return
}
```

- 删除 `msg := err.Error()`、`strings.Contains(strings.ToLower(msg), "username already exists")` 与硬编码 `409/40900`；`username already exists` 的**文案与码值逐字保持**（改由类型 + `iamConflictField` 判定）。
- 本地校验分支（用户名 4–32 位、密码规则、`id_photo_other_type` 枚举、`40903` 已认证不可改）**逐字不变**，仍在 IAM 调用之前。

#### 4. 新增 `backend/handlers/iam_user_update_failure_2614_test.go` — fake IAM 状态矩阵

- 复用既有夹具形态：`testfixtures.SetupTestDB` + `httptest.Server` + `services.SetIAMInternalURLForTesting` + `t.Setenv("IAM_SECRET", testIAMSecret)`；fake IAM 必须同时服务 `POST /api/v1/auth/token`（否则 `doRequest` 先取 token 时即失败，全部用例退化）与 `PUT /api/v1/users/{id}`；「不可达」由**已关闭的** `httptest.Server` 注入。
- `TestUpdateIAMUser2614_Triples`：上游 `400 / 401 / 403 / 404 / 409(email) / 409(phone) / 409(unknown) / 500 / 不可达` → 断言 `(status, code, message)`；负例「持久类不含『请稍后再试』」「瞬时类不得为 409」「message 不含上游原始串 / `50000`」；并断言失败时本地 `users.name` 未被改写。
- `TestUpdateCurrentUser2614_Triples`：同一矩阵 + `409 username`（文案逐字「用户名已被占用，请换一个」）+ 错误被 `fmt.Errorf("…: %w")` 包裹后仍命中类型判据；负例同上。
- 事件：受影响的既有测试（R3）同批跑通。

#### 5. 新增 `scripts/verify-2614.sh`

`r1`–`r5` 与 `gate`，逐行产出 Evidence Contract 需要的 token；沿用 #2601 的骨架（provenance 头 `# build=1.0.<short> # commit=<full> # dirty=<false>` + `GENERATED_AT_HEAD=<full>`；`FIX_COMMIT` 覆盖锚点；重命令经 `~/.config/opencode/scripts/heavy-run.sh`；`r5` 的编译冒烟为 `go test ./... -run '^$' -count=1`）。

#### 6. 文档（见 Core Update）

### 被翻转 / 被强化的既有断言（强制清单）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `backend/handlers/iam_proxy.go:601-608`（**无**既有断言；`grep` 仅实现行命中） | 任意 IAM 失败 → `500/50000` + `"Failed to update user in IAM: <原始串>"` | 四类三元组（`502/50200` / `409/40900` / `400/40002` / `500/50015`），message 不含原始串 | 持久失败不得表达成可重试（#2598 口径）；负例断言 `UPDATEIAM_LEGACY_50000_GONE=1` 证明旧行为不再发生 |
| `backend/handlers/user_staff.go:1687-1695`（**无**既有断言覆盖失败分支） | 任意 IAM 失败 → `409/40900` + `err.Error()` | 四类三元组；`409` 仅在真冲突，且为用户可纠正的中文文案 | 瞬时故障不得表达成数据冲突；负例断言 `UPDATECURRENT_TRANSIENT_NOT_CONFLICT=1` |
| `backend/handlers/update_current_user_test.go:93`（#1600 冲突中止） | 上游 409（mock body `phone already registered`）→ `409/40900`，本地不写 | 保持 `409/40900` + 本地不写；文案改为中文冲突兜底（mock body 不在已知 reason 表） | **强化不翻转**：断言只校验 status/code 与本地未改写 |
| `backend/handlers/update_current_user_2446_test.go:128-137` | 上游 409 + `{"error":"username already exists"}` → `409` + 含「用户名已被占用」 | 逐字保持 | 类型判据替换字符串 `Contains`，文案契约不变 |
| `backend/handlers/iam_proxy_update_2249_test.go:24` | 成功路径下零租户本地行被更新 | 保持 | 失败路径改造不得波及成功路径 |
| `backend/handlers/update_current_user_test.go:48`（mock body 漂移） | mock 返回 `phone already registered`，而 beaconiam 实际为 `phone already exists` | 不动既有 mock（断言与 reason 无关）；新用例使用真实 reason 串 | 记录漂移，避免把旧 mock 误当已知 reason |

### Core Update

`[Core Update Required]`

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §2.4.1 只有一行「`40002` 用户名格式/密码规则；`40900` 用户名冲突；`40104` 未登录」；§2.6 有 2.6.1–2.6.4，**无** `PUT /api/iam/users/:id` 章节；§14.4「适用端点」只列 transit-sites 与 4 个微信绑定端点 | ①§2.4.1 `PUT /api/users/me` 增「IAM 失败三元组（#2614）」表：`400/40002`（上游 400/422 →「保存失败：资料格式不正确，请修改后重试」）、`409/40900`（上游 username →「用户名已被占用，请换一个」；email →「邮箱已被其他账户使用，请更换后重试」；phone →「手机号已被其他账户使用，请更换后重试」；未知 → 冲突兜底文案）、`500/50015`（上游 401/403/404 → `iamConfigErrorMessage`）、`502/50200`（瞬时 →「保存失败，请稍后再试」）；②新增 `#### 2.6.5 更新 IAM 用户（PUT /api/iam/users/:id）`：请求字段 `name/email/phone/password`、成功响应（`20000` + `data.id/status`，邮箱变更追加 `data.email_confirmation="pending"`）、同四类失败表（`409` 细化 email/phone）、以及「`message` 不拼接上游原始串，诊断只进日志」；③§14.4「适用端点」补 `PUT /api/iam/users/:id`、`PUT /api/users/me` | 这两个端点的失败语义自此成为跨端契约；前端 `resolveErrorMessage` 依赖码值家族与 CJK 文案形态 |
| `docs/spec/ui/ui.md` | §2.6.2「编辑资料页」有 `#2598`、`#2601` 的绑定失败提示表，但**没有**资料保存（`PUT /users/me`）失败提示口径 | §2.6.2 增「资料保存失败提示（#2614）」表：`500/50015` → 「身份服务配置异常，请联系平台管理员处理」（**不得**出现「请稍后」「数据冲突」）；`502/50200` → 调用点 fallback「保存失败」（`50200` 未入码表，语义为可重试）；`400/40002` → 透传后端 CJK 文案；`409/40900` → 透传冲突对象中文文案（「用户名已被占用，请换一个」等）；并注明文案唯一来源 = `resolveErrorMessage`（`EditProfile.jsx:154` / `Profile.jsx:106`），页面不得自写业务文案 | 用户可见文案是 UI 契约；持久失败从「数据冲突，请刷新后重试」改为「身份服务配置异常…」是行为变化，必须落文档 |

### Dependencies

- #2601

### Workload

- Size: L
- Basis: 3 个后端实现文件（新增 1 个 helper + 2 处调用点）+ 1 个新测试文件 + 1 个证据 harness + 2 份核心文档 + 6 行合同/6 类证据产物；无 migration、无跨仓改动、无前端代码改动
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状 | 管理员保存 IAM 用户遇权限缺失/账户不存在时看到「服务器开小差了，请稍后再试」；用户保存本人资料遇 IAM 抖动看到「数据冲突，请刷新后重试」 | `iam_proxy.go:601-608`、`user_staff.go:1687-1695`、`ERROR_CODE_MAP[50000]/[40900]` |
| 2 | 触发条件 | 上游 IAM 返回 401/403/404（权限模板缺失、client/namespace 配错、账户已删）或不可达/超时/5xx；或用户改名/改手机/改邮箱命中 409 | beaconiam `user_handler.go:425-508`；`iam_client.go:1295-1323` |
| 3 | 根因 | 两处调用点在 #2598 收敛统一出口时**未接线**（#2601 的同类扫描记录了它们但归入 Out of Scope，且 Out of Scope 引用的 #2489 实为 PC 前端 toast 改造，后端无人修） | 本单来源段；`docs/evidence/2601/plan.md` Out of Scope |
| 4 | 测试盲点 | 既有测试只覆盖 `UpdateIAMUser` 成功（#2249）与 `UpdateCurrentUser` 的 409 中止/成功/用户名冲突，**从未**断言 400/401/403/404/5xx/不可达的三元组 | `iam_proxy_update_2249_test.go`、`update_current_user_test.go`、`update_current_user_2446_test.go` |
| 5 | 同类扫描 | `IAMClient.UpdateUser` 共 4 个非测试调用点：本单 2 处修复；`wechat_bind.go:373` 已由 #2601 收敛；`registration_helpers.go:81` 属支付回调链无用户三元组 | 本计划「同批扫描判定」表 |
| 6 | 修复层 | 修在「`services` 类型化状态（#2601，已具备）→ `respondIAMFailure` 统一出口（#2598，复用）→ 409 冲突对象细化（复用 `iamConflictField` #2290）→ 2 个调用点 → 文档契约」；不碰成功/本地缓存顺序 | 变更点 1–4 |
| 7 | 红绿实证 | 红：未修实现跑新测试，期望 `UPDATEIAM_403_CODE=50015` 实得 `50000`、`UPDATECURRENT_500_STATUS=502` 实得 `409`、`UPDATECURRENT_PERSISTENT_NO_RETRY=0` 实得 `1`。绿：修后全 token 成立 | R1/R2；Work Summary `### 红绿实证` |
| 8 | 防复发 | 每条路径 × 七种上游状态 + 不可达；负例「持久类不含『请稍后再试』」「瞬时类不得 409」「message 不含上游原始串/50000」；文档端点清单防止新增路径漏登记；`### 被翻转/被强化的既有断言` 防止旧断言固化 | R1–R4 的 negative 列 |
| 9 | 影响面 | 只改**失败分支**：`UpdateIAMUser` 成功路径（含 `email_confirmation=pending`）与 `UpdateCurrentUser` 成功路径（`relogin`、`email_confirmation`、本地缓存、`40903`、`40002` 本地校验）逐字不变 | R3 回归 token |
| 10 | 未修边界 | PC `StaffEdit.jsx` 对 `updateIAMUser` 的 catch 固定 warning、`UpdateUserWithToken` 及其余 IAMClient 方法类型化、`user_staff.go` 其它 IAM 调用点（40010/50000）、`registration_helpers.go` 回调链、全站 `message` 直出清理 → 均在 Out of Scope 记录理由 | Out of Scope |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 `UpdateIAMUser` 失败出口接入统一三元组（含 409 冲突对象细化） | `backend/handlers/iam_proxy.go`、`backend/handlers/iam_user_conflict.go`、`backend/handlers/iam_user_update_failure_2614_test.go` | `PUT /api/iam/users/:id` | fake IAM `PUT /api/v1/users/{id}` → 400 / 401 / 403 / 404 / 409(email) / 409(phone) / 409(unknown) / 500 / 不可达 | backend（Postgres 测试库 + httptest） | `UPDATEIAM_400_CODE=40002`、`UPDATEIAM_401_CODE=50015`、`UPDATEIAM_403_STATUS=500`、`UPDATEIAM_403_CODE=50015`、`UPDATEIAM_404_CODE=50015`、`UPDATEIAM_409_EMAIL_CODE=40900`、`UPDATEIAM_409_EMAIL_TEXT=1`、`UPDATEIAM_409_PHONE_TEXT=1`、`UPDATEIAM_409_UNKNOWN_CODE=40900`、`UPDATEIAM_500_STATUS=502`、`UPDATEIAM_500_CODE=50200`、`UPDATEIAM_DEAD_CODE=50200`、`UPDATEIAM_PERSISTENT_NO_RETRY=0`、`UPDATEIAM_RAW_LEAK=0`、`UPDATEIAM_LEGACY_50000_GONE=1`、`UPDATEIAM_LOCAL_CACHE_KEPT=1` | R1 |
| R2 `UpdateCurrentUser` 失败出口接入统一三元组（username 文案逐字保持） | `backend/handlers/user_staff.go`、`backend/handlers/iam_user_conflict.go`、`backend/handlers/iam_user_update_failure_2614_test.go` | `PUT /api/users/me` | fake IAM `PUT /api/v1/users/{id}` → 400 / 401 / 403 / 404 / 409(username) / 409(email) / 409(phone) / 500 / 不可达；错误被 `%w` 包裹 | backend（Postgres 测试库 + httptest） | `UPDATECURRENT_400_CODE=40002`、`UPDATECURRENT_401_CODE=50015`、`UPDATECURRENT_403_STATUS=500`、`UPDATECURRENT_403_CODE=50015`、`UPDATECURRENT_404_CODE=50015`、`UPDATECURRENT_409_USERNAME_STATUS=409`、`UPDATECURRENT_409_USERNAME_CODE=40900`、`UPDATECURRENT_409_USERNAME_TEXT_KEPT=1`、`UPDATECURRENT_409_EMAIL_TEXT=1`、`UPDATECURRENT_409_PHONE_TEXT=1`、`UPDATECURRENT_500_STATUS=502`、`UPDATECURRENT_500_CODE=50200`、`UPDATECURRENT_DEAD_CODE=50200`、`UPDATECURRENT_PERSISTENT_NO_RETRY=0`、`UPDATECURRENT_RAW_LEAK=0`、`UPDATECURRENT_TRANSIENT_NOT_CONFLICT=1`、`UPDATECURRENT_LOCAL_CACHE_KEPT=1`、`UPDATECURRENT_WRAPPED_TYPED=1` | R2 |
| R3 既有断言回归（#1600 / #2249 / #2446 / IAMMock 成功路径；既有测试文件本身不修改） | `scripts/verify-2614.sh`、`backend/handlers/iam_user_update_failure_2614_test.go` | `go test ./handlers` 定向 6 个用例 | 修后工作树 | backend | `REGRESSION_CONFLICT_ABORT=1`、`REGRESSION_SUCCESS_CACHE=1`、`REGRESSION_2446_USERNAME_TEXT=1`、`REGRESSION_2446_CREDENTIALS=1`、`REGRESSION_IAMMOCK_EMAIL=1`、`REGRESSION_ZERO_TENANT=1` | R3 |
| R4 契约文档 + 双端码表登记（静态） | `docs/spec/api/api.md`、`docs/spec/ui/ui.md` | 文档结构断言（`grep`） | 修后文档 | 文档 | `API_IAM_UPDATE_ENDPOINT=1`、`API_IAM_UPDATE_FAILURE_ROWS=1`、`API_ME_UPDATE_FAILURE_ROWS=1`、`API_TRIPLE_APPLIES_ENDPOINTS=1`、`DOC_CONFIG_TEXT_KEPT=1`、`UI_ME_SAVE_FAILURE_ROWS=1`、`FE_ERROR_MAP_50015_PC=1`、`FE_ERROR_MAP_50015_MOBILE=1` | R4 |
| R5 构建 / gofmt / 全包编译冒烟 | `scripts/verify-2614.sh`、`backend/handlers/iam_proxy.go`、`backend/handlers/user_staff.go`、`backend/handlers/iam_user_conflict.go` | `cd backend && go build ./...`、`gofmt -l`、`go test ./... -run '^$'` | 修后工作树 | backend | `GO_BUILD_OK=1`、`GOFMT_DIFF=0`、`GO_COMPILE_ALL_OK=1` | R5 |
| R6 pre-ready gate（四源 + 合同完整性 + 远端可达） | `docs/evidence/2614/pre-ready-2614.json` | `pre-ready-check.sh 2614` | 修后干净树 + 最新 Accepted Plan | backend | exit 0、`overall=PASS`、P12 每行 `pass=true`、P13 提交在 `origin/issue-2614` | R6 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | integration | handler + fake IAM + Postgres 测试库；断言 `(status, code, message)` 三元组与负例 |
| R2 | integration | 同上（`PUT /api/users/me`），含 username 文案逐字保持与 `%w` 包裹命中 |
| R3 | integration | 既有 6 个用例的 `--- PASS` 逐项判定 |
| R4 | static | 文档结构 + 双端码表登记断言（无 UI 代码改动，故无 render 行） |
| R5 | build | `go build ./...` + `gofmt -l` + 全包编译冒烟 |
| R6 | gate | `pre-ready-check.sh` 报告（P1–P17） |

### Input Truth & Tool Preflight

- **Source of truth**：
  - 失败分类唯一 owner = `services.ClassifyIAMFailure` + `handlers.respondIAMFailure`（#2598，**本单不复制、不改语义**）；
  - 上游状态唯一来源 = `*services.IAMClientHTTPError.Status`（`iam_client.go`，#2601 已类型化）；
  - 冲突对象唯一解析 = `iamConflictField`（`user_staff.go:2383`，#2290），**禁止**另写 body 解析；
  - 上游 409 原因串事实 = beaconiam `internal/handler/user_handler.go:489/498/508`（`username|email|phone already exists`）；
  - 用户文案唯一 owner = `frontend-pc`/`frontend-mobile` 的 `errorMessages.js`（码表/解析层），页面不得自写业务文案；
  - 契约事实 = `docs/spec/api/api.md` §14.4（#2598 三元组口径）、§2.4.1、新增 §2.6.5；`docs/spec/ui/ui.md` §2.6.2；
  - 路由事实 = `backend/main.go:303`（`authRequired.PUT("/iam/users/:id")`）、`PUT /api/users/me`（`user_staff.go:1632`）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `git merge-base --is-ancestor origin/issue-2601 origin/develop; echo $?` → 期望非 0（未集成）⇒ 按 D1 用 `--base origin/issue-2601`；
  - `grep -q "func respondIAMFailure" backend/handlers/iam_failure.go && grep -q "IAMClientHTTPError" backend/services/iam_client.go && grep -q "func iamConflictField" backend/handlers/user_staff.go` → 三者必须成立，否则 **HOLD**（D1/D2：不得自行实现分类器或 body 解析）；
  - `grep -n "Failed to update user in IAM" backend/handlers/iam_proxy.go` → 修前 1 命中（604）；修后 0 命中；
  - `grep -n "strings.Contains(strings.ToLower(msg), \"username already exists\")" backend/handlers/user_staff.go` → 修前 1 命中（1690）；修后 0 命中；
  - `grep -rn "iamUserUpdateConflictText" backend` → 修前 0 命中（本单新增）；
  - `grep -rn "Failed to update user in IAM" backend --include=*_test.go` → 0（无既有断言依赖旧串）。
- **损坏输入拒绝规则**：
  - `respondIAMFailure(nil)` 不得出现（调用点必须先判 `err != nil`）；
  - `iamUserUpdateConflictText` 对非 409 / 非 `*IAMClientHTTPError`（如传输层 `fmt.Errorf` 包装）必须返回 `""`，禁止误判成冲突；
  - fake IAM 返回非 JSON body → 409 仍按状态分类（`iamConflictField` 解析失败返回 `""` → 走本地冲突兜底文案），不得退化成 500 或 200；
  - `*IAMClientHTTPError` 被 `%w` 包裹 → `errors.As` 仍须命中（R2 覆盖）。
- **工具盲区**：
  - fake IAM 只证明我方分类与响应，不证明 beaconiam 侧 reason 串演化（新增 reason 会落本地冲突兜底文案，不会泄漏 ASCII）；
  - `doRequest` 的 401 单次重试使 401 用例需要 fake 同时稳定服务 token 端点（R1/R2 已覆盖）；
  - 测试库依赖 `POSTGRES_PORT=15433`（`docs/ops/test-db-lifecycle.md`）；不可用时 `testfixtures.SetupTestDB` 返回 nil → 用例必须 `t.Fatal`（**禁止**静默 skip 成假绿）。
- **契约影响**：`docs/spec/api/api.md` 变更 → Work Summary 必须含 `### 契约三向核对`（pre-ready P7）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 依赖基线：`#2601` 已 `done`，其 fix/evidence 仅在 `origin/issue-2601`，未集成 `origin/develop`。执行时先判 `git merge-base --is-ancestor origin/issue-2601 origin/develop`：假 → `worktree-create.sh --project tuneloop --issue 2614 --base origin/issue-2601`；真 → 默认 `origin/develop`。基线缺 `respondIAMFailure` / `IAMClientHTTPError` / `iamConflictField` → **HOLD**，不得自行实现 | 本计划锁定 |
| D2 | 三元组沿用 #2598 口径（`502/50200`、`409/40900`、`400/40002`、`500/50015` + `iamConfigErrorMessage`），**不新增错误码**，不改 `ClassifyIAMFailure` / `respondIAMFailure` 语义，不修改 `iam_failure.go` | 本计划锁定 |
| D3 | 409 冲突对象判定 = `errors.As(*IAMClientHTTPError)` + `Status==409` 后复用 `iamConflictField`（#2290）；**禁止**对整个 `err.Error()` 做子串匹配，**禁止**新增第二份 body 解析 | 本计划锁定 |
| D4 | `username already exists` → `409/40900` +「用户名已被占用，请换一个」**逐字保持**（`update_current_user_2446_test.go:137`、`api.md:590` 依赖该串），仅把判据从字符串 `Contains` 换成类型 + `iamConflictField` | 本计划锁定 |
| D5 | 未知 409 reason：`UpdateIAMUser` 用「邮箱或手机号已被其他账户占用，请修改后重试」（该端点不提交 username）；`UpdateCurrentUser` 用「保存失败：资料与现有账户冲突，请检查用户名/邮箱/手机号后重试」。两者都必须指明可纠正动作，不得回退到「请稍后再试」 | 本计划锁定 |
| D6 | 不改变成功/本地缓存语义：`UpdateIAMUser` 的 `email_confirmation=pending`、本地 `users` 更新（`iam_sub` 过滤，#2249）、`UpdateCurrentUser` 的 `relogin` / `email_confirmation` / `40903` / 本地校验 `40002` 逐字不变 | 本计划锁定 |
| D7 | 判据分层：`400` 在**本端点语义**下归 userAction（payload 直接来自用户输入），不使用 `UserActionAsConfig` | 本计划锁定 |
| D8 | 前端不改代码：双端码表已登记 `50015`（#2598）；`50200` 不入码表（#2598 口径）→ 瞬时类落调用点 fallback。`StaffEdit.jsx` 的 catch 固定 warning 属 #2489 族，不在本单 | 本计划锁定 |
| D9 | 重命令（`go build`/`go test`）经 `scripts/heavy-run.sh` 取锁；合同行禁止把全量 `go test` 当 exit 0 合同（P5 baseline 差分承担）；`go test ./... -run '^$'` 编译冒烟允许作为 harness 内部步骤 | 本计划锁定 |
| D10 | 并行交叉：无。若执行期发现 `iam_proxy.go` / `user_staff.go` 的这两处 hunk 已被他人改动（内容与计划不符）→ HOLD 并回 Architect，禁止就地改写他人语义 | 本计划锁定 |

### Out of Scope

- PC `frontend-pc/src/pages/StaffEdit.jsx:94-101`：`updateIAMUser` 失败被 catch 成固定 `message.warning('用户更新成功，但邮箱变更请求发送失败')`，新三元组在该页面不会展示给用户；PC toast/提示统一属 **#2489 族**（前端 6 页 `<FormConflictAlert>` 改造），本单不改前端代码（D8）。
- `UpdateUserWithToken` 及其余 `IAMClient` 方法（`BindUserToOrganization` / `UpdateUserRoleInOrg` / `DeleteUser` 等）的类型化与三元组：非本单 `UpdateUser` 路径。
- `user_staff.go` 其余 IAM 调用点（`CreateUser` 40010 / 其它 50000 分支）：不同方法族，创建冲突已由 #2290 用 40910/40911/40913/40914 收敛。
- `registration_helpers.go:81`（`activateReservedUser`）：微信支付回调链，错误落 `registration_sessions.status/error` 供人工处理并回滚事务，无用户响应三元组。
- 全站 `message` 直出清理、`50200` 入前端码表、`c.ShouldBindJSON` 英文原文族（#2603/#2604/#2605/#2608）：属 #2489 族与后续全站错误码收敛。
- `backend/handlers/registration_session.go:529`（`403 + code 40902`）：码/状态家族不一致属另一轴，且被 `id_photo_gate_2039_test.go` 断言锁定，需单独评估（客户端可能依赖 403）。
- `docs/en/api.md` 英文版同步：该文件最后更新 2026-05-03，已与 `docs/spec/api/api.md` 结构性脱节（#2598/#2601 均未同步），英文文档重建另立任务。
- 全量 `go test ./...` 作为合同行（由 pre-ready P5 baseline 差分承担；编译冒烟 `-run '^$'` 已覆盖全包类型检查）。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `testfixtures.SetupTestDB` + `users` 行 `{iam_sub: <id>, name:"旧名字"}`；fake IAM 服务 `POST /api/v1/auth/token` 且 `PUT /api/v1/users/{id}` 返回 400/401/403/404/409(email)/409(phone)/409(unknown)/500，另以关闭的 server 表达不可达 | `PUT /api/iam/users/:id`（`TestUpdateIAMUser2614_Triples`） | `docs/evidence/2614/R1-iam-update-triples.txt` | `UPDATEIAM_400_CODE=40002`、`UPDATEIAM_401_CODE=50015`、`UPDATEIAM_403_STATUS=500`、`UPDATEIAM_403_CODE=50015`、`UPDATEIAM_404_CODE=50015`、`UPDATEIAM_409_EMAIL_CODE=40900`、`UPDATEIAM_409_PHONE_TEXT=1`、`UPDATEIAM_409_UNKNOWN_CODE=40900`、`UPDATEIAM_500_STATUS=502`、`UPDATEIAM_500_CODE=50200`、`UPDATEIAM_DEAD_CODE=50200`、`UPDATEIAM_PERSISTENT_NO_RETRY=0`、`UPDATEIAM_RAW_LEAK=0`、`UPDATEIAM_LEGACY_50000_GONE=1`、`UPDATEIAM_LOCAL_CACHE_KEPT=1` | 403 仍 500/50000（`LEGACY_50000_GONE=0`）、持久类含「请稍后再试」、瞬时类 409、message 含 `Failed to update user in IAM` 或 `returned status`、失败后本地 `users.name` 被改写 → FAIL |
| R2 | 同上 DB，`users` 行 `{iam_sub: <id>, username:"13800002614"}`；fake IAM 矩阵含 409 的三种已知 reason；另一用例把错误用 `fmt.Errorf("wrap: %w")` 包裹后判定 | `PUT /api/users/me`（`TestUpdateCurrentUser2614_Triples`） | `docs/evidence/2614/R2-me-update-triples.txt` | `UPDATECURRENT_400_CODE=40002`、`UPDATECURRENT_401_CODE=50015`、`UPDATECURRENT_403_CODE=50015`、`UPDATECURRENT_404_CODE=50015`、`UPDATECURRENT_409_USERNAME_STATUS=409`、`UPDATECURRENT_409_USERNAME_CODE=40900`、`UPDATECURRENT_409_USERNAME_TEXT_KEPT=1`、`UPDATECURRENT_409_EMAIL_TEXT=1`、`UPDATECURRENT_409_PHONE_TEXT=1`、`UPDATECURRENT_500_STATUS=502`、`UPDATECURRENT_500_CODE=50200`、`UPDATECURRENT_DEAD_CODE=50200`、`UPDATECURRENT_PERSISTENT_NO_RETRY=0`、`UPDATECURRENT_RAW_LEAK=0`、`UPDATECURRENT_TRANSIENT_NOT_CONFLICT=1`、`UPDATECURRENT_LOCAL_CACHE_KEPT=1`、`UPDATECURRENT_WRAPPED_TYPED=1` | 瞬时类返回 409、持久类含「请稍后再试」、`username already exists` 文案漂移、message 含上游原始串、`%w` 包裹后落 502、失败后本地 `users` 被改写 → FAIL |
| R3 | 修后工作树（同一测试库） | `go test ./handlers -run 'TestUpdateCurrentUser_IAMConflict_AbortsLocalUpdate|TestUpdateCurrentUser_IAMSuccess_UpdatesLocalCache|TestUpdateCurrentUser_2446_|TestIAMMock_UserUpdate_EmailChange_ReturnsPending|TestIAMProxy_UpdateIAMUser_ZeroTenant_2249' -v` | `docs/evidence/2614/R3-regression.txt` | `REGRESSION_CONFLICT_ABORT=1`、`REGRESSION_SUCCESS_CACHE=1`、`REGRESSION_2446_USERNAME_TEXT=1`、`REGRESSION_2446_CREDENTIALS=1`、`REGRESSION_IAMMOCK_EMAIL=1`、`REGRESSION_ZERO_TENANT=1` | 任一既有用例 `--- FAIL` 或被测试过滤掉（计数不足）→ FAIL |
| R4 | 修后 `docs/spec/api/api.md` 与 `docs/spec/ui/ui.md` + 双端 `errorMessages.js` | `bash scripts/verify-2614.sh r4` | `docs/evidence/2614/R4-docs.txt` | `API_IAM_UPDATE_ENDPOINT=1`、`API_IAM_UPDATE_FAILURE_ROWS=1`、`API_ME_UPDATE_FAILURE_ROWS=1`、`API_TRIPLE_APPLIES_ENDPOINTS=1`、`DOC_CONFIG_TEXT_KEPT=1`、`UI_ME_SAVE_FAILURE_ROWS=1`、`FE_ERROR_MAP_50015_PC=1`、`FE_ERROR_MAP_50015_MOBILE=1` | 端点章节缺失、失败表缺行、§14.4 适用端点未登记、`50015` 文案与后端不一致、双端码表缺 50015 → FAIL |
| R5 | 修后工作树 | `bash scripts/verify-2614.sh r5` | `docs/evidence/2614/R5-build.txt` | `GO_BUILD_OK=1`、`GOFMT_DIFF=0`、`GO_COMPILE_ALL_OK=1` | 构建失败、gofmt 差异、任一包编译失败 → FAIL |
| R6 | 修后干净树 + 最新 Accepted Plan + `docs/evidence/2614/evidence.json`（`dirty=false`） | `pre-ready-check.sh 2614` | `docs/evidence/2614/pre-ready-2614.json` | exit 0、`overall=PASS`、P12 每行 `pass=true`、P13 提交在 `origin/issue-2614` | 任一 P 项失败、evidence 缺行、`dirty=true` → gate 非 0，禁止 ready |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2614.sh r1 > docs/evidence/2614/R1-iam-update-triples.txt` | `docs/evidence/2614/R1-iam-update-triples.txt` | exit 0 且含 `UPDATEIAM_400_CODE=40002`、`UPDATEIAM_401_CODE=50015`、`UPDATEIAM_403_STATUS=500`、`UPDATEIAM_403_CODE=50015`、`UPDATEIAM_404_CODE=50015`、`UPDATEIAM_409_EMAIL_CODE=40900`、`UPDATEIAM_409_EMAIL_TEXT=1`、`UPDATEIAM_409_PHONE_TEXT=1`、`UPDATEIAM_409_UNKNOWN_CODE=40900`、`UPDATEIAM_500_STATUS=502`、`UPDATEIAM_500_CODE=50200`、`UPDATEIAM_DEAD_CODE=50200`、`UPDATEIAM_PERSISTENT_NO_RETRY=0`、`UPDATEIAM_RAW_LEAK=0`、`UPDATEIAM_LEGACY_50000_GONE=1`、`UPDATEIAM_LOCAL_CACHE_KEPT=1` | 十六项 token 全为期望值 | 403 仍 50000、持久类含「请稍后再试」、瞬时类 409、message 含上游原始串、失败后本地缓存被改写 → FAIL | dev |
| R2 | `bash scripts/verify-2614.sh r2 > docs/evidence/2614/R2-me-update-triples.txt` | `docs/evidence/2614/R2-me-update-triples.txt` | exit 0 且含 `UPDATECURRENT_400_CODE=40002`、`UPDATECURRENT_401_CODE=50015`、`UPDATECURRENT_403_STATUS=500`、`UPDATECURRENT_403_CODE=50015`、`UPDATECURRENT_404_CODE=50015`、`UPDATECURRENT_409_USERNAME_STATUS=409`、`UPDATECURRENT_409_USERNAME_CODE=40900`、`UPDATECURRENT_409_USERNAME_TEXT_KEPT=1`、`UPDATECURRENT_409_EMAIL_TEXT=1`、`UPDATECURRENT_409_PHONE_TEXT=1`、`UPDATECURRENT_500_STATUS=502`、`UPDATECURRENT_500_CODE=50200`、`UPDATECURRENT_DEAD_CODE=50200`、`UPDATECURRENT_PERSISTENT_NO_RETRY=0`、`UPDATECURRENT_RAW_LEAK=0`、`UPDATECURRENT_TRANSIENT_NOT_CONFLICT=1`、`UPDATECURRENT_LOCAL_CACHE_KEPT=1`、`UPDATECURRENT_WRAPPED_TYPED=1` | 十八项 token 全为期望值 | 瞬时类 409、持久类含「请稍后再试」、username 文案漂移、原始串外溢、`%w` 包裹后落 502、本地缓存被改写 → FAIL | dev |
| R3 | `bash scripts/verify-2614.sh r3 > docs/evidence/2614/R3-regression.txt` | `docs/evidence/2614/R3-regression.txt` | exit 0 且含 `REGRESSION_CONFLICT_ABORT=1`、`REGRESSION_SUCCESS_CACHE=1`、`REGRESSION_2446_USERNAME_TEXT=1`、`REGRESSION_2446_CREDENTIALS=1`、`REGRESSION_IAMMOCK_EMAIL=1`、`REGRESSION_ZERO_TENANT=1` | 六项 token 全为期望值 | 任一既有用例 FAIL 或未被选中执行 → FAIL | dev |
| R4 | `bash scripts/verify-2614.sh r4 > docs/evidence/2614/R4-docs.txt` | `docs/evidence/2614/R4-docs.txt` | exit 0 且含 `API_IAM_UPDATE_ENDPOINT=1`、`API_IAM_UPDATE_FAILURE_ROWS=1`、`API_ME_UPDATE_FAILURE_ROWS=1`、`API_TRIPLE_APPLIES_ENDPOINTS=1`、`DOC_CONFIG_TEXT_KEPT=1`、`UI_ME_SAVE_FAILURE_ROWS=1`、`FE_ERROR_MAP_50015_PC=1`、`FE_ERROR_MAP_50015_MOBILE=1` | 八项 token 全为期望值 | 章节/失败行/端点清单/码表任一缺失或 `50015` 文案不一致 → FAIL | dev |
| R5 | `bash scripts/verify-2614.sh r5 > docs/evidence/2614/R5-build.txt` | `docs/evidence/2614/R5-build.txt` | exit 0 且含 `GO_BUILD_OK=1`、`GOFMT_DIFF=0`、`GO_COMPILE_ALL_OK=1` | 三项 token 全为期望值 | 构建失败、gofmt 差异、任一包编译失败 → FAIL | dev |
| R6 | `~/.config/opencode/scripts/pre-ready-check.sh 2614 --summary-file docs/evidence/2614/summary.md --changed-files docs/evidence/2614/changed-files.txt --evidence-file docs/evidence/2614/evidence.json --affected-cmd "bash scripts/verify-2614.sh gate" --report docs/evidence/2614/pre-ready-2614.json` | `docs/evidence/2614/pre-ready-2614.json` | exit 0 且 `jq -e '.overall == "PASS"' docs/evidence/2614/pre-ready-2614.json` 成立，P12 对 `docs/evidence/2614/evidence.json` 每行判定 `pass=true` | `overall=PASS` 且证据行全 `pass=true` | 任一 P 项失败、evidence 缺行、`dirty=true` → gate 非 0，禁止 ready | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2614.sh`（`r1`–`r5` 见合同；`gate` 供 `--affected-cmd` 复用 `go build ./...` + 三条定向测试 + 编译冒烟）。
- 每个文本 artifact 首行：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，另起一行 `GENERATED_AT_HEAD=<40 位 sha>`；锚点为本单 fix commit（`FIX_COMMIT` 可覆盖），证据必须在 fix commit **之后**生成。
- 逐条清单（全部位于 `docs/evidence/2614/`）：
  - `docs/evidence/2614/R1-iam-update-triples.txt`
  - `docs/evidence/2614/R2-me-update-triples.txt`
  - `docs/evidence/2614/R3-regression.txt`
  - `docs/evidence/2614/R4-docs.txt`
  - `docs/evidence/2614/R5-build.txt`
  - `docs/evidence/2614/pre-ready-2614.json`
  - `docs/evidence/2614/evidence.json`
  - `docs/evidence/2614/summary.md`
  - `docs/evidence/2614/changed-files.txt`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Files（变更文件清单）

- `backend/handlers/iam_user_conflict.go`
- `backend/handlers/iam_proxy.go`
- `backend/handlers/user_staff.go`
- `backend/handlers/iam_user_update_failure_2614_test.go`
- `scripts/verify-2614.sh`
- `docs/spec/api/api.md`
- `docs/spec/ui/ui.md`
- `docs/evidence/2614/plan.md`
- `docs/evidence/2614/R1-iam-update-triples.txt`
- `docs/evidence/2614/R2-me-update-triples.txt`
- `docs/evidence/2614/R3-regression.txt`
- `docs/evidence/2614/R4-docs.txt`
- `docs/evidence/2614/R5-build.txt`
- `docs/evidence/2614/evidence.json`
- `docs/evidence/2614/pre-ready-2614.json`
- `docs/evidence/2614/summary.md`
- `docs/evidence/2614/changed-files.txt`

*Model: DeepSeek V4.1 Flash 1017*

