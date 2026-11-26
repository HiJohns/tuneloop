## Accepted Plan — #2497（5xx 内部诊断不再透传：后端口径收敛 + 前端 5xx 守卫）

### 背景与裁定

`#2480` 把前端解析升级为「含 CJK 的 message 优先于 `ERROR_CODE_MAP`」，导致后端 22 处「中文前缀 + `err.Error()`」的**内部诊断**被原样展示给用户。

**Architect 裁定（D1）：两手都做（防御纵深），不是二选一**：

- **后端（主修）**：22 处 `"message": "<中文>" + err.Error()` 改为**固定用户文案**，内部错误只进 `log.Printf`；
- **前端（守卫）**：解析器增加「`code >= 50000` 时忽略 `human_message`，走 `ERROR_CODE_MAP`」的窄规则——保证即使将来后端再漏，也不会把内部串喷给用户。

### 变更点

1. `backend/handlers` 12 个文件共 **22 处** CJK+`err.Error()` 拼接改为固定文案 + `log.Printf`（清单见 Verification Matrix R1 的扫描断言；`grep -P` 正则即判定口径）；
2. `frontend-pc/src/services/errorMessages.js`：`human_message` 层增加 `code >= 50000` 排除；
3. `frontend-mobile/src/services/errorMessageResolution.mjs`：同口径（保持双端一致）；
4. 新增回归测试：PC/mobile 各一组正/负例（`40900` 中文透传、`50000` 中文内部串 → 通用文案）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 后端 CJK 拼接归零 | 12 个 handler 文件 | 静态扫描 | 全量 `backend/handlers` | 后端 | 命中数 **0**（正则见 R1） | R1 |
| 内部错误仍进日志 | 同上 22 处 | 静态扫描 | 改动点 | 后端 | 每处有 `log.Printf` 且含 `err` | R2 |
| 前端 5xx 守卫（负例） | `frontend-pc/src/services/errorMessages.js` | `resolveErrorMessage` | `{code:50000,message:'标记删除失败：pq: …'}` | PC | 输出通用文案「服务器开小差了，请稍后再试」 | R3 |
| 正例不回退 | 同文件 | 同函数 | `{code:40900,message:'网点名称已存在'}` | PC | 原样透传该中文 | R4 |
| 双端一致 | `frontend-mobile/src/services/errorMessageResolution.mjs` | parity 脚本 | 与 PC 同用例 | H5 | 两端口径一致 | R5 |
| 构建与 lint | `backend` + `frontend-pc` | `go build ./...` + `npm run build` + `eslint` | — | 后端 + PC | 全部 exit 0 | R6 |

## Input Truth & Tool Preflight

- **Source of truth**：Issue #2497 的 22 处清单（本计划以 `grep -rP '"message":\s*"[^"]*[\x{4e00}-\x{9fff}][^"]*"\s*\+\s*err\.Error\(\)' backend/handlers/*.go` 为唯一判定口径，当前 22 命中 / 12 文件）。
- **差异核对**：改动前后该 grep 的**文件集合**必须一致（只允许行内容变化，不允许漏改文件）。
- **损坏输入拒绝**：前端对 `code >= 50000` 且 message 含 CJK 的输入一律走 code map。
- **工具盲区**：`err.Error()` 也可能出现在 `gin.H{"error": …}` 等非 `message` 字段；本单只收敛 `message`（其余字段前端不展示）。

## Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 收敛方式 | 后端改固定文案（主修）+ 前端 `code>=50000` 守卫（防御纵深），**不是二选一** |
| D2 | 保留信息 | 内部错误必须进 `log.Printf`，不得丢弃（排障需要） |
| D3 | 正例边界 | `40002/40004/40005/40900` 等业务码的中文 message 继续透传（#2480 行为不回退） |
| D4 | 范围 | 仅收敛 `message` 字段的 CJK 拼接；不改错误码、不改前端通用文案表 |

## Out of Scope

