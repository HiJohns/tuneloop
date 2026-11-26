## Accepted Plan — #2563（PC api 客户端非 2xx 保留业务 code：40002/40004/40005 持久提示可达）｜Rev.2

### 背景与现状核对（仓库 HEAD 只读核对）

| 事实 | 位置 |
|---|---|
| `request()` 非 2xx 处理：`401` 分支（40104/40105/40106/40107 + 刷新）先于 `!response.ok`；`!response.ok` 内 404 返回对象、409 返回对象、**其余一律 `throw new Error(已翻译文案)`（Error 上无 `code`/`status`/`data`）** | `frontend-pc/src/services/api.js:245`（request 起点）、`282`（perm_version）、`297-330`（401）、`331-353`（404 = 334、409 = 343、throw = 352） |
| 分类重名：`CreateCategory` → `HTTP 400 + 40002 + "分类名称已存在"`；`UpdateCategory` 同 | `backend/handlers/api.go:874-879`、`backend/handlers/api.go:953-958` |
| 网点创建：多处 `HTTP 400 + 40002`（invalid parameters / 商户 org 缺失 / parent_id 非法 等） | `backend/handlers/site.go:232-290` |
| 「业务可纠正冲突」白名单唯一 owner = `40002/40004/40005/40900`（`isBusinessCorrectableConflict` 取 `Number(result.code)`） | `frontend-pc/src/utils/formConflict.js:8-14` |
| 现有 409 消费者在 `try` 内判 `result.code`（HTTP 409 返回对象，故可达） | `frontend-pc/src/pages/SiteManagement.jsx:285-287`、`frontend-pc/src/pages/StaffManagement.jsx:351-358` |
| 网点提交 `catch` 只做通用 toast：HTTP 400 的 40002 落到这里，code 已丢，无法转持久提示 | `frontend-pc/src/pages/SiteManagement.jsx:306-310` |
| 员工创建走 `HTTP 400 + 40001`（即时提示域，不在白名单）→ 本单不改其行为 | `backend/handlers/user_staff.go:205-225` |
| PC 无组件测试运行器；纯逻辑用 `node --test`（`frontend-pc/tests/*.mjs` 既有 9 个） | `frontend-pc/package.json`（无 test 脚本）、`frontend-pc/tests/` |
| 消息翻译层唯一出口 `resolveErrorMessage`（L1 message_map → L2 中文透传 → L3 code_map → L5 fallback），纯函数 | `frontend-pc/src/services/errorMessages.js:110-137` |
| 全仓 `await api.*/xxxApi.*` 调用点 256 处（含 209 处邻近 `catch`），全部依赖「非 2xx 抛错」语义 | `grep -rn "await \(api\|[a-zA-Z]*Api\)\." frontend-pc/src` |

### 变更点

**A. 传输层（形态决策见 D1）**

1. 新增 `frontend-pc/src/services/apiError.js`（非 2xx 失败映射的**唯一 owner**，纯函数）：
   - 导出 `resolveHttpFailure(status, payload)`，返回 `{ kind: 'result', value }` 或 `{ kind: 'throw', value }`；
   - `404` → `{ code: 40400, message: resolveErrorMessage({code:40400, message}, '未找到相关数据'), data: null, status: 404 }`（与现行逐字一致）；
   - `409` → `{ code: payload.code||40900, message: resolveErrorMessage({code, message}, '数据冲突，请刷新后重试'), data: payload.data, status: 409 }`（与现行逐字一致）；
   - **其余非 2xx（400/403/404 之外的 4xx、3xx、5xx）** → `Error`（`name='ApiError'`），把 `code`（有限数字时才写）、`status`、`data` 挂到 Error 上；`message` 仍走 `resolveErrorMessage(..., '请求失败，请重试')`，**文案与现状逐字一致**；
   - 非对象/损坏 payload（`null`、字符串、数组、`code` 非数字）一律安全降级：不抛解析错、不写入非法 `code`。
