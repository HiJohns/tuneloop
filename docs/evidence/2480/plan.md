## Accepted Plan — #2480（409 业务冲突文案：后端具体 message 优先于 `ERROR_CODE_MAP` 通用文案）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| PC 解析顺序：L1 英文精确 map → L2 `ERROR_CODE_MAP[code]` → L3 任意 message 透传 → fallback | `frontend-pc/src/services/errorMessages.js:90-101` |
| `ERROR_CODE_MAP` 仍含 `40900: '数据冲突，请刷新后重试'`、`40901: 同` → 具体中文 message 在 L2 被遮蔽 | `errorMessages.js:24-25` |
| `4091x`（IAM 字段冲突）**不在** map，靠 L3 透传（#2290/#2104 的历史修补，注释在 `:95-98`） | 同上 |
| mobile 解析顺序：L1 英文精确 map → L2 `ERROR_CODE_MAP[code]` → **fallback（无 message 透传层）** | `frontend-mobile/src/services/errorMessages.js:96-102` |
| mobile `ERROR_CODE_MAP` 同样含 `40900/40901` 通用文案 | `frontend-mobile/src/services/errorMessages.js:26-27` |
| mobile 出口统一经 `t()`（#2214）；`t(K) === K` 当字典无该键 → 对后端中文 message 恒等 | `frontend-mobile/src/i18n/index.js:16-23` |
| mobile 纯逻辑测试惯例：抽出 `.mjs` 供 `node --test`（`.js` 在 mobile 无 `type:module`） | `frontend-mobile/src/services/sessionToken.test.mjs:1-10` |
| 后端大量 409 携带**可读中文** message（如 `定损已处理，请勿重复操作`、`该用户已有待处理的邀请申请`、`网点名称已存在`） | `backend/handlers/appeal.go:489`、`member_invite_2114.go:158`、`#2477` 新增预检 |

**结论**：把「**可读中文 message**」置于 code 家族通用文案**之前**（L1 之后），使 409 家族（及其它携带中文的业务冲突）显示具体原因；ASCII message 视为机器码仍走 map（保持向后兼容）；mobile 同时补齐与 PC 一致的 L4 透传层。

### 变更点

1. `frontend-pc/src/services/errorMessages.js`：
   - 新增导出 `isHumanReadableMessage(message)`：`typeof === 'string'` 且含 CJK 统一表意文字（`/[\u3400-\u4dbf\u4e00-\u9fff]/`）→ `true`；
   - `resolveErrorMessage` 顺序调整为：① `ERROR_MESSAGE_MAP[message]`（精确英文）→ ② `isHumanReadableMessage(message)` → 直接返回 `message` → ③ `ERROR_CODE_MAP[code]` → ④ `if (message) return message` → ⑤ fallback；
   - 注释固化四层口径并引用 #2480（含「40900 通用文案仅在无可读 message 时兜底」）。
2. `frontend-mobile/src/services/errorMessageResolution.mjs`（**新增纯核心，供两端平价与单测**）：
   - 导出 `isHumanReadableMessage`、`resolveErrorCore({code, message}, { codeMap, messageMap, fallback })`；
   - 返回 `{ text, layer }`，`layer ∈ {'message_map','human_message','code_map','raw_message','fallback'}`（用于断言「哪一层胜出」，而非只比字符串）。
3. `frontend-mobile/src/services/errorMessages.js`：
   - `resolveErrorMessage` 改为调用 `resolveErrorCore`（传入本文件的 `ERROR_CODE_MAP` / `ERROR_MESSAGE_MAP` / `fallback`）；
   - 仅对 map 命中层（`message_map` / `code_map` / `fallback`）套 `t()`；`human_message` / `raw_message` 直接返回后端原串（避免对用户文案做无意义的字典查找）；
   - `api.js` 的 re-export 与 `ERROR_CODE_MAP`/`ERROR_MESSAGE_MAP` 导出保持不变（调用点零改动）。
