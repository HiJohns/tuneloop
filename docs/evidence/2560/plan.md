## Accepted Plan — #2560（5xx 守卫收窄为「内部诊断形态」判定：可操作 5xx 文案恢复透传）

### 背景与裁定

#2497 引入的前端守卫是**粗粒度**规则：「`code >= 50000` 且 message 含 CJK → 忽略 `human_message`，一律走 `ERROR_CODE_MAP`」。它确实兜住了「中文前缀 + `err.Error()`」的内部诊断泄漏，但同时把后端**面向用户、可操作**的 5xx 固定文案一并吞掉，例如：

- `切换网点失败且 IAM 回滚失败，请联系管理员人工核对 IAM 组织关系`（`backend/handlers/user_staff.go` 4 处，develop 实测存在）；
- `SMTP 配置校验失败，请检查端口与凭据后重试`（#2497 后端收敛后的固定文案）。

**Architect 裁定（D1）：采用「形态判定」（option 1），否决选项 2 / 3**

| 选项 | 结论 | 理由 |
|---|---|---|
| 1 形态判定（仅屏蔽内部诊断形态） | **采纳** | 改动收敛在解析层（2 个实现 + 测试），既恢复可操作文案，又保留 #2497 的负例防线；内部诊断的**主保证**仍是 #2497 的后端静态扫描（`CJK_ERR_CONCAT=0`），前端守卫只是二次网 |
| 2 后端白名单（`user_visible` 标记） | 否决 | develop 实测 `code:5xxxx + CJK message` 共 **65 处 / 20 文件**，逐一标注 + 契约 + 文档扩面到约 10+ 文件；且新文案漏标时**静默降级**为通用文案（不可见的 UX 退化），review 成本高于本单目标 |
| 3 维持现状 + 只写文档 | 否决 | #2497 的后端主修已把 5xx 文案改为固定用户文案，此时仍吞掉「请联系管理员人工核对 IAM 组织关系」这类**处置指引**，是可感知的 UX 回退；与 Issue 正例验收直接冲突 |

### 变更点

1. **PC 解析器**：`frontend-pc/src/services/errorMessages.js` 把 `resolveErrorCore` 中的粗粒度 5xx 分支替换为「5xx + CJK + `isInternalDiagnosticMessage(message)`」判定：命中形态 → `code_map`/`fallback`（保持 #2497 行为）；未命中 → `human_message` 原样透传。导出 `isInternalDiagnosticMessage` 供测试与平价脚本断言。
2. **mobile 纯核心**：`frontend-mobile/src/services/errorMessageResolution.mjs` 同口径同实现（同名函数、同 marker 表、同正则），保持跨包逐字一致。
3. **判定口径（唯一 owner 为 D2 表，双端逐字实现）**：
   - `INTERNAL_DIAGNOSTIC_MARKERS`（小写化后 substring 匹配，20 项）：`pq:`、`sqlstate`、`rpc error`、`grpc:`、`dial tcp`、`connection refused`、`connection reset`、`no such host`、`i/o timeout`、`context deadline exceeded`、`context canceled`、`unexpected eof`、`invalid memory address`、`nil pointer`、`panic:`、`runtime error`、`unsupported protocol scheme`、`x509:`、`cannot unmarshal`、`record not found`；
   - `DIAGNOSTIC_TAIL_RE`：`/[：:]\s*[A-Za-z][A-Za-z0-9_.:@\/'"()\[\]{}<>,;=+\-* ]{2,}$/`（「中文前缀 + 冒号 + ASCII 尾串」= 拼接诊断的特征形态）；
   - 判定仅对 `Number(code) >= 50000` 生效；4xx/业务码路径与 #2480 行为**完全不变**。