- 不改 314 处「英文前缀 + `err.Error()`」（不属于本单症状；英文前缀会被前端 code map 覆盖）；
- 不改 `gin.H{"error": …}` 等非用户可见字段；
- 不重构错误处理中间件；不改移动端 UI。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `backend/handlers/*.go` 全量 | `bash scripts/verify-2497.sh r1` | `docs/evidence/2497/R1-scan.txt` | `CJK_ERR_CONCAT=0`；扫描文件集合 = 基线 12 文件 | 任一命中 → FAIL |
| R2 | 22 处改动点 | `bash scripts/verify-2497.sh r2` | `docs/evidence/2497/R2-logs.txt` | `LOG_PRESENT>=22`；`LOST_DETAIL=0` | 缺日志 → FAIL |
| R3 | `{code:50000,message:'标记删除失败：pq: …'}` | `bash scripts/verify-2497.sh r3` | `docs/evidence/2497/R3-5xx-guard.txt` | 输出 == 「服务器开小差了，请稍后再试」；`LEAK=0` | 回显内部串 → FAIL |
| R4 | `{code:40900,message:'网点名称已存在'}` | `bash scripts/verify-2497.sh r4` | `docs/evidence/2497/R4-passthrough.txt` | 输出 == 原文；`PASSTHROUGH=1` | 被通用文案替换 → FAIL |
| R5 | PC/mobile 同用例 | `bash scripts/verify-2497.sh r5` | `docs/evidence/2497/R5-parity.txt` | `PARITY=1` | 两端不一致 → FAIL |
| R6 | HEAD 源码 | `bash scripts/verify-2497.sh r6` | `docs/evidence/2497/R6-build.txt` | `BUILD_OK` + `ESLINT_OK` + `NPM_BUILD_OK` | 任一失败 → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2497.sh r1` | docs/evidence/2497/R1-scan.txt | exit 0；含 `CJK_ERR_CONCAT=0` | PASS | 任一命中 → FAIL | dev |
| R2 | `bash scripts/verify-2497.sh r2` | docs/evidence/2497/R2-logs.txt | exit 0；含 `LOG_PRESENT>=22` 与 `LOST_DETAIL=0` | PASS | 缺日志 → FAIL | dev |
| R3 | `bash scripts/verify-2497.sh r3` | docs/evidence/2497/R3-5xx-guard.txt | exit 0；含 `LEAK=0` | PASS | 回显内部串 → FAIL | dev |
| R4 | `bash scripts/verify-2497.sh r4` | docs/evidence/2497/R4-passthrough.txt | exit 0；含 `PASSTHROUGH=1` | PASS | 被替换 → FAIL | dev |
| R5 | `bash scripts/verify-2497.sh r5` | docs/evidence/2497/R5-parity.txt | exit 0；含 `PARITY=1` | PASS | 不一致 → FAIL | dev |
| R6 | `bash scripts/verify-2497.sh r6` | docs/evidence/2497/R6-build.txt | exit 0；含 `BUILD_OK` 与 `ESLINT_OK` 与 `NPM_BUILD_OK` | PASS | 任一失败 → FAIL | dev |

## Artifact Provenance

- R1–R6（脚本生成，首行 provenance）：写 `# build=1.0.<短码>`、`# commit=<40 位 sha>`、`# dirty=false`。
  - docs/evidence/2497/R1-scan.txt
  - docs/evidence/2497/R2-logs.txt
  - docs/evidence/2497/R3-5xx-guard.txt
  - docs/evidence/2497/R4-passthrough.txt
  - docs/evidence/2497/R5-parity.txt
  - docs/evidence/2497/R6-build.txt
- gate（由 /work 执行）：`~/.config/opencode/scripts/pre-ready-check.sh 2497 --summary-file … --changed-files … --evidence-file docs/evidence/2497/evidence.json --report docs/evidence/2497/pre-ready-2497.json`；报告 `build` 须匹配 `evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$`。

## Dependencies

- #2480

### 依赖说明（非声明）

- 前端解析层与 `scripts/check-error-message-parity-2480.mjs` 由 #2480 引入（提交在 `origin/issue-2480`，**未合入 develop**）；合入前本单 `/work` 必须 HOLD。
- 关联：#2495（#2480 的第 4 轮审计发现项，本单为其承接）。

