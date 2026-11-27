## Work Summary — #2563（PC api 客户端非 2xx 保留业务 code：40002/40004/40005 持久提示可达）

### 修改内容

按 Accepted Plan（`## Accepted Plan — #2563` Rev.2，plan_sha256=829f87e5…）逐条执行：

1. **传输层唯一 owner（新增）**：`frontend-pc/src/services/apiError.js` 导出纯函数 `resolveHttpFailure(status, payload)`：
   - `404` → `{ code: 40400, data: null, status: 404 }`（与改动前逐字一致）；
   - `409` → `{ code: 后端 code || 40900, data, status: 409 }`（与改动前逐字一致）；
   - 其余非 2xx（400 / 403 / 429 / 5xx）→ `Error`（`name=ApiError`），把 `code`（仅有限数字时写入）、`status`、`data` 挂到 Error 上；文案仍由 `resolveErrorMessage` 解析，用户可见文本不变；
   - 损坏 payload（`null` / 字符串 / 数组）与数字字符串 code（`'40002'`）安全处理：不抛解析错、不写非法 code。
2. **客户端接线**：`frontend-pc/src/services/api.js` 的 非 2xx 块改为「解析 payload → 调用唯一 owner → `kind==='result'` 返回对象 / 否则 throw」；**401 分支、perm_version 检测、idle 超时守卫、refreshAccessToken、fetchBlob 零改动**（R2 逐项断言）。
   - 形态选择遵守 D1：**没有**把 400 改成返回对象（`40002` 在后端同时表示「缺少必填参数」与「重名冲突」，整族返回对象会让无 `else` 分支的调用点变成静默成功）。
3. **消费者（证明新逻辑在生产入口可达）**：`frontend-pc/src/pages/SiteManagement.jsx` 的 `handleSubmit` `catch` 先用共享 rubric 判定 `isBusinessCorrectableConflict(err)` → 命中则 `setFormConflict(conflictText(err, '请修改后重新提交'))` 持久提示；其余错误保留原 toast。40900/40901 的既有 `try` 内分支与 Modal 引导全部保持。
4. **测试**：新增 `frontend-pc/tests/apiError.test.mjs`（400/403/429/500/502 → throw 且带 code/status/data；404/409 → 对象；数字字符串 code；损坏 payload；rubric 判定）；`frontend-pc/tests/formConflict.test.mjs` 增补 Error 实例用例（enriched → true、plain → false、`conflictText` 取 Error.message）。
5. **文档（核心文档）**：`docs/spec/api/api.md` §1.3 新增「**PC 客户端非 2xx 契约（#2563）**」表：2xx / 401（不变）/ 404 / 409 / 其余非 2xx 五种形态 + 禁止私自把 4xx 改成返回对象。
6. **证据**：`scripts/verify-2563.sh` r1–r6（纯函数回归 / 接线结构 / 消费者接线 / 红绿对照 / 文档契约 / 构建 lint），每个 artifact 首行带 provenance。

### 涉及文件

| 类别 | 路径 |
|---|---|
| 传输映射（新增） | `frontend-pc/src/services/apiError.js` |
| API 客户端 | `frontend-pc/src/services/api.js` |
| 页面消费者 | `frontend-pc/src/pages/SiteManagement.jsx` |
| 单测（新增） | `frontend-pc/tests/apiError.test.mjs` |
| 单测（增补） | `frontend-pc/tests/formConflict.test.mjs` |
| 证据生成器（新增） | `scripts/verify-2563.sh` |
| 核心文档 | `docs/spec/api/api.md` |
| 证据 | `docs/evidence/2563/R1-transport-unit.txt`、`docs/evidence/2563/R2-client-wiring-static.txt`、`docs/evidence/2563/R3-consumer-wiring.txt`、`docs/evidence/2563/R4-redgreen-inventory.txt`、`docs/evidence/2563/R5-docs-contract.txt`、`docs/evidence/2563/R6-build-lint.txt`、`docs/evidence/2563/evidence.json`、`docs/evidence/2563/plan.md`、`docs/evidence/2563/summary.md`、`docs/evidence/2563/changed-files.txt`、`docs/evidence/2563/pre-ready-2563.json` |