4. **测试**：
   - `frontend-pc/tests/errorMessages-5xx-2497.test.mjs`（#2497 引入，本单扩写）：新增正例 2 条（IAM 回滚文案、SMTP 配置文案 → `human_message` 原文），保留并显式覆盖负例 3 条（`pq:` / `rpc error: connection refused` / `dial tcp … i/o timeout` → 通用文案）；头部注释由「粗守卫」改为「形态判定」。
   - `frontend-mobile/src/services/errorMessageResolution.test.mjs`：新增同 5 条 fixture 的 mobile 侧断言（正例 `human_message`、负例 `code_map`/`fallback`）。
   - `scripts/check-error-message-parity-2480.mjs`：`mobileCodeMap` 抽取补 `50000`，`CASES` 增 5 条 5xx fixture（2 正 3 负），断言 `DIFFS=0` / `LAYER_DIFFS=0` / `PARITY=1`。
5. **生产接线证据（不改实现文件）**：断言 PC `frontend-pc/src/services/api.js` 与 mobile `frontend-mobile/src/services/api.js` 的非 2xx 分支仍把 `{code, message}` 交给 `resolveErrorMessage`（#2497 已接线；本单只做**只读断言**，防止将来有人绕过解析层让形态判定失效）。
6. **Core Update（先文档后代码）**：`docs/spec/api/api.md` 第 30 行 L2 口径窄化 + 5xx 契约三句（见 Issue body `## Core Update`）；必须先更新文档，再改解析器。
7. **证据生成器**：新增 `scripts/verify-2560.sh`（r1–r7），每行写入带 provenance 首行的 `docs/evidence/2560/R*.txt`；重命令（eslint / `npm run build` / `npm run build:h5`）经 `heavy-run.sh` 持 host 锁执行。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| PC 解析器：可操作 5xx 文案透传 + 内部诊断仍屏蔽（R1） | `frontend-pc/src/services/errorMessages.js` | `resolveErrorCore` / `resolveErrorMessage` | `{code:50000,message:'切换网点失败且 IAM 回滚失败，请联系管理员人工核对 IAM 组织关系'}`、`{code:50000,message:'SMTP 配置校验失败，请检查端口与凭据后重试'}`、`{code:50000,message:'标记删除失败：pq: duplicate key value violates unique constraint'}`、`{code:50001,message:'级联解除商户成员失败：rpc error: connection refused'}`、`{code:50999,message:'内部诊断：dial tcp 10.0.0.1: i/o timeout'}` | PC | 正例 layer=`human_message` 且 text==原文；负例 layer∈{code_map,fallback} 且不含内部特征串 | R1 |
| mobile 纯核心同口径 + 双端平价（R2） | `frontend-mobile/src/services/errorMessageResolution.mjs` / `scripts/check-error-message-parity-2480.mjs` | `resolveErrorCore` / parity 脚本 | 与 R1 同 5 条 fixture + 既有 12 条 parity fixture | H5 + weapp | 两端 text 与 layer 逐条相等；`PARITY=1`、`DIFFS=0`、`LAYER_DIFFS=0` | R2 |
| 生产接线：请求层不绕过解析层（R3） | `frontend-pc/src/services/api.js` / `frontend-mobile/src/services/api.js` | 非 2xx 错误分支 | 静态接线断言（源码正则 + 解析后文案推演） | PC + H5 + weapp | 两端均把 `{code,message}` 交给 `resolveErrorMessage`，抛出的错误文本为解析结果 | R3 |
| 语义不变：错误码表与后端零改动（R4） | `frontend-pc/src/services/errorMessages.js` / `frontend-mobile/src/services/errorMessages.js` | `ERROR_CODE_MAP` 块 + git diff | 与 `origin/develop` 对比 | PC + H5 + weapp + 后端 | `ERROR_CODE_MAP` 块逐字未变；`backend/` 零改动 | R4 |
| 构建与 lint（PC）（R5） | `frontend-pc/src/services/errorMessages.js` | `npm run eslint` + `npm run build` | 合并后 HEAD 树 | PC | 两条命令 exit 0 | R5 |
| 构建（mobile H5）（R6） | `frontend-mobile/src/services/errorMessageResolution.mjs` | `npm run build:h5` | 合并后 HEAD 树 | H5 | 构建 exit 0（无 module 解析/语法错误） | R6 |
| Core Update：api.md 层序口径同步（R7） | `docs/spec/api/api.md` | 文档第 30 行「用户可见文案」段落 | 静态断言 | 文档 | 含 5xx 内部诊断排除与固定文案约束；旧「含 CJK 一律透传」表述已消失 | R7 |