### Workload

- Size: M
- Basis: 12 个后端文件 22 处文案 + 2 个前端解析器 + 1 个验证脚本 + 6 条证据行；无 migration
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/invoice.go`
- `backend/handlers/merchant.go`
- `backend/handlers/merchant_invoice.go`
- `backend/handlers/merchant_member.go`
- `backend/handlers/payscore_handler.go`
- `backend/handlers/registration_session.go`
- `backend/handlers/site_member.go`
- `backend/handlers/smtp_config.go`
- `backend/handlers/transit_site_admin.go`
- `backend/handlers/user_management.go`
- `backend/handlers/user_staff.go`
- `backend/handlers/webhook_config.go`
- `frontend-pc/src/services/errorMessages.js`
- `frontend-mobile/src/services/errorMessageResolution.mjs`
- `frontend-pc/tests/errorMessages-5xx-2497.test.mjs`
- `scripts/verify-2497.sh`
- `docs/evidence/2497/R1-scan.txt`
- `docs/evidence/2497/R2-logs.txt`
- `docs/evidence/2497/R3-5xx-guard.txt`
- `docs/evidence/2497/R4-passthrough.txt`
- `docs/evidence/2497/R5-parity.txt`
- `docs/evidence/2497/R6-build.txt`
- `docs/evidence/2497/pre-ready-2497.json`
- `docs/evidence/2497/evidence.json`

*Model: DeepSeek V4.1 Flash 1017*

## Accepted Plan — #2497（5xx 内部诊断不再透传：后端口径收敛 + 前端 5xx 守卫）

### 背景与裁定

`#2480` 把前端解析升级为「含 CJK 的 message 优先于 `ERROR_CODE_MAP`」，导致后端 22 处「中文前缀 + `err.Error()`」的**内部诊断**被原样展示给用户。

**Architect 裁定（D1）：两手都做（防御纵深），不是二选一**：

- **后端（主修）**：22 处 `"message": "<中文>" + err.Error()` 改为**固定用户文案**，内部错误只进 `log.Printf`；
- **前端（守卫）**：解析器增加「`code >= 50000` 时忽略 `human_message`，走 `ERROR_CODE_MAP`」的窄规则——保证即使将来后端再漏，也不会把内部串喷给用户。

### 变更点

1. `backend/handlers` 12 个文件共 **22 处** CJK+`err.Error()` 拼接改为固定文案 + `log.Printf`（清单见 Verification Matrix R1 的扫描断言；`grep -P` 正则即判定口径）；
2. `frontend-pc/src/services/errorMessages.js`：`human_message` 层增加 `code >= 50000` 排除；
3. `frontend-mobile/src/services/errorMessageResolution.mjs`：同口径（保持双端一致）；
4. 新增回归测试：PC/mobile 各一组正/负例（`40900` 中文透传、`50000` 中文内部串 → 通用文案）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 后端 CJK 拼接归零 | 12 个 handler 文件 | 静态扫描 | 全量 `backend/handlers` | 后端 | 命中数 **0**（正则见 R1） | R1 |
| 内部错误仍进日志 | 同上 22 处 | 静态扫描 | 改动点 | 后端 | 每处有 `log.Printf` 且含 `err` | R2 |
| 前端 5xx 守卫（负例） | `frontend-pc/src/services/errorMessages.js` | `resolveErrorMessage` | `{code:50000,message:'标记删除失败：pq: …'}` | PC | 输出通用文案「服务器开小差了，请稍后再试」 | R3 |
| 正例不回退 | 同文件 | 同函数 | `{code:40900,message:'网点名称已存在'}` | PC | 原样透传该中文 | R4 |
| 双端一致 | `frontend-mobile/src/services/errorMessageResolution.mjs` | parity 脚本 | 与 PC 同用例 | H5 | 两端口径一致 | R5 |
| 构建与 lint | `backend` + `frontend-pc` | `go build ./...` + `npm run build` + `eslint` | — | 后端 + PC | 全部 exit 0 | R6 |

