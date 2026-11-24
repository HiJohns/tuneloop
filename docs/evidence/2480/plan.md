## Accepted Plan — #2480 v2（409 业务冲突：具体文案优先 + 表单内持久提示；Tier 2 已另立 #2485）

> **v2 说明**：v1（`2026-11-24T20:49` 那条）只覆盖「文案层序」，遗漏 Owner `2026-11-24T20:34` 追加的「提示闪现太快、读不完」4 条追加期望 —— 审计已 REJECT 并回 analysis（`#2480` 审计报告）。v2 **保留** v1 的 R1–R6（实现已存在且审计通过，fix `03e336fa5`，不重做），新增 R7–R13 覆盖**持久提示 + 同类转换 + 扫描分层**；证据需在新 fix commit 后**全部重生成**（禁止只改 `build` 字段）。

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| v1 已实现四层解析：`message_map → human_message → code_map → raw_message → fallback`（PC/mobile 同序 + 平价脚本） | `frontend-pc/src/services/errorMessages.js`、`frontend-mobile/src/services/errorMessageResolution.mjs`（`origin/issue-2480@03e336fa5`） |
| 网点重名仍用 **3 秒 toast**：`if (result.code === 40900) { message.error(result.message \|\| '网点名称已存在'); return }` | `frontend-pc/src/pages/SiteManagement.jsx:279-282` |
| 创建员工冲突同样用 toast（把冲突员工名拼进一句 toast） | `frontend-pc/src/pages/StaffManagement.jsx:334-337` |
| `message.error(` 在 PC 端共 **347** 处（多数是瞬时/网络/权限，**不能**机械全改） | `grep -rn "message.error(" frontend-pc/src` |
| 以业务冲突 code 判定的表单提交点仅 **2 处**（`40900`）；`40901/40902` 分支已是 `Modal` 引导（持久形态） | `grep -rn "code === 40900\|code === 40901\|code === 40902" frontend-pc/src` |
| PC 无组件测试运行器（`devDependencies` 无 vitest/@testing-library）→ 纯函数用 `node --test`（`frontend-pc/tests/*.mjs` 既有 2 个） | `frontend-pc/package.json`、`frontend-pc/tests/` |

**结论**：文案层序已修好，但「业务可纠正错误」的可读性仍被 3 秒 toast 破坏。v2 = 共享持久提示件 + 2 处 Tier 1 转换 + 全量扫描分层（Tier 2 已另立 #2485，避免一次提交吞掉 347 处的逐页判定）。

### 变更点

#### A. 已完成（v1，保留不重做）

1. PC `errorMessages.js`：`isHumanReadableMessage` / `resolveErrorCore`，层序 `message_map → human_message → code_map → raw_message → fallback`。
2. mobile `errorMessageResolution.mjs`（纯核心）+ `errorMessages.js` 仅做 `t()` 包装（`human_message`/`raw_message` 不套 `t`）。
3. `scripts/check-error-message-parity-2480.mjs`（10 例平价，漂移时 exit 1）。

#### B. 本轮新增

4. `frontend-pc/src/utils/formConflict.js`（纯函数，单测覆盖）：
   - `BUSINESS_CORRECTABLE_CODES = [40002, 40004, 40005, 40900]`（**排除** `40901/40902`：一人一号/既有账户分支已有 `Modal` 引导）；
   - `isBusinessCorrectableConflict(result)` → `Boolean(result) && codes.includes(Number(result.code))`；
   - `conflictText(result, fallback = '请修改后重新提交')` → 非空 `message` 优先，否则 fallback。
