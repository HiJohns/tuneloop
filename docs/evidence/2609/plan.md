## Accepted Plan — #2609 [Gap] PC 三页网络错误透传浏览器 ASCII + `CATCH_NETWORK_FALLBACK` 探针可证伪

Tags: bug, code:frontend, code:test

### 背景与现状核对（代码为据）

| # | 事实 | 依据 |
|---|---|---|
| 1 | 待修 8 处「catch 直出异常 message」：`BannerManagePage.jsx:135`、`:216`；`MallImageManageCard.jsx:135`、`:216`；`InstrumentStock.jsx:277`、`:296`、`:384`、`:412` | `git grep -nE "message\.(error&#124;warning&#124;info)\(" origin/issue-2488 -- frontend-pc/src` 过滤 `(e&#124;err&#124;error)\.message` → 8 行 |
| 2 | **基座不是 develop**：#2488 的代码（含这 8 处与 `isBusinessCorrectableConflict` / `FormConflictAlert` 接线）**只在 `origin/issue-2488`**，develop 上仍是改造前的 `message.error('操作失败')` | `git merge-base --is-ancestor origin/issue-2488 origin/develop` → 非零（NO）；`git show develop:frontend-pc/src/pages/System/BannerManagePage.jsx` → `message.error('操作失败')` |
| 3 | `request()` 的 `await fetch(...)` **无 try/catch**，断网时 rejection 直接冒泡到调用点 | `frontend-pc/src/services/api.js:236` |
| 4 | 断网 rejection 的 `message` 是浏览器 ASCII 技术串（`Failed to fetch` / `NetworkError when attempting to fetch resource.`），不带 `code`、不经 `resolveHttpFailure` | 浏览器 `fetch` 规范；`apiError.js` 只在 `!response.ok` 后被调用 |
| 5 | `ApiError` 判定字段已具备：非 2xx 抛出的 `Error` 被设置 `name='ApiError'`，且 `message` 已由 `resolveErrorMessage` 中文化；`404`/`409` 返回对象不抛 | `frontend-pc/src/services/apiError.js:52-62`、`frontend-pc/src/services/errorMessages.js` L1–L5 |
| 6 | 现有探针 `catch_metrics` **不可证伪**：`msg` 与 `fb` 两条 grep 都命中同一个 `e?.message &#124;&#124; '<中文>'` 表达式，缺陷版本也恒为 1 | `scripts/verify-2488.sh:105-112`（在 `origin/issue-2488`） |
| 7 | `BannerManagePage.jsx:133-134` 注释声明「网络/未知错误（非 ApiError）仍用友好兜底」，实现却是无条件 `e?.message &#124;&#124;` —— 注释与实现相反 | `git show origin/issue-2488:frontend-pc/src/pages/System/BannerManagePage.jsx` |
| 8 | `fetchBlob` 非 2xx 抛中文 `Error('导出失败')`（`name='Error'`），网络失败仍是 `TypeError` ASCII | `frontend-pc/src/services/api.js:241` |
| 9 | 三页当前**无任何**运行时错误文案测试（`frontend-pc/tests/` 无 userFacing* 用例） | `ls frontend-pc/tests` |
| 10 | 域外同类 **117 行 / 37 文件**（`Detail.jsx` 11、`instrument/List.jsx` 9、`UserManagement.jsx` 8 …）→ 已另立 **#2612**（`status:todo`，依赖本单） | `git grep` 同口径（排除本单三页）逐文件统计 |
| 11 | `verify-2488.sh` 的 r2/r3 只对 banner/mall 调 `catch_metrics`，**stock 被显式跳过**（`case "$STOCK_PAGE") ;;`） | `scripts/verify-2488.sh:145`（`"$STOCK_PAGE") ;;` 直接跳过） |

**结论**：缺陷是**两层**的——(a) 6~8 处调用点把「异常对象」的 `message` 无条件当用户文案，断网时散出英文技术串，与新注释/`api.md` §1.3「ASCII 机器串不外溢」相反；(b) 验收探针只 grep 表达式字面量，**恒为真**，把 (a) 的回归放行了。修复 = 一个唯一出口 `userFacingErrorMessage`（只透传 `ApiError`）+ 8 处接线 + 把探针改成「结构判定 + 缺陷注入红绿」+ 运行时单测。

### 变更点

#### 1. 唯一出口：`frontend-pc/src/services/apiError.js` 新增 `userFacingErrorMessage(error, fallback)`