## Input Truth & Tool Preflight

- **Source of truth**：Issue #2497 的 22 处清单（本计划以 `grep -rP '"message":\s*"[^"]*[\x{4e00}-\x{9fff}][^"]*"\s*\+\s*err\.Error\(\)' backend/handlers/*.go` 为唯一判定口径，当前 22 命中 / 12 文件）。
- **差异核对**：改动前后该 grep 的**文件集合**必须一致（只允许行内容变化，不允许漏改文件）。
- **损坏输入拒绝**：前端对 `code >= 50000` 且 message 含 CJK 的输入一律走 code map。
- **工具盲区**：`err.Error()` 也可能出现在 `gin.H{"error": …}` 等非 `message` 字段；本单只收敛 `message`（其余字段前端不展示）。

## Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 收敛方式 | 后端改固定文案（主修）+ 前端 `code>=50000` 守卫（防御纵深），**不是二选一** |
| D2 | 保留信息 | 内部错误必须进 `log.Printf`，不得丢弃（排障需要） |
| D3 | 正例边界 | `40002/40004/40005/40900` 等业务码的中文 message 继续透传（#2480 行为不回退） |
| D4 | 范围 | 仅收敛 `message` 字段的 CJK 拼接；不改错误码、不改前端通用文案表 |

## Out of Scope

- 不改 314 处「英文前缀 + `err.Error()`」（不属于本单症状；英文前缀会被前端 code map 覆盖）；
- 不改 `gin.H{"error": …}` 等非用户可见字段；
- 不重构错误处理中间件；不改移动端 UI。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `backend/handlers/*.go` 全量 | `bash scripts/verify-2497.sh r1` | `docs/evidence/2497/R1-scan.txt` | `CJK_ERR_CONCAT=0`；扫描文件集合 = 基线 12 文件 | 任一命中 → FAIL |
| R2 | 22 处改动点 | `bash scripts/verify-2497.sh r2` | `docs/evidence/2497/R2-logs.txt` | `LOG_PRESENT>=22`；`LOST_DETAIL=0` | 缺日志 → FAIL |
| R3 | `{code:50000,message:'标记删除失败：pq: …'}` | `bash scripts/verify-2497.sh r3` | `docs/evidence/2497/R3-5xx-guard.txt` | 输出 == 「服务器开小差了，请稍后再试」；`LEAK=0` | 回显内部串 → FAIL |
| R4 | `{code:40900,message:'网点名称已存在'}` | `bash scripts/verify-2497.sh r4` | `docs/evidence/2497/R4-passthrough.txt` | 输出 == 原文；`PASSTHROUGH=1` | 被通用文案替换 → FAIL |
| R5 | PC/mobile 同用例 | `bash scripts/verify-2497.sh r5` | `docs/evidence/2497/R5-parity.txt` | `PARITY=1` | 两端不一致 → FAIL |
| R6 | HEAD 源码 | `bash scripts/verify-2497.sh r6` | `docs/evidence/2497/R6-build.txt` | `BUILD_OK` + `ESLINT_OK` + `NPM_BUILD_OK` | 任一失败 → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2497.sh r1` | docs/evidence/2497/R1-scan.txt | exit 0；含 `CJK_ERR_CONCAT=0` | PASS | 任一命中 → FAIL | dev |
| R2 | `bash scripts/verify-2497.sh r2` | docs/evidence/2497/R2-logs.txt | exit 0；含 `LOG_PRESENT>=22` 与 `LOST_DETAIL=0` | PASS | 缺日志 → FAIL | dev |
| R3 | `bash scripts/verify-2497.sh r3` | docs/evidence/2497/R3-5xx-guard.txt | exit 0；含 `LEAK=0` | PASS | 回显内部串 → FAIL | dev |
| R4 | `bash scripts/verify-2497.sh r4` | docs/evidence/2497/R4-passthrough.txt | exit 0；含 `PASSTHROUGH=1` | PASS | 被替换 → FAIL | dev |
| R5 | `bash scripts/verify-2497.sh r5` | docs/evidence/2497/R5-parity.txt | exit 0；含 `PARITY=1` | PASS | 不一致 → FAIL | dev |
| R6 | `bash scripts/verify-2497.sh r6` | docs/evidence/2497/R6-build.txt | exit 0；含 `BUILD_OK` 与 `ESLINT_OK` 与 `NPM_BUILD_OK` | PASS | 任一失败 → FAIL | dev |

## Artifact Provenance

- R1–R6（脚本生成，首行 provenance）：写 `# build=1.0.<短码>`、`# commit=<40 位 sha>`、`# dirty=false`。
  - docs/evidence/2497/R1-scan.txt
  - docs/evidence/2497/R2-logs.txt
  - docs/evidence/2497/R3-5xx-guard.txt
  - docs/evidence/2497/R4-passthrough.txt
  - docs/evidence/2497/R5-parity.txt
  - docs/evidence/2497/R6-build.txt