4. `scripts/check-error-message-parity-2480.mjs`：用同一 fixture 表（10 条）分别调用 **PC 的 `resolveErrorMessage`** 与 **mobile 的 `resolveErrorCore`**，断言二者输出的 `text` 完全一致（仅使用两端 map 文本相同的 code：`40900`/`40901`/未知 `49999`）→ `PARITY=1`。
5. 测试：`frontend-pc/tests/errorMessages.test.mjs`（`node --test`，PC 为 `type:module`）、`frontend-mobile/src/services/errorMessageResolution.test.mjs`（`node --test`，纯 `.mjs` 无 i18n/Taro 依赖）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 PC 解析器四层顺序 | `frontend-pc/src/services/errorMessages.js` | `node --test frontend-pc/tests/errorMessages.test.mjs` | 8 条 fixture（见 G0） | PC | `40900`+中文 → 中文原文（`LAYER=human_message`）；`40900`+`site name conflict` → 通用「数据冲突…」（`LAYER=code_map`）；`40912`+中文 → 透传；`order not found` → L1「未找到订单」；`null` → fallback | R1 |
| R2 mobile 纯核心等价行为 | `frontend-mobile/src/services/errorMessageResolution.mjs` | `node --test frontend-mobile/src/services/errorMessageResolution.test.mjs` | 同上 8 条 fixture | 小程序/H5 | 与 R1 同层的 5 类判定全部一致；`LAYER` 序列一致 | R2 |
| R3 双端平价 | `scripts/check-error-message-parity-2480.mjs`、`frontend-pc/src/services/errorMessages.js`、`frontend-mobile/src/services/errorMessageResolution.mjs` | `node scripts/check-error-message-parity-2480.mjs` | 同 fixture 表（10 条，仅 40900/40901/49999） | 双端 | `PARITY=1`、`CASES=10`、`DIFFS=0` | R3 |
| R4 mobile 出口接线不变 | `frontend-mobile/src/services/errorMessages.js`、`frontend-mobile/src/services/api.js` | 静态结构断言（无测试运行器的 UI 层） | 修后源码 | 小程序/H5 | `SINGLE_EXIT=1`（`resolveErrorMessage` 仍是唯一出口）、`RAW_MESSAGE_NO_T=1`（`human_message`/`raw_message` 不套 `t`）、`REEXPORT_KEPT=1`（api.js 三导出仍在） | R4 |
| R5 构建与 lint | `frontend-pc`、`frontend-mobile` | `cd frontend-pc && npm run build`；`npx eslint` 改动文件（两包） | 修后树 | PC + mobile | `PC_BUILD=0`、`LINT_EXIT=0`（0 `no-undef`） | R5 |
| R6 gate 四源一致 + 全量 PC 构建 | `docs/evidence/2480/pre-ready-2480.json`、`docs/evidence/2480/plan.md` | `/work` 以 `--full-gate-cmd "cd frontend-pc && npm run build"` 跑 pre-ready，随后 `plan-source.sh --issue 2480` | 修后树（干净） | PC | gate `result=PASS` 且 `FULL_SUITE_EXIT=0`（不得 N/A）；`PLAN_SOURCE: PASS` | R6 |

## Input Truth & Tool Preflight