### Commit Hash

- fix：`16faa351d`
- evidence：见「Remote」行的 evidence sha（证据与摘要同批提交）

### 验证层级

- 独立 Issue（无 `Parent`）：定向（`node --test` 纯函数 + 结构断言 + 改动文件 eslint）+ 全量 PC 构建 `cd frontend-pc && npm run build`（R6 / gate P5，经 host heavy-lock）。
- 父/批次全量 gate：不适用。

### Remote

`remote: origin/issue-2563; fix=16faa351d pushed=yes; evidence pushed=yes（见 P13 校验）`

### 红绿实证

**① R1 传输映射（红：400 返回对象 → 断言失败）**

```
RED（resolveHttpFailure 让 400 返回 kind=result）:
  # THROW_40002=0
  # pass 2 / # fail 3
GREEN（400 抛 Error 且带 code）:
  # THROW_40002=1 … # THROW_502=1
  # pass 5 / # fail 0
```

**② R2 接线结构（红：块内重新内联 404 返回）**

```
RED（!response.ok 内加入 if (response.status === 404) { return { code: 40400 } }）:
  INLINE_STATUS_MAP=1  NONOK_INLINE_RETURNS=1  R2_FAIL  rc=1
GREEN（改为唯一 owner 适配）:
  FAILURE_RESOLVER_CALLS=1  INLINE_STATUS_MAP=0  NONOK_INLINE_RETURNS=0  AUTH_BLOCK_BEFORE_NONOK=1  AUTH_CODES=4  R2_OK
```

**③ R4 红绿对照（由生成器自带，不依赖手写 sha）**

```
RED_BASE=58cb3a54a…（git show <fix>^ 的 request() 非 2xx 块）
RED_THROW_SITE=1  RED_NO_CODE=1        ← 修前：throw new Error(resolveErrorMessage…) 无 error.code 赋值
GREEN_CODE_ATTACHED=1                   ← 修后：apiError.js 写入 error.code
AWAIT_CALL_SITES=259  CALL_SITES_GE_150=1  NONOK_INLINE_RETURNS=0  R4_OK
```

**④ R6 真实负例（首次运行即命中）**

```
RED（worktree 未装依赖）: BUILD_EXIT=127（vite not found）  ESLINT_EXIT=2  → R6_FAIL
GREEN（npm ci 后）:      BUILD_EXIT=0  ESLINT_EXIT=0  ESLINT_NO_UNDEF=0  → R6_OK
```

### Accepted Plan 对照

| 计划项 | 状态 |
|---|---|
| A1 新增 `frontend-pc/src/services/apiError.js`（唯一 owner，404/409/其余三分支，损坏输入降级） | 完成 |
| A2 `frontend-pc/src/services/api.js` 的非 2xx 块改为 3 行适配，401/perm_version/idle 零改动 | 完成（R2 断言） |
| B3 `SiteManagement` catch 增加 rubric 判定 + 保留兜底 toast | 完成（R3 断言） |
| C4 `frontend-pc/tests/apiError.test.mjs`（400/403/429/500/502、404/409、损坏 payload、rubric 红绿） | 完成（R1，5 用例） |
| C5 `frontend-pc/tests/formConflict.test.mjs` 增补 Error 实例用例 | 完成（R3 断言） |
| C6 `scripts/verify-2563.sh` r1–r6 + provenance | 完成 |
| D7 `docs/spec/api/api.md` §1.3 增补 PC 客户端契约 | 完成（R5 断言） |

→ 7/7。

### Pre-ready gate