- gate（由 /work 执行）：`~/.config/opencode/scripts/pre-ready-check.sh 2497 --summary-file … --changed-files … --evidence-file docs/evidence/2497/evidence.json --report docs/evidence/2497/pre-ready-2497.json`；报告 `build` 须匹配 `evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$`。

## Dependencies

- #2480

### 依赖说明（非声明）

- 前端解析层与 `scripts/check-error-message-parity-2480.mjs` 由 #2480 引入（提交在 `origin/issue-2480`，**未合入 develop**）；合入前本单 `/work` 必须 HOLD。
- 关联：#2495（#2480 的第 4 轮审计发现项，本单为其承接）。

### Workload

- Size: M
- Basis: 12 个后端文件 22 处文案 + 2 个前端解析器 + 1 个验证脚本 + 6 条证据行；无 migration
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/invoice.go`
- `backend/handlers/merchant.go`
- `backend/handlers/merchant_invoice.go`
- `backend/handlers/merchant_member.go`
- `backend/handlers/payscore_handler.go`
- `backend/handlers/registration_session.go`
- `backend/handlers/site_member.go`
- `backend/handlers/smtp_config.go`
- `backend/handlers/transit_site_admin.go`
- `backend/handlers/user_management.go`
- `backend/handlers/user_staff.go`
- `backend/handlers/webhook_config.go`
- `frontend-pc/src/services/errorMessages.js`
- `frontend-mobile/src/services/errorMessageResolution.mjs`
- `frontend-pc/tests/errorMessages-5xx-2497.test.mjs`
- `scripts/verify-2497.sh`
- `docs/evidence/2497/R1-scan.txt`
- `docs/evidence/2497/R2-logs.txt`
- `docs/evidence/2497/R3-5xx-guard.txt`
- `docs/evidence/2497/R4-passthrough.txt`
- `docs/evidence/2497/R5-parity.txt`
- `docs/evidence/2497/R6-build.txt`
- `docs/evidence/2497/pre-ready-2497.json`
- `docs/evidence/2497/evidence.json`

*Model: DeepSeek V4.1 Flash 1017*


## Accepted Plan — #2497 v3（口径扩到 15 文件/31 处 + mobile chokepoint 可达 + 新增 R7/R8；Rev.3 修 R8 工件与 P12/P14 互斥）

> **v2 说明（修 Round 1 审计 Bug 1–6）**：① 判定口径由「`err.Error()` 字面量」扩为**任意接收者** `.Error()`，基线 `15 文件 / 31 处`（含 `site.go`、`wechat_bind.go`、`member_invite_2114.go`）；② 补 **mobile 生产 chokepoint** `frontend-mobile/src/services/api.js`；③ 新增 **R7**（全量后端回归）与 **R8**（plan.md 四源可复现）；④ R1/R2/R5 改为 R1'/R2'/R5'。
>
> **v3 说明（修 R8 的 P12/P14 互斥）**：R8 原以 `docs/evidence/2497/plan.md` 作为 artifact，但 `plan.md` 必须与 Accepted Plan 评论**字节一致**（P17/P12 文本比对），而 P14 要求每个 artifact 首行为 provenance —— 三者互斥，R8 永远无法 `pass=true`。v3 改为：**R8 的 artifact = `docs/evidence/2497/R8-plan-source.txt`**（首行 provenance + 内容为 `plan-source.sh --issue 2497` 的输出）；`plan.md` 仍作为计划快照存在于 Files，但**不再出现在 Verification Matrix / G0 / Evidence Contract 的 artifact 列**。

### 背景与裁定

`#2480` 把前端解析升级为「含 CJK 的 message 优先于 `ERROR_CODE_MAP`」，导致后端 22 处「中文前缀 + `err.Error()`」的**内部诊断**被原样展示给用户。