2. `frontend-pc/src/services/api.js`：`request()` 的 `!response.ok` 块改为 3 行适配（`payload` → `resolveHttpFailure` → `kind==='result'` 返回、否则 `throw`）；**不得**改动 401 分支、`perm_version` 检测、idle 超时（`isIdleTimeoutPayload`）、`refreshAccessToken`、`fetchBlob`。

**B. 消费者（证明新逻辑在生产入口可达）**

3. `frontend-pc/src/pages/SiteManagement.jsx` 的 `handleSubmit` `catch`：先 `if (isBusinessCorrectableConflict(err)) { setFormConflict(conflictText(err, '请修改后重新提交')); return }`，再保留原有 `message.error` 兜底；40311/40901/40902 的既有 Modal 引导分支与 40900 的 `try` 内分支全部保持。`StaffManagement`／分类页不改（理由见 D4）。

**C. 测试与证据**

4. 新增 `frontend-pc/tests/apiError.test.mjs`（`node --test`）：400/403/429/500/502 → `kind=throw` 且 `error.code/status/data` 正确；404/409 → `kind=result` 且 code/status/message 与现行契约一致；损坏 payload 降级；`isBusinessCorrectableConflict(new Error without code)=false` vs `isBusinessCorrectableConflict(enriched 40002)=true`（红绿语义）。
5. `frontend-pc/tests/formConflict.test.mjs` 增补：带 `code` 的 `Error` 实例判 true、普通 `Error` 判 false、`conflictText` 取 `Error.message`。
6. 新增 `scripts/verify-2563.sh`（r1–r6 证据生成器，每个 artifact 首行写 provenance）。

**D. 文档**

7. `docs/spec/api/api.md` §1.3（统一响应格式）追加「PC 客户端非 2xx 契约」段落：404/409 返回对象、其余非 2xx 抛 Error 且带 `code`/`status`/`data`；业务可纠正冲突必须在 `catch` 判定；401/perm_version/idle 不变。

- Tags: bug, scope:pc, scope:frontend, api-contract

### Workload

- Size: L
- Basis: 7 个文件（api 客户端 + 新纯函数模块 + 页面消费者 + 2 个测试 + 证据脚本 + 核心文档）+ 7 条证据行；无 migration、无后端改动、无外部依赖
- Confidence: high

## Core Update

| 文档路径 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §1.3 只定义服务端统一响应格式与消息翻译层序（L1–L5），未定义 PC 客户端对非 2xx 的三种处理形态 | 在 §1.3 的「用户可见文案」提示块之后追加「**PC 客户端非 2xx 契约（#2563）**」：`404` → 返回 `{code:40400,...}`；`409` → 返回 `{code:后端 code 或 40900,...}`；其余非 2xx（400/403/429/5xx）→ 抛 `Error` 并携带 `code`/`status`/`data`（owner 为 `frontend-pc/src/services/apiError.js`）；业务可纠正冲突（40002/40004/40005）须在 `catch` 分支判定；401/perm_version/idle 分支不变 | 该契约是「持久提示可否达」的前置条件；不写进 API 规格，后续页面会再次假设「400 拿不到 code」而把行判为 defer（本 Issue 与 #2485 D3 的来源） |

## Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 分类重名、网点表单业务冲突等 HTTP 400 错误只能播 3 秒 toast，用户读不完即消失；`isBusinessCorrectableConflict(result)` 永不命中 | `frontend-pc/src/utils/formConflict.js:11-15` 与 `frontend-pc/src/services/api.js:352` 互斥 |
| 2 | 触发条件（trigger） | 后端返回 `HTTP 400 + code 40002/40004/40005`（非 404/409），客户端进入 throw 分支 | `backend/handlers/api.go:874-879`、`frontend-pc/src/services/api.js:331-353` |
| 3 | 根因（root cause） | 传输层把「HTTP 状态」与「业务 code」解耦：非 404/409 只 `throw new Error(message)`，业务 code 在边界处被丢弃；上层 rubric 以 `result.code` 为判定入口 → 结构上不可达 | `frontend-pc/src/services/api.js:352` |
| 4 | 测试盲点（test blind spot） | 既有测试只覆盖 `formConflict` 纯函数（`{code:40002}` 直接造对象），从不验证「该对象由客户端真实产出」；PC 无组件测试运行器，集成路径无任何自动化断言 | `frontend-pc/tests/formConflict.test.mjs`、`frontend-pc/package.json` |
| 5 | 同类扫描（same-class paths） | `frontend-mobile/src/services/api.js` 同族缺口（`throw new Error(error.message || error.code)`，Error 无 code）；PC 侧 `refreshAccessToken`/`fetchBlob` 亦有意不暴露 code（属鉴权/下载域，本单不动） | `frontend-mobile/src/services/api.js:491-498`、`frontend-pc/src/services/api.js:168-181/245+` |
| 6 | 修复层（fix layer） | 落在传输层（唯一 owner 模块 `services/apiError.js`），不在每个页面各自解析响应；页面只消费共享判定函数 | 本计划 R1/R2 |
| 7 | 红绿实证（red-green） | 红：`git show <fix>^:frontend-pc/src/services/api.js` 的非 2xx 块无 `error.code` 赋值；绿：修后 `apiError.js` 对 400/403/429/500 全部产出带 `code` 的 Error，且 `isBusinessCorrectableConflict(enriched)=true` / `(plain Error)=false` | 本计划 R1/R4 |
| 8 | 防复发（prevention） | 契约写入 `docs/spec/api/api.md` §1.3；`NONOK_INLINE_RETURNS=0` 断言锁死「不得再把某个 4xx 私自改成返回对象」；rubric 白名单仍唯一 owner 于 `formConflict.js`，传输层不复制 | 本计划 R2/R5 |
| 9 | 影响面（blast radius） | 256 个调用点全部依赖 throw 语义；本次只做 **Error 增强**（新增字段），不改「抛/不抛」与文案，故调用点控制流不变；仅 `SiteManagement` 主动新增 catch 分支 | 本计划 R4，`grep -rn "await \(api\|[a-zA-Z]*Api\)\." frontend-pc/src` |
| 10 | 未修边界（deferred） | 分类页 40002 重名恢复持久提示 → #2564（承接 #2485 D3 defer）；Batch 2 其余 13 文件 → #2486；mobile 同族缺口无消费者不立项 | #2564、#2486 |

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 非 2xx 失败分类纯函数（code 保留、404/409 不变） | `frontend-pc/src/services/apiError.js`、`frontend-pc/tests/apiError.test.mjs` | `node --test`（`resolveHttpFailure`） | 400/403/429/500/502 带 code；404；409（含 40900/40901/中文 message）；损坏 payload | Node 20 | 400/403/429/500/502 → `kind=throw` 且 `error.code`/`status`/`data` 正确；404 → `code=40400,status=404`；409 → code 保留、中文 message 透传；负例：任一非 404/409 返回 `kind=result` → FAIL | R1 |
| R2 客户端接线（401/perm_version/idle 不变 + throw 契约保留） | `frontend-pc/src/services/api.js` | `request()` 的 `!response.ok` 分支 | 修后源码结构 | PC | `FAILURE_RESOLVER_CALLS=1`、`INLINE_STATUS_MAP=0`、`THROW_PATH=1`、`NONOK_INLINE_RETURNS=0`、`AUTH_BLOCK_BEFORE_NONOK=1`、`AUTH_CODES=4`、`PERM_VERSION=1`、`IDLE_GUARD_CALLS>=2`；负例：非 404/409 出现 `return {` → FAIL | R2 |
| R3 消费者接线（网点提交 400 冲突转持久提示） | `frontend-pc/src/pages/SiteManagement.jsx`、`frontend-pc/tests/formConflict.test.mjs` | 网点新建/编辑提交 → `handleSubmit` 的 catch | catch 收到 40002/40004/40005 或 40900 | PC | `CONFLICT_BRANCH_IN_CATCH=1`、`CONFLICT_TEXT_FALLBACK=1`、`FALLBACK_TOAST=1`、`ALERT_RENDERED=1`；`node --test` 中 `TRUE_ENRICHED_ERROR=1`、`FALSE_PLAIN_ERROR=1`；负例：catch 内无 code 判定或删掉兜底 toast → FAIL | R3 |
| R4 红绿实证 + 回归面不扩张（全仓 throw 依赖不变） | `scripts/verify-2563.sh`、`frontend-pc/src` | 静态调用点扫描 + `git show <fix>^` 对照 | 修后全树 + 修前 api.js | PC | `RED_NO_CODE=1`（修前非 2xx 块无 code 赋值）、`GREEN_CODE_ATTACHED=1`、`AWAIT_CALL_SITES>=150`、`NONOK_INLINE_RETURNS=0`；负例：修后仍出现「非 404/409 内联返回对象」→ FAIL | R4 |
| R5 API 规格同步客户端非 2xx 契约 | `docs/spec/api/api.md` | §1.3 统一响应格式 | 修后文本 | 文档 | 含 `PC 客户端非 2xx 契约`、`code`/`status`/`data`、`catch`、`40002` 字样；401 分支「不变」声明存在；负例：仍缺「非 404/409 抛 Error 带 code」句 → FAIL | R5 |
| R6 构建 + ESLint | `frontend-pc` | `npm run build`、`npx eslint` | 修后干净树 | PC | `BUILD_EXIT=0`、`ESLINT_NO_UNDEF=0`；负例：任一非 0 或 `no-undef` 计数 > 0 → FAIL | R6 |
| R7 pre-ready gate（四源一致 + 证据完整 + P13 已推送） | `docs/evidence/2563/pre-ready-2563.json` | `pre-ready-check.sh 2563` | 修后树 + 最新 Accepted Plan | PC | `GATE_RESULT=PASS`、`jq -e '.overall == "PASS"'`、evidence 每行 `pass=true`、`dirty=false`；负例：任一 P 项失败或 evidence 缺行 → FAIL | R7 |

