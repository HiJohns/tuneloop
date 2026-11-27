## Accepted Plan — #2601 [Bug] wechat_bind.go ConfirmBind/Unbind 持久 IAM 失败仍表达为 500/50000「稍后再试」

### 背景与现状核对（代码为据）

基线：`origin/issue-2598` @ `1e207b9a8`（#2598 已 `status:done`，fix `18c81bd36` **尚未集成到 `origin/develop`**，见 D1）。行号为该基线 `backend/handlers/wechat_bind.go`。

| # | 事实 | 位置 |
|---|---|---|
| 1 | `ConfirmBind` 把 `WxBind` 的**任何**失败（含 IAM 403 权限/模板缺失、404 client/user not found 这类**持久**配置错误，以及上游 400 exchange_token 失效）写成 `500/50000` +「failed to bind: + 上游原始串」：PC/mobile `ERROR_CODE_MAP[50000]` =「服务器开小差了，请稍后再试」→ 用户被告知重试一件永远失败的事 | `wechat_bind.go:222` |
| 2 | `Unbind` 同构：`iamClient.UpdateUser` 任一失败 → `500/50000` +「unbind failed: + 上游原始串」 | `wechat_bind.go:381` |
| 3 | `MergeBind` 的 token 状态前置校验用 `400 + code 40902`：`409xx` 冲突码配 `400`（码/状态家族不一致），且 message 为 ASCII 内部状态串（`token is not waiting for merge (status=<state>)`），4xx 下前端 `human_message` 不透传北京话 → 用户只看到 fallback | `wechat_bind.go:274` |
| 4 | `Unbind` 所用 `IAMClient.UpdateUser` 的非 2xx 分支**未类型化**（403 → `permission denied: %s`、404 → `user not found: %s`、其余 → `UpdateUser returned status %d: %s`），`services.ClassifyIAMFailure` 无法用 `errors.As` 取状态 → 换成统一出口后仍会退化成分不清的瞬时类 | `backend/services/iam_client.go:1295-1317`（403/404/其余分支 1305-1313） |
| 5 | #2598 已交付统一出口：`services.ClassifyIAMFailure`（只用 `errors.As`，支持 `*IAMAPIError` / `*IAMClientHTTPError`）与 `handlers.respondIAMFailure`（transient→`502/50200`、conflict→`409/40900`、userAction→`400/40002`、config→`500/50015` + `iamConfigErrorMessage`） | `backend/services/iam_failure.go`、`backend/handlers/iam_failure.go` |
| 6 | `ConfirmBind` 的 `needs_merge` 分支（`errors.As(&*services.WxNeedsMergeError)` → `409/40900` + `data.needs_merge/merge_ticket` + token 置 `needs_merge`）与 `MergeBind` 的幂等 `merged` 分支是既有契约，**不得**进入失败分类 | `wechat_bind.go:196-221`、`wechat_bind.go:262-270` |
| 7 | 真实路由：`POST /api/wechat-bind/confirm`、`POST /api/wechat-bind/merge`、`POST /api/users/me/wechat-unbind`（**不存在** `/api/wechat-bind/unbind`；Issue 正文该路径为笔误，以 `main.go:277-283`、`main.go:901` 为准） | `backend/main.go` |
| 8 | 唯一消费者：weapp `Bind.jsx` 只判 `code===20000` / `code===40900 && data.needs_merge`，其余一律落 `status='error'` 且**硬编码**「请重试」（`merge_error` 硬编码「合并凭证可能已过期…」）→ 50015（持久配置）也被表达成可重试 | `frontend-mobile/src/pages-weapp/bind/Bind.jsx:61-108,195-213` |
| 9 | 双端码表已登记 `50015`（#2598），mobile 解析层 `resolveErrorMessage(result, fallback)` 的层序为 message_map → human_message → code_map → fallback | `frontend-mobile/src/services/errorMessages.js`、`errorMessageResolution.mjs` |
| 10 | 既有渲染测试档位可直接复用：`scripts/page-smoke/bind-needs-merge.test.jsx` 用 `@testing-library/react` 的 `render(` / `waitFor` / `fireEvent` 渲染同一 `Bind.jsx`（`vitest.config.js` include = `scripts/page-smoke/**/*.test.jsx`） | `frontend-mobile/scripts/page-smoke/`、`frontend-mobile/vitest.config.js` |

**三元组口径（沿用 #2598，本单只扩适用端点，不新增码）**

| 失败类别 | 判据（只用 `errors.As` 取上游状态） | HTTP / code | 文案要求 |
|---|---|---|---|
| 瞬时 | 传输层失败（无上游状态）/ 上游 `429`、`5xx` | `502 / 50200` | 含「请稍后再试」 |
| 冲突 | 上游 `409` | `409 / 40900` | 指明冲突对象 + 可纠正动作 |
| 用户可重做 | 上游 `400` / `422` | `400 / 40002` | 指明用户动作（重新扫码/重新进入），**不含**「稍后再试」 |
| 持久配置/权限 | 上游 `401` / `403` / `404` / 其余 4xx | `500 / 50015` | `iamConfigErrorMessage`（「身份服务配置异常，请联系平台管理员处理」），**不含**「稍后再试」 |

### 同批扫描判定（同类路径全量，`grep -rn "StatusInternalServerError\|StatusConflict" backend/handlers/wechat_bind.go` + 「请稍后」交叉扫）

