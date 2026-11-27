# Work Summary — #2601 wechat_bind.go 持久 IAM 失败三元组（Rev.2 / 修 Round 1 audit Bug 1）

## 变更摘要

ConfirmBind / Unbind 曾把 IAM 的**任一**失败压成 HTTP 500 + code 50000 并拼接「failed to bind: / unbind failed: <上游原始串>」，PC/mobile 码表 50000 =「服务器开小差了，请稍后再试」——IAM 401/403/404（权限模板缺失、client/user 不存在）这类**必须改配置**的持久错误被表达成可重试。Rev.1 已改接 #2598 的统一出口（services 的 ClassifyIAMFailure + handlers 的 respondIAMFailure），并按同一口径修掉同批扫描发现的两处同类点（MergeBind 状态前置码/状态不一致、Bind 页失败文案硬编码「请重试」）。

**Rev.2（本轮）只做 Round 1 audit Bug 1 的收口**：Bind 页失败态改由 `resolveErrorMessage` 后，两个**可达**的既有 ASCII 分支（`400/40004` `invalid or expired token`、`400/40002` `invalid or expired merge ticket` / `merge ticket does not match binding token`）会落到 `ERROR_CODE_MAP` 的语义无关文案（「数据已存在，请勿重复操作」/「信息不完整，请补充必填项」）。按 Rev.2 **D6b（L1）** 在 `ERROR_MESSAGE_MAP` 补 3 条 ASCII→中文映射（`ERROR_CODE_MAP` 的 40004/40002 **一律不动**），并补 R8 渲染证据与 harness `r8`。

| 变更点 | 文件 |
|---|---|
| ConfirmBind 失败出口四类分流（needs_merge 分支逐字不变） | `backend/handlers/wechat_bind.go` |
| Unbind 失败出口四类分流 | `backend/handlers/wechat_bind.go` |
| MergeBind token 状态前置 `400/40002` + 可行动中文（内部状态串只进日志） | `backend/handlers/wechat_bind.go` |
| `backend/services/iam_client.go` 的 UpdateUser 非 2xx 类型化（`IAMClientHTTPError`，403 文本逐字保持） | `backend/services/iam_client.go` |
| Bind 页失败态文案改由 `resolveErrorMessage` 给出 | `frontend-mobile/src/pages-weapp/bind/Bind.jsx` |
| **Rev.2**：ASCII 过期/失效串 → 中文可行动文案（3 条，D6b/L1） | `frontend-mobile/src/services/errorMessages.js` |
| handler 状态矩阵测试（fake IAM） | `backend/handlers/wechat_bind_failure_2601_test.go` |
| Bind 页渲染测试（jsdom，含 R8 三例） | `frontend-mobile/scripts/page-smoke/bind-failure-2601.test.jsx` |
| 证据 harness（r1–r6、r8、gate） | `scripts/verify-2601.sh` |
| 契约文档同步 | `docs/spec/api/api.md`、`docs/spec/ui/ui.md` |

- fix（Rev.1 主体）：`48081b74e3f6755d956027c0101d36c9740c2792`
- **fix（Rev.2，本轮，evidence.json `commit` 锚点）**：`927158cf24e146f1a8cd492f0ba1e7296f667284`
- 分支 / 基线：`issue-2601`，基于 `origin/issue-2598@1e207b9a8`（#2598 的 fix `18c81bd36` 未集成 `origin/develop`，D1 口径已复核）

## 验证层级

- 独立 Issue（正文无 `Parent: #N`）：backend `go build ./...` + 三条定向测试（`TestConfirmBind2601_Triples` / `TestUnbind2601_Triples` / `TestMergeBind2601_StateMismatch`）+ gofmt + eslint（no-undef）；前端 jsdom 渲染 9 例（R4 六 token + R8 三 token）。
- 全量 `go test ./...` 不在本单（#2651 吞吐规则，由批次 gate 承担）。
- 上一轮已回归的既有用例（`TestConfirmBind*` / `TestMergeBind*` / `TestBindMergeFlow_2445` / `TestMigrateMergedAccountLocal_2445` / `TestWxFailure2598` / `TestUpdateUser_*`）在 `gate` 的 `go build` + 定向测试口径下保持通过；本单未改其语义。

