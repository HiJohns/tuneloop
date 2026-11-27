# Work Summary — #2601 wechat_bind.go 持久 IAM 失败三元组

## 变更摘要

ConfirmBind / Unbind 曾把 IAM 的**任一**失败压成 HTTP 500 + code 50000 并拼接「failed to bind: / unbind failed: <上游原始串>」，PC/mobile 码表 50000 =「服务器开小差了，请稍后再试」——IAM 401/403/404（权限模板缺失、client/user 不存在）这类**必须改配置**的持久错误被表达成可重试。本次改接 #2598 的统一出口（services.ClassifyIAMFailure + handlers.respondIAMFailure），并按同一口径修掉同批扫描发现的两处同类点（MergeBind 状态前置码/状态不一致、Bind 页失败文案硬编码「请重试」）。

| 变更点 | 文件 |
|---|---|
| ConfirmBind 失败出口四类分流（needs_merge 分支逐字不变） | backend/handlers/wechat_bind.go |
| Unbind 失败出口四类分流 | backend/handlers/wechat_bind.go |
| MergeBind token 状态前置 400/40002 + 可行动中文（内部状态串只进日志） | backend/handlers/wechat_bind.go |
| IAMClient.UpdateUser 非 2xx 类型化（IAMClientHTTPError，403 文本逐字保持） | backend/services/iam_client.go |
| Bind 页失败态文案改由 resolveErrorMessage 给出 | frontend-mobile/src/pages-weapp/bind/Bind.jsx |
| 新增 handler 状态矩阵测试（fake IAM） | backend/handlers/wechat_bind_failure_2601_test.go |
| 新增 Bind 页渲染测试（jsdom） | frontend-mobile/scripts/page-smoke/bind-failure-2601.test.jsx |
| 证据 harness（r1–r6 + gate） | scripts/verify-2601.sh |
| 契约文档同步 | docs/spec/api/api.md、docs/spec/ui/ui.md |

- Fix commit：48081b74e3f6755d956027c0101d36c9740c2792（subject 前缀 fix(issue-2601)）
- 分支 / 基线：issue-2601，基于 origin/issue-2598（#2598 的 fix 18c81bd36 未集成 origin/develop，见 D1）

## 验证层级

- 单 Issue：backend 的 go build ./... + 三条定向测试（TestConfirmBind2601_Triples / TestUnbind2601_Triples / TestMergeBind2601_StateMismatch）+ 相关既有用例回归（TestConfirmBind* / TestMergeBind* / TestBindMergeFlow_2445 / TestMigrateMergedAccountLocal_2445 / TestWxFailure2598 / TestWxMergeCurrentUser* / TestUpdateUser_*，全 PASS）。
- frontend-mobile：npm run smoke 全量 22 文件 / 116 用例 PASS（含既有 wechat-bind-ui-2426.test.jsx），新增 bind-failure-2601.test.jsx 5 用例。
- 本单为独立 Issue（正文无 Parent: #N），不涉及父子/批次 gate；全量 go test ./... 按 #2651 吞吐规则由批次 gate 承担。

## 红绿实证

先加测试、在**未修**实现上跑（红），再实现（绿）：

```
# 红：backend（未修 wechat_bind.go / iam_client.go）
--- FAIL: TestConfirmBind2601_Triples
    Messages: 上游 400 必须 400（不得 500/50000）: failed to bind: IAM returned status: 400, body: {"error":"upstream rejects bind"}
--- FAIL: TestUnbind2601_Triples
    Messages: 上游 400 必须 400（不得 500/50000）: unbind failed: UpdateUser returned status 400: {"error":"upstream rejects update"}
--- FAIL: TestMergeBind2601_StateMismatch
    Messages: 状态前置必须 40002（与相邻 invalid/expired merge ticket 同族），不得 40902

# 红：Bind 页（未修 Bind.jsx，页面硬编码「请重试」）
× 500/50015 → expected '❌绑定失败请重试返回' to contain '身份服务配置异常，请联系平台管理员处理'
× 502/50200 → expected '❌绑定失败请重试返回' to contain '请稍后'
× 400/40002 → expected '❌绑定失败请重试返回' to contain '微信授权已失效，请重新进入页面重试'

# 绿：修后
--- PASS: TestConfirmBind2601_Triples / TestUnbind2601_Triples / TestMergeBind2601_StateMismatch
✓ scripts/page-smoke/bind-failure-2601.test.jsx (5 tests)
```