| # | 位置 | 判定 | 处置 |
|---|---|---|---|
| 1 | `wechat_bind.go:222` `ConfirmBind` | `WxBind` 任意失败压成 `500/50000` + 原始串 | **本单修**（R1） |
| 2 | `wechat_bind.go:381` `Unbind` | `UpdateUser` 任意失败压成 `500/50000` + 原始串 | **本单修**（R2，含项 3 的类型化前置） |
| 3 | `wechat_bind.go:274` `MergeBind` 状态前置 | `400/40902` 码状态家族不一致 + 内部状态串 | **本单修**（R3） |
| 4 | `wechat_bind.go:196-221` `ConfirmBind` needs_merge | `409/40900` + `data.needs_merge` 是**成功分支**（#2410/#2445 契约），不是失败 | 保持（D3，回归断言 `CONFIRMBIND_NEEDS_MERGE_KEPT=1`） |
| 5 | `wechat_bind.go:296-303` `MergeBind` 合并失败 | 已由 #2598 改为 `respondIAMFailure` 四类分流 | 保持（仅回归） |
| 6 | `wechat_bind.go:76/104/113` `GenBindToken` | 本地 rand / 微信 `access_token` / `wxacode` 失败 → `500/50000`：**微信上游**瞬时故障（非 IAM 分类族），文案为 ASCII 且不含误导性冲突语义 | 保持（Out of Scope 记录理由） |
| 7 | `wechat_bind.go:279/303` 其余 400 分支 | `400/40002` + ASCII（`invalid or expired merge ticket` / `merge ticket does not match binding token`） | 保持（码/状态一致；文案改造属 Out of Scope） |
| 8 | `services/iam.go:722-724` `WxMergeByTicket` 200 但 `merged=false` | 协议异常 → 归瞬时（重试可能成功），非本单判定 | 保持（Out of Scope 记录） |

### 变更点

#### 1. `backend/handlers/wechat_bind.go` — `ConfirmBind`（:222）

```go
if bindErr != nil {
    log.Printf("[ConfirmBind] IAM wx-bind failed for token=%s: %v", req.Token, bindErr)
    respondIAMFailure(c, bindErr, iamFailureTexts{
        Transient:  "绑定失败，请稍后再试",
        Conflict:   "该微信已绑定其他账户，请让管理员重新生成绑定二维码",
        UserAction: "微信授权已失效，请重新进入页面重试",
    })
    return
}
```

- 删除 `500/50000` 与 `bindErr.Error()` 拼接（细节只进日志，`CONFIRMBIND_RAW_LEAK=0`）。
- `needs_merge` 分支（:196-221）**逐字不变**，仍先于失败出口。

#### 2. `backend/services/iam_client.go` — `UpdateUser` 非 2xx 类型化（前置）

- 403：`return &IAMClientHTTPError{Op: "UpdateUser", Status: http.StatusForbidden, Body: string(respBody)}` → `Error()` 仍产出 `permission denied: <body>`（**逐字保持**，`UPDATEUSER_403_TEXT_KEPT=1`）。
- 404：`return &IAMClientHTTPError{Op: "UpdateUser", Status: http.StatusNotFound, Body: string(respBody)}`（旧文本 `user not found: <iamsub>` 变为 `UpdateUser returned status 404: <body>`，携带上游 body，仅落日志 / `user_staff.go:1687` 与 `iam_proxy.go:600` 的 ASCII message，二者前端都走码表，见 D5）。
- 其余非 2xx：`&IAMClientHTTPError{Op: "UpdateUser", Status: statusCode, Body: ...}` → `Error()` 与旧串 `UpdateUser returned status %d: %s` **逐字一致**。
- 传输层失败保持 `fmt.Errorf("UpdateUser request failed: %w", err)`（无上游状态 → 瞬时）。
- **不改** `UpdateUserWithToken` 及其余 `IAMClient` 方法（Out of Scope）。

#### 3. `backend/handlers/wechat_bind.go` — `Unbind`（:381）

```go
if err := iamClient.UpdateUser(user.IAMSub, &services.UpdateUserRequest{WxOpenid: &empty}); err != nil {
    log.Printf("[Unbind] IAM update user failed for %s: %v", user.IAMSub, err)
    respondIAMFailure(c, err, iamFailureTexts{
        Transient:  "解绑失败，请稍后再试",
        Conflict:   "解绑请求与当前数据状态冲突，请刷新页面后重试",
        UserAction: "解绑请求已失效，请刷新页面后重试",
    })
    return
}
```

#### 4. `backend/handlers/wechat_bind.go` — `MergeBind` token 状态前置（:274）

```go
if entry.Status != "needs_merge" {
    status := entry.Status
    bindTokensMu.Unlock()
    log.Printf("[MergeBind] token not in needs_merge state: token=%s status=%s", req.Token, status)
    c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "绑定二维码已失效，请让管理员重新生成二维码后重试"})
    return
}
```

- 选 `400/40002`（与相邻 :279「invalid or expired merge ticket」同族；`40902` 在 api.md §14.4 **无定义**且其既有语义被「一人一号」占用，见 D4）。
- 内部状态串移入日志（`MERGEBIND_STATE_INTERNAL_LEAK=0`）；幂等 `merged` 分支不变。

#### 5. `frontend-mobile/src/pages-weapp/bind/Bind.jsx` — 失败态文案走解析层（对称改造）

- 新增 `const [failText, setFailText] = useState('')`。
- `doBind` 的失败出口：`setFailText(resolveErrorMessage(confResult, '绑定失败，请稍后重试'))`（`confResult` 为 undefined 时也要安全，wrapper 内已判空）。
- `doMerge` 的失败出口：`setFailText(resolveErrorMessage(result, '合并失败，请稍后重试'))`。
- `status === 'error'` 的正文由硬编码「请重试」改为 `{failText || '绑定失败，请稍后重试'}`；`status === 'merge_error'` 的正文改为 `{failText || '合并失败，请稍后重试'}` + 保留原有「请让管理员重新生成绑定二维码后再扫码」提示行。
- 判定分支**不动**：`code===20000` 成功、`code===40900 && data.needs_merge` 进合并引导（不得被判为失败）。
- 文案来源唯一 owner = `services/errorMessages.js`（`resolveErrorMessage`），不在页面内新增业务文案常量；本次改造显式取代 #2598 D6「`merge_error` 静态文案保持」的锁定（理由见 D6）。

#### 6. 文档（见 Core Update）

#### 7. 测试与证据