## 红绿实证

**Rev.2（R8，本轮）** —— 同一条 `frontend-mobile/scripts/page-smoke/bind-failure-2601.test.jsx`，先回退 `frontend-mobile/src/services/errorMessages.js` 的三条映射再跑：

```
$ git checkout -- frontend-mobile/src/services/errorMessages.js && bash scripts/verify-2601.sh r8
 Tests  3 failed | 6 passed (9)
 AssertionError: expected '❌合并失败信息不完整，请补充必填项请让管理员重新生成二维码后再扫码。返回' to contain '已失效'
 AssertionError: ... expected ... to contain '已失效'
 (确认用例) code 40004 + 'invalid or expired token' → 正文落 ERROR_CODE_MAP[40004]「数据已存在，请勿重复操作」
FAIL(r8): vitest bind-failure-2601 (expired rows) exit=1
```

恢复三条映射后：`BIND_UI_EXPIRED_TOKEN_TEXT=1`、`BIND_UI_EXPIRED_NO_CODE_MAP_MISMATCH=1`、`BIND_UI_EXPIRED_RENDER_FAIL=0`（9/9 通过）。

**Rev.1（R1–R4，上一轮已落库，本轮重跑保持绿）** —— 未修实现上：

```
--- FAIL: TestConfirmBind2601_Triples
    Messages: 上游 400 必须 400（不得 500/50000）: failed to bind: IAM returned status: 400, ...
--- FAIL: TestUnbind2601_Triples
    Messages: 上游 400 必须 400（不得 500/50000）: unbind failed: UpdateUser returned status 400: ...
--- FAIL: TestMergeBind2601_StateMismatch
    Messages: 状态前置必须 40002（与相邻 invalid/expired merge ticket 同族），不得 40902
```

## Accepted Plan（Rev.2）对照

1. ConfirmBind 失败出口四类分流（needs_merge 逐字不变） — 完成（R1）
2. `backend/services/iam_client.go` 的 UpdateUser 非 2xx 类型化（403 文本保持 / 404 可 errors.As） — 完成（R2）
3. Unbind 失败出口四类分流 — 完成（R2）
4. MergeBind 状态前置 `400/40002` + 日志化内部状态串，幂等分支不变 — 完成（R3）
5. Bind 页 `error` / `merge_error` 文案走 `resolveErrorMessage`，判定分支不动 — 完成（R4）
6. Core Update：`docs/spec/api/api.md` §14.4 适用端点 + confirm 四行失败三元组 + merge 状态行 + unbind 表；`docs/spec/ui/ui.md` §2.6.2 Bind 页失败提示 — 完成（R5）
7. **Rev.2 D6b**：`ERROR_MESSAGE_MAP` +3 条 ASCII→中文，`ERROR_CODE_MAP` 40004/40002 未动 — 完成（R8）
8. 测试/证据：handler 三组测试 + 渲染 9 例 + harness `r1–r6`/`r8`/`gate` + 8 行证据 — 完成（R1–R8）

### 契约三向核对（P7）

- **Plan / API**：confirm 失败 → `400/40002`（上游 400/422）、`409/40900`（上游 409）、`500/50015`（401/403/404）、`502/50200`（瞬时）；merge 状态前置 `400/40002`；unbind 四类同构 —— 与 Accepted Plan「三元组口径」表逐项一致。
- **文档**：`docs/spec/api/api.md` §14.4 适用端点含 `POST /api/wechat-bind/confirm`、`POST /api/users/me/wechat-unbind`；confirm 段四行失败三元组；merge 段状态前置行；unbind 四类失败表。`docs/spec/ui/ui.md` §2.6.2 新增「Bind 页（扫码绑定）失败提示（#2601）」。
- **handler / frontend**：handler 只走 `respondIAMFailure` + `iamFailureTexts`（无第二份码表）；前端文案唯一 owner = `frontend-mobile/src/services/errorMessages.js`（`ERROR_CODE_MAP` / `ERROR_MESSAGE_MAP` / `resolveErrorMessage`），页面仅保留 fallback 与引导句；Rev.2 的 3 条映射落在 `ERROR_MESSAGE_MAP`（L1），未触碰 `ERROR_CODE_MAP`。