## Accepted Plan 对照（7/7 完成）

1. ConfirmBind 失败出口接入 respondIAMFailure（四类文案，删除 500/50000 与原始串）✅
2. IAMClient.UpdateUser 非 2xx 类型化（403 文本逐字保持、404 携带上游 body、其余逐字一致）✅
3. Unbind 失败出口接入 respondIAMFailure ✅
4. MergeBind token 状态前置改为 400/40002 + 可行动中文，内部状态串只进日志 ✅
5. Bind 页失败态走 resolveErrorMessage（error / merge_error 两态），判定分支不动 ✅
6. Core Update：API 文档 §14.4 与 confirm/merge/unbind 失败三元组；UI 文档 §2.6.2 Bind 页失败提示 ✅
7. 测试与证据（新增 2 个测试文件 + 证据 harness + R1–R7 产物）✅

## 矩阵覆盖（7/7）

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 ConfirmBind 失败出口接入统一三元组 | PASS | docs/evidence/2601/R1-confirm-triples.txt |
| R2 Unbind 失败出口 + UpdateUser 类型化 | PASS | docs/evidence/2601/R2-unbind-triples.txt |
| R3 MergeBind token 状态前置码/状态一致 | PASS | docs/evidence/2601/R3-mergebind-state.txt |
| R4 Bind 页失败态展示（渲染） | PASS | docs/evidence/2601/R4-bind-ui.txt |
| R5 契约文档同步（端点清单 + 失败三元组） | PASS | docs/evidence/2601/R5-docs.txt |
| R6 构建 / gofmt / ESLint（no-undef） | PASS | docs/evidence/2601/R6-build-lint.txt |
| R7 pre-ready gate（四源 + 合同完整性 + 远端可达） | PASS | docs/evidence/2601/pre-ready-2601.json |

## Evidence Contract（R1–R7 全部 pass=true）

- R1 → PASS → docs/evidence/2601/R1-confirm-triples.txt
- R2 → PASS → docs/evidence/2601/R2-unbind-triples.txt
- R3 → PASS → docs/evidence/2601/R3-mergebind-state.txt
- R4 → PASS → docs/evidence/2601/R4-bind-ui.txt
- R5 → PASS → docs/evidence/2601/R5-docs.txt
- R6 → PASS → docs/evidence/2601/R6-build-lint.txt
- R7 → PASS → docs/evidence/2601/pre-ready-2601.json

详见 docs/evidence/2601/evidence.json（schema v1，dirty=false，每行恰一条记录，产物均为仓库内持久路径）。

## G0 Golden Acceptance