- 命令见 `docs/evidence/2563/evidence.json` 的 R7 行（`--summary-file docs/evidence/2563/summary.md --changed-files docs/evidence/2563/changed-files.txt --evidence-file docs/evidence/2563/evidence.json --full-gate-cmd "cd frontend-pc && npm run build" --report docs/evidence/2563/pre-ready-2563.json`）。
- 关键输出见评论末段 `### Gate 关键输出`（`RESULT: PASS`）。
- plan_sha256 = 829f87e5ef2cd3c53d4c70539fa72787a9ab02bcd897b2f2708efebb734deb66（= 计划快照 = 台账 plan_sha256 = gate 报告 plan_sha256，Rev.2）。

### Evidence Contract（7/7）

| 行 | 结果 | 产物 | 关键断言 |
|---|---|---|---|
| R1 | PASS | `docs/evidence/2563/R1-transport-unit.txt` | THROW_40002=1 … THROW_502=1、RESULT_404=1、RESULT_409=1、CODE_STRING_COERCED=1、BAD_PAYLOAD_SAFE=1、# fail 0 |
| R2 | PASS | `docs/evidence/2563/R2-client-wiring-static.txt` | FAILURE_RESOLVER_CALLS=1、NONOK_INLINE_RETURNS=0、AUTH_BLOCK_BEFORE_NONOK=1、AUTH_CODES=4、PERM_VERSION=1、IDLE_GUARD_CALLS=5、R2_OK |
| R3 | PASS | `docs/evidence/2563/R3-consumer-wiring.txt` | CONFLICT_BRANCH_IN_CATCH=1、CONFLICT_TEXT_FALLBACK=1、FALLBACK_TOAST=1、ALERT_RENDERED=1、TRUE_ENRICHED_ERROR=1、FALSE_PLAIN_ERROR=1、# fail 0 |
| R4 | PASS | `docs/evidence/2563/R4-redgreen-inventory.txt` | RED_NO_CODE=1、GREEN_CODE_ATTACHED=1、AWAIT_CALL_SITES=259、NONOK_INLINE_RETURNS=0、R4_OK |
| R5 | PASS | `docs/evidence/2563/R5-docs-contract.txt` | DOC_CONTRACT_OK=1、DOC_SECTION_LINES=13、七个 token 全 1 |
| R6 | PASS | `docs/evidence/2563/R6-build-lint.txt` | BUILD_EXIT=0、ESLINT_EXIT=0、ESLINT_NO_UNDEF=0、R6_OK |
| R7 | PASS | `docs/evidence/2563/pre-ready-2563.json` | gate 全项 PASS（含 P5 构建、P12 七行、P13 远端）；`jq -e '.overall == "PASS"' docs/evidence/2563/pre-ready-2563.json` → `true`，`exit=0` |

台账：`docs/evidence/2563/evidence.json`（schema v1，build=1.0.16faa351d，commit=16faa351d…，dirty=false，7 行各恰好一条记录）。

### G0 Golden Acceptance

| 行 | artifact | assertions（可重放） | 负例 |
|---|---|---|---|
| R1 | `docs/evidence/2563/R1-transport-unit.txt` | 12 个 token + node `# fail 0` | 任一非 404/409 产出结果对象 → FAIL |
| R2 | `docs/evidence/2563/R2-client-wiring-static.txt` | 8 个结构 token + R2_OK | 块内 return 对象 / 401 分支后移 → FAIL |
| R3 | `docs/evidence/2563/R3-consumer-wiring.txt` | 4 个 catch token + 2 个 rubric token + # fail 0 | catch 无判定 / 删兜底 toast / plain Error 判 true → FAIL |
| R4 | `docs/evidence/2563/R4-redgreen-inventory.txt` | RED_NO_CODE=1、GREEN_CODE_ATTACHED=1、调用点 ≥150 | 红样失效 / 修后仍内联返回 → FAIL |
| R5 | `docs/evidence/2563/R5-docs-contract.txt` | 段落 13 行 + 7 token + DOC_CONTRACT_OK=1 | 缺关键句 → FAIL |
| R6 | `docs/evidence/2563/R6-build-lint.txt` | BUILD_EXIT=0、ESLINT_NO_UNDEF=0 | 构建非 0 / no-undef > 0 → FAIL |
| R7 | `docs/evidence/2563/pre-ready-2563.json` | gate `.overall == PASS`、P5/P12/P13 通过 | 任一 P 失败 / 缺行 / dirty → 非 0 |