**Architect 裁定（D1）：两手都做（防御纵深），不是二选一**：

- **后端（主修）**：22 处 `"message": "<中文>" + err.Error()` 改为**固定用户文案**，内部错误只进 `log.Printf`；
- **前端（守卫）**：解析器增加「`code >= 50000` 时忽略 `human_message`，走 `ERROR_CODE_MAP`」的窄规则——保证即使将来后端再漏，也不会把内部串喷给用户。

### 变更点

1. `backend/handlers` **15 个文件 / 31 处** CJK + 任意 `.Error()` 拼接改为固定文案 + `log.Printf`（新增 `site.go`、`wechat_bind.go`、`member_invite_2114.go`；判定正则见 D5）；
2. `frontend-pc/src/services/errorMessages.js`：`human_message` 层增加 `code >= 50000` 排除；
3. `frontend-mobile/src/services/errorMessageResolution.mjs`：同口径（保持双端一致）；
4. 新增回归测试：PC/mobile 各一组正/负例（`40900` 中文透传、`50000` 中文内部串 → 通用文案）。
5. **mobile 生产 chokepoint**：`frontend-mobile/src/services/api.js` 的 `throw new Error(error.message || error.code || 'Request failed')` 改为先 `resolveErrorMessage({ code, message }, '请求失败，请重试')` 再 throw（或挂 `code`），使 5xx 守卫在移动端可达；`40310` 分支不变。
6. 落盘 `docs/evidence/2497/plan.md` 并提交（四源可复现，R8）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 后端 CJK 拼接归零 | 12 个 handler 文件 | 静态扫描 | 全量 `backend/handlers` | 后端 | 命中数 **0**（正则见 R1） | R1 |
| 内部错误仍进日志 | 同上 22 处 | 静态扫描 | 改动点 | 后端 | 每处有 `log.Printf` 且含 `err` | R2 |
| 前端 5xx 守卫（负例） | `frontend-pc/src/services/errorMessages.js` | `resolveErrorMessage` | `{code:50000,message:'标记删除失败：pq: …'}` | PC | 输出通用文案「服务器开小差了，请稍后再试」 | R3 |
| 正例不回退 | 同文件 | 同函数 | `{code:40900,message:'网点名称已存在'}` | PC | 原样透传该中文 | R4 |
| 双端一致 | `frontend-mobile/src/services/errorMessageResolution.mjs` | parity 脚本 | 与 PC 同用例 | H5 | 两端口径一致 | R5 |
| 构建与 lint | `backend` + `frontend-pc` | `go build ./...` + `npm run build` + `eslint` | — | 后端 + PC | 全部 exit 0 | R6 |
| R7 全量后端回归 | `backend` | `go test ./... -timeout 20m` | 修后树 | 后端 | `FULL_SUITE_EXIT=0`、`FAIL_COUNT=0`、`PANIC_COUNT=0` | R7 |
| R8 四源可复现 | `scripts/plan-source.sh`（产物 `docs/evidence/2497/R8-plan-source.txt`） | `bash scripts/plan-source.sh --issue 2497` | 修后树 | 后端 | `PLAN_SOURCE: PASS` | R8 |

## Input Truth & Tool Preflight