- **Source of truth**：后端 `message` 是唯一可执行信息载体（`code` 仅为家族分类）。判定「可读」= 含 CJK（`\u3400-\u4dbf`/`\u4e00-\u9fff`）；**ASCII message 视为机器码**，继续走 `ERROR_MESSAGE_MAP` / `ERROR_CODE_MAP`（保障 #1675/#2080/#1793 等既有英文键行为不变）。
- **差异核对**：PC 有 L4 任意 message 透传（#2104），mobile **没有**；本单让 mobile 与 PC 对齐（`raw_message` 层），并让两端共享同一判定顺序。两端 map 文本对 `40900/40901` 一致（均为「数据冲突，请刷新后重试」），故平价断言只需这三类 code。
- **损坏输入拒绝**：`message` 为 `null`/`undefined`/空串/纯空白 → 视为无 message（走 code map 或 fallback）；`message` 为对象/数组 → `isHumanReadableMessage` 返回 `false`（不 `throw`，不渲染 `[object Object]`）。
- **工具盲区**：mobile `.js` 在无 `type:module` 时按 CJS 解析，且 `errorMessages.js` 依赖 `../i18n`（引 JSON）→ **不能**用 `node --test` 直接测 `.js`；因此把判定逻辑抽到 `.mjs`（沿用 #2325 的 `sessionToken.mjs` 惯例）。PC `type:module` 可直接 `import`。mobile 的 Taro H5 构建不在本单合同内（由发布流水线 `make release` 承担，见 D4）。
- **影响面预检**：`resolveErrorMessage` 在 PC 有 9 处调用、mobile 经 `services/api` 再导出遍布页面；本单只改**解析顺序**，不改签名（`(result, fallback)`）与返回值类型 → 调用点零改动。`4091x` 不在 map，行为不变（透传）。
- **回归风险**：若某 code 的中文 message 实际是技术诊断（如 `failed to load site: …` 为英文，不受影响），仍可能透出中文内部提示；由 D1 的「仅 CJK 透传」把范围限定在作者面向用户的中文文案，并在 R1/R2 负例中锁定 ASCII 不外溢。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 判定口径：**含 CJK 的 message 优先于 `ERROR_CODE_MAP`**（Issue 期望 2 的通用规则），ASCII message 仍走 map；不逐 code 白名单（避免 409 家族继续漏） | Architect | 本计划锁定 | 若 Owner 只想改 409 家族 → 回 Architect（改为 `code` 白名单，会漏其它业务冲突） |
| D2 | 不改 `ERROR_CODE_MAP` 内容（`40900/40901` 保留为兜底），避免影响「无 message」场景 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | mobile 与 PC 出口行为对齐（新增 `raw_message` 层），且后端 message 不套 `t()`；map 命中层仍套 `t()`（#2214 口径不变） | Architect | 本计划锁定 | 阻塞实现 |
| D4 | mobile 证据止于「纯核心单测 + lint + 静态接线」；Taro H5/小程序构建与真机文案走查由发布流水线与 `[Ops-Followup of #2480]` 承担（前端无组件测试运行器，Owner 豁免 `evidence_type: static`） | Architect | owner_approved（沿用本仓前端豁免惯例） | 若审计要求 render 证据 → 另立前端测试基建 Issue |
| D5 | 单一语义 owner：mobile 的判定逻辑落在 `errorMessageResolution.mjs`，`errorMessages.js` 只做 `t()` 包装；PC 复用同名函数与同一顺序（跨包无法共享模块，用 R3 平价脚本兜住漂移） | Architect | 本计划锁定 | 若要求抽公共包 → 回 Architect（需新增 workspace 包与构建接线） |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R4 | static | mobile 出口接线源码结构断言，Owner 豁免见 D4 |

## Out of Scope

- `ERROR_CODE_MAP` 全表重审（400xx/403xx/500xx 的通用文案是否应删除）。
- 后端返回码/文案本身（如 `#2477` 的 409 message 措辞）。
- 小程序真机/浏览器实际文案走查（部署侧 Ops Follow-up）。
- 把 `errorMessages` 抽成跨包共享 workspace 包（D5 备选）。
- 新增前端组件测试基建（D4 备选）。
- 全量 `npm test`（两包均无测试脚本；由 R5/R6 构建分担）。

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|---|---|---|---|
| R1 | 8 条见 Assertions（`code`/`message` 组合） | PC `resolveErrorMessage`（`frontend-pc/src/services/errorMessages.js`） | `docs/evidence/2480/R1-pc-resolver.txt` |
| R2 | 同 R1 的 8 条 | mobile `resolveErrorCore`（`errorMessageResolution.mjs`） | `docs/evidence/2480/R2-mobile-core.txt` |
| R3 | 10 条（`40900`/`40901`/`49999` × 中文/ASCII/空） | `node scripts/check-error-message-parity-2480.mjs` | `docs/evidence/2480/R3-parity.txt` |
| R4 | 修后 `errorMessages.js` + `api.js` | 静态结构断言 | `docs/evidence/2480/R4-wiring-static.txt` |
| R5 | 修后 `frontend-pc` / `frontend-mobile` | `npm run build` + `npx eslint` | `docs/evidence/2480/R5-build-lint.txt` |
| R6 | 修后干净树 + gate 报告 | 读校验（`plan-source.sh` + gate 解析） | `docs/evidence/2480/R6-gate.txt` |