## Evidence Contract（8/8 PASS）

| 行 | 结果 | 产物 |
|---|---|---|
| R1 | PASS | `docs/evidence/2601/R1-confirm-triples.txt` |
| R2 | PASS | `docs/evidence/2601/R2-unbind-triples.txt` |
| R3 | PASS | `docs/evidence/2601/R3-mergebind-state.txt` |
| R4 | PASS | `docs/evidence/2601/R4-bind-ui.txt` |
| R5 | PASS | `docs/evidence/2601/R5-docs.txt` |
| R6 | PASS | `docs/evidence/2601/R6-build-lint.txt` |
| R7 | PASS | `docs/evidence/2601/pre-ready-2601.json` |
| R8 | PASS | `docs/evidence/2601/R8-bind-expired.txt` |

证据账本：`docs/evidence/2601/evidence.json`（schema v1，`dirty=false`，`commit=927158cf2…`，build=1.0.927158cf2，8 行各一条记录、`pass=true`、产物全部存在）。

## G0 Golden Acceptance

| 行 | artifact | 关键断言（可重放） |
|---|---|---|
| R1 | `docs/evidence/2601/R1-confirm-triples.txt` | `CONFIRMBIND_400_CODE=40002`、`CONFIRMBIND_403_STATUS=500`、`CONFIRMBIND_403_CODE=50015`、`CONFIRMBIND_409_CODE=40900`、`CONFIRMBIND_TRANSIENT_STATUS=502`、`CONFIRMBIND_TRANSIENT_CODE=50200`、`CONFIRMBIND_PERSISTENT_NO_RETRY=0`、`CONFIRMBIND_RAW_LEAK=0`、`CONFIRMBIND_NEEDS_MERGE_KEPT=1` |
| R2 | `docs/evidence/2601/R2-unbind-triples.txt` | `UNBIND_400_CODE=40002`、`UNBIND_403_STATUS=500`、`UNBIND_403_CODE=50015`、`UNBIND_TRANSIENT_STATUS=502`、`UNBIND_TRANSIENT_CODE=50200`、`UNBIND_PERSISTENT_NO_RETRY=0`、`UNBIND_RAW_LEAK=0`、`UPDATEUSER_403_TEXT_KEPT=1`、`UPDATEUSER_404_TYPED=1` |
| R3 | `docs/evidence/2601/R3-mergebind-state.txt` | `MERGEBIND_STATE_STATUS=400`、`MERGEBIND_STATE_CODE=40002`、`MERGEBIND_STATE_MSG_ACTIONABLE=1`、`MERGEBIND_STATE_INTERNAL_LEAK=0`、`MERGEBIND_IDEMPOTENT_KEPT=1` |
| R4 | `docs/evidence/2601/R4-bind-ui.txt` | `BIND_UI_NEEDS_MERGE_RENDER=1`、`BIND_UI_50015_CONFIG_TEXT=1`、`BIND_UI_50015_NO_RETRY_HINT=1`、`BIND_UI_50200_RETRY_TEXT=1`、`BIND_UI_40002_SERVER_TEXT=1`、`BIND_UI_RENDER_FAIL=0` |
| R5 | `docs/evidence/2601/R5-docs.txt` | `API_CONFIRM_ENDPOINT=1`、`API_UNBIND_ENDPOINT=1`、`API_CONFIRM_FAILURE_ROWS=1`、`API_MERGE_STATE_ROW=1`、`DOC_BIND_FAILURE_TEXT=1` |
| R6 | `docs/evidence/2601/R6-build-lint.txt` | `GO_BUILD_OK=1`、`GOFMT_DIFF=0`、`ESLINT_NOUNDEF=0` |
| R7 | `docs/evidence/2601/pre-ready-2601.json` | `overall=PASS`、P12 每行 `pass=true`、P13 提交在 `origin/issue-2601` |
| R8 | `docs/evidence/2601/R8-bind-expired.txt` | `BIND_UI_EXPIRED_TOKEN_TEXT=1`、`BIND_UI_EXPIRED_NO_CODE_MAP_MISMATCH=1`、`BIND_UI_EXPIRED_RENDER_FAIL=0` |