5. `frontend-pc/src/components/FormConflictAlert.jsx`（共享件，唯一 owner）：`text` 为空返回 `null`；否则 `Alert type="error" showIcon`，带 `data-testid` 透传（默认 `form-conflict-alert`），样式 marginBottom 8。
6. `frontend-pc/src/pages/SiteManagement.jsx`：
   - 新增 `const [formConflict, setFormConflict] = useState('')`；
   - `code === 40900` 分支改为 `setFormConflict(conflictText(result, '网点名称已存在')); return`（**删除** `message.error`）；
   - 渲染：网点名称 `Form.Item` 之后插入 `<FormConflictAlert text={formConflict} />`；
   - 清理（四处）：名称 `Input` 的 `onChange`、`handleSubmit` 开头 `setFormConflict('')`、取消按钮、创建/编辑成功后的 `setViewMode('detail')` 之前。
7. `frontend-pc/src/pages/StaffManagement.jsx`：
   - 同样接入 `FormConflictAlert` + `formConflict` state；`code === 40900` 分支（冲突员工姓名列表）改为持久提示（文本 = 既有拼接文案），删除该分支的 `message.error`；
   - 清理：冲突相关字段（姓名/手机号/邮箱）任一 `onChange`、`handleSubmit` 开头、取消/成功。
8. `scripts/scan-form-error-toasts-2480.sh`（只读扫描，产出 Tier 2 输入）：
   - 枚举 `frontend-pc/src` 全部 `message.error(` 调用点（文件:行 + 前 8 行内的 `code === <n>` 判定 + 是否 `Modal` 形态）；
   - 分类：`converted`（Tier 1 已转持久）/ `keep:instant`（成功/网络/权限/异常兜底）/ `defer:tier2`（形态待判定）；
   - **无任何写操作**（`READONLY=1`）。
9. `scripts/verify-2480.sh`：扩展 `r7..r13`（保留 `r1..r6` 不变）。

### Tier 分层（Decision Lock D5 落地）