- **Source of truth**：Issue #2497 的 22 处清单（本计划以 `grep -rP '"message":\s*"[^"]*[\x{4e00}-\x{9fff}][^"]*"\s*\+\s*err\.Error\(\)' backend/handlers/*.go` 为唯一判定口径，当前 22 命中 / 12 文件）。
- **差异核对**：改动前后该 grep 的**文件集合**必须一致（只允许行内容变化，不允许漏改文件）。
- **损坏输入拒绝**：前端对 `code >= 50000` 且 message 含 CJK 的输入一律走 code map。
- **工具盲区**：`err.Error()` 也可能出现在 `gin.H{"error": …}` 等非 `message` 字段；本单只收敛 `message`（其余字段前端不展示）。

## Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 收敛方式 | 后端改固定文案（主修）+ 前端 `code>=50000` 守卫（防御纵深），**不是二选一** |
| D2 | 保留信息 | 内部错误必须进 `log.Printf`，不得丢弃（排障需要） |
| D3 | 正例边界 | `40002/40004/40005/40900` 等业务码的中文 message 继续透传（#2480 行为不回退） |
| D4 | 范围 | 仅收敛 `message` 字段的 CJK 拼接；不改错误码、不改前端通用文案表 |
| D5 | 判定口径 | `"message":\s*"[^"]*[\x{4e00}-\x{9fff}][^"]*"\s*\+\s*[A-Za-z_][\w.]*\.Error\(\)`（任意接收者）；基线 `BASELINE_FILES=15` / `BASELINE_HITS=31`，目标 `CJK_ERR_CONCAT=0` |

## Out of Scope

- 不改 314 处「英文前缀 + `err.Error()`」（不属于本单症状；英文前缀会被前端 code map 覆盖）；
- 不改 `gin.H{"error": …}` 等非用户可见字段；
- 不重构错误处理中间件；不改移动端 UI。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `backend/handlers/*.go` 全量 | `bash scripts/verify-2497.sh r1` | `docs/evidence/2497/R1-scan.txt` | `CJK_ERR_CONCAT=0`；扫描文件集合 = 基线 12 文件 | 任一命中 → FAIL |
| R2 | 22 处改动点 | `bash scripts/verify-2497.sh r2` | `docs/evidence/2497/R2-logs.txt` | `LOG_PRESENT>=22`；`LOST_DETAIL=0` | 缺日志 → FAIL |
| R3 | `{code:50000,message:'标记删除失败：pq: …'}` | `bash scripts/verify-2497.sh r3` | `docs/evidence/2497/R3-5xx-guard.txt` | 输出 == 「服务器开小差了，请稍后再试」；`LEAK=0` | 回显内部串 → FAIL |
| R4 | `{code:40900,message:'网点名称已存在'}` | `bash scripts/verify-2497.sh r4` | `docs/evidence/2497/R4-passthrough.txt` | 输出 == 原文；`PASSTHROUGH=1` | 被通用文案替换 → FAIL |
| R5 | PC/mobile 同用例 | `bash scripts/verify-2497.sh r5` | `docs/evidence/2497/R5-parity.txt` | `PARITY=1` | 两端不一致 → FAIL |
| R6 | HEAD 源码 | `bash scripts/verify-2497.sh r6` | `docs/evidence/2497/R6-build.txt` | `BUILD_OK` + `ESLINT_OK` + `NPM_BUILD_OK` | 任一失败 → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2497.sh r1` | docs/evidence/2497/R1-scan.txt | exit 0；含 `CJK_ERR_CONCAT=0` | PASS | 任一命中 → FAIL | dev |
| R2 | `bash scripts/verify-2497.sh r2` | docs/evidence/2497/R2-logs.txt | exit 0；含 `LOG_PRESENT>=22` 与 `LOST_DETAIL=0` | PASS | 缺日志 → FAIL | dev |
| R3 | `bash scripts/verify-2497.sh r3` | docs/evidence/2497/R3-5xx-guard.txt | exit 0；含 `LEAK=0` | PASS | 回显内部串 → FAIL | dev |
| R4 | `bash scripts/verify-2497.sh r4` | docs/evidence/2497/R4-passthrough.txt | exit 0；含 `PASSTHROUGH=1` | PASS | 被替换 → FAIL | dev |
| R5 | `bash scripts/verify-2497.sh r5` | docs/evidence/2497/R5-parity.txt | exit 0；含 `PARITY=1` | PASS | 不一致 → FAIL | dev |
| R6 | `bash scripts/verify-2497.sh r6` | docs/evidence/2497/R6-build.txt | exit 0；含 `BUILD_OK` 与 `ESLINT_OK` 与 `NPM_BUILD_OK` | PASS | 任一失败 → FAIL | dev |
| R7 | `bash scripts/verify-2497.sh r7` | docs/evidence/2497/R7-gotest.txt | exit 0；含 `FULL_SUITE_EXIT=0` 与 `FAIL_COUNT=0` 与 `PANIC_COUNT=0` | PASS | 任一失败 → FAIL | dev |
| R8 | `bash scripts/verify-2497.sh r8` | docs/evidence/2497/R8-plan-source.txt | exit 0；首行 provenance，正文含 `PLAN_SOURCE: PASS` | PASS | 四源不一致 → FAIL | architect |

## Artifact Provenance

- R1–R6（脚本生成，首行 provenance）：写 `# build=1.0.<短码>`、`# commit=<40 位 sha>`、`# dirty=false`。
  - docs/evidence/2497/R1-scan.txt
  - docs/evidence/2497/R2-logs.txt
  - docs/evidence/2497/R3-5xx-guard.txt
  - docs/evidence/2497/R4-passthrough.txt
  - docs/evidence/2497/R5-parity.txt
  - docs/evidence/2497/R6-build.txt
  - docs/evidence/2497/R7-gotest.txt
  - docs/evidence/2497/R8-plan-source.txt
  - docs/evidence/2497/plan.md