- 新增 `backend/handlers/wechat_bind_failure_2601_test.go`（独立 fake IAM，避免改动 #2598 的 `wxFailureIAM2598`）：fake IAM 必须同时服务 `POST /api/v1/auth/token`（`{access_token, expires_in, token_type}`，`IAMClient.doRequest` 先取 client token，缺该端会让所有 `Unbind` 用例退化成瞬时类），以及 `/api/v1/auth/wx-bind`、`/api/v1/auth/wx-merge-confirm`、`PUT /api/v1/users/{id}` 三端点按配置返回状态；IAM 不可达由「关闭的 `httptest.Server`」注入。
  - `TestConfirmBind2601_Triples`：上游 `400/403/404/409/500` + 不可达 + `needs_merge(200)` → 断言状态/code/文案可行动性；负例「持久/重做类不含『请稍后再试』」「瞬时类不得为 409」「message 不含 `IAM returned status`」。
  - `TestUnbind2601_Triples`：同一矩阵（`PUT /api/v1/users/{id}`）；`user` 记录用 `testfixtures.SetupTestDB`，`middleware.ContextKeyUserID` 注入 `iam_sub`。
  - `TestMergeBind2601_StateMismatch`：token 置 `bound` → `400/40002` + 中文可行动文案 + message 不含 `status=`；token 置 `merged` → 仍 `20000`（幂等回归）。
- 新增 `frontend-mobile/scripts/page-smoke/bind-failure-2601.test.jsx`：复用 `bind-needs-merge.test.jsx` 的 mock 形态（`vi.mock('../../src/platform')`），四例：needs_merge 渲染合并引导；`500/50015` → 正文 = 「身份服务配置异常，请联系平台管理员处理」且不含「请重试」；`502/50200` → 正文含「请稍后」；`400/40002` + 中文 message → 正文透传该 message 且不进合并引导。
- 新增 `scripts/verify-2601.sh`：`r1`–`r6` 与 `gate`，逐行产出 Evidence Contract 需要的 token；`r4` 内部 `source ~/.nvm/nvm.sh && nvm use 22` 后跑 `npx vitest run --config vitest.config.js scripts/page-smoke/bind-failure-2601.test.jsx`（node 不可用 → exit 2，绝不静默通过）。
- 红绿顺序：先加测试（R1/R2/R3/R4）在**未修**实现上跑 → 记录 red（例如 ConfirmBind 上游 403 当时返回状态 500 / code `50000`，而断言要求 code `50015`）；再实现 → green。red 片段进 Work Summary `### 红绿实证`。

### 被翻转 / 被强化的既有断言（强制清单）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `backend/handlers/wechat_bind.go:274`（无既有测试覆盖） | 无断言；文档未定义 `40902` | `400 / 40002` + 中文可行动文案 | 码/状态家族一致性；`40902` 语义被「一人一号」占用 |
| `backend/services/iam_client.go:1308-1310`（无既有断言） | 404 文本 `user not found: <iamsub>` | `UpdateUser returned status 404: <body>`（带 `IAMClientHTTPError`） | 类型化所需的文本调整；无测试/文档依赖旧串（`grep -rn "user not found: " backend/**/*_test.go` = 0） |
| `frontend-mobile/src/pages-weapp/bind/Bind.jsx:206-213`（无既有测试覆盖） | 任何失败 → 「请重试」 | 解析层文案（持久配置 → 联系管理员；瞬时 → 请稍后） | 用户面「持久失败被表达成可重试」的最后一公里 |
| **强化（不改弱）**：`backend/handlers/wechat_bind_test.go:289 TestConfirmBind_NeedsMerge` | `409/40900` + `data.needs_merge=true` + token `needs_merge` | 保持不变（本单仅加回归 token `CONFIRMBIND_NEEDS_MERGE_KEPT=1`） | 防止把成功分支误并入失败分类 |
| **强化**：`backend/handlers/bind_merge_2445_test.go` 全量 token | 合并成功/幂等/本地同步 | 保持不变 | 失败路径改造不得波及成功路径 |

### Core Update

`[Core Update Required]`

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §14.4「错误三元组一致性（#2598）」的「适用端点」只列 `transit-sites` / `/users/me/wx-bind` / `/users/me/wx-merge` / `/wechat-bind/merge`；`POST /api/wechat-bind/confirm` 的响应表只有成功与 needs_merge 两行；`POST /api/users/me/wechat-unbind` 只有行为说明 | ①§14.4「适用端点」补 `POST /api/wechat-bind/confirm`、`POST /api/users/me/wechat-unbind`；②`POST /api/wechat-bind/confirm` 响应表补失败行：`400/40002`（上游 400/422 → 「微信授权已失效，请重新进入页面重试」）、`409/40900`（上游 409 → 「该微信已绑定其他账户，请让管理员重新生成绑定二维码」）、`500/50015`（上游 401/403/404 → `iamConfigErrorMessage`）、`502/50200`（瞬时 → 「绑定失败，请稍后再试」）；③`POST /api/wechat-bind/merge` 补「token 状态非 needs_merge → `400/40002` +「绑定二维码已失效，请让管理员重新生成二维码后重试」」；④`POST /api/users/me/wechat-unbind` 补四类失败三元组一行表 | 这三个端点的失败语义成为跨端契约；前端 `resolveErrorMessage` 依赖码值与 CJK 文案形态 |
| `docs/spec/ui/ui.md` | §2.6.2「绑定微信与账户合并」已有 #2598 的失败提示表（针对 `/users/me/wx-bind`、`/users/me/wx-merge`），但未描述 weapp **Bind 页**（`/wechat-bind/confirm` / `/merge`）的失败态 | §2.6.2 增「Bind 页（扫码绑定）失败提示（#2601）」小表：`500/50015` → 正文「身份服务配置异常，请联系平台管理员处理」，**不得**出现「请重试」；`502/50200` → 「绑定失败，请稍后重试」/「合并失败，请稍后重试」；`400/40002` → 透传后端 CJK 文案（如「微信授权已失效，请重新进入页面重试」）；`409/40900 + data.needs_merge` → 仍是合并引导（非失败） | 用户可见文案是 UI 契约；同时锁定「文案来源 = `resolveErrorMessage`，页面不得自写业务文案」 |