| 层 | 内容 | 归属 |
|---|---|---|
| Tier 1 | `code ∈ {40900,40002,40004,40005}` 且当前为 `message.error` 的表单提交点 —— 实扫 **2 处**（`SiteManagement.jsx:279`、`StaffManagement.jsx:334`），全部改为 `<FormConflictAlert>` | **本单（R8/R9/R10）** |
| Tier 2 | 其余 `message.error(resolveErrorMessage(result))` 表单提交点（未按 code 分流，需逐页判定可纠正/瞬时） | **#2485**（已创建，`status:todo`） |

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 PC 解析器四层顺序（v1 保留） | `frontend-pc/src/services/errorMessages.js` | `node --test frontend-pc/tests/errorMessages.test.mjs` | 8 条 fixture | PC | 中文透传 / ASCII 走 map / 40912 透传 / L1 命中 / fallback | R1 |
| R2 mobile 纯核心等价（v1 保留） | `frontend-mobile/src/services/errorMessageResolution.mjs` | `node --test frontend-mobile/src/services/errorMessageResolution.test.mjs` | 同 R1 | 小程序/H5 | `layer` 序列与 R1 完全一致 | R2 |
| R3 双端平价（v1 保留） | `scripts/check-error-message-parity-2480.mjs` | `node scripts/check-error-message-parity-2480.mjs` | 10 条 fixture | 双端 | `PARITY=1`、`CASES=10`、`DIFFS=0` | R3 |
| R4 出口接线（v1 保留） | `frontend-mobile/src/services/errorMessages.js`、`frontend-mobile/src/services/api.js` | 静态结构断言 | 修后源码 | 小程序/H5 | `SINGLE_EXIT=1`、`RAW_MESSAGE_NO_T=1`、`REEXPORT_KEPT=1` | R4 |
| R5 构建与 lint（v1 保留，**需重跑**） | `frontend-pc`、`frontend-mobile` | `cd frontend-pc && npm run build` + `npx eslint` 改动文件 | 修后树 | PC + mobile | `PC_BUILD=0`、`LINT_EXIT=0` | R5 |
| R6 gate 四源（v1 保留，**需重跑**） | `docs/evidence/2480/pre-ready-2480.json`、`docs/evidence/2480/plan.md` | `--full-gate-cmd "cd frontend-pc && npm run build"` + `plan-source.sh --issue 2480` | 修后树 | PC | `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | R6 |
| R7 业务冲突判定纯函数 | `frontend-pc/src/utils/formConflict.js` | `node --test frontend-pc/tests/formConflict.test.mjs` | `{code:40900}`/`{code:40002}`/`{code:50000}`/`{code:20000}`/`null`/`{code:40901}` | PC | `40900/40002` → true；`50000/20000/40901/null` → false；`conflictText({message:''},'x')` → `x` | R7 |
| R8 网点表单持久提示（主诉） | `frontend-pc/src/pages/SiteManagement.jsx`、`frontend-pc/src/components/FormConflictAlert.jsx` | `/organization/sites` 创建/编辑网点 | 后端 `409/40900`（名称已存在） | PC | 40900 分支**不再** `message.error`；渲染 `FormConflictAlert`；改名/取消/成功提交均清空提示 | R8 |
| R9 创建员工持久提示（同类） | `frontend-pc/src/pages/StaffManagement.jsx`、`frontend-pc/src/components/FormConflictAlert.jsx` | `/staff` 创建员工 | 后端 `409/40900`（姓名/手机/邮箱冲突） | PC | 同 R8；冲突员工列表文本进持久提示，不用 toast | R9 |
| R10 Tier 1 完整性与残留 | `frontend-pc/src/pages/SiteManagement.jsx`、`frontend-pc/src/pages/StaffManagement.jsx`、`frontend-pc/src/components/FormConflictAlert.jsx` | `bash scripts/verify-2480.sh r10`（grep 度量） | 全仓扫描 | PC | `TIER1_SITES=2`、`TIER1_CONVERTED=2`、`RESIDUAL_BUSINESS_TOAST=0`、`SHARED_REUSE=2` | R10 |
| R11 Tier 2 扫描清单（只读） | `scripts/scan-form-error-toasts-2480.sh` | `bash scripts/scan-form-error-toasts-2480.sh` | 修后树 | PC | 输出每行「文件:行 + tier + 理由」，`SCAN_ROWS>0`、`CLASSIFIED_ROWS=SCAN_ROWS`、`READONLY=1`、`DB_WRITES=0` | R11 |
| R12 构建与 lint（含新文件） | `frontend-pc` | `cd frontend-pc && npm run build` + `npx eslint` 改动文件 | 修后树 | PC | `PC_BUILD=0`、`LINT_EXIT=0` | R12 |
| R13 gate 四源 + 全量 PC 构建 | `docs/evidence/2480/pre-ready-2480.json`、`docs/evidence/2480/plan.md` | `--full-gate-cmd "cd frontend-pc && npm run build"` + `plan-source.sh --issue 2480` | 修后树（干净） | PC | `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS`、`REBUILT_ROWS=13` | R13 |

## Input Truth & Tool Preflight

- **Source of truth**：Owner 追加期望（`2026-11-24T20:34:55Z`）原文 3 条 + 「回评证据」1 条；后端 `409` 家族语义（`code` 家族 = 用户可改输入后重试）。
- **分类 rubric（Tier 判定，唯一口径）**：
  - **业务可纠正**：`code ∈ {40002,40004,40005,40900}` 且提示内容指向**用户可修改的输入**（名称/字段值/状态前置）→ 必须持久提示；
  - **保留 toast（instant）**：成功提示、网络异常（`err.message`）、权限（`403xx`）、服务端故障（`500xx`）、以及 `40901/40902`（一人一号/既有账户，已有 `Modal` 引导分支）；
  - **无法判定**：标记 `defer:tier2` 并计入 #2485，不得在本单猜改。
- **差异核对**：`grep -rn "code === 40900\|code === 40002\|code === 40004\|code === 40005" frontend-pc/src` 命中 = `SiteManagement.jsx`、`StaffManagement.jsx`（各 1 处 `message.error`）；`MerchantMemberManagement.jsx` 的 `40901/40902` 已走 `Modal` → 排除。
- **损坏输入拒绝**：`result` 为 `null`/`undefined`/非对象 → `isBusinessCorrectableConflict=false`（不得抛错）；`message` 非字符串 → 用 fallback；`text` 为空 → 组件返回 `null`（不渲染空 Alert）。
- **工具盲区**：PC 无组件测试运行器 → UI 行用「纯函数 `node --test` + 静态接线断言」，**真实渲染截图由 `[Ops-Followup of #2480]` 承载**（Owner 豁免 D6，`evidence_type: static`）。扫描脚本只能做**文本级**分类（同一行有多分支时会标注 `ambiguous`），不能替代人工判定。
- **影响面预检**：`<FormConflictAlert>` 仅新增，不改 `message.success` / `message.error` 的瞬时语义；`SiteManagement`/`StaffManagement` 的成功路径、`40901` 冲突选择列表（`setSearchResults`）逻辑**不动**。
- **证据时序**：v1 的 R1–R6 artifact 生成于 `03e336fa5`；本轮新 fix commit 之后必须**全部重生成**（`verify-2480.sh r1..r13`），`evidence.json` 的 `build/commit` 由脚本从 `git rev-parse` 派生，禁止手改。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 判定口径：**含 CJK 的 message 优先于 `ERROR_CODE_MAP`**（v1 已实现，保留），ASCII 仍走 map | Architect | 已锁定（v1） | — |
| D2 | 不改 `ERROR_CODE_MAP` 内容（`40900/40901` 保留为兜底） | Architect | 已锁定（v1） | — |
| D3 | mobile 与 PC 出口行为对齐（`raw_message` 层 + 后端原文不套 `t()`） | Architect | 已锁定（v1） | — |
| D4 | `40901/40902` **不**改持久提示（一人一号/既有账户已有 `Modal` 引导，且冲突需选择而非「改输入」） | Architect | 本计划锁定 | 若 Owner 要求改 → 回 Architect |
| D5 | **Tier 分层**：Tier 1（`40900/40002/40004/40005` 的 toast）= 本单 2 处全改；Tier 2（未按 code 分流的表单提示）= **#2485** 承接（已创建并回链） | Architect | owner_approved（Owner 要求「同类统一」，本计划给出分层执行方案） | Tier 2 若不接受分层 → 回 Architect 合并成大盘 |
| D6 | UI 行证据形态：纯函数 `node --test` + 静态接线断言（PC 无测试运行器）；**真实渲染/截图由 Ops Follow-up 回评** | Architect | owner_approved（沿用本仓前端豁免惯例） | 若审计要求 render → 另立前端测试基建 Issue |
| D7 | 共享件唯一 owner：`FormConflictAlert` + `formConflict.js`；两页只消费，**禁止**各页自写 Alert markup / 各自维护 code 白名单 | Architect | 本计划锁定 | 阻塞实现 |
| D8 | 提示清空时机：字段编辑 / 提交开始 / 取消 / 成功跳转 —— 四处必须同时清空（避免陈旧提示残留） | Architect | 本计划锁定 | 阻塞实现 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R8 | static | 网点表单接线结构断言（Owner 豁免见 D6） |
| R9 | static | 创建员工接线结构断言（同上） |
| R10 | static+scan | grep 度量（可失败），非渲染证据 |

## Out of Scope

- **Tier 2 全量转换**（347 处 `message.error` 的逐页分类）→ **#2485**（已创建，`status:todo`，`Parent` 关系为「承接」而非拆分）。
- mobile 端 `dialog.alert` 提示形态（Owner 追加期望仅涉 PC 表单；移动端提示改版另议）。
- 全局 toast 默认时长配置（`message.config`）与 `key` 去重：本单已把业务冲突改为**无 toast**，故追加期望 ② 在本单路径上自然满足；其它 toast 的时长策略随 #2485 分类处理。
- `40901/40902` 分支（已有 `Modal` 引导）。
- PC 组件测试基建（vitest/@testing-library 引入）。
- 全量 `npm test`（两包均无测试脚本；由 R5/R6/R12/R13 构建分担）。

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|---|---|---|---|
| R1 | 8 条 fixture（中文/ASCII/空/40912/L1/ null/未知 code 中文/未知 code ASCII） | PC `resolveErrorMessage` | `docs/evidence/2480/R1-pc-resolver.txt` |
| R2 | 同 R1 的 8 条 | mobile `resolveErrorCore` | `docs/evidence/2480/R2-mobile-core.txt` |
| R3 | 10 条（40900/40901/49999 × 中文/ASCII/空） | `node scripts/check-error-message-parity-2480.mjs` | `docs/evidence/2480/R3-parity.txt` |
| R4 | 修后 `errorMessages.js` + `api.js` | 静态结构断言 | `docs/evidence/2480/R4-wiring-static.txt` |
| R5 | 修后 `frontend-pc` / `frontend-mobile` | `npm run build` + `npx eslint` | `docs/evidence/2480/R5-build-lint.txt` |
| R6 | 修后干净树 + gate 报告 | 读校验（`plan-source.sh`） | `docs/evidence/2480/R6-gate.txt` |
| R7 | `{code:40900}`、`{code:40002}`、`{code:50000}`、`{code:20000}`、`null`、`{code:40901}`、`{code:40900,message:''}` | `node --test frontend-pc/tests/formConflict.test.mjs` | `docs/evidence/2480/R7-conflict-fn.txt` |
| R8 | 修后 `SiteManagement.jsx`（40900 分支 + 4 处清空 + Alert 渲染） | 静态结构断言 + Ops 截图回评 | `docs/evidence/2480/R8-site-alert.txt` |
| R9 | 修后 `StaffManagement.jsx`（40900 分支 + 清空 + Alert） | 静态结构断言 + Ops 截图回评 | `docs/evidence/2480/R9-staff-alert.txt` |
| R10 | 全仓 `frontend-pc/src` 业务冲突判定集 | `bash scripts/verify-2480.sh r10` | `docs/evidence/2480/R10-residual.txt` |
| R11 | 修后 `frontend-pc/src` 全量 `message.error(` | `bash scripts/scan-form-error-toasts-2480.sh` | `docs/evidence/2480/R11-scan.txt` |
| R12 | 修后 `frontend-pc` | `npm run build` + `npx eslint` | `docs/evidence/2480/R12-build-lint.txt` |
| R13 | 修后干净树 + gate 报告 | 读校验（`plan-source.sh` + gate 解析） | `docs/evidence/2480/R13-gate.txt` |

### Assertions

- R1：`C40900_CN=网点名称已存在`、`L40900_CN=human_message`、`C40900_ASCII=数据冲突，请刷新后重试`、`L40900_ASCII=code_map`、`C40912_CN` 透传、`C_L1=未找到订单`、`C_NULL=操作失败，请重试`、`L49999_ASCII=raw_message`、`LAYERS=`（8 项）；负例：中文被遮蔽或层序错位 → FAIL。
- R2：`L_CN=human_message`、`L_ASCII=code_map`、`L_EMPTY=code_map`、`L_40912=human_message`、`L_NULL=fallback`、`LAYERS=` 与 R1 逐项相同；负例：任一 layer 不同 → FAIL。
- R3：`PARITY=1`、`CASES=10`、`DIFFS=0`、`PARITY_EXIT=0`；负例：任一 case 文本不同 → FAIL。
- R4：`SINGLE_EXIT=1`、`RAW_MESSAGE_NO_T=1`、`REEXPORT_KEPT=1`；负例：无条件套 `t` → FAIL。
- R5/R12：`PC_BUILD=0`、`LINT_EXIT=0`；负例：任一非 0 → FAIL。
- R6/R13：`GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS`；R13 另需 `REBUILT_ROWS=13`（证明 13 份 artifact 均为新 commit 生成）；负例：`P5=NA`、`REBUILT_ROWS<13` 或 sha 不一致 → FAIL。
- R7：`TRUE_40900=1`、`TRUE_40002=1`、`FALSE_50000=1`、`FALSE_20000=1`、`FALSE_NULL=1`、`FALSE_40901=1`、`TEXT_FALLBACK=请修改后重新提交`、`TEXT_MESSAGE=网点名称已存在`；负例：`40901` 判 true → FAIL。
- R8：`PERSIST_ALERT=1`、`NO_40900_TOAST=1`、`CLEAR_ON_TYPING=1`、`CLEAR_ON_CANCEL=1`、`CLEAR_ON_SUCCESS=1`、`CLEAR_ON_SUBMIT=1`；负例：40900 分支仍出现 `message.error` → FAIL。
- R9：`PERSIST_ALERT=1`、`NO_40900_TOAST=1`、`CLEAR_ON_TYPING=1`、`CLEAR_ON_SUBMIT=1`；负例：仍把冲突员工列表拼进 `message.error` → FAIL。
- R10：`TIER1_SITES=2`、`TIER1_CONVERTED=2`、`RESIDUAL_BUSINESS_TOAST=0`、`SHARED_REUSE=2`、`LOCAL_ALERT_DUP=0`；负例：残留 ≥1 或页面自写 Alert → FAIL。
- R11：`SCAN_ROWS>0`、`CLASSIFIED_ROWS=SCAN_ROWS`、`TIER1_SITES=2`、`READONLY=1`、`DB_WRITES=0`、`AMBIGUOUS=`（有则列出）；负例：出现 `UPDATE`/`DELETE` 或存在未分类行 → FAIL。
- 通用 provenance：每份 artifact 首行 `# build=1.0.<fix> # commit=<sha> # dirty=false`。
- 负例（跨行）：任何 artifact 出现 `[object Object]`、`undefined` 字面量或被遮蔽的「数据冲突，请刷新后重试」（R8/R9 的 40900 路径）→ FAIL。

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2480/R1-pc-resolver.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R2-mobile-core.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R3-parity.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R4-wiring-static.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R5-build-lint.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R6-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R7-conflict-fn.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R8-site-alert.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R9-staff-alert.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R10-residual.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R11-scan.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R12-build-lint.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R13-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| symptom | trigger | root cause | test blind spot | same-class paths | fix layer | red-green | prevention |
|---|---|---|---|---|---|---|---|
| 网点重名只显示「数据冲突，请刷新后重试」 | 后端 409 + 具体中文 message | code map 先于 message 透传 | 无「code+中文 message」用例 | 邀请重复、定损重复 | 前端解析器 | R1 红绿 | D1 判定顺序 |
| 具体文案只显示 3 秒，读不完 | 用户需回去改输入 | 业务冲突用瞬时 toast | 无停留时长/形态用例 | 创建员工冲突 | 前端表单层 | R8/R9 红绿（旧：`message.error`；新：Alert） | D7 共享件 |
| mobile 端同类冲突只显示通用文案 | 同上 | mobile 无 message 透传层 | 无 mobile 解析用例 | 全部 mobile 错误提示 | 前端解析器 | R2 红绿 | 纯核心 + 平价 |
| 双端文案不一致 | 同一后端错误 | 两套实现各自演进 | 无跨端平价用例 | PC/mobile 全量提示 | 前端解析器 | R3 红绿 | D5（v1）/ 平价脚本 |
| 提示残留（改了输入还显示旧错误） | 用户改名后未重新提交 | 无清空时机 | 无状态清空断言 | 任何持久提示 | 前端表单层 | R8 `CLEAR_ON_*` | D8 四处清空 |
| 各页自写 Alert 形态不一 | 多人各自实现 | 无共享件 | 无结构断言 | 其它表单页 | 前端组件层 | R10 `LOCAL_ALERT_DUP=0` | D7 单一 owner |
| Tier 2 无限膨胀 / 被静默延期 | 347 处 toast | 无分层与清单 | 无扫描产物 | 分类/促销/成员等页 | 流程/清单 | R11 `CLASSIFIED_ROWS` | D5 + #2485 |
| ASCII 英文 message 被直接抛给用户 | 后端英文技术串 | 全透传 | 无 ASCII 负例 | 500xx 内部错误 | 前端解析器 | R1 `C49999_ASCII` | 仅 CJK 透传 |
| 4091x 历史行为被翻转 | IAM 字段冲突 | 有人把 4091x 加进 map | 无回归用例 | 40912/40913 | 前端解析器 | R1 `C40912_CN` | D2 不改 map |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2480.sh r1` | `docs/evidence/2480/R1-pc-resolver.txt` | exit 0；grep `L40900_CN=human_message`、`L40900_ASCII=code_map`、`LAYERS=` | PASS | 中文被通用文案遮蔽 → FAIL | dev |
| R2 | `bash scripts/verify-2480.sh r2` | `docs/evidence/2480/R2-mobile-core.txt` | exit 0；grep `L_CN=human_message`、`L_ASCII=code_map`、`LAYERS=` | PASS | layer 序列与 PC 不同 → FAIL | dev |
| R3 | `bash scripts/verify-2480.sh r3` | `docs/evidence/2480/R3-parity.txt` | exit 0；grep `PARITY=1`、`CASES=10`、`DIFFS=0` | PASS | 任一 case 不同 → FAIL | dev |
| R4 | `bash scripts/verify-2480.sh r4` | `docs/evidence/2480/R4-wiring-static.txt` | exit 0；grep `SINGLE_EXIT=1`、`RAW_MESSAGE_NO_T=1`、`REEXPORT_KEPT=1` | PASS | 无条件套 `t` → FAIL | dev |
| R5 | `bash scripts/verify-2480.sh r5` | `docs/evidence/2480/R5-build-lint.txt` | exit 0；grep `PC_BUILD=0`、`LINT_EXIT=0` | PASS | 任一非 0 → FAIL | dev |
| R6 | `bash scripts/verify-2480.sh r6` | `docs/evidence/2480/R6-gate.txt` | exit 0；grep `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | PASS | `P5=NA` → FAIL | dev |
| R7 | `bash scripts/verify-2480.sh r7` | `docs/evidence/2480/R7-conflict-fn.txt` | exit 0；grep `TRUE_40900=1`、`FALSE_40901=1`、`TEXT_FALLBACK=` | PASS | 40901 判 true → FAIL | dev |
| R8 | `bash scripts/verify-2480.sh r8` | `docs/evidence/2480/R8-site-alert.txt` | exit 0；grep `PERSIST_ALERT=1`、`NO_40900_TOAST=1`、`CLEAR_ON_CANCEL=1` | PASS | 40900 仍 `message.error` → FAIL | dev |
| R9 | `bash scripts/verify-2480.sh r9` | `docs/evidence/2480/R9-staff-alert.txt` | exit 0；grep `PERSIST_ALERT=1`、`NO_40900_TOAST=1`、`CLEAR_ON_TYPING=1` | PASS | 冲突列表仍在 toast → FAIL | dev |
| R10 | `bash scripts/verify-2480.sh r10` | `docs/evidence/2480/R10-residual.txt` | exit 0；grep `TIER1_CONVERTED=2`、`RESIDUAL_BUSINESS_TOAST=0`、`SHARED_REUSE=2` | PASS | 残留 ≥1 → FAIL | dev |
| R11 | `bash scripts/verify-2480.sh r11` | `docs/evidence/2480/R11-scan.txt` | exit 0；grep `CLASSIFIED_ROWS=`、`READONLY=1`、`DB_WRITES=0` | PASS | 存在未分类行 → FAIL | dev |
| R12 | `bash scripts/verify-2480.sh r12` | `docs/evidence/2480/R12-build-lint.txt` | exit 0；grep `PC_BUILD=0`、`LINT_EXIT=0` | PASS | 任一非 0 → FAIL | dev |
| R13 | `bash scripts/verify-2480.sh r13` | `docs/evidence/2480/R13-gate.txt` | exit 0；grep `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`REBUILT_ROWS=13` | PASS | `REBUILT_ROWS<13` → FAIL | dev |

### Workload

- Size: M
- Basis: 2 个新前端文件（纯函数 + 共享组件）+ 2 页接线 + 1 个测试 + 1 个扫描脚本 + verify 扩展 7 行 + 13 条证据行；无后端、无 migration；Tier 2 已拆出 #2485
- Confidence: medium（UI 行无组件测试运行器，依赖静态断言 + Ops 截图回评）

### Files（变更文件清单）

- `frontend-pc/src/utils/formConflict.js`
- `frontend-pc/src/components/FormConflictAlert.jsx`
- `frontend-pc/src/pages/SiteManagement.jsx`
- `frontend-pc/src/pages/StaffManagement.jsx`
- `frontend-pc/tests/formConflict.test.mjs`
- `frontend-pc/src/services/errorMessages.js`
- `frontend-pc/tests/errorMessages.test.mjs`
- `frontend-mobile/src/services/errorMessageResolution.mjs`
- `frontend-mobile/src/services/errorMessages.js`
- `frontend-mobile/src/services/errorMessageResolution.test.mjs`
- `frontend-mobile/src/services/api.js`
- `scripts/check-error-message-parity-2480.mjs`
- `scripts/scan-form-error-toasts-2480.sh`
- `scripts/verify-2480.sh`
- `docs/evidence/2480/plan.md`
- `docs/evidence/2480/pre-ready-2480.json`
- `docs/evidence/2480/R1-pc-resolver.txt`
- `docs/evidence/2480/R2-mobile-core.txt`
- `docs/evidence/2480/R3-parity.txt`
- `docs/evidence/2480/R4-wiring-static.txt`
- `docs/evidence/2480/R5-build-lint.txt`
- `docs/evidence/2480/R6-gate.txt`
- `docs/evidence/2480/R7-conflict-fn.txt`
- `docs/evidence/2480/R8-site-alert.txt`
- `docs/evidence/2480/R9-staff-alert.txt`
- `docs/evidence/2480/R10-residual.txt`
- `docs/evidence/2480/R11-scan.txt`
- `docs/evidence/2480/R12-build-lint.txt`
- `docs/evidence/2480/R13-gate.txt`
- `docs/evidence/2480/evidence.json`

## Dependencies

- 无

### 依赖与关联说明（非硬依赖）

- **#2485（已创建，`status:todo`）**：Tier 2 承接单（其余未按 code 分流的表单提示逐页分类改持久）。本单产出 `R11-scan.txt` 作为其输入；两单互不阻塞（本单 Tier 1 可独立交付）。
- **#2477 / #2455（accepted）**：网点重名的 409 message 来源；本单保证该 message 以持久形态呈现。
- **#2459（analysis，返工中）**：商户重名 409 同样受益（若其前端使用 `resolveErrorMessage` + toast，则落入 #2485 批次）。
- **#2100 / #2104 / #2290**：同一处解析器的历史教训。
- **部署侧**：预生产截图（网点重名持久提示 + 员工冲突持久提示）+ 真机/浏览器停留形态回评，由 `[Ops-Followup of #2480]`（PASS 时创建，`waitForConfirm`）承载。

*Model: DeepSeek V4.1 Flash 1017*