## 矩阵 n/n

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 ConfirmBind 失败出口接入统一三元组 | PASS | `docs/evidence/2601/R1-confirm-triples.txt` |
| R2 Unbind 失败出口 + UpdateUser 类型化 | PASS | `docs/evidence/2601/R2-unbind-triples.txt` |
| R3 MergeBind token 状态前置码/状态一致 | PASS | `docs/evidence/2601/R3-mergebind-state.txt` |
| R4 Bind 页失败态展示（渲染） | PASS | `docs/evidence/2601/R4-bind-ui.txt` |
| R5 契约文档同步（端点清单 + 失败三元组） | PASS | `docs/evidence/2601/R5-docs.txt` |
| R6 构建 / gofmt / ESLint（no-undef） | PASS | `docs/evidence/2601/R6-build-lint.txt` |
| R7 pre-ready gate（四源 + 合同完整性 + 远端可达） | PASS | `docs/evidence/2601/pre-ready-2601.json` |
| R8 Bind 页过期/失效分支文案（渲染） | PASS | `docs/evidence/2601/R8-bind-expired.txt` |

## Remote

- `remote: origin/issue-2601`；fix `48081b74e`（Rev.1）+ `927158cf2`（Rev.2）pushed=yes；evidence `docs(evidence): issue-2601`（本轮）pushed=yes。

## 证据真实性 / 持久化

7 份文本 artifact 的首行（逐字）与账本 `docs/evidence/2601/evidence.json`（build=1.0.927158cf2、commit=927158cf24e146f1a8cd492f0ba1e7296f667284、dirty=false）一致：

| artifact | 首行（逐字） |
|---|---|
| `docs/evidence/2601/R1-confirm-triples.txt` | `# build=1.0.927158cf2 # commit=927158cf24e146f1a8cd492f0ba1e7296f667284 # dirty=false` |
| `docs/evidence/2601/R2-unbind-triples.txt` | `# build=1.0.927158cf2 # commit=927158cf24e146f1a8cd492f0ba1e7296f667284 # dirty=false` |
| `docs/evidence/2601/R3-mergebind-state.txt` | `# build=1.0.927158cf2 # commit=927158cf24e146f1a8cd492f0ba1e7296f667284 # dirty=false` |
| `docs/evidence/2601/R4-bind-ui.txt` | `# build=1.0.927158cf2 # commit=927158cf24e146f1a8cd492f0ba1e7296f667284 # dirty=false` |
| `docs/evidence/2601/R5-docs.txt` | `# build=1.0.927158cf2 # commit=927158cf24e146f1a8cd492f0ba1e7296f667284 # dirty=false` |
| `docs/evidence/2601/R6-build-lint.txt` | `# build=1.0.927158cf2 # commit=927158cf24e146f1a8cd492f0ba1e7296f667284 # dirty=false` |
| `docs/evidence/2601/R8-bind-expired.txt` | `# build=1.0.927158cf2 # commit=927158cf24e146f1a8cd492f0ba1e7296f667284 # dirty=false` |

- 七份的第二行均为 `GENERATED_AT_HEAD=ddfe4268f3bfc05285cb68d582f67a47e30cdb68`（本次重跑时的分支 HEAD；首行的 `build/commit` 锚点始终指向 Rev.2 fix commit `927158cf2`，与 `docs/evidence/2601/evidence.json` 一致）。
- R1–R3 的断言为真实 handler + fake IAM + Postgres 测试库（127.0.0.1:15433/tuneloop_test）读回；R4/R8 为 jsdom + `@testing-library/react` 真实渲染（`render` / `waitFor` / `fireEvent`）。
- R5 为文档结构断言（`evidence_type: static`），R6 为 build/lint，R7 为 gate 报告（报告内的 provenance 同样锚定 build=1.0.927158cf2 / commit=927158cf2… / dirty=false，由 gate 从 `docs/evidence/2601/evidence.json` 继承）。
- 不同矩阵行使用各自独立 artifact，无复用；`tmp/` 仅作原始日志。