### 依赖与基线说明

`#2598` 已 `done`，其 fix `18c81bd36` 只在 `origin/issue-2598`，**未集成 `origin/develop`**。执行时先判 `git merge-base --is-ancestor 18c81bd36 origin/develop`：为假 → worktree 以 `--base origin/issue-2598` 创建；为真 → 用默认 `origin/develop`。合并顺序 #2598 → #2601。禁止在本单复制 `ClassifyIAMFailure` / `respondIAMFailure`（唯一 owner = #2598）。

### Dependencies

- #2598


### Workload

- Size: L
- Basis: 后端 2 个实现文件（`wechat_bind.go` 三处、`iam_client.go` 一段）+ 1 个新测试文件；前端 1 个页面改造 + 1 个渲染测试；2 份核心文档；1 个证据 harness（7 行合同）+ 9 个证据产物；无 migration、无跨仓改动
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状 | 微信绑定/解绑遇 IAM 权限缺失、user/client 不存在这类**必须改配置**的失败时，用户看到「服务器开小差了，请稍后再试」并被引导重试；监控侧只看到 500/50000，与真实网络故障不可区分 | `wechat_bind.go:222/381` + `ERROR_CODE_MAP[50000]` |
| 2 | 触发条件 | 上游 IAM 返回 401/403/404（权限模板缺失、命名空间/client 不存在、账户已删）或 400（exchange_token 失效）；或 IAM 不可达/超时 | beaconiam `wx-bind` 4xx 语义（#2598 背景表）；`services/iam.go:593-617` |
| 3 | 根因 | 失败分类在 `wechat_bind.go` 这两处**没有接线**到 #2598 的统一出口；`Unbind` 依赖的 `IAMClient.UpdateUser` 未类型化，即使接线也拿不到上游状态 | `wechat_bind.go:222/381`、`iam_client.go:1295-1317` |
| 4 | 测试盲点 | 既有测试只覆盖 `ConfirmBind` 成功/needs_merge/缺参，**从未**断言失败三元组；`Unbind` 无任何测试 | `wechat_bind_test.go`（3 个 ConfirmBind 用例）、`grep -c "func TestUnbind"` = 0 |
| 5 | 同类扫描 | `wechat_bind.go` 内 3 处同类（ConfirmBind / Unbind / MergeBind 状态前置）；`MergeBind` 合并失败已由 #2598 修；`GenBindToken` 属微信上游非同类 | 本计划「同批扫描判定」表 |
| 6 | 修复层 | 修在「`services` 类型化上游状态 → `respondIAMFailure` 统一出口（复用 #2598）→ 三个调用点 → Bind 页文案解析层 → 文档契约」；不碰成功/幂等路径 | 变更点 1–6 |
| 7 | 红绿实证 | 红：未修实现跑新测试，`CONFIRMBIND_403_CODE` 期望 `50015` 实得 `50000`、`UNBIND_403_CODE` 同理、`MERGEBIND_STATE_CODE` 期望 `40002` 实得 `40902`、`BIND_UI_50015_NO_RETRY_HINT` 期望 1 实得 0。绿：修后全 token 成立 | R1/R2/R3/R4；Work Summary `### 红绿实证` |
| 8 | 防复发 | 每条路径 × 六种上游状态；负例「瞬时类不得 409」「持久类不含『请稍后再试』」「message 不含上游原始串」；Bind 页渲染断言「50015 不出现『请重试』」；文档端点清单防止新增路径漏登记 | R1–R5 的 negative 列 |
| 9 | 影响面 | 只改**失败分支**：`ConfirmBind` 成功/needs_merge、`MergeBind` 幂等 `merged`、本地 `user not found → 404/40004`、`migrateMergedAccountLocal` 与 `local_sync` 警告、PC 轮询状态机均不变 | D3/D7、R1/R3 回归 token |
| 10 | 未修边界 | `GenBindToken` 微信上游失败、`UpdateUserWithToken` 及其余 IAMClient 方法类型化、`MergeBind` 200-without-merged 协议异常、PC 解绑 toast 文案、全站 message 直出 → 均在 Out of Scope 记录理由 | Out of Scope |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 `ConfirmBind` 失败出口接入统一三元组 | `backend/handlers/wechat_bind.go`、`backend/handlers/wechat_bind_failure_2601_test.go` | `POST /api/wechat-bind/confirm` | fake IAM `wx-bind` → 400 / 403 / 404 / 409 / 500 / 不可达 / 200+needs_merge | backend（Postgres 测试库 + httptest） | `CONFIRMBIND_400_STATUS=400`、`CONFIRMBIND_400_CODE=40002`、`CONFIRMBIND_403_STATUS=500`、`CONFIRMBIND_403_CODE=50015`、`CONFIRMBIND_409_CODE=40900`、`CONFIRMBIND_TRANSIENT_STATUS=502`、`CONFIRMBIND_TRANSIENT_CODE=50200`、`CONFIRMBIND_PERSISTENT_NO_RETRY=0`、`CONFIRMBIND_RAW_LEAK=0`、`CONFIRMBIND_NEEDS_MERGE_KEPT=1` | R1 |
| R2 `Unbind` 失败出口 + `UpdateUser` 类型化 | `backend/handlers/wechat_bind.go`、`backend/services/iam_client.go`、`backend/handlers/wechat_bind_failure_2601_test.go` | `POST /api/users/me/wechat-unbind` | fake IAM `PUT /users/{id}` → 400 / 403 / 404 / 500 / 不可达 | backend（Postgres 测试库 + httptest） | `UNBIND_400_CODE=40002`、`UNBIND_403_STATUS=500`、`UNBIND_403_CODE=50015`、`UNBIND_TRANSIENT_STATUS=502`、`UNBIND_TRANSIENT_CODE=50200`、`UNBIND_PERSISTENT_NO_RETRY=0`、`UNBIND_RAW_LEAK=0`、`UPDATEUSER_403_TEXT_KEPT=1`、`UPDATEUSER_404_TYPED=1` | R2 |
| R3 `MergeBind` token 状态前置码/状态一致 | `backend/handlers/wechat_bind.go`、`backend/handlers/wechat_bind_failure_2601_test.go` | `POST /api/wechat-bind/merge` | token 状态 `bound`（非 needs_merge）；token 状态 `merged`（幂等） | backend（Postgres 测试库） | `MERGEBIND_STATE_STATUS=400`、`MERGEBIND_STATE_CODE=40002`、`MERGEBIND_STATE_MSG_ACTIONABLE=1`、`MERGEBIND_STATE_INTERNAL_LEAK=0`、`MERGEBIND_IDEMPOTENT_KEPT=1` | R3 |
| R4 Bind 页失败态展示（渲染） | `frontend-mobile/src/pages-weapp/bind/Bind.jsx`、`frontend-mobile/scripts/page-smoke/bind-failure-2601.test.jsx` | weapp `pages-weapp/bind`（`render(Bind)` + `waitFor`） | confirm 返回 needs_merge / `500+50015` / `502+50200` / `400+40002`（CJK message） | weapp（jsdom + `@testing-library/react`） | `BIND_UI_NEEDS_MERGE_RENDER=1`、`BIND_UI_50015_CONFIG_TEXT=1`、`BIND_UI_50015_NO_RETRY_HINT=1`、`BIND_UI_50200_RETRY_TEXT=1`、`BIND_UI_40002_SERVER_TEXT=1`、`BIND_UI_RENDER_FAIL=0` | R4 |
| R5 契约文档同步（端点清单 + 失败三元组） | `docs/spec/api/api.md`、`docs/spec/ui/ui.md` | 文档结构断言 | 修后文档 | 文档 | `API_CONFIRM_ENDPOINT=1`、`API_UNBIND_ENDPOINT=1`、`API_CONFIRM_FAILURE_ROWS=1`、`API_MERGE_STATE_ROW=1`、`DOC_BIND_FAILURE_TEXT=1` | R5 |
| R6 构建 / gofmt / ESLint（no-undef） | `scripts/verify-2601.sh`、`backend/handlers/wechat_bind.go`、`backend/services/iam_client.go`、`frontend-mobile/src/pages-weapp/bind/Bind.jsx` | `cd backend && go build ./...`、`gofmt -l`、`npx eslint` | 修后工作树 | backend + weapp | `GO_BUILD_OK=1`、`GOFMT_DIFF=0`、`ESLINT_NOUNDEF=0` | R6 |
| R7 pre-ready gate（四源 + 合同完整性 + 远端可达） | `docs/evidence/2601/pre-ready-2601.json` | `pre-ready-check.sh 2601` | 修后干净树 + 最新 Accepted Plan | backend + weapp | exit 0、`overall=PASS`、P12 每行 `pass=true`、P13 在 `origin/issue-2601` | R7 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | integration | handler + fake IAM + Postgres 测试库；断言 `(status, code, message)` 三元组与负例 |
| R2 | integration | 同上（`PUT /api/v1/users/{id}`）；含 `IAMClientHTTPError` 文本保持与类型命中断言 |
| R3 | integration | 同 handler 的状态前置分支 + 幂等分支回归 |
| R4 | render | `@testing-library/react` 的 `render(` / `waitFor` / `fireEvent`，jsdom 真实渲染 Bind 页 |
| R5 | static | 文档结构断言 |
| R6 | build | `go build ./...` + `gofmt -l` + `eslint`（`no-undef`） |
| R7 | gate | `pre-ready-check.sh` 报告（P1–P15） |