## Input Truth & Tool Preflight

- **Source of truth（唯一口径）**：
  1. `origin/issue-2497`（未合并）引入的守卫实现 —— `/home/coder/tuneloop` 实测 `git show origin/issue-2497:frontend-pc/src/services/errorMessages.js` 中 `isServerError` 分支；
  2. `origin/develop`（`43b17b764`）后端 5xx 文案现状 —— `git grep -P '"code":\s*5\d{4},?\s*"message":\s*"[^"]*[\x{4e00}-\x{9fff}]'` = **65 处 / 20 文件**，其中 47 处为固定用户文案、18 处为拼接诊断；
  3. `docs/spec/api/api.md` 第 30 行（层序契约的唯一文档 owner）。
- **差异核对（/work 开工前必做）**：`git fetch origin` 后重跑上述 grep 并比对 `origin/develop`；若 `#2497` 的 v4 计划改动守卫形态（例如已改为形态判定、或改为后端白名单），**不得**按本计划硬套，转 `## Investigation Packet` 给 Architect（D4）。
- **损坏输入拒绝规则**：`message` 非字符串 / 空串 / 纯空白 / 纯 ASCII / `code` 非数字 / 4xx 业务码 —— 一律不进入形态判定，保持现有层级行为。
- **工具盲区（显式声明）**：形态判定是**启发式**，无法枚举未来所有诊断形态（如 `sql: no rows`、`json: cannot unmarshal …` 的中文混排变体）。因此：
  - 主保证 = #2497 的后端静态扫描（`CJK_ERR_CONCAT=0`，禁止 `"<中文>" + err.Error()` 拼接）；
  - 前端形态判定 = 二次网，只覆盖已登记 marker 与「冒号 + ASCII 尾串」形态；
  - 新增诊断形态需同步更新 D2 表 + 双向 fixture，否则视为计划缺口。
- **重命令前置**：`npm run build` / `npm run build:h5` / eslint 必须经 `$HOME/.config/opencode/scripts/heavy-run.sh`（host 锁，单条命令粒度）。

## Decision Lock

| 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|
| D1 收敛方向 = 形态判定（option 1） | Architect | 已锁定（见「背景与裁定」对比表；option 2/3 否决理由已记录） | Owner 若改判 option 3，须回到 `status:analysis` 重写计划，本计划作废 |
| D2 判定口径 = 20 项 marker 表 + `DIAGNOSTIC_TAIL_RE`（变更点 3） | Architect | 已锁定；双端逐字实现，任何增删须回 Architect 更新 D2 | 未锁定不得实现；`/work` 不得自行增删 marker |
| D3 #2497 负例不回退 | Architect | 已锁定：#2497 的 3 条内部诊断负例必须仍为通用文案（R1/R2 断言 `LEAK=0`） | 负例回退 → 直接 FAIL，不得以「正例优先」妥协 |
| D4 前置基线 = #2497 合入 `origin/develop` | Architect | `## Dependencies` 未满足 → `/work` HOLD；合入后若守卫形态已等价变更 → HOLD + Investigation Packet | 阻塞实现，禁止在 `origin/issue-2497` 分支上叠加或自行改计划 |
| D5 Core Update 先于代码 | Architect | `docs/spec/api/api.md` 必须先改（R7 证据在 R1/R2 之前可独立成立） | 未改文档不得提交解析器改动 |
| D6 5xx 语义边界 = 「可操作固定文案透传，诊断形态屏蔽」 | Architect | 已锁定；不新增后端 `user_visible` 字段（option 2 已否决） | 若后续需要精确白名单，另立 Issue（不得在本单扩面） |

## Out of Scope