| 行 | 产物 | 断言（可重放） | 负例 |
|---|---|---|---|
| R1 | docs/evidence/2601/R1-confirm-triples.txt | CONFIRMBIND_400_CODE=40002、CONFIRMBIND_403_STATUS=500、CONFIRMBIND_403_CODE=50015、CONFIRMBIND_404_CODE=50015、CONFIRMBIND_409_STATUS=409、CONFIRMBIND_409_CODE=40900、CONFIRMBIND_TRANSIENT_STATUS=502、CONFIRMBIND_TRANSIENT_CODE=50200、CONFIRMBIND_DEAD_STATUS=502、CONFIRMBIND_PERSISTENT_NO_RETRY=0、CONFIRMBIND_RAW_LEAK=0、CONFIRMBIND_NEEDS_MERGE_KEPT=1 | 持久/重做类含「请稍后再试」、瞬时类 409、message 含上游原始串、needs_merge 回归 |
| R2 | docs/evidence/2601/R2-unbind-triples.txt | UNBIND_400_CODE=40002、UNBIND_403_STATUS=500、UNBIND_403_CODE=50015、UNBIND_404_CODE=50015、UNBIND_TRANSIENT_STATUS=502、UNBIND_TRANSIENT_CODE=50200、UNBIND_PERSISTENT_NO_RETRY=0、UNBIND_RAW_LEAK=0、UPDATEUSER_403_TEXT_KEPT=1、UPDATEUSER_404_TYPED=1 | 403 仍 50000、原始串外溢、permission denied 前缀漂移、404 未类型化 |
| R3 | docs/evidence/2601/R3-mergebind-state.txt | MERGEBIND_STATE_STATUS=400、MERGEBIND_STATE_CODE=40002、MERGEBIND_STATE_MSG_ACTIONABLE=1、MERGEBIND_STATE_INTERNAL_LEAK=0、MERGEBIND_IDEMPOTENT_KEPT=1 | 仍 40902、message 含 status=、幂等分支回归 |
| R4 | docs/evidence/2601/R4-bind-ui.txt | BIND_UI_NEEDS_MERGE_RENDER=1、BIND_UI_50015_CONFIG_TEXT=1、BIND_UI_50015_NO_RETRY_HINT=1、BIND_UI_50200_RETRY_TEXT=1、BIND_UI_40002_SERVER_TEXT=1、BIND_UI_RENDER_FAIL=0 | 50015 正文出现「请重试」、needs_merge 未渲染、40002 中文未透传、渲染抛错 |
| R5 | docs/evidence/2601/R5-docs.txt | API_CONFIRM_ENDPOINT=1、API_UNBIND_ENDPOINT=1、API_CONFIRM_FAILURE_ROWS=1、API_MERGE_STATE_ROW=1、DOC_BIND_FAILURE_TEXT=1 | 端点清单/失败行/Bind 页文档任一缺失 |
| R6 | docs/evidence/2601/R6-build-lint.txt | GO_BUILD_OK=1、GOFMT_DIFF=0、ESLINT_NOUNDEF=0 | 构建失败、gofmt 差异、eslint no-undef |
| R7 | docs/evidence/2601/pre-ready-2601.json | overall=PASS、P12 全行 pass=true、P13 提交在 origin/issue-2601 | 任一 P 项失败 / evidence 缺行 / dirty=true |

### 契约三向核对

- 计划 ↔ API 文档：POST /api/wechat-bind/confirm 响应表新增 400/40002、409/40900（失败，与 needs_merge 成功分支互斥）、500/50015、502/50200；POST /api/wechat-bind/merge 新增「token 状态非 needs_merge → 400/40002」；POST /api/users/me/wechat-unbind 新增四类失败表；§14.4 适用端点补登 confirm 与 wechat-unbind。
- 文档 ↔ handler：三元组与 backend/handlers/iam_failure.go 的 respondIAMFailure 逐项一致（conflict → 409/40900、userAction → 400/40002、config → 500/50015 且文案与 iamConfigErrorMessage 逐字相同、transient → 502/50200）；message 不再拼接上游原始串。
- 文档 ↔ 前端：resolveErrorMessage 层序（message_map → human_message → code_map → fallback）与文档一致；50015 文案与后端常量逐字相同（码表 #2598 已入，本单未改码表）；UI 文档明确页面不得自写业务文案。

## 依赖顺序 / 验收规则

- 计划 Dependencies：#2598（统一出口 owner）已 done，其 fix 18c81bd36 在 origin/issue-2598（本单基线）；未复制 ClassifyIAMFailure / respondIAMFailure。
- 本单未修改任何验收工具或规则（pre-ready-check、gate-run、scanner、码表口径均未改动）。
- 执行前 preflight（计划 Input Truth & Tool Preflight）实测：git merge-base --is-ancestor 18c81bd36 origin/develop = 1（未集成，故用 origin/issue-2598 基线）；ClassifyIAMFailure 与 respondIAMFailure 均存在；grep "failed to bind|unbind failed|is not waiting for merge" 修前 3 命中、修后 0；grep 40902 backend/handlers/wechat_bind.go 修后 0；grep "user not found: " 的 Go 测试命中 0；修前 Bind.jsx 对 resolveErrorMessage 0 命中、func TestUnbind 0 个。