### Input Truth & Tool Preflight

- **Source of truth**：
  - 失败分类唯一 owner = `services.ClassifyIAMFailure` + `handlers.respondIAMFailure`（#2598，**本单不复制、不改语义**）；
  - 上游状态唯一来源 = `*services.IAMAPIError.StatusCode`（`services/iam.go:29-33`）与 `*services.IAMClientHTTPError.Status`（`services/iam_client.go`）；
  - 用户文案唯一 owner = `frontend-mobile/src/services/errorMessages.js`（码表 / 解析层），页面不得自写业务文案；
  - 路由事实 = `backend/main.go:277-283`（confirm/merge/status）、`main.go:901`（`/users/me/wechat-unbind`，`userOptionalAuth`）；
  - 契约事实 = `docs/spec/api/api.md` §14.4（#2598 三元组口径）、`POST /api/wechat-bind/confirm`、`POST /api/wechat-bind/merge`、`POST /api/users/me/wechat-unbind` 段。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `git merge-base --is-ancestor 18c81bd36 origin/develop; echo $?` → 期望非 0（未集成）⇒ 按 D1 用 `--base origin/issue-2598`；
  - `grep -q "func respondIAMFailure" backend/handlers/iam_failure.go && grep -q "func ClassifyIAMFailure" backend/services/iam_failure.go` → 必须成立，否则 HOLD（D1）；
  - `grep -n "failed to bind\|unbind failed\|is not waiting for merge" backend/handlers/wechat_bind.go` → 修前 3 命中（222 / 274 / 381）；
  - `grep -rn "40902" backend frontend-* docs/spec` → 修后 `wechat_bind.go` 内 0 命中（`docs/topics/iam/iam-notes.md` 的「一人一号」语义**不动**）；
  - `grep -rn "user not found: " backend --include=*_test.go` → 0（404 文本调整无断言依赖）；
  - `grep -n "resolveErrorMessage" frontend-mobile/src/pages-weapp/bind/Bind.jsx` → 修前 0 命中；
  - `grep -c "func TestUnbind" backend/handlers/*_test.go` → 修前 0（本单新增）。