行间不复用证据：R1 纯函数、R2 客户端结构、R3 消费者结构 + rubric、R4 修前/修后对照、R5 文档、R6 构建 lint、R7 gate 报告各自独立 artifact。

### Dependency Order / 验收规则

- 计划 `## Dependencies` = 无；`#2480` 共享件（`frontend-pc/src/utils/formConflict.js`、`frontend-pc/src/components/FormConflictAlert.jsx`）已在 develop，本单只消费、未复制白名单。
- 本单**未修改**任何验收工具/规则（`frontend-pc/tests/formConflict.test.mjs` 仅**新增**用例，既有断言未改；`frontend-pc/src/utils/formConflict.js` 未动）。
- 未改后端状态码/错误码语义（D7）；未改 401/perm_version/idle（D3）；未动 `frontend-mobile`（Out of Scope）。
- 范围内消费者只有 `SiteManagement`（D4）；分类页由 #2564 承接，Batch 2 由 #2486 承接。

### 矩阵 n/n（7/7）

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 非 2xx 失败分类纯函数（code 保留、404/409 不变） | PASS | `docs/evidence/2563/R1-transport-unit.txt` |
| R2 客户端接线（401/perm_version/idle 不变 + throw 契约保留） | PASS | `docs/evidence/2563/R2-client-wiring-static.txt` |
| R3 消费者接线（网点提交 400 冲突转持久提示） | PASS | `docs/evidence/2563/R3-consumer-wiring.txt` |
| R4 红绿实证 + 回归面不扩张（全仓 throw 依赖不变） | PASS | `docs/evidence/2563/R4-redgreen-inventory.txt` |
| R5 API 规格同步客户端非 2xx 契约 | PASS | `docs/evidence/2563/R5-docs-contract.txt` |
| R6 构建 + ESLint | PASS | `docs/evidence/2563/R6-build-lint.txt` |
| R7 pre-ready gate（四源一致 + 证据完整 + P13 已推送） | PASS | `docs/evidence/2563/pre-ready-2563.json` |

### 证据真实性/持久化

- 全部 artifact 在 `docs/evidence/2563/`（已提交），首行 provenance：build=1.0.16faa351d、commit=16faa351de3842a750f8f09c4bb0fe6f063d32a0、dirty=false。
- 证据在 fix commit **之后**从干净树重新生成（R1–R6 均以 `dirty=false` 重跑），未在脏树/修复前构建上取证据。
- 构建与 eslint 经 host heavy-lock，未绕过锁。

### 测试保真

- R1/R3 的 `node --test` 断言直接 `import` 生产模块（`frontend-pc/src/services/apiError.js`、`frontend-pc/src/utils/formConflict.js`），不是复制逻辑；
- R2/R4 的 awk/grep 断言按 `request()` 函数体与非 2xx 块边界取值（`frontend-pc/src/services/api.js` 的 `export const api` 为终止锚点，规避该函数内历史列 0 缩进的 `}`）；
- R3 的 catch 块断言绑定真实 `handleSubmit` 源码切片，不是全文 grep。

### 契约三向核对

| 方向 | 内容 | 证据 |
|---|---|---|
| Plan/API | 计划 A1/A2 规定「404/409 返回对象；其余非 2xx 抛 Error 且带 code/status/data」；实现逐字落地，401/perm_version/idle 零改动 | `docs/evidence/2563/R2-client-wiring-static.txt` |
| Docs | `docs/spec/api/api.md` §1.3 新增「PC 客户端非 2xx 契约（#2563）」表，与服务端响应格式并列；明确禁止私自把 4xx 改成返回对象 | `docs/evidence/2563/R5-docs-contract.txt` |
| Handler/前端 | 后端未改（CreateCategory / UpdateCategory 仍为 HTTP 400 + 40002）；前端消费者 `frontend-pc/src/pages/SiteManagement.jsx` 的 catch 用共享 rubric 判定 | `docs/evidence/2563/R3-consumer-wiring.txt` |