## 同类路径对称扫描

- wechat_bind.go:222 ConfirmBind → 本单修（R1）
- wechat_bind.go:381 Unbind → 本单修（R2，含 UpdateUser 类型化前置）
- wechat_bind.go:274 MergeBind 状态前置 → 本单修（R3）
- needs_merge（409/40900 + data.needs_merge）与 merged 幂等分支 → 保持（R1/R3 回归 token）
- MergeBind 合并失败已由 #2598 分流 → 保持（TestWxFailure2598 回归 PASS）
- GenBindToken（wechat_bind.go:82/104/113，微信上游瞬时故障）→ 保持 500/50000：不属 IAM 分类族，文案不含误导性冲突语义（计划 Out of Scope）
- wechat_bind.go:279/303 两条 400 分支 → 保持：码/状态已一致，仅文案体验（计划 Out of Scope）
- 扫描另见 backend/handlers/iam_proxy.go:600（IAM 用户代理更新仍 50000 + err.Error()）与 backend/handlers/user_staff.go:1687（409 + 上游串）为旧口径；二者不在本单 Files/矩阵内，属计划 Out of Scope「全站 message 直出清理（#2489 族）」，未扩面。

## 测试保真

- Unbind 用例通过 middleware.ContextKeyUserID 显式注入请求上下文构造（与 genBindTokenRouter 同模式），断言用户解析链路真实生效（非空 user 命中 DB 行）。
- fake IAM 同时服务 /api/v1/auth/token（IAMClient.doRequest 先取 client token）与 PUT /api/v1/users/{id}；缺 token 端会让 403/404 用例退化成瞬时类——R2 的 UNBIND_403_CODE=50015 通过即证明 client-token 链路确实执行。
- UPDATEUSER_404_TYPED=1 直接对 services.NewIAMClient().UpdateUser(...) 做 errors.As 的 IAMClientHTTPError 断言 + Status=404，证明分类器输入（而非仅 handler 输出）已类型化。

## 端到端验收（handler 全链路 replay）

三个入口均以 httptest 走完整 gin handler（ShouldBindJSON → bindTokens 状态机 → DB 查询 → IAM 调用 → 响应），非纯函数单测：

- POST /wechat-bind/confirm：DB 用户行 → bindTokenEntry{Status:pending} → services.NewIAMService().WxBind → fake 上游状态 → 行级断言 (status, code, message)；needs_merge 场景额外断言 token 落到 needs_merge、merge_ticket 透传。
- POST /users/me/wechat-unbind：middleware.GetUserID → users.iam_sub 查询 → IAMClient.UpdateUser → 四类三元组。
- POST /wechat-bind/merge：token 状态 bound → 400/40002 + 中文；token 状态 merged → 200 + code 20000 + data.merged=true（幂等）。

## Remote

- origin/issue-2601（fix 提交 + docs(evidence) 提交，push 后由 P13 校验可达性）

## Pre-ready gate

命令：pre-ready-check.sh 2601 --summary-file docs/evidence/2601/summary.md --changed-files docs/evidence/2601/changed-files.txt --evidence-file docs/evidence/2601/evidence.json --affected-cmd "bash scripts/verify-2601.sh gate" --report docs/evidence/2601/pre-ready-2601.json

- 报告：docs/evidence/2601/pre-ready-2601.json（overall=PASS）

## 人工 / 部署侧

无。本单全部验收行均可在 Developer 侧完成（隔离测试库 + fake IAM），不含真机/部署侧回执需求，不创建 waitForConfirm 跟踪单。

*Model: deepseek/deepseek-flash*