- **损坏输入拒绝规则**：
  - `respondIAMFailure(nil)` 不得出现（调用点必须先判 `err != nil`）；`WxNeedsMergeError` 必须先判，禁止落入失败分类；
  - fake IAM 返回非 JSON / 空 body → `IAMAPIError.StatusCode` 仍足以分类（不得因解析失败改判）；
  - `IAMClientHTTPError` 被 `%w` 包裹 → 仍须 `errors.As` 命中（R2 覆盖）；
  - Bind 页 `confResult` 为 `undefined`（fetch 异常）→ `resolveErrorMessage(undefined, fallback)` 返回 fallback，不抛二次异常（R4 覆盖 `502` 分支）。
- **工具盲区**：
  - fake IAM 只证明我方分类与响应，不证明 beaconiam 侧错误码演化（状态类变化由 #2598 分类器表驱动承担）；
  - `wx-merge-confirm` 上游语义不在本地 beaconiam 检出内（#2598 已记录）→ 只按状态类分流；
  - vitest 只覆盖 jsdom 渲染，不覆盖真机交互/样式；真机回执在 done 时按需开 `[Ops-Followup of #2601]`（`waitForConfirm`，无 `status:*`），不写进矩阵状态。
- **契约影响**：`docs/spec/api/api.md` 变更 → Work Summary 必须含 `### 契约三向核对`（pre-ready P7）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 依赖基线：`#2598` 已 `done` 但 fix `18c81bd36` 仅在 `origin/issue-2598`。执行时先判 `git merge-base --is-ancestor 18c81bd36 origin/develop`：假 → `worktree-create.sh --project tuneloop --issue 2601 --base origin/issue-2598`；真 → 默认 `origin/develop`。两文件缺失即 HOLD（不得自行实现分类器） | 本计划锁定 |
| D2 | 三元组沿用 #2598 口径（`502/50200`、`409/40900`、`400/40002`、`500/50015` + `iamConfigErrorMessage`），**不新增错误码**，不改分类器语义 | 本计划锁定 |
| D3 | `ConfirmBind` 的 `needs_merge`（`409/40900` + `data.needs_merge` + `merge_ticket` + token `needs_merge`）与 `MergeBind` 幂等 `merged` 分支保持逐字不变；Bind 页不得把 `needs_merge` 判为失败 | 本计划锁定 |
| D4 | `MergeBind` token 状态前置选 `400 / 40002`（不选 `409/40902`）：与相邻「invalid or expired merge ticket → `400/40002`」同族；`40902` 在 §14.4 无定义且被「一人一号」占用 | 本计划锁定 |
| D5 | `IAMClient.UpdateUser` 非 2xx 类型化（`IAMClientHTTPError`）：403 文本逐字保持；404 文本改携带上游 body（旧串 `user not found: <id>`），仅影响日志与 ASCII message（前端走码表），不影响用户可见中文文案 | 本计划锁定 |
| D6 | Bind 页两个失败态（`error` / `merge_error`）文案改为 `resolveErrorMessage` 输出，**显式取代 #2598 D6**「`merge_error` 静态文案保持」：同类路径对称（confirm/merge 都会返回 50015/50200），静态文案会把持久失败表达成可重试；页面仅保留 fallback（「绑定失败，请稍后重试」/「合并失败，请稍后重试」）与既有引导句 | 本计划锁定 |
| D7 | 不改变任何成功/幂等/本地回滚语义：`ConfirmBind` 成功置 `bound`、`MergeBind` 的 `migrateMergedAccountLocal` + `local_sync=false` 警告、`Unbind` 成功 `20000`、本地 `user not found → 404/40004` | 本计划锁定 |
| D8 | 重命令（`go build`/`go test`/`vitest`/`eslint`）经 `scripts/heavy-run.sh` 取锁；合同行禁止把全量 `go test ./...` 当 exit 0 合同（P5 baseline 差分承担） | 本计划锁定 |
| D9 | 证据形态：R4 用 `@testing-library/react` 真实渲染（非 `evidence_type: static` 豁免）；R5 为文档结构断言（`static`） | 本计划锁定 |
| D10 | 并行交叉：无。`#2598` 已 `done` 且本单基于其分支；若执行期发现 `wechat_bind.go` 与本单三处 hunk 冲突（他人改动同一文件）→ HOLD 并回 Architect，禁止就地改写他人语义 | 本计划锁定 |

### Out of Scope