三者一致：服务端码语义不变（40002 仍两义），客户端只新增「Error 携带 code」而不改变抛/不抛的控制流；前端只在真正可达的 catch 判定（`SiteManagement`）。

### Round 2 审计修复（Rev.2 收口）

Round 2 审计判 REJECT，三个阻断项均已修复，**产品逻辑零改动**：

| Bug | 修复 | 证据 |
|---|---|---|
| Bug 1：注释/Ops-1 把「网点重名」写成 `HTTP 400 + 40002` | 注释改为真实可达的 400+40002 场景（参数不合法 / 商户 org 缺失 / 商户 org 非法 / 锚点网点禁止改名，即 `backend/handlers/site.go` 的 233、273、277、608 行），并显式声明网点重名走 `HTTP 409 + 40900` 的 try 内分支；Ops-1 同上改写，重名验证拆为 Ops-1b（40900 既有路径）与 Ops-2（#2564 分类页 400+40002） | `frontend-pc/src/pages/SiteManagement.jsx`（307-313 行）、本节 Ops-1/Ops-1b/Ops-2 |
| Bug 2：R7 `negative_result` 仍写已废弃的 result 键，且缺 jq 实跑输出 | `negative_result` 改为 overall 键（`overall != PASS`）；jq 实跑输出（`true` / `exit=0`）写入 R7 `actual` 与本摘要 R7 行（无新增 artifact，避免与 Files 清单错位） | `docs/evidence/2563/evidence.json` 的 R7 行、本摘要 R7 行 |
| Bug 3：summary 仍写 Rev.1 的 `plan_sha256=975dcebf…` | 两处改为 Rev.2 `829f87e5ef2cd3c53d4c70539fa72787a9ab02bcd897b2f2708efebb734deb66`，与 `docs/evidence/2563/plan.md` / `docs/evidence/2563/evidence.json` / `docs/evidence/2563/pre-ready-2563.json` 四源一致 | `docs/evidence/2563/summary.md`（第 5、102 行） |

**修订说明（Ops-1 原文与后端事实不符）**：Round 1 的 Ops-1 沿用「网点重名 = HTTP 400 + 40002」，但仓库事实是 `backend/handlers/site.go` 的 343 行（IAM 组织名冲突）、625-627 行（duplicate site name）、666-668 行（`ErrIAMOrgNameConflict`）全部返回 **HTTP 409 + 40900**；`backend/handlers/site.go` 的 16 处 `40002` 中无一条是重名。原措辞会让预生产回执指向一条不可达路径，故按真实码路径改写，重名验证改由 40900 既有分支 + #2564 承担。

### 人工/部署侧

| 项 | 内容 | 责任人 | 回执要求 |
|---|---|---|---|
| Ops-1 | 预生产：网点新建/改名业务冲突（**HTTP 400 + 40002**：参数不合法 / 商户 org 缺失 / 商户 org 非法 / 锚点网点禁止改名，即 `backend/handlers/site.go` 的 233、273、277、608 行）→ 表单内常驻提示截图 | Owner/运维 | 截图 + 停留形态（非 3 秒消失） |
| Ops-1b | 预生产：**网点重名**（仓库事实为 **HTTP 409 + 40900**，`backend/handlers/site.go` 的 duplicate name / IAM org 名冲突分支，**不是** 400+40002）→ 既有 409 分支截图（不改逻辑） | Owner/运维 | 截图 |
| Ops-2 | 预生产：**分类**重名（HTTP 400 + 40002，`backend/handlers/api.go` 的 874-879 行）在 #2564 落地后 → 常驻提示截图 | Owner/运维 | #2564 的验收 |
| Ops-3 | 下游：`#2486` Batch 2 在各页 `/analyze` 时按新契约把 40002/40004/40005 行判为可转换（不再 defer） | Architect | #2486 计划更新记录 |

*Model: DeepSeek V4.1 Flash 1017*