## Input Truth & Tool Preflight

- **Source of truth（唯一权威）**：
  - 传输层行为 = `frontend-pc/src/services/api.js`（`request()` 现状，`331-353`）；
  - 「业务可纠正」白名单 = `frontend-pc/src/utils/formConflict.js`（**本单不复制第二份**）；
  - 后端码语义 = `backend/handlers/*.go` 的实际导出（`api.go:874-879/953-958` 分类重名 40002；`site.go:232-290` 网点 40002；`user_staff.go:205-225` 员工 40001）；
  - 文案解析层序 = `frontend-pc/src/services/errorMessages.js`（`resolveErrorCore`）。
- **差异核对（/analyze 只读实测）**：
  - `sed -n '331,353p' frontend-pc/src/services/api.js` → 404/409 返回对象，其余 `throw new Error(...)`（无 code）；
  - `grep -rn "40002" backend/handlers/*.go | grep -v _test` → 40002 同时用于「缺少必填参数」与「重名冲突」两类语义（**故不可把整个 40002 家族改成返回对象**，见 D1）；
  - `grep -rn "isBusinessCorrectableConflict" frontend-pc/src` → 消费者仅 `SiteManagement`、`StaffManagement`（均在 `try` 内判定，HTTP 400 不可达）；
  - `grep -c` 调用点统计：`await api.*/xxxApi.*` 256 处、邻近 `catch` 209 处 → 依赖 throw 的开销面极大；
  - `frontend-pc/tests/` 现有 9 个 `node --test` 用例，`package.json` 无 test 脚本 → 证据形态按纯函数测试 + 静态接线。
- **损坏输入拒绝规则**：`resolveHttpFailure` 收到 `null`/字符串/数组/`code` 非数字/非有限数时，禁止抛解析异常、禁止写入非法 `code`；`error.message` 必须与现状逐字一致（fallback `请求失败，请重试`）；404/409 返回对象字段集必须与现状逐字一致。
- **工具盲区声明**：
  - `scripts/verify-2563.sh` 是**文本级**结构断言（grep 块边界 + 计数），无法证明运行时渲染；UI 侧依赖 D5 的静态豁免 + Ops Follow-up 回评。
  - 静态断言无法区分同名函数的重载/条件分支，故断言只锁定「`request()` 内 `!response.ok` 块不含内联 `return {`」这一结构事实，不推断语义。
  - `git show <fix>^` 红样依赖 fix commit 之前最后一次改 `api.js` 的提交；若 fix 与证据提交分离，脚本用 `git log -1 --format=%H -- frontend-pc/src/services/api.js` 取锚点，禁止手写 sha。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 传输层形态：**保留 throw，把 `code`/`status`/`data` 挂到 Error 上**（Issue 任务 1 的备选路径「或至少把 code/data 挂到抛出的 Error 上」）；**禁止**把任意非 404/409 改成返回对象 | Architect | 本计划锁定：40002 在后端同时表示「缺少必填参数」与「重名冲突」，若整族改成返回对象，256 个调用点中无 `else` 分支者会变成静默成功（违反红线「禁止静默吞错」） | 若 Owner 要求对象形态 → 回 Architect 另立 Issue，须同时改造全部调用点（不在本单） |