- gate（由 /work 执行）：`~/.config/opencode/scripts/pre-ready-check.sh 2497 --summary-file … --changed-files … --evidence-file docs/evidence/2497/evidence.json --report docs/evidence/2497/pre-ready-2497.json`；报告 `build` 须匹配 `evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$`。

## Dependencies

- #2480

### 依赖说明（非声明）

- 前端解析层与 `scripts/check-error-message-parity-2480.mjs` 由 #2480 引入（提交在 `origin/issue-2480`，**未合入 develop**）；合入前本单 `/work` 必须 HOLD。
- 关联：#2495（#2480 的第 4 轮审计发现项，本单为其承接）。

### Workload

- Size: L
- Basis: 15 个后端文件 31 处文案 + 2 个前端解析器 + mobile 生产入口 `api.js` + 1 个验证脚本 + 8 条证据行 + plan.md 落盘；无 migration
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/invoice.go`
- `backend/handlers/merchant.go`
- `backend/handlers/merchant_invoice.go`
- `backend/handlers/merchant_member.go`
- `backend/handlers/payscore_handler.go`
- `backend/handlers/registration_session.go`
- `backend/handlers/site_member.go`
- `backend/handlers/smtp_config.go`
- `backend/handlers/transit_site_admin.go`
- `backend/handlers/user_management.go`
- `backend/handlers/user_staff.go`
- `backend/handlers/webhook_config.go`
- `frontend-pc/src/services/errorMessages.js`
- `backend/handlers/site.go`
- `backend/handlers/wechat_bind.go`
- `backend/handlers/member_invite_2114.go`
- `frontend-mobile/src/services/api.js`
- `frontend-mobile/src/services/errorMessageResolution.mjs`
- `frontend-pc/tests/errorMessages-5xx-2497.test.mjs`
- `scripts/verify-2497.sh`
- `docs/evidence/2497/R1-scan.txt`
- `docs/evidence/2497/R2-logs.txt`
- `docs/evidence/2497/R3-5xx-guard.txt`
- `docs/evidence/2497/R4-passthrough.txt`
- `docs/evidence/2497/R5-parity.txt`
- `docs/evidence/2497/R6-build.txt`
- `docs/evidence/2497/R7-gotest.txt`
- `docs/evidence/2497/R8-plan-source.txt`
- `docs/evidence/2497/plan.md`
- `docs/evidence/2497/pre-ready-2497.json`
- `docs/evidence/2497/evidence.json`

*Model: DeepSeek V4.1 Flash 1017*