## Round 2 审计阻断点闭环

| 阻断点 | 整改动作 | 可核验证据 |
|---|---|---|
| 1 R1–R6 provenance 锚 Rev.1 | 六份在 Rev.2 fix commit 锚点重跑并**入库**（此前重跑结果只留在工作树、未提交） | 上表七行首行；`bash ~/.config/opencode/scripts/evidence-artifacts.sh --issue 2601 --repo .` → `PASS: 8 artifacts` |
| 2 Summary 自证与产物冲突 | 本节改为逐 artifact 列首行（可逐字比对），删除「均已重跑」式无产物支撑表述 | 本节表格 vs 各 artifact 的 head -1 |
| 3 gate P14 在已提交状态不可复现 | R1–R6 入库后再跑 gate（报告提交在证据之后），并在干净树把 report 指向 tmp/ 复跑同一命令验证可复现 | `docs/evidence/2601/pre-ready-2601.json`（overall=PASS、P14 PASS、plan_sha256=7c98a665…） |
| 4 `docs/evidence/2601/changed-files.txt` 漏 R8 | 补 `docs/evidence/2601/R8-bind-expired.txt`，与 1e207b9a8..HEAD 的 diff 名单 21 行逐行相等 | `docs/evidence/2601/changed-files.txt` 共 21 行；与 diff 名单对比无输出 |

## Dependency Order / 验收规则

- 前置 `#2598` 已 `done`；其 fix `18c81bd36` 未集成 `origin/develop`，本单按 D1 以 `origin/issue-2598@1e207b9a8` 为基线（worktree 已复核 `git merge-base --is-ancestor 18c81bd36 HEAD` = 0）；合并顺序 #2598 → #2601。
- 本单**未修改任何验收工具或规则**（未改 opencode 的 pre-ready 校验脚本 / 工作流仓库的 gate-run.sh / scanner / 码表口径）；`scripts/verify-2601.sh` 属计划 Files 内的证据 harness。
- Out of Scope 保持未触碰：`GenBindToken` 的微信上游 `500/50000`、`wx-merge-confirm` 200 但 `merged=false`、`UpdateUserWithToken` 及其余 IAM 方法、两条 400 分支的 ASCII 文案、`backend/handlers/auth.go` 注册流程 WxBind 分支 注册回滚、PC `frontend-pc/src/components/WechatBinding.jsx` toast、全站 `50200` 入码表、`docs/cases/h5-weapp-differences.md`（Bind 页仅 weapp）。

## 人工 / 部署侧

- weapp 真机复验（用户是否真看到「已失效/请稍后/联系管理员」而非通用文案）在 closing 时按 D10 开 `[Ops-Followup of #2601]`（`waitForConfirm`，无 `status:*`），不写成已完成。

## Pre-ready gate

- 命令：

```
~/.config/opencode/scripts/pre-ready-check.sh 2601 \
  --summary-file docs/evidence/2601/summary.md \
  --changed-files docs/evidence/2601/changed-files.txt \
  --evidence-file docs/evidence/2601/evidence.json \
  --affected-cmd "bash scripts/verify-2601.sh gate" \
  --report docs/evidence/2601/pre-ready-2601.json
```

- 结果：

```
RESULT: PASS
REPORT: docs/evidence/2601/pre-ready-2601.json
P1/P2/P3/P4/P6/P7/P11/P12/P13/P14/P17/P18 = PASS；P5 = N/A（无 go.mod/package.json 于根，全量由批次 gate 承担）；P8/P9/P10/P15/P16 = N/A
```

- plan_sha256 = `7c98a6652062115762d681826c976c94fd4dc1374f6ee4d82298838e8aa64515`（Accepted Plan Rev.2 评论，plan-source FRESH）。

*Model: DeepSeek V4.1 Flash 1017*