| D2 | 「业务可纠正」白名单唯一 owner 仍是 `frontend-pc/src/utils/formConflict.js`；传输层**不**硬编码 40002/40004/40005 列表 | Architect | 本计划锁定（避免两份白名单漂移） | 阻塞实现 |
| D3 | 401 分支（40104/40105/40106/40107）、`perm_version` 检测、idle 超时（`isIdleTimeoutPayload`）、`refreshAccessToken`、`fetchBlob` **零改动** | Architect | 本计划锁定（R2 逐项断言） | 阻塞实现 |
| D4 | 消费者范围：本单只接 `SiteManagement`（40002 可达）；`StaffManagement` 不改（其 400 路径为 40001 即时域）；分类页由 **#2564** 承接（#2485 正在 wip 且该文件在其 Files 内） | Architect | 本计划锁定 + #2564 已立项 | 若 Owner 要求本单改分类页 → 回 Architect 与 #2485 owner 协调后重排 |
| D5 | R3 证据形态：**静态结构断言 + `node --test` 纯函数**（PC 无组件测试运行器）；真实渲染/点击回执由 `[Ops-Followup of #2563]` 承载 —— **Owner 豁免（UI static exemption）**，沿用 #2480 D6 / #2516 D5 | Architect | Owner 豁免已记录 | 若审计要求 render → 另立前端测试基建 Issue（Out of Scope） |
| D6 | 文档口径：`docs/spec/api/api.md` §1.3 增补 PC 客户端契约；`frontend-mobile` 不同步（无消费者） | Architect | 本计划锁定 | 阻塞 R5 |
| D7 | 不改后端状态码/错误码语义（40002 的两义性不在本单重构），不改后端文案 | Architect | 本计划锁定 | 若需要收敛码语义 → 另立 Issue |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | unit | `node --test` 纯函数用例（无渲染诉求） |
| R2 | static | `api.js` 结构断言（唯一 owner 接线 + 401 顺序不变），Owner 豁免见 D5 |
| R3 | static | 页面 catch 结构断言 + `node --test`；真实点击回执见 D5 Ops Follow-up |
| R4 | static+scan | 红绿 `git show` 对照 + 调用点计数，非渲染证据 |
| R5 | static | 文档段落断言（可失败） |

## Out of Scope

- 分类页 40002 重名恢复持久提示（#2485 D3 的 `defer:tier2` 行）→ **#2564**（已创建，`Parent: #2563`）。
- Tier 2 Batch 2 其余 13 个页面 → **#2486**（holdon，待 /analyze；本单只在其 Issue 留契约说明）。
- `frontend-mobile/src/services/api.js` 同族缺口（`throw new Error(message)` 不带 code）：当前无任何移动端消费者做 code 判定（无持久冲突提示 rubric），不构成用户可见缺陷 → 不立项。
- 后端错误码语义重构（40002 同时表示「缺少必填参数」与「重名冲突」；40004 同时表示「已存在」与「导入失败」）→ 另立 Issue，需 Owner 决策；本单不改后端。
- `refreshAccessToken` / `fetchBlob` / `reportPCActivity` 的错误语义（鉴权与下载域，不暴露业务 code）。
- 401/perm_version/idle 超时行为（D3 零改动）。
- 新增全局 toast / 持久提示组件（复用 #2480 `FormConflictAlert` 与 `formConflict.js`）。
- PC 组件测试基建（vitest/@testing-library/jsdom 引入）→ 另立 Issue。
- 上传进度 XHR（`instrumentsApi.displayImageUpload` 等）的错误体解析。