- **不改后端**：`backend/handlers/*` 的 5xx 文案与错误码由 #2497 负责；本单 R4 只断言「零改动」。
- **不引入后端 `user_visible` 白名单**（option 2，见 D1/D6）；如需精确标记，另立 Issue。
- **不做 weapp 打包**：本单无 page/组件/样式改动，mobile 改动仅在 `services/` 纯核心；编译证据以 `npm run build:h5` 覆盖（R6），weapp 打包留给父/批次 gate。
- **不改错误码语义、不改 `ERROR_MAP` 文案、不改 toast/渲染层**（R4 断言零变化）。
- **不修改 #2497 的验收工具与证据**：`scripts/verify-2497.sh`、`docs/evidence/2497/*` 只读引用，不在本单改动（acceptance-rule integrity）。
- **不做 4xx 路径的形态判定**（#2497/#2480 行为不变）。
- **形态判定的完全确定性**不在本单承诺（见 Input Truth 工具盲区）；需要完全确定性时另立「后端标记」Issue。

## Bug Reflection Matrix

| 项 | 内容 |
|---|---|
| symptom | 后端返回可操作的 5xx 中文文案（如「切换网点失败且 IAM 回滚失败，请联系管理员人工核对 IAM 组织关系」「SMTP 配置校验失败，请检查端口与凭据后重试」）时，PC/H5/weapp 均展示「服务器开小差了，请稍后再试」，用户失去处置指引 |
| trigger | `code >= 50000` 且 `message` 含 CJK —— #2497 的粗粒度守卫无条件走 `ERROR_CODE_MAP` |
| root cause | 守卫用「HTTP 状态族（5xx）」近似「内部诊断」语义，而 5xx 里同时存在固定用户文案与拼接诊断；粗细度错配导致把用户文案当诊断屏蔽（修复层：解析层按**形态**而非**状态族**判定） |
| test blind spot | #2497 的测试只断言 5xx 负例（内部串不泄漏），没有「可操作 5xx 必须透传」的正例，因此粗守卫的过度屏蔽在测试全绿下被固化（#2497 Round 2 审计只能以 Not blocking 记录） |
| same-class paths | ① 4xx 分支若将来也加粗粒度守卫；② mobile `api.js` 非 2xx 分支与 PC `api.js` 错误分支（绕过解析层即失效）；③ parity 脚本若只覆盖 4xx fixture，双端漂移不会被发现；④ `ERROR_CODE_MAP` 增删码时的层序假设 |
| fix layer | 解析层唯一出口（`resolveErrorCore`）双端同步；不做 per-page/pin 特判（避免 presentation 复制） |
| red-green | 红：在 #2497 守卫上跑新正例 → 输出「服务器开小差了，请稍后再试」（R1 `PC_POS=0`）；绿：形态判定落地后同 fixture → 原文透传（`PC_POS=2`）。负例同时证明回退可检出（`LEAK>0`） |
| prevention | ① 正负例成对纳入 `node --test` + parity 脚本（双端）；② #2497 后端静态扫描作为主保证（拼接归零）；③ `api.md` 写明 5xx 文案必须是固定字面量，禁止拼接；④ 新增 marker 必须同步 D2 表与 fixture |
| scope discipline | 不在本单扩到后端标注/错误码分类（option 2），避免「防御纵深」变成 20 文件大改与静默降级风险 |

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|---|---|---|---|
| R1 PC 解析器正负例 | 5 条固定 payload（见 Verification Matrix 触发状态列） | `resolveErrorCore`（`api.js` 错误分支唯一出口） | `docs/evidence/2560/R1-pc-parser.txt` |
| R2 mobile + 平价 | 同 5 条 + 既有 12 条 parity fixture | `resolveErrorCore`（mobile 纯核心）/ `scripts/check-error-message-parity-2480.mjs` | `docs/evidence/2560/R2-mobile-parity.txt` |
| R3 生产接线 | 两包 `services/api.js` 源码 | 非 2xx 请求分支 | `docs/evidence/2560/R3-chokepoint.txt` |
| R4 语义不变 | `origin/develop` 的 `ERROR_CODE_MAP` 块 + `backend/` 树 | git diff / 静态抽取 | `docs/evidence/2560/R4-semantics.txt` |
| R5 PC 构建与 lint | 合并后 HEAD 树 | `npm run eslint` + `npm run build` | `docs/evidence/2560/R5-pc-build.txt` |
| R6 mobile H5 构建 | 合并后 HEAD 树 | `npm run build:h5` | `docs/evidence/2560/R6-mobile-build.txt` |
| R7 Core Update | `docs/spec/api/api.md` | 文档第 30 行段落 | `docs/evidence/2560/R7-api-doc.txt` |