```js
// #2609：把 catch 到的异常转成「用户可见文案」。
// 只在传输层 ApiError（#2563 抛出，message 已经 resolveErrorMessage 中文化）时透传；
// fetch 断网/跨域失败抛的是浏览器 TypeError（'Failed to fetch' /
// 'NetworkError when attempting to fetch resource.'，ASCII 技术串），
// 以及任何非 Error 抛出值一律回落调用点中文兜底（api.md §1.3：ASCII 机器串不外溢）。
export function userFacingErrorMessage(error, fallback) {
  if (error && error.name === 'ApiError' && typeof error.message === 'string' && error.message !== '') {
    return error.message
  }
  return fallback
}
```

- **不改** `resolveHttpFailure` 的任何分支（`ApiError` 类 owner 契约不变，#2563/#2488 D9b 保持）。
- 页面不 import `ApiError` 类（D9b 禁令不变），只 import 该纯函数。

#### 2. 8 处接线（逐字机械替换，保留原中文兜底文案）

| # | 位置（`origin/issue-2488`） | before | after |
|---|---|---|---|
| 1 | `BannerManagePage.jsx:135` | `message.error(e?.message &#124;&#124; '操作失败')` | `message.error(userFacingErrorMessage(e, '操作失败'))` |
| 2 | `BannerManagePage.jsx:216` | `message.error(e.message &#124;&#124; '上传失败')` | `message.error(userFacingErrorMessage(e, '上传失败'))` |
| 3 | `MallImageManageCard.jsx:135` | `message.error(e?.message &#124;&#124; '操作失败')` | `message.error(userFacingErrorMessage(e, '操作失败'))` |
| 4 | `MallImageManageCard.jsx:216` | `message.error(e.message &#124;&#124; '上传失败')` | `message.error(userFacingErrorMessage(e, '上传失败'))` |
| 5 | `InstrumentStock.jsx:277` | `message.error(e?.message &#124;&#124; '批量修改价格失败')` | `message.error(userFacingErrorMessage(e, '批量修改价格失败'))` |
| 6 | `InstrumentStock.jsx:296` | `message.error(e.message &#124;&#124; '导出失败')` | `message.error(userFacingErrorMessage(e, '导出失败'))` |
| 7 | `InstrumentStock.jsx:384` | `message.error(e.message &#124;&#124; '登记失败')` | `message.error(userFacingErrorMessage(e, '登记失败'))` |
| 8 | `InstrumentStock.jsx:412` | `message.error(e.message &#124;&#124; '恢复失败')` | `message.error(userFacingErrorMessage(e, '恢复失败'))` |

- `isBusinessCorrectableConflict(e)` 前置分支、`errorFields` 早退、`resp.message` 分支**全部原样保留**；只换「非业务分支」的文案表达式。
- 同步修正 `BannerManagePage.jsx` / `MallImageManageCard.jsx` catch 上方注释（第 7 条事实：注释与实现相反）→「仅 `ApiError`（后端中文）透传，网络/未知错误落中文兜底」。
- `InstrumentStock.jsx:236-237` 等既有「网络错误」常量分支不改（已是中文）。

#### 3. 可证伪探针（单一 owner）：`scripts/scan-catch-guard-2609.sh`（新增）

判定口径（唯一实现，`verify-2488.sh` 与 `verify-2609.sh` 均只消费它）：

- **站点**（两类，互斥口径，`CATCH_SITES` = 两者之和）：某行匹配 `message\.(error|warning|info)\(` 且满足其一——
  (i) **裸透传**：含异常接收者的 `.message`（接收者限 `e|err|error|ex|exc`，可带 `?.`）→ 计入 `CATCH_RAW`；
  (ii) **守卫透传**：含 `userFacingErrorMessage(` → 计入 `CATCH_GUARDED`。
  （修前 = 8 raw / 0 guarded；修后 = 0 raw / 8 guarded，站点总数恒为 8，不会因改造而「站点消失」而假绿。）
- **兜底可用**：`CATCH_GUARDED` 中第二实参是含非 ASCII 字符的单引号字面量的行数 → `CATCH_FALLBACK_OK`。
- `CATCH_MESSAGE=1` ⇔ `CATCH_RAW=0` 且 `CATCH_SITES>0` 且 `CATCH_GUARDED=CATCH_SITES`（ApiError 可透传）。
- `CATCH_NETWORK_FALLBACK=1` ⇔ 上一条件成立 **且** helper 源码含 `name === 'ApiError'` 门（`HELPER_GATE=1`）**且** `CATCH_FALLBACK_OK=CATCH_GUARDED`。
- 参数：`--files "<path>..."` 只扫指定文件；`--src DIR` 覆盖根目录（缺陷注入用 tmp 副本，仅复制三页）；`--helper FILE` 显式指定 helper（注入时仍指向真实文件）。

`scripts/verify-2488.sh` 同步改造（item 3 + item 4）：