## G0 Golden Acceptance

| 行 ID | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） | assertions（含负例） |
|---|---|---|---|---|
| R1 | `status=400 + {code:40002,message:'分类名称已存在'}`、`400 + {code:40004}`、`403 + {code:40300}`、`429 + {code:42900}`、`500 + {code:50000,message:'db down'}`、`502 + null`、`404 + {message:'not found'}`、`409 + {code:40901,message:'name exists'}`、`409 + {}`、`400 + {code:'40002'}`、`400 + 'oops'` | `frontend-pc/src/services/apiError.js` 经 `node --test` | `docs/evidence/2563/R1-transport-unit.txt` | `THROW_40002=1`、`THROW_40004=1`、`THROW_403=1`、`THROW_429=1`、`THROW_500=1`、`THROW_502=1`、`MSG_40002=CJK_PASSTHROUGH`、`RESULT_404=1`、`RESULT_409=1`、`RESULT_409_DEFAULT_CODE=40900`、`CODE_STRING_COERCED=1`、`BAD_PAYLOAD_SAFE=1`、`# fail 0`；负例：任一非 404/409 产出 `kind=result` → FAIL |
| R2 | 修后 `frontend-pc/src/services/api.js` 的 `request()` 函数体 | `request()` 的 `!response.ok` 块 | `docs/evidence/2563/R2-client-wiring-static.txt` | `FAILURE_RESOLVER_CALLS=1`、`INLINE_STATUS_MAP=0`、`THROW_PATH=1`、`NONOK_INLINE_RETURNS=0`、`AUTH_BLOCK_BEFORE_NONOK=1`、`AUTH_CODES=4`、`PERM_VERSION=1`、`IDLE_GUARD_CALLS>=2`；负例：块内出现 `return {` 或 401 分支被移到其后 → FAIL |
| R3 | 修后 `SiteManagement.jsx` 的 `handleSubmit` catch 段 + `node --test frontend-pc/tests/formConflict.test.mjs` | 网点新建/编辑提交（`/sites`） | `docs/evidence/2563/R3-consumer-wiring.txt` | `CONFLICT_BRANCH_IN_CATCH=1`、`CONFLICT_TEXT_FALLBACK=1`、`FALLBACK_TOAST=1`、`ALERT_RENDERED=1`、`TRUE_ENRICHED_ERROR=1`、`FALSE_PLAIN_ERROR=1`、`# fail 0`；负例：catch 无 `isBusinessCorrectableConflict` 判定、或删除兜底 `message.error` → FAIL |
| R4 | 修前 `api.js`（`git log -1 -- frontend-pc/src/services/api.js` 的父提交）+ 修后全树 | 静态扫描 + `git show` 对照 | `docs/evidence/2563/R4-redgreen-inventory.txt` | `RED_NO_CODE=1`（修前非 2xx 块无 `error.code` 赋值）、`GREEN_CODE_ATTACHED=1`、`AWAIT_CALL_SITES>=150`、`NONOK_INLINE_RETURNS=0`；负例：修后仍存在「非 404/409 内联返回对象」或调用点计数 <150 → FAIL |
| R5 | 修后 `docs/spec/api/api.md` 的 §1.3 段 | 文档正文 | `docs/evidence/2563/R5-docs-contract.txt` | 命中 `PC 客户端非 2xx 契约`、`error.code`、`40002`、`catch`、`401` 不变句；`DOC_CONTRACT_OK=1`；负例：缺「其余非 2xx 抛 Error 且带 code」句 → FAIL |
| R6 | 修后干净树 | `cd frontend-pc` | `docs/evidence/2563/R6-build-lint.txt` | `BUILD_EXIT=0`、`ESLINT_NO_UNDEF=0`；负例：构建非 0 或 `no-undef` > 0 → FAIL |
| R7 | 修后树 + 最新 Accepted Plan + evidence.json | `pre-ready-check.sh 2563` | `docs/evidence/2563/pre-ready-2563.json` | `GATE_RESULT=PASS`、`jq -e '.overall == "PASS"'`、每行 `pass=true`、`dirty=false`、P13 在 `origin/issue-2563`；负例：任一 P 失败 / evidence 缺行 / `dirty=true` → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2563.sh r1 > docs/evidence/2563/R1-transport-unit.txt` | `docs/evidence/2563/R1-transport-unit.txt` | exit 0；`grep -q 'THROW_40002=1'`；`grep -q '# fail 0'` | 400/403/429/500/502 全部 `kind=throw` 且带 code；404/409 为 `kind=result`；node 用例全通过 | 任一非 404/409 产出 `kind=result`（如 400 被当对象返回）→ 脚本非 0 退出 | dev |
| R2 | `bash scripts/verify-2563.sh r2 > docs/evidence/2563/R2-client-wiring-static.txt` | `docs/evidence/2563/R2-client-wiring-static.txt` | exit 0；`grep -q '^R2_OK$'`；`grep -q 'NONOK_INLINE_RETURNS=0'` | `FAILURE_RESOLVER_CALLS=1`、`INLINE_STATUS_MAP=0`、`THROW_PATH=1`、`AUTH_BLOCK_BEFORE_NONOK=1`、`AUTH_CODES=4`、`PERM_VERSION=1`、`IDLE_GUARD_CALLS>=2` | 块内出现 `return {`、或 401 分支被移到 `!response.ok` 之后 → 输出 `R2_FAIL` 并非 0 退出 | dev |
| R3 | `bash scripts/verify-2563.sh r3 > docs/evidence/2563/R3-consumer-wiring.txt` | `docs/evidence/2563/R3-consumer-wiring.txt` | exit 0；`grep -q '^R3_OK$'`；`grep -q 'CONFLICT_BRANCH_IN_CATCH=1'` | `FALLBACK_TOAST=1`、`ALERT_RENDERED=1`、`TRUE_ENRICHED_ERROR=1`、`FALSE_PLAIN_ERROR=1`、`# fail 0` | catch 缺 code 判定、或删除兜底 `message.error`、或普通 Error 被判 true → 输出 `R3_FAIL` 并非 0 退出 | dev |
| R4 | `bash scripts/verify-2563.sh r4 > docs/evidence/2563/R4-redgreen-inventory.txt` | `docs/evidence/2563/R4-redgreen-inventory.txt` | exit 0；`grep -q '^R4_OK$'`；`grep -q 'RED_NO_CODE=1'`；`grep -q 'GREEN_CODE_ATTACHED=1'` | `AWAIT_CALL_SITES` ≥ 150、`NONOK_INLINE_RETURNS=0` | 修后仍存在非 404/409 内联返回对象、或修前样本含 code 赋值（红样失效）→ `R4_FAIL` 并非 0 退出 | dev |
| R5 | `bash scripts/verify-2563.sh r5 > docs/evidence/2563/R5-docs-contract.txt` | `docs/evidence/2563/R5-docs-contract.txt` | exit 0；`grep -q '^R5_OK$'`；`grep -q 'DOC_CONTRACT_OK=1'` | api.md §1.3 含「PC 客户端非 2xx 契约」与 `error.code` 句，且含 401 不变声明 | 缺「其余非 2xx 抛 Error 带 code」句或 401 不变声明 → `R5_FAIL` 并非 0 退出 | dev |
| R6 | `bash scripts/verify-2563.sh r6 > docs/evidence/2563/R6-build-lint.txt` | `docs/evidence/2563/R6-build-lint.txt` | exit 0；`grep -q '^R6_OK$'`；`grep -q 'BUILD_EXIT=0'`；`grep -q 'ESLINT_NO_UNDEF=0'` | 构建 0 退出、`no-undef` 计数 0 | 构建非 0 或 `no-undef` 计数 > 0 → `R6_FAIL` 并非 0 退出 | dev |
| R7 | `/home/coder/.config/opencode/scripts/pre-ready-check.sh 2563 --summary-file docs/evidence/2563/summary.md --changed-files docs/evidence/2563/changed-files.txt --evidence-file docs/evidence/2563/evidence.json --full-gate-cmd "cd frontend-pc && npm run build" --report docs/evidence/2563/pre-ready-2563.json` | `docs/evidence/2563/pre-ready-2563.json` | exit 0；`GATE_RESULT=PASS`；`jq -e '.overall == "PASS"'` | 每条合同行 `pass=true`、`dirty=false`、P13 提交在 `origin/issue-2563` | 任一 P 项失败 / evidence.json 缺行 / `dirty=true` → gate 非 0，禁止 ready | dev |

### Files（变更文件清单）

| 类别 | 路径 |
|---|---|
| 传输映射（新增） | `frontend-pc/src/services/apiError.js` |
| API 客户端 | `frontend-pc/src/services/api.js` |
| 页面消费者 | `frontend-pc/src/pages/SiteManagement.jsx` |
| 单测（新增） | `frontend-pc/tests/apiError.test.mjs` |
| 单测（增补） | `frontend-pc/tests/formConflict.test.mjs` |
| 证据生成器（新增） | `scripts/verify-2563.sh` |
| 核心文档 | `docs/spec/api/api.md` |
| 证据（计划快照） | `docs/evidence/2563/plan.md` |
| 证据（摘要） | `docs/evidence/2563/summary.md` |
| 证据（改动清单） | `docs/evidence/2563/changed-files.txt` |
| 证据（合同汇总） | `docs/evidence/2563/evidence.json` |
| 证据 R1 | `docs/evidence/2563/R1-transport-unit.txt` |
| 证据 R2 | `docs/evidence/2563/R2-client-wiring-static.txt` |
| 证据 R3 | `docs/evidence/2563/R3-consumer-wiring.txt` |
| 证据 R4 | `docs/evidence/2563/R4-redgreen-inventory.txt` |
| 证据 R5 | `docs/evidence/2563/R5-docs-contract.txt` |
| 证据 R6 | `docs/evidence/2563/R6-build-lint.txt` |
| Gate 报告 | `docs/evidence/2563/pre-ready-2563.json` |

## Artifact Provenance

- 生成器：`scripts/verify-2563.sh`（r1–r6）；每个文本 artifact 首行必须为
  `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`，锚定本单 fix commit（`FIX_REF` 传入；证据收尾提交不改变锚点）。
- 逐条清单（全部位于 `docs/evidence/2563/`，均需带上述首行）：
  - `docs/evidence/2563/R1-transport-unit.txt`（`node --test frontend-pc/tests/apiError.test.mjs` 输出）
  - `docs/evidence/2563/R2-client-wiring-static.txt`（`api.js` 结构断言输出）
  - `docs/evidence/2563/R3-consumer-wiring.txt`（页面 catch 结构断言 + `formConflict` 用例输出）
  - `docs/evidence/2563/R4-redgreen-inventory.txt`（红绿 `git show` 对照 + 调用点计数）
  - `docs/evidence/2563/R5-docs-contract.txt`（`api.md` §1.3 段落断言输出）
  - `docs/evidence/2563/R6-build-lint.txt`（`npm run build` + `npx eslint` 输出）
  - `docs/evidence/2563/pre-ready-2563.json`（gate 报告，JSON 自带 `.provenance.{build,commit,dirty}`）
- `docs/evidence/2563/evidence.json` 字段口径：`build` 形如 `1.0.<7-40 位短码>`，`commit` 为 40 位 sha，`dirty:false`。
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$
- 证据重生成纪律：fix commit 之后统一重跑 `scripts/verify-2563.sh r1`…`r6`，禁止只改 `build/commit` 冒充新证据。

## Dependencies

无

### 依赖说明（非声明）

- 本单不依赖任何未完成 Issue：#2480 的共享件（`formConflict.js` / `FormConflictAlert.jsx`）与 #2516 的终判惯例均已在 develop。
- 下游（不阻塞本单）：#2564 消费本单契约（分类页恢复持久提示）；#2486 Batch 2 在其 /analyze 阶段按 `catch` 形态判定 40002/40004/40005。

*Model: DeepSeek V4.1 Flash 1017*