### Assertions

- R1：`PC_POS=2`（正例 text 逐字等于原文、layer=`human_message`）；`PC_NEG=3`（负例 layer ∈ {code_map, fallback}）；`LEAK=0`（输出不含 `pq:` / `rpc error` / `i/o timeout`）。
- R2：`MOBILE_POS=2` / `MOBILE_NEG=3` / `PARITY=1` / `DIFFS=0` / `LAYER_DIFFS=0`；parity 输出必须出现 5 条 `CASE=5xx_*` 行。
- R3：`PC_CHOKEPOINT=1` / `MOBILE_CHOKEPOINT=1`（两包 `api.js` 均把 `{code, message}` 交给解析层）；`RAW_THROW=0`（不得原样抛后端 message）。
- R4：`BACKEND_DIFF=0`（`git diff --name-only origin/develop...HEAD -- backend/` 为空）；`CODE_MAP_BLOCK_UNCHANGED=1`（PC 与 mobile 的 `ERROR_CODE_MAP` 块与 `origin/develop` 逐字一致）。
- R5：`PC_ESLINT_OK=1` 且 `PC_BUILD_OK=1`，命令 exit 0。
- R6：`MOBILE_H5_BUILD_OK=1`，命令 exit 0。
- R7：`API_DOC_5XX_RULE=1`（api.md 含 5xx 内部诊断排除 + 固定文案约束）；`API_DOC_STALE_RULE=0`（不再出现旧的不设限表述）。
- provenance：每条 artifact 首行 `# build=1.0.<short> # commit=<40 位> # dirty=false`，commit = 本单最新 fix commit。
- negative：R1/R2 负例回显内部特征串、R3 任一端绕过解析层、R4 后端或码表被改、R5/R6 构建非 0、R7 文档未同步 —— 任一出现即 FAIL。

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2560.sh r1` | docs/evidence/2560/R1-pc-parser.txt | exit 0 且含 `PC_POS=2` `PC_NEG=3` `LEAK=0` | PASS | 正例被吞或负例回显 `pq:` `rpc error` `i/o timeout` 之一 → FAIL | dev |
| R2 | `bash scripts/verify-2560.sh r2` | docs/evidence/2560/R2-mobile-parity.txt | exit 0 且含 `MOBILE_POS=2` `MOBILE_NEG=3` `PARITY=1` `DIFFS=0` `LAYER_DIFFS=0` | PASS | 任一端文案或 layer 不一致、或负例回显内部串 → FAIL | dev |
| R3 | `bash scripts/verify-2560.sh r3` | docs/evidence/2560/R3-chokepoint.txt | exit 0 且含 `PC_CHOKEPOINT=1` `MOBILE_CHOKEPOINT=1` `RAW_THROW=0` | PASS | 任一端请求层原样抛后端 message → FAIL | dev |
| R4 | `bash scripts/verify-2560.sh r4` | docs/evidence/2560/R4-semantics.txt | exit 0 且含 `BACKEND_DIFF=0` `CODE_MAP_BLOCK_UNCHANGED=1` | PASS | `backend/` 有改动或 `ERROR_CODE_MAP` 块变化 → FAIL | dev |
| R5 | `bash scripts/verify-2560.sh r5` | docs/evidence/2560/R5-pc-build.txt | exit 0 且含 `PC_ESLINT_OK=1` `PC_BUILD_OK=1` | PASS | eslint 报错或 `npm run build` 非 0 → FAIL | dev |
| R6 | `bash scripts/verify-2560.sh r6` | docs/evidence/2560/R6-mobile-build.txt | exit 0 且含 `MOBILE_H5_BUILD_OK=1` | PASS | `npm run build:h5` 非 0 → FAIL | dev |
| R7 | `bash scripts/verify-2560.sh r7` | docs/evidence/2560/R7-api-doc.txt | exit 0 且含 `API_DOC_5XX_RULE=1` `API_DOC_STALE_RULE=0` | PASS | api.md 未同步或仍写旧口径 → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|----------|------------|
| docs/evidence/2560/R1-pc-parser.txt | `# build=1.0.<fix> # commit=<fix> # dirty=false` |
| docs/evidence/2560/R2-mobile-parity.txt | `# build=1.0.<fix> # commit=<fix> # dirty=false` |
| docs/evidence/2560/R3-chokepoint.txt | `# build=1.0.<fix> # commit=<fix> # dirty=false` |
| docs/evidence/2560/R4-semantics.txt | `# build=1.0.<fix> # commit=<fix> # dirty=false` |
| docs/evidence/2560/R5-pc-build.txt | `# build=1.0.<fix> # commit=<fix> # dirty=false` |
| docs/evidence/2560/R6-mobile-build.txt | `# build=1.0.<fix> # commit=<fix> # dirty=false` |
| docs/evidence/2560/R7-api-doc.txt | `# build=1.0.<fix> # commit=<fix> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

- provenance 由 `scripts/verify-2560.sh` 统一写入：`FIX_SHA` 取 `git log --grep="^fix(issue-2560)" -1`（缺省回退 HEAD），`SHORT=git rev-parse --short=8`，`DIRTY` 排除 `docs/evidence/2560` 与 `tmp`。
- 证据必须在 fix commit 之后重新生成；禁止只改 `evidence.json` 的 build/commit 冒充新证据。

## Dependencies

- #2497

### 依赖说明（非声明）

- `#2497`（`status:analysis`）引入本单要收窄的守卫与配套测试文件（`frontend-pc/tests/errorMessages-5xx-2497.test.mjs`、`frontend-mobile/src/services/api.js` 接线）。它**未合入** `origin/develop`（实测：`git merge-base --is-ancestor origin/issue-2497 origin/develop` = 否），本单 `### Files` 中的守卫行、测试文件与接线在本单基线上尚不存在。
- 因此 `/work` 必须在 `#2497` 合入 `origin/develop` 后开工（`work_deps` dependency unsatisfied → HOLD）；合入后先按 Input Truth 复查守卫形态，若已被等价收窄则 HOLD + `## Investigation Packet`，由 Architect 决定关闭或重规划。
- 关联：#2495（#2480 承接项）、#2480（层序 owner）。