### Assertions

- R1：`C40900_CN=网点名称已存在`、`L40900_CN=human_message`、`C40900_ASCII=数据冲突，请刷新后重试`、`L40900_ASCII=code_map`、`C40900_EMPTY=数据冲突，请刷新后重试`、`C40912_CN=学生证作为第二证件需介绍信`（透传）、`C_L1=未找到订单`、`C_NULL=操作失败，请重试`、`C49999_ASCII=unknown conflict`（`raw_message`）；负例：`C40900_CN=数据冲突，请刷新后重试`（仍被遮蔽）→ FAIL。
- R2：`L_CN=human_message`、`L_ASCII=code_map`、`L_EMPTY=code_map`、`L_40912=human_message`、`L_NULL=fallback`、`LAYERS=`（8 条 `layer` 序列与 R1 完全一致）；负例：mobile 任一 `layer` 与 PC 不同 → FAIL。
- R3：`PARITY=1`、`CASES=10`、`DIFFS=0`；负例：任一 case 文本不同 → FAIL。
- R4：`SINGLE_EXIT=1`（`resolveErrorMessage` 仅定义于 `errorMessages.js` 并经 `api.js` 再导出）、`RAW_MESSAGE_NO_T=1`（`human_message`/`raw_message` 分支直接 `return core.text`，无 `t(`）、`REEXPORT_KEPT=1`；负例：`t(core.text)` 无条件套用或 `api.js` 丢失 `resolveErrorMessage` 导出 → FAIL。
- R5：`PC_BUILD=0`、`LINT_EXIT=0`；负例：任一非 0 → FAIL。
- R6：`GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS`；负例：`P5=NA` 或四源 sha 不一致 → FAIL。
- 通用 provenance：每个 artifact 首行 `# build=1.0.<fix> # commit=<sha> # dirty=false`。
- 负例（跨行）：任何 artifact 出现 `[object Object]` 或被解析的 message 为 `undefined` 字面量 → FAIL。

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2480/R1-pc-resolver.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R2-mobile-core.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R3-parity.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R4-wiring-static.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R5-build-lint.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2480/R6-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| symptom | trigger | root cause | test blind spot | same-class paths | fix layer | red-green | prevention |
|---|---|---|---|---|---|---|---|
| 网点重名只显示「数据冲突，请刷新后重试」 | 后端 409 + 具体中文 message | code map 在 message 透传之前 | 无「code+中文 message」用例 | 部门重名、邀请重复、定损重复 | 前端解析器 | R1 红绿（改前 L=code_map，改后 L=human_message） | D1 判定顺序 |
| mobile 端同类冲突也只显示通用文案 | 同上 | mobile 无 message 透传层 | 无 mobile 解析用例 | 全部 mobile 业务冲突提示 | 前端解析器 | R2 红绿 | 抽出纯核心 + 平价脚本 |
| 双端文案不一致（PC 具体 / mobile 通用） | 同一后端错误 | 两套实现各自演进 | 无跨端平价用例 | PC/mobile 所有错误提示 | 前端解析器 | R3 红绿 | D5 单一 owner + R3 平价 |
| ASCII 英文 message 被直接抛给用户 | 后端英文技术串 | 若不做 CJK 判定则全透传 | 无 ASCII 负例 | 500xx 内部错误 | 前端解析器 | R1 `C49999_ASCII` 负例 | 仅 CJK 透传 |
| 4091x 历史行为被翻转 | IAM 字段冲突 | 有人把 4091x 加进 map | 无回归用例 | 40912/40913 | 前端解析器 | R1 `C40912_CN` | D2 不改 map |
| `message` 为对象时渲染 `[object Object]` | 非标准响应体 | 未做类型守卫 | 无畸形输入用例 | 全部错误提示 | 前端解析器 | R1/R2 类型用例 | `typeof === 'string'` |
| mobile 后端文案被 `t()` 二次查找 | 出口统一经 `t()` | 包装层对所有分支套 `t` | 无「message 不套 t」用例 | i18n 迁移后的新代码 | 前端包装层 | R4 `RAW_MESSAGE_NO_T=1` | 分支级区分 |
| 调用点绕过解析器自拼文案 | 新页面直取 `res.message` | 无唯一出口约束 | 无静态接线断言 | 新增页面 | 前端约定 | R4 `SINGLE_EXIT=1` | 唯一出口 + 审计清单 |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2480.sh r1` | `docs/evidence/2480/R1-pc-resolver.txt` | exit 0；grep `L40900_CN=human_message`、`L40900_ASCII=code_map`、`C40912_CN=` | PASS | 中文被通用文案遮蔽 → FAIL | dev |
| R2 | `bash scripts/verify-2480.sh r2` | `docs/evidence/2480/R2-mobile-core.txt` | exit 0；grep `L_CN=human_message`、`L_ASCII=code_map`、`LAYERS=` | PASS | layer 序列与 PC 不同 → FAIL | dev |
| R3 | `bash scripts/verify-2480.sh r3` | `docs/evidence/2480/R3-parity.txt` | exit 0；grep `PARITY=1`、`CASES=10`、`DIFFS=0` | PASS | 任一 case 文本不同 → FAIL | dev |
| R4 | `bash scripts/verify-2480.sh r4` | `docs/evidence/2480/R4-wiring-static.txt` | exit 0；grep `SINGLE_EXIT=1`、`RAW_MESSAGE_NO_T=1`、`REEXPORT_KEPT=1` | PASS | 无条件套 `t` → FAIL | dev |
| R5 | `bash scripts/verify-2480.sh r5` | `docs/evidence/2480/R5-build-lint.txt` | exit 0；grep `PC_BUILD=0`、`LINT_EXIT=0` | PASS | 任一非 0 → FAIL | dev |
| R6 | `bash scripts/verify-2480.sh r6` | `docs/evidence/2480/R6-gate.txt` | exit 0；grep `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | PASS | `P5=NA` → FAIL | dev |