- `GenBindToken`（`wechat_bind.go:76/104/113`）的本地 rand / 微信 `access_token` / `wxacode` 失败保持 `500/50000`：属**微信上游瞬时故障**，不是 IAM 分类族，文案不含误导性冲突语义。
- `services/iam.go:722-724`（`wx-merge-confirm` 返回 200 但 `merged=false`）仍归瞬时：协议异常下重试可能成功，且无真实样本。
- `UpdateUserWithToken` 与其余 `IAMClient` 方法（`BindUserToOrganization`/`UpdateUserRoleInOrg` 等）的类型化：本单只覆盖 `Unbind` 实际调用的 `UpdateUser`。
- `wechat_bind.go:279/303` 两条 400 分支的 ASCII 文案中文化（码/状态已一致，仅文案体验）。
- `auth.go:487`（注册流程 `WxBind` 失败 → 409 + purge）：与 #1644 注册回滚契约耦合，非本单同类。
- PC `frontend-pc/src/components/WechatBinding.jsx` 解绑失败 toast 不透传服务端文案（显示固定「解绑失败」，不含误导性重试建议）；PC toast 持久化/文案统一属 #2489 族。
- 全站 `message` 直出清理与 `50200` 入码表：属 #2489 族与后续全站错误码收敛。
- H5 端：`Bind` 页仅 weapp 存在（`app.config.ts` 只注册 `pages-weapp/bind/index`），本单不新增 H5/weapp 差异，故不改 `docs/cases/h5-weapp-differences.md`。
- 全量 `go test ./...` 作为合同行（由 pre-ready P5 baseline 差分承担）。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `testfixtures.SetupTestDB` + 用户 `U{iam_sub, status:active}`；绑定 token 表注入 `{Status:"pending"}`；fake IAM `wx-bind` 状态 ∈ {400,403,404,409,500}、关闭端口、200+`needs_merge:true` | `POST /api/wechat-bind/confirm`（`TestConfirmBind2601_Triples`） | `docs/evidence/2601/R1-confirm-triples.txt` | `CONFIRMBIND_400_CODE=40002`、`CONFIRMBIND_403_CODE=50015`、`CONFIRMBIND_403_STATUS=500`、`CONFIRMBIND_409_CODE=40900`、`CONFIRMBIND_TRANSIENT_STATUS=502`、`CONFIRMBIND_RAW_LEAK=0`、`CONFIRMBIND_NEEDS_MERGE_KEPT=1` | 持久/重做类含「请稍后再试」（`PERSISTENT_NO_RETRY=0` 必须成立）、瞬时类为 409、message 含 `IAM returned status`、needs_merge 不再是 409/needs_merge → FAIL |
| R2 | 同上用户 + fake IAM `PUT /api/v1/users/{id}` 状态 ∈ {400,403,404,500}、关闭端口；直接断言 `IAMClientHTTPError` 文本 | `POST /api/users/me/wechat-unbind`（`TestUnbind2601_Triples`） | `docs/evidence/2601/R2-unbind-triples.txt` | `UNBIND_400_CODE=40002`、`UNBIND_403_CODE=50015`、`UNBIND_TRANSIENT_STATUS=502`、`UNBIND_PERSISTENT_NO_RETRY=0`、`UNBIND_RAW_LEAK=0`、`UPDATEUSER_403_TEXT_KEPT=1`、`UPDATEUSER_404_TYPED=1` | 403 仍 500/50000、文案含上游原始串、`permission denied: ` 前缀漂移、404 未类型化 → FAIL |
| R3 | 同 R1 用户；token 表注入 `{Status:"bound"}` 与 `{Status:"merged", MergedToUserID:...}` | `POST /api/wechat-bind/merge`（`TestMergeBind2601_StateMismatch`） | `docs/evidence/2601/R3-mergebind-state.txt` | `MERGEBIND_STATE_STATUS=400`、`MERGEBIND_STATE_CODE=40002`、`MERGEBIND_STATE_MSG_ACTIONABLE=1`、`MERGEBIND_STATE_INTERNAL_LEAK=0`、`MERGEBIND_IDEMPOTENT_KEPT=1` | 仍返回 `40902`、状态仍为 400 但 code 为 40902、message 含 `status=`、幂等分支回归 → FAIL |
| R4 | mock `platform.request`：`/auth/wx-accounts` → `{code:20000,data:{openid,exchange_token}}`；`/wechat-bind/confirm` → 四种响应（needs_merge / 500+50015 / 502+50200 / 400+40002 CJK） | weapp `pages-weapp/bind`（`render(Bind)` + `waitFor`） | `docs/evidence/2601/R4-bind-ui.txt` | `BIND_UI_NEEDS_MERGE_RENDER=1`、`BIND_UI_50015_CONFIG_TEXT=1`、`BIND_UI_50015_NO_RETRY_HINT=1`、`BIND_UI_50200_RETRY_TEXT=1`、`BIND_UI_40002_SERVER_TEXT=1`、`BIND_UI_RENDER_FAIL=0` | 50015 时正文出现「请重试」、needs_merge 未渲染合并按钮、40002 中文未透传、渲染抛错 → FAIL |
| R5 | 修后 `api.md` / `ui.md` | `bash scripts/verify-2601.sh r5` | `docs/evidence/2601/R5-docs.txt` | `API_CONFIRM_ENDPOINT=1`、`API_UNBIND_ENDPOINT=1`、`API_CONFIRM_FAILURE_ROWS=1`、`API_MERGE_STATE_ROW=1`、`DOC_BIND_FAILURE_TEXT=1` | 端点清单缺一、confirm 失败行缺失、merge 状态行缺失、ui.md 未写 Bind 页失败提示 → FAIL |
| R6 | 修后工作树 | `bash scripts/verify-2601.sh r6` | `docs/evidence/2601/R6-build-lint.txt` | `GO_BUILD_OK=1`、`GOFMT_DIFF=0`、`ESLINT_NOUNDEF=0` | 构建失败、gofmt 差异、eslint `no-undef` → FAIL |
| R7 | 修后干净树 + 最新 Accepted Plan + `evidence.json`（`dirty=false`） | `pre-ready-check.sh 2601` | `docs/evidence/2601/pre-ready-2601.json` | exit 0、`overall=PASS`、P12 每行 `pass=true`、P13 提交在 `origin/issue-2601` | 任一 P 项失败 / evidence 缺行 / `dirty=true` → gate 非 0，禁止 ready |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2601.sh r1 > docs/evidence/2601/R1-confirm-triples.txt` | `docs/evidence/2601/R1-confirm-triples.txt` | exit 0 且含 `CONFIRMBIND_400_CODE=40002`、`CONFIRMBIND_403_STATUS=500`、`CONFIRMBIND_403_CODE=50015`、`CONFIRMBIND_409_CODE=40900`、`CONFIRMBIND_TRANSIENT_STATUS=502`、`CONFIRMBIND_TRANSIENT_CODE=50200`、`CONFIRMBIND_PERSISTENT_NO_RETRY=0`、`CONFIRMBIND_RAW_LEAK=0`、`CONFIRMBIND_NEEDS_MERGE_KEPT=1` | 九项 token 全为期望值 | 持久/重做类含「请稍后再试」、瞬时类 409、message 含上游原始串、needs_merge 回归 → FAIL | dev |
| R2 | `bash scripts/verify-2601.sh r2 > docs/evidence/2601/R2-unbind-triples.txt` | `docs/evidence/2601/R2-unbind-triples.txt` | exit 0 且含 `UNBIND_400_CODE=40002`、`UNBIND_403_STATUS=500`、`UNBIND_403_CODE=50015`、`UNBIND_TRANSIENT_STATUS=502`、`UNBIND_TRANSIENT_CODE=50200`、`UNBIND_PERSISTENT_NO_RETRY=0`、`UNBIND_RAW_LEAK=0`、`UPDATEUSER_403_TEXT_KEPT=1`、`UPDATEUSER_404_TYPED=1` | 九项 token 全为期望值 | 403 仍 50000、原始串外溢、403 文本漂移、404 未类型化 → FAIL | dev |
| R3 | `bash scripts/verify-2601.sh r3 > docs/evidence/2601/R3-mergebind-state.txt` | `docs/evidence/2601/R3-mergebind-state.txt` | exit 0 且含 `MERGEBIND_STATE_STATUS=400`、`MERGEBIND_STATE_CODE=40002`、`MERGEBIND_STATE_MSG_ACTIONABLE=1`、`MERGEBIND_STATE_INTERNAL_LEAK=0`、`MERGEBIND_IDEMPOTENT_KEPT=1` | 五项 token 全为期望值 | 仍 40902、message 含 `status=`、幂等分支回归 → FAIL | dev |
| R4 | `bash scripts/verify-2601.sh r4 > docs/evidence/2601/R4-bind-ui.txt` | `docs/evidence/2601/R4-bind-ui.txt` | exit 0 且含 `BIND_UI_NEEDS_MERGE_RENDER=1`、`BIND_UI_50015_CONFIG_TEXT=1`、`BIND_UI_50015_NO_RETRY_HINT=1`、`BIND_UI_50200_RETRY_TEXT=1`、`BIND_UI_40002_SERVER_TEXT=1`、`BIND_UI_RENDER_FAIL=0` | 六项 token 全为期望值 | 50015 正文含「请重试」、needs_merge 未渲染、中文未透传、渲染异常 → FAIL | dev |
| R5 | `bash scripts/verify-2601.sh r5 > docs/evidence/2601/R5-docs.txt` | `docs/evidence/2601/R5-docs.txt` | exit 0 且含 `API_CONFIRM_ENDPOINT=1`、`API_UNBIND_ENDPOINT=1`、`API_CONFIRM_FAILURE_ROWS=1`、`API_MERGE_STATE_ROW=1`、`DOC_BIND_FAILURE_TEXT=1` | 五项 token 全为期望值 | 端点清单/失败行/Bind 页文档任一缺失 → FAIL | dev |
| R6 | `bash scripts/verify-2601.sh r6 > docs/evidence/2601/R6-build-lint.txt` | `docs/evidence/2601/R6-build-lint.txt` | exit 0 且含 `GO_BUILD_OK=1`、`GOFMT_DIFF=0`、`ESLINT_NOUNDEF=0` | 三项 token 全为期望值 | 构建失败、gofmt 差异、eslint no-undef → FAIL | dev |
| R7 | `~/.config/opencode/scripts/pre-ready-check.sh 2601 --summary-file docs/evidence/2601/summary.md --changed-files docs/evidence/2601/changed-files.txt --evidence-file docs/evidence/2601/evidence.json --affected-cmd "bash scripts/verify-2601.sh gate" --report docs/evidence/2601/pre-ready-2601.json` | `docs/evidence/2601/pre-ready-2601.json` | exit 0 且 `jq -e '.overall == "PASS"' docs/evidence/2601/pre-ready-2601.json` 成立，P12 对 `docs/evidence/2601/evidence.json` 每行判定 `pass=true` | `overall=PASS` 且证据行全 `pass=true` | 任一 P 项失败、evidence 缺行、`dirty=true` → gate 非 0，禁止 ready | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2601.sh`（`r1`–`r7` 见合同；`gate` 供 `--affected-cmd` 复用 `go build ./...` + 定向测试）。
- 每个文本 artifact 首行：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，另起一行 `GENERATED_AT_HEAD=<40 位 sha>`；锚点为本单 fix commit（`FIX_COMMIT` 可覆盖），证据必须在 fix commit **之后**生成。
- 逐条清单（全部位于 `docs/evidence/2601/`）：
  - `docs/evidence/2601/R1-confirm-triples.txt`
  - `docs/evidence/2601/R2-unbind-triples.txt`
  - `docs/evidence/2601/R3-mergebind-state.txt`
  - `docs/evidence/2601/R4-bind-ui.txt`
  - `docs/evidence/2601/R5-docs.txt`
  - `docs/evidence/2601/R6-build-lint.txt`
  - `docs/evidence/2601/evidence.json`
  - `docs/evidence/2601/pre-ready-2601.json`
  - `docs/evidence/2601/summary.md`
  - `docs/evidence/2601/changed-files.txt`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Files（变更文件清单）

- `backend/handlers/wechat_bind.go`
- `backend/services/iam_client.go`
- `backend/handlers/wechat_bind_failure_2601_test.go`
- `frontend-mobile/src/pages-weapp/bind/Bind.jsx`
- `frontend-mobile/scripts/page-smoke/bind-failure-2601.test.jsx`
- `docs/spec/api/api.md`
- `docs/spec/ui/ui.md`
- `scripts/verify-2601.sh`
- `docs/evidence/2601/plan.md`
- `docs/evidence/2601/R1-confirm-triples.txt`
- `docs/evidence/2601/R2-unbind-triples.txt`
- `docs/evidence/2601/R3-mergebind-state.txt`
- `docs/evidence/2601/R4-bind-ui.txt`
- `docs/evidence/2601/R5-docs.txt`
- `docs/evidence/2601/R6-build-lint.txt`
- `docs/evidence/2601/evidence.json`
- `docs/evidence/2601/pre-ready-2601.json`
- `docs/evidence/2601/summary.md`
- `docs/evidence/2601/changed-files.txt`

*Model: deepseek/deepseek-flash*