### Workload

- Size: L
- Basis: 7 个变更文件（2 解析器 + 2 测试 + 1 parity 脚本 + 1 证据脚本 + 1 核心文档）+ 7 条证据行 + 1 个前置 Issue 依赖；无 migration、无 API/DB 契约变更（仅文档口径）
- Confidence: medium（形态判定为启发式，且前置 #2497 的 v4 可能改变守卫形态，需在开工时复查）

### Files（变更文件清单）

- `docs/spec/api/api.md`
- `frontend-pc/src/services/errorMessages.js`
- `frontend-mobile/src/services/errorMessageResolution.mjs`
- `frontend-pc/tests/errorMessages-5xx-2497.test.mjs`
- `frontend-mobile/src/services/errorMessageResolution.test.mjs`
- `scripts/check-error-message-parity-2480.mjs`
- `scripts/verify-2560.sh`
- `frontend-pc/src/services/api.js`
- `frontend-mobile/src/services/api.js`
- `frontend-mobile/src/services/errorMessages.js`
- `docs/evidence/2560/R1-pc-parser.txt`
- `docs/evidence/2560/R2-mobile-parity.txt`
- `docs/evidence/2560/R3-chokepoint.txt`
- `docs/evidence/2560/R4-semantics.txt`
- `docs/evidence/2560/R5-pc-build.txt`
- `docs/evidence/2560/R6-mobile-build.txt`
- `docs/evidence/2560/R7-api-doc.txt`
- `docs/evidence/2560/evidence.json`
- `docs/evidence/2560/pre-ready-2560.json`
- `docs/evidence/2560/plan.md`

> `frontend-pc/src/services/api.js` / `frontend-mobile/src/services/api.js` / `frontend-mobile/src/services/errorMessages.js` 为**只读断言目标**（R3 证明接线未被绕过、R4 证明 `ERROR_CODE_MAP` 未变），本单不修改其内容。

*Model: DeepSeek V4.1 Flash 1017*