- `catch_metrics()` 改为委托 `bash "$REPO/scripts/scan-catch-guard-2609.sh" --files "$page"`（输出同名字段，`require_tokens` 不用改）。
- `page_row()` 去掉 `case "$STOCK_PAGE") ;;` 特例 → stock 也走 `catch_metrics`；r4 追加 `CATCH_MESSAGE=1`、`CATCH_NETWORK_FALLBACK=1` 两个 token。
- **禁止**放宽任何既有断言（只允许变严）。
- `verify-2609.sh r6` 语义（既有 harness 复跑）：以 `OUT_DIR=tmp/verify-2609-2488` + `FIX_REF=<本单 fix commit>` 调用 `verify-2488.sh` 的 `r2`/`r3`/`r4`，记录三者退出码；并额外用**改造前**的两条字面量 grep 对修后 banner/mall 求值——两者都命中不到（旧探针在修后代码上只能给 0），输出 `OLD_METRIC_WOULD_FAIL=1` 说明「旧探针与字面量耦合、必须改为结构判定」；再断言 `git status --porcelain docs/evidence/2488` 为空（`EVIDENCE_2488_UNTOUCHED=1`）。

#### 4. 运行时单测：`frontend-pc/tests/userFacingErrorMessage.test.mjs`（新增，`node --test`）

fixture（8 类输入）→ 期望值：

| 输入 | 期望 |
|---|---|
| `TypeError('Failed to fetch')` | `'操作失败'`（不含 ASCII 技术串） |
| `TypeError('NetworkError when attempting to fetch resource.')` | `'操作失败'` |
| `TypeError('Load failed')` | `'操作失败'` |
| `{ name:'ApiError', message:'分类名称已存在' }` | `'分类名称已存在'`（透传） |
| `{ name:'ApiError', message:'' }` | 兜底 |
| `new Error('导出失败')`（`name='Error'`） | 兜底（兜底文案逐字保留） |
| `throw 'boom'`（非对象） / `throw null` | 兜底 |
| 兜底实参 `'登记失败，请调整后重试'` | 原样返回 |

并打印 `NAIVE_LEAK=1`：用朴素实现 `(e, fb) => e?.message &#124;&#124; fb` 跑同一 fixture，证明**该 fixture 能区分两种实现**（缺陷实现必红），而不是「恒真断言」。

#### 5. 文档：`docs/spec/api/api.md` §1.3 追加「传输层网络错误」口径（见 Core Update）

### 既有断言影响（0 翻转 / 1 处变严）

| 检查 | 结果 | 依据 |
|---|---|---|
| 既有测试断言这 8 处文案 | **0 处** | `grep -rn "Failed to fetch&#124;操作失败&#124;上传失败" frontend-pc/tests` → 无相关用例 |
| `apiError.test.mjs` 断言 `resolveHttpFailure` 分支 | **0 改动** | 本单不碰 `resolveHttpFailure`；`git diff` 只新增一个导出函数 |
| `verify-2488.sh` 的 `CATCH_*` 断言 | **变严**（非放宽） | item 3 明示委派；`verify-2488.sh` 当前只 grep 字面量，缺陷版本恒真 → 改为结构判定 + 注入红绿 |
| 需要翻转的语义断言 | **无** | 只改「非业务分支」的文案来源；`code` / HTTP / 分支顺序 / 冲突持久化不变 |
| `#2612` 域外 117 处 | **必须不变** | R4 断言 `OUT_OF_SCOPE_SITES=117`（防顺手扩面） |

### Files（变更文件清单）

- `frontend-pc/src/services/apiError.js`
- `frontend-pc/src/pages/System/BannerManagePage.jsx`
- `frontend-pc/src/pages/System/MallImageManageCard.jsx`
- `frontend-pc/src/pages/InstrumentStock.jsx`
- `frontend-pc/tests/userFacingErrorMessage.test.mjs`
- `scripts/scan-catch-guard-2609.sh`
- `scripts/verify-2609.sh`
- `scripts/verify-2488.sh`
- `docs/spec/api/api.md`
- `docs/evidence/2609/plan.md`
- `docs/evidence/2609/R1-catch-guard.txt`
- `docs/evidence/2609/R2-helper-runtime.txt`
- `docs/evidence/2609/R3-build-lint.txt`
- `docs/evidence/2609/R4-residual-scan.txt`
- `docs/evidence/2609/R5-api-doc.txt`
- `docs/evidence/2609/R6-2488-harness.txt`
- `docs/evidence/2609/evidence.json`
- `docs/evidence/2609/pre-ready-2609.json`

### Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §1.3 已写五层回退与「ASCII 机器串不外溢」，但只覆盖**后端 message**；未写「传输层网络错误」这一子类 | §1.3 追加一段 **传输层网络错误（#2609）**：`request()` 的 `fetch` reject（断网/跨域）产生的是浏览器 `TypeError`（`Failed to fetch` / `NetworkError when attempting to fetch resource.`），**不经** `resolveHttpFailure`、无 `code`、`name !== 'ApiError'`；PC 端用户可见文案必须经唯一出口 `frontend-pc/src/services/apiError.js` 的 `userFacingErrorMessage(err, fallback)`——仅 `name === 'ApiError'` 透传（已中文化），其余回落调用点中文兜底；**禁止**页面内联 `err?.message &#124;&#124; '中文'` | 该写法在断网时把英文技术串直接弹给用户，与本节「ASCII 机器串不外溢」相反；不写清唯一出口会导致新页面继续复制坏模式（域外仍有 117 处，见 #2612） |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 8 处接线（唯一出口）+ 探针可证伪 | `frontend-pc/src/pages/System/BannerManagePage.jsx`、`frontend-pc/src/pages/System/MallImageManageCard.jsx`、`frontend-pc/src/pages/InstrumentStock.jsx`、`scripts/scan-catch-guard-2609.sh` | `bash scripts/scan-catch-guard-2609.sh`（源码结构断言 + tmp 副本缺陷注入） | 修后工作树；注入 A（还原 `e?.message &#124;&#124; '<中文>'`）、注入 B（保留守卫但兜底改为 `e.message`） | PC（静态结构） | `CATCH_SITES=8`、`CATCH_GUARDED=8`、`CATCH_RAW=0`、`CATCH_MESSAGE=1`、`CATCH_NETWORK_FALLBACK=1`；注入 A 两 token 归 0、注入 B 仅 `CATCH_NETWORK_FALLBACK` 归 0 → `CATCH_INJECT_RED=1` | R1 |
| R2 运行时语义（网络错误落兜底 / ApiError 透传） | `frontend-pc/tests/userFacingErrorMessage.test.mjs`、`frontend-pc/src/services/apiError.js` | `cd frontend-pc && node --test tests/userFacingErrorMessage.test.mjs` | 8 类 fixture（3 种浏览器网络串、ApiError 有/无 message、普通 Error、非对象抛出值、兜底实参原样） | Node 运行时（纯函数） | `HELPER_TEST_EXIT=0`、`NETWORK_FALLBACK=1`、`APIERROR_PASSTHROUGH=1`、`APIERROR_EMPTY_FALLBACK=1`、`PLAIN_ERROR_FALLBACK=1`、`NON_OBJECT_FALLBACK=1`、`FALLBACK_VERBATIM=1`、`ASCII_LEAK=0`、`NAIVE_LEAK=1`、`RUNTIME_INJECT_RED=1` | R2 |
| R3 PC 构建 + ESLint | `frontend-pc/src/services/apiError.js` + 三页 | `cd frontend-pc && npm run build`；`npx eslint` 改动文件 | 修后工作树 | PC 构建产物 | `PC_BUILD=0`、`LINT_EXIT=0`、`HELPER_TEST_EXIT=0`、`BUILD_LINT_OK` | R3 |
| R4 同类残留对称扫描（防扩面） | `scripts/scan-catch-guard-2609.sh` | `bash scripts/verify-2609.sh r4` | 修后 `frontend-pc/src` 全目录 | PC（静态） | `RESIDUAL_INSCOPE_RAW=0`、`INSCOPE_SITES=8`、`OUT_OF_SCOPE_SITES=117`、`FOLLOWUP_ISSUE=2612` | R4 |
| R5 API 口径文档同步 | `docs/spec/api/api.md` | 文档结构断言 | 修后文档 | 文档 | `API_NETWORK_RULE=1`、`API_FORBID_INLINE=1`、`API_HELPER_REF=1` | R5 |
| R6 既有 harness 强化后可复跑（#2488 r2/r3/r4） | `scripts/verify-2488.sh` | `bash scripts/verify-2609.sh r6`（内部以 `OUT_DIR` 覆盖调用 `verify-2488.sh` 的 r2/r3/r4） | 修后工作树（不覆写 `docs/evidence/2488/`） | PC（静态接线） | `R2_EXIT=0`、`R3_EXIT=0`、`R4_EXIT=0`、`STOCK_CATCH_COVERED=1`、`OLD_METRIC_WOULD_FAIL=1` | R6 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 修后三页源码 + `tmp/verify-2609-inject/{raw,nofb}/src` 两个变异副本（A：`e?.message &#124;&#124; '<中文>'`；B：`userFacingErrorMessage(e, e.message)`） | `bash scripts/verify-2609.sh r1` | `docs/evidence/2609/R1-catch-guard.txt` | `SCAN_EXIT=0`、`CATCH_SITES=8`、`CATCH_GUARDED=8`、`CATCH_RAW=0`、`CATCH_MESSAGE=1`、`CATCH_NETWORK_FALLBACK=1`、`HELPER_GATE=1`、`INJ_RAW_GUARDED=0`、`INJ_RAW_MESSAGE=0`、`INJ_RAW_FALLBACK=0`、`INJ_NOFB_MESSAGE=1`、`INJ_NOFB_FALLBACK=0`、`CATCH_INJECT_RED=1` | 任一站点残留 `e?.message &#124;&#124;`（`CATCH_RAW>0`）、站点数 ≠ 8、helper 无 `name === 'ApiError'` 门、兜底实参含 ASCII、注入后 token 仍为 1 → 任一即 FAIL |
| R2 | 8 类输入（见变更点 4 表）+ 朴素实现对照 | `cd frontend-pc && node --test tests/userFacingErrorMessage.test.mjs` | `docs/evidence/2609/R2-helper-runtime.txt` | `HELPER_TEST_EXIT=0`、`NETWORK_FALLBACK=1`、`APIERROR_PASSTHROUGH=1`、`APIERROR_EMPTY_FALLBACK=1`、`PLAIN_ERROR_FALLBACK=1`、`NON_OBJECT_FALLBACK=1`、`FALLBACK_VERBATIM=1`、`ASCII_LEAK=0`、`NAIVE_LEAK=1`、`RUNTIME_INJECT_RED=1` | 网络 TypeError 原样返回（`Failed to fetch`/`NetworkError`/`Load failed` 任一泄漏即 `ASCII_LEAK` 非 0）、ApiError 被兜底吞掉、朴素实现与真实实现不可区分（`NAIVE_LEAK` 不为 1）→ 任一即 FAIL |
| R3 | 修后工作树（`frontend-pc` 依赖已装） | `bash scripts/verify-2609.sh r3` | `docs/evidence/2609/R3-build-lint.txt` | `PC_BUILD=0`、`LINT_EXIT=0`、`HELPER_TEST_EXIT=0`、`BUILD_LINT_OK` | 构建失败 / ESLint（改动文件）非 0 / 单测失败 → FAIL |
| R4 | 修后 `frontend-pc/src`（37 个域外文件 + 本单 3 页） | `bash scripts/verify-2609.sh r4` | `docs/evidence/2609/R4-residual-scan.txt` | `RESIDUAL_INSCOPE_RAW=0`、`INSCOPE_SITES=8`、`OUT_OF_SCOPE_SITES=117`、`OUT_OF_SCOPE_FILES=37`、`FOLLOWUP_ISSUE=2612` | 本单三页仍有 ≤1 处裸 `.message`、域外计数被顺手改动（≠117/≠37）、followup 号缺失 → 任一即 FAIL |
| R5 | 修后 `docs/spec/api/api.md` §1.3 | `bash scripts/verify-2609.sh r5` | `docs/evidence/2609/R5-api-doc.txt` | `API_NETWORK_RULE=1`、`API_FORBID_INLINE=1`、`API_HELPER_REF=1`、`API_ASCII_CLAUSE=1` | §1.3 未写传输层网络错误、未禁内联 `e?.message &#124;&#124;`、未引用 `userFacingErrorMessage` → 任一即 FAIL |
| R6 | 修后工作树 + `OUT_DIR=tmp/verify-2609-2488` 覆盖 | `bash scripts/verify-2609.sh r6` | `docs/evidence/2609/R6-2488-harness.txt` | `R2_EXIT=0`、`R3_EXIT=0`、`R4_EXIT=0`、`STOCK_CATCH_COVERED=1`、`OLD_METRIC_WOULD_FAIL=1`、`EVIDENCE_2488_UNTOUCHED=1` | 强化后的 r2/r3/r4 任一非 0、stock 仍被 `case` 跳过、`docs/evidence/2488/` 被覆写、旧字面量探针在修后代码上仍恒真 → 任一即 FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2609.sh r1 > docs/evidence/2609/R1-catch-guard.txt` | `docs/evidence/2609/R1-catch-guard.txt` | exit 0 且含 `SCAN_EXIT=0` `CATCH_SITES=8` `CATCH_GUARDED=8` `CATCH_RAW=0` `CATCH_MESSAGE=1` `CATCH_NETWORK_FALLBACK=1` `HELPER_GATE=1` `INJ_RAW_GUARDED=0` `INJ_RAW_MESSAGE=0` `INJ_RAW_FALLBACK=0` `INJ_NOFB_MESSAGE=1` `INJ_NOFB_FALLBACK=0` `CATCH_INJECT_RED=1` | PASS（13 项 token 全为期望值） | 残留 `e?.message &#124;&#124;`、站点数≠8、helper 无 ApiError 门、兜底含 ASCII、注入后仍为 1 → FAIL | dev |
| R2 | `bash scripts/verify-2609.sh r2 > docs/evidence/2609/R2-helper-runtime.txt` | `docs/evidence/2609/R2-helper-runtime.txt` | exit 0 且含 `HELPER_TEST_EXIT=0` `NETWORK_FALLBACK=1` `APIERROR_PASSTHROUGH=1` `APIERROR_EMPTY_FALLBACK=1` `PLAIN_ERROR_FALLBACK=1` `NON_OBJECT_FALLBACK=1` `FALLBACK_VERBATIM=1` `ASCII_LEAK=0` `NAIVE_LEAK=1` `RUNTIME_INJECT_RED=1` | PASS（10 项 token 全为期望值） | 浏览器网络串外溢（`ASCII_LEAK` 非 0）、ApiError 被吞、`NAIVE_LEAK` 不为 1（fixture 不能区分实现）→ FAIL | dev |
| R3 | `bash scripts/verify-2609.sh r3 > docs/evidence/2609/R3-build-lint.txt` | `docs/evidence/2609/R3-build-lint.txt` | exit 0 且含 `PC_BUILD=0` `LINT_EXIT=0` `HELPER_TEST_EXIT=0` `BUILD_LINT_OK` | PASS（4 项 token 全为期望值） | 构建失败 / ESLint 非 0 / 单测失败 → FAIL | dev |
| R4 | `bash scripts/verify-2609.sh r4 > docs/evidence/2609/R4-residual-scan.txt` | `docs/evidence/2609/R4-residual-scan.txt` | exit 0 且含 `RESIDUAL_INSCOPE_RAW=0` `INSCOPE_SITES=8` `OUT_OF_SCOPE_SITES=117` `OUT_OF_SCOPE_FILES=37` `FOLLOWUP_ISSUE=2612` | PASS（5 项 token 全为期望值） | 本单三页仍有裸 `.message`、域外计数被改（≠117 或 ≠37）、缺 followup 号 → FAIL | dev |
| R5 | `bash scripts/verify-2609.sh r5 > docs/evidence/2609/R5-api-doc.txt` | `docs/evidence/2609/R5-api-doc.txt` | exit 0 且含 `API_NETWORK_RULE=1` `API_FORBID_INLINE=1` `API_HELPER_REF=1` `API_ASCII_CLAUSE=1` | PASS（4 项 token 全为期望值） | §1.3 缺网络错误口径 / 未禁内联 / 未引用唯一出口 → FAIL | dev |
| R6 | `bash scripts/verify-2609.sh r6 > docs/evidence/2609/R6-2488-harness.txt` | `docs/evidence/2609/R6-2488-harness.txt` | exit 0 且含 `R2_EXIT=0` `R3_EXIT=0` `R4_EXIT=0` `STOCK_CATCH_COVERED=1` `OLD_METRIC_WOULD_FAIL=1` `EVIDENCE_2488_UNTOUCHED=1` | PASS（6 项 token 全为期望值） | 强化后 r2/r3/r4 任一非 0、stock 仍被跳过、`docs/evidence/2488/` 被改写 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2609.sh`（`r1`–`r6`；沿用 `scripts/verify-2488.sh` 结构：provenance 首行、`heavy-run.sh` 取锁、`FIX_REF` 可覆盖锚点）。
- 每个文本 artifact 首行：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `# tree=<HEAD sha>`；证据必须在 fix commit **之后**生成（默认锚点 `git log --grep="^fix(issue-2609)"`）。
- R6 以 `OUT_DIR` 覆盖调用 `scripts/verify-2488.sh` 的 r2/r3/r4（产物落 `tmp/verify-2609-2488/`），**不覆写** `docs/evidence/2488/`；`FIX_REF` 传本单 fix commit，保证 provenance 指向本单。
- 重命令（`npm run build`）一律经 `~/.config/opencode/scripts/heavy-run.sh` 取锁。
- 逐条清单（全部位于 `docs/evidence/2609/`）：
  - `docs/evidence/2609/R1-catch-guard.txt`
  - `docs/evidence/2609/R2-helper-runtime.txt`
  - `docs/evidence/2609/R3-build-lint.txt`
  - `docs/evidence/2609/R4-residual-scan.txt`
  - `docs/evidence/2609/R5-api-doc.txt`
  - `docs/evidence/2609/R6-2488-harness.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2609 --summary-file tmp/pre-ready-2609-summary.md --changed-files tmp/pre-ready-2609-changed.txt --evidence-file docs/evidence/2609/evidence.json --report docs/evidence/2609/pre-ready-2609.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - 站点口径 = `scripts/scan-catch-guard-2609.sh`（唯一实现；`verify-2488.sh` 只消费）；
  - 用户可见文案出口 = `frontend-pc/src/services/apiError.js` 的 `userFacingErrorMessage`（本单新增，唯一 owner）；
  - 传输层契约 = `docs/spec/api/api.md` §1.3 + `frontend-pc/src/services/apiError.js`（#2563）；
  - 文案解析 = `frontend-pc/src/services/errorMessages.js`（`resolveErrorMessage` 五层，L1–L5 保证 ApiError.message 无 ASCII 技术串）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `git merge-base --is-ancestor origin/issue-2488 origin/develop` → 非零（基座必须切自 `origin/issue-2488`）；
  - `bash scripts/scan-catch-guard-2609.sh --files <三页>` → 修前 8 个站点**全部无守卫**（残留 8，`CATCH_MESSAGE` 与 `CATCH_NETWORK_FALLBACK` 均不为 1）；修后残留 0；
  - `bash scripts/verify-2488.sh r2` 在**旧**探针 + 修后代码上 → 非 0（缺陷版本恒真、修后字面量消失），证明旧探针与实现耦合而非语义；
  - `OUT_OF_SCOPE_SITES=117` / `OUT_OF_SCOPE_FILES=37`（口径见 #2612 清单）。
- **损坏输入拒绝规则**（R2 fixture 必须覆盖）：
  - `TypeError` 且 message 为 ASCII（`Failed to fetch` / `NetworkError …` / `Load failed`）→ 必须返回兜底；
  - `name === 'ApiError'` 但 message 为空/非字符串 → 兜底；
  - 非对象抛出值（字符串 / `null` / `undefined`）→ 兜底，**不得**抛二次异常；
  - 兜底实参必须逐字返回（不得拼接、不得加前缀）。
- **工具盲区（必须由 Work Summary 记录）**：
  - 静态扫描是**源码结构**判定，不能证明浏览器真实断网行为；真实断网/真机截图归部署侧 `[Ops-Followup of #2609]`；
  - ESLint 配置 `ignores` 只覆盖 `*.test.js(x)`，`tests/*.test.mjs` 不在 lint 范围 → R3 不把单测文件纳入 `npx eslint`，其正确性由 R2 的 `node --test` 负责；
  - `node --test` 直接 import `src/services/apiError.js`（已有 `apiError.test.mjs` 先例），不依赖浏览器环境；
  - 域外 117 处不在本单；若扫描口径变化必须先更新 #2612，不得就地放宽阈值。
- **契约影响**：`docs/spec/api/api.md` §1.3 新增「传输层网络错误」口径 → Work Summary 必须含 `### 契约三向核对`（pre-ready P7）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | **基座 = `origin/issue-2488`**（不是 develop）：待修 8 处只存在于 #2488 分支（事实 2）。`/work` 必须 `worktree-create.sh --project tuneloop --issue 2609 --base origin/issue-2488`；集成顺序 = 先 `issue-2488` → develop，再 `issue-2609`（堆叠分支），禁止把 #2488 的改动重做到 develop | 本计划锁定 |
| D2 | 用户可见文案唯一出口 = `frontend-pc/src/services/apiError.js` 的 `userFacingErrorMessage(err, fallback)`；页面禁止内联 `err?.message &#124;&#124; '<中文>'`，禁止 import `ApiError` 类（#2488 D9b 不变） | 本计划锁定 |
| D3 | 8 处接线只替换「非业务分支」文案表达式；`isBusinessCorrectableConflict` 前置、`errorFields` 早退、`resp.message` 分支、`code`/HTTP/顺序全部不变 | 本计划锁定 |
| D4 | 不新增第三方依赖（无 `@testing-library`）；运行时证据用 `node --test`（纯函数），UI 真实渲染归 Ops 回评 | 本计划锁定 |
| D5 | `scripts/verify-2488.sh` 的 `catch_metrics` 改造属 **#2609 正文第 3 条明示委派**（不是为通过验收放宽）：只能变严（字面量 grep → 结构判定 + 注入红绿）；同时把 stock 纳入 `catch_metrics`（正文第 4 条） | 本计划锁定 |
| D6 | **不覆写** `docs/evidence/2488/` 任何产物（#2488 已 done，其证据绑 fd2d0fdd8 provenance）；强化后 harness 的复跑产物落 `docs/evidence/2609/R6-2488-harness.txt` | 本计划锁定 |
| D7 | UI 行证据 = 源码结构断言 + 运行时纯函数单测 + 缺陷注入红绿；真实断网/真机表现由 Ops 截图回评（**Owner 豁免**，`evidence_type: static`，沿用本仓前端豁免惯例） | Architect |
| D8 | 域外 117 行 / 37 文件 → **#2612**（已建，`status:todo`，依赖本单）；本单只断言其计数不变（117/37） | #2612 |
| D9 | 重命令（`npm run build`）经 `heavy-run.sh`；`node --test`/`npx eslint` 为轻命令直跑 | 本计划锁定 |
| D10 | 全量回归由 pre-ready P5 baseline 差分承担，本单合同不写全量 `go test`/`npm test` 作为 exit 0 断言 | 本计划锁定 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | static | 源码结构判定（站点/守卫/兜底）+ tmp 副本缺陷注入红绿；Owner 豁免见 D7 |
| R2 | runtime | `node --test` 纯函数语义（网络串 / ApiError / 非对象）+ 朴素实现对照 |
| R3 | build | `npm run build` + `npx eslint`（改动文件）+ 单测复跑 |
| R4 | static | 全仓同类残留扫描（本单 0 残留、域外计数锁定）；Owner 豁免见 D7 |
| R5 | doc | `docs/spec/api/api.md` §1.3 结构断言 |
| R6 | static | 既有 harness（强化后）复跑的退出码与 token；Owner 豁免见 D7 |

### Out of Scope

- 域外 117 行 / 37 文件同型写法（`Detail.jsx`、`admin/instrument/List.jsx`、`System/UserManagement.jsx` 等）→ **#2612**（已建）。
- `frontend-mobile`（H5 / weapp）的错误文案路径：本单只处理 PC；移动端另有 `resolveErrorMessage` 路径，改动需独立分析。
- 修改 `resolveHttpFailure` / `fetchBlob` 的失败分类（例如让 `fetchBlob` 抛 `ApiError`）：属传输层契约变更，另立 Issue。
- 断网/网络抖动的真实浏览器与真机复验 → `[Ops-Followup of #2609]`（closing 时建，`waitForConfirm`，无 `status:*`）。
- 覆写或重跑 `docs/evidence/2488/` 下的历史产物（见 D6）。
- 把 `tests/*.test.mjs` 纳入 ESLint 配置（需独立评审 lint 范围）。

### Dependencies

- #2488

### Workload

- Size: M
- Basis: 4 个实现文件（1 helper + 3 页面共 8 处）+ 1 个运行时单测 + 2 个证据脚本（新增 scanner/verify）+ 1 个既有 harness 强化 + 1 份核心文档 + 6 条证据行；无 migration、无新端点、无跨仓依赖；基座为 `origin/issue-2488`（堆叠分支）
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | PC 管理端断网/网络故障时弹出英文技术串（`Failed to fetch` / `NetworkError when attempting to fetch resource.`），比修复前的「操作失败」更差 | #2609 正文现象表；事实 3/4 |
| 2 | 触发条件（trigger） | `fetch` 自身 reject（断网、DNS 失败、CORS、代理中断）→ `TypeError` 冒泡到 catch 的「非业务分支」 | `api.js:236` 无 try/catch |
| 3 | 根因（root cause） | 把「异常对象」的 `message` 无条件当用户文案（`e?.message &#124;&#124;`），未区分传输层 `ApiError`（已中文化）与浏览器 `TypeError`（ASCII） | 事实 1/5/6 |
| 4 | 测试盲点（test blind spot） | 三页无任何错误文案运行时测试；探针只 grep 字面量表达式，缺陷版本与修复版本同为 1（不可证伪） | 事实 6/9；`verify-2488.sh:105-112` |
| 5 | 同类扫描（same-class paths） | 域外 117 行 / 37 文件同型写法 → #2612；本单断言其计数不变（防扩面） | 事实 10；R4 |
| 6 | 修复层（fix layer） | 提示层单点收口：唯一出口 `userFacingErrorMessage` + 8 处接线；不碰传输层分类、不碰冲突持久化分支 | 变更点 1/2 |
| 7 | 红绿实证（red-green） | 红 A：还原 `e?.message &#124;&#124; '<中文>'` → `CATCH_MESSAGE` 与 `CATCH_NETWORK_FALLBACK` 均不为 1；红 B：保留守卫但兜底改 `e.message` → 仅 `CATCH_NETWORK_FALLBACK` 不为 1；运行时：朴素实现 `NAIVE_LEAK=1` 与真实实现 `ASCII_LEAK=0` 对照 | R1、R2 |
| 8 | 防复发（prevention） | ① 结构判定探针（`CATCH_RAW=0`）纳入 #2488 harness（stock 也覆盖）；② 运行时单测钉死网络串与 ApiError 两类；③ `api.md` §1.3 写明唯一出口与禁令；④ 域外清单落 #2612 | R1/R2/R5/R6、#2612 |
| 9 | 影响面（blast radius） | 仅「非业务分支」的提示文案来源；`code`/HTTP/分支顺序/冲突持久提示/成功路径不变；域外计数 117/37 不变 | 既有断言影响表、R4 |
| 10 | 未修边界（deferred） | 域外 117 处 → #2612；移动端同类路径、`fetchBlob` 分类 → 另立；真实断网/真机 → `[Ops-Followup of #2609]` | Out of Scope、D6/D8 |

*Model: deepseek/deepseek-flash*