### Workload

- Size: M
- Basis: 2 个前端解析器改造 + 1 个新纯核心 `.mjs` + 3 个测试/平价脚本 + 1 个 verify 脚本 + 6 条证据行；跨 PC/mobile 两包，无后端、无 migration
- Confidence: high

### Files（变更文件清单）

- `frontend-pc/src/services/errorMessages.js`
- `frontend-pc/tests/errorMessages.test.mjs`
- `frontend-mobile/src/services/errorMessageResolution.mjs`
- `frontend-mobile/src/services/errorMessages.js`
- `frontend-mobile/src/services/errorMessageResolution.test.mjs`
- `frontend-mobile/src/services/api.js`
- `scripts/check-error-message-parity-2480.mjs`
- `scripts/verify-2480.sh`
- `docs/evidence/2480/plan.md`
- `docs/evidence/2480/pre-ready-2480.json`
- `docs/evidence/2480/R1-pc-resolver.txt`
- `docs/evidence/2480/R2-mobile-core.txt`
- `docs/evidence/2480/R3-parity.txt`
- `docs/evidence/2480/R4-wiring-static.txt`
- `docs/evidence/2480/R5-build-lint.txt`
- `docs/evidence/2480/R6-gate.txt`
- `docs/evidence/2480/evidence.json`

## Dependencies

- 无

### 依赖与关联说明（非硬依赖）

- **#2477（accepted）**：其 CreateSite 409 预检会带具体中文 message（如含既有网点名）；本单保证该 message 能到达用户（两单互补，互不阻塞）。
- **#2455（accepted）**：其 R3「并发 23505 → 409」同样受益于本单（冲突原因可见）。
- **#2100 / #2104 / #2290**：同一处解析器的历史教训（英文键遮蔽、未知 code 遮蔽、4091x 排除）；本单把规则从「逐例放行」升级为「CJK 优先」。
- **#1675**：`ERROR_MESSAGE_MAP` 的英文键约定不变。
- **部署侧**：预生产「网点重名 → 显示具体文案」与小程序真机文案由 `[Ops-Followup of #2480]`（`waitForConfirm`）承载。

*Model: DeepSeek V4.1 Flash 1017*

