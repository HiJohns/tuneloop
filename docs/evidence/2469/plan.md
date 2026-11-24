## Accepted Plan — #2469（IAM 孤儿面板：空态中性化 + 告警条件化 + 扫描计数可见）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| warning `Alert` **无条件渲染**（不看 `orphans.length`），空列表也常驻红条 | `frontend-pc/src/pages/System/UserManagement.jsx:640-646` |
| 空态仅靠 `Table` 的 `locale.emptyText`（12px 灰字 + 8 列表头 + 禁用全选框仍渲染） | 同上 `:648-660` |
| 后端已返回 `data.scanned` / `data.min_age_minute`，前端**只取 `data.list`**，其余字段丢弃 | `:176-188`（`fetchOrphans`） |
| 加载中由 `Table.loading` 承载；空态与加载态未互斥（会先闪空态） | `:652`、Owner DOM 回执 |
| 清理语义：二次确认（`confirmPurgeSelected`）、批量按钮计数与禁用（`orphanSelected.length`）、`purge` 不可撤销 —— 本次**不改变** | `:633-638`、`:305-340` |

**结论**：纯前端展示层问题（无后端改动）；噪音来源是「无条件红条 + 空表格骨架」。

### 变更点

1. 新增纯函数模块 `frontend-pc/src/utils/orphanPanel.js`（便于 `node --test` 直接覆盖，frontend-pc 无测试运行器）：
   - `shouldShowWarning(list)` → `list.length > 0`
   - `orphanTitle(list, scanned)` → 空：`'IAM 孤儿账号'`；非空：`'扫描 {scanned} 个，命中 {list.length} 个'`
   - `orphanEmptyText(scanned, minAgeMinute)` → `'本次扫描 {scanned} 个 pending 账号；孤儿判定：无本地记录 + 无组织 + 挂起 ≥ {minAgeMinute} 分钟'`
   - `isEmptyState({ loading, error, list })` → 仅 `!loading && !error && list.length === 0` 为真（加载中/失败**不**显示空态，避免误报「未发现」）
2. `frontend-pc/src/pages/System/UserManagement.jsx`：
   - `fetchOrphans` 保存 `scanned` / `min_age_minute`（新增 `orphanScanned` / `orphanMinAge` 状态），并新增 `orphanError`（失败时置位，成功后清除）；失败仍 `message.error`（保留）。
   - Modal `title` 使用 `orphanTitle(...)`。
   - 渲染分支：
     a. `orphanLoading` → `<Spin tip="正在扫描 IAM 账号…" />`（不渲染表格/空态）；
     b. `orphanError && orphans.length === 0` → 错误占位（`Alert type="error"` 或 `Empty` + 「加载失败，请点击刷新重试」），**不得**显示「未发现孤儿账号」；
     c. `isEmptyState(...)` → 居中 `Empty`：图标 + 「未发现孤儿账号」+ `orphanEmptyText(...)` + 「刷新」按钮（footer 刷新保留）；
     d. 有数据 → 精简后的 `Alert type="warning"`（一行「清理会物理删除 IAM 账号，不可撤销」）+ 判定条件收进 `Tooltip`（挂在标题旁 `QuestionCircleOutlined`）+ 正常 `Table`。
   - 判定条件文案（四条）只在 Tooltip/折叠中展示，不再以红条正文表达。
   - **不改变**：批量清理按钮计数/禁用、二次确认弹窗、`purge` 不可撤销语义、刷新与关闭按钮行为。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 空态判定纯函数 | `frontend-pc/src/utils/orphanPanel.js` | `node --test` | 四态：空列表 / 1 条 / loading / error | JS | `shouldShowWarning=false/true`；`isEmptyState` 仅空且非加载非错误为 true；`orphanEmptyText` 含 scanned 与 minAge；`orphanTitle` 空态为固定标题、非空含「命中 1 个」 | R1 |
| R2 条件渲染与空态结构 | `frontend-pc/src/pages/System/UserManagement.jsx` | PC 账户管理 → IAM 孤儿账号（空列表） | 空列表、非加载 | PC | 源码结构断言：`Alert` 仅在 `shouldShowWarning` 为真时渲染；空态分支用 `Empty` 且**不含** `Table`/`rowSelection`；空态内含「刷新」 | R2 |
| R3 有数据分支 | 同上 | 同上（1 条孤儿） | 有数据 | PC | warning 精简为一行（含「不可撤销」）；Modal 标题含「扫描 N 个，命中 1 个」；`Table` 与批量按钮计数恢复正常 | R3 |
| R4 接住后端字段 | 同上 | `GET /admin/iam-orphans` 响应 | 返回 `scanned=12`、`min_age_minute=60` | PC | `fetchOrphans` 写入 `orphanScanned` / `orphanMinAge`；空态文案含 `12` 与 `60` | R4 |
| R5 加载/失败不误报 | 同上 | 同上 | loading 中 / 请求失败 | PC | loading 分支渲染 `Spin` 且不渲染空态；错误分支不出现「未发现孤儿账号」，`message.error` 保留 | R5 |
| R6 判定条件降噪 | 同上 | 同上 | 任意 | PC | 四条判定条件出现在 `Tooltip`（`QuestionCircleOutlined`）内；红条正文不含四条判定清单 | R6 |
| R7 清理语义回归 | 同上 | 批量清理 / 单条清理 | 有数据勾选 | PC | 二次确认弹窗仍存在（`confirmPurgeSelected`）；批量按钮文案含选中计数、0 选中时 `disabled` | R7 |
| R8 构建与 lint | `frontend-pc` | `npm run build`、`eslint` 改动文件 | 修后树 | PC | build exit 0；lint 0 `no-undef` | R8 |
| R9 gate 四源一致 + 全量 build | `docs/evidence/2469/pre-ready-2469.json`、`docs/evidence/2469/plan.md` | `/work` 以 `--full-gate-cmd` 跑一次 pre-ready（前端-only → `cd frontend-pc && npm run build`），随后 `plan-source.sh --issue 2469` | 修后树（干净） | PC | gate `result=PASS` 且含 `FULL_SUITE_EXIT=0`（不得为 N/A）；`PLAN_SOURCE: PASS` | R9 |

## Input Truth & Tool Preflight

- **Source of truth**：后端 `GET /admin/iam-orphans` 已返回 `{list, scanned, min_age_minute}`（`backend` 侧既有契约，本单不改）；前端当前丢弃 `scanned`/`min_age_minute`（`UserManagement.jsx:176-188`）。
- **差异核对**：Owner DOM 回执（Issue 评论）显示空列表仍渲染 8 列表头 + 禁用全选框 + 红条 → 与本单「空态不渲染 Table」一致；有数据时表头/勾选正常。
- **损坏输入拒绝**：`scanned`/`min_age_minute` 缺失或非数字 → 文案回退「扫描 — 个」/不展示挂起条件，不抛异常（纯函数内 `Number.isFinite` 守卫）。
- **工具盲区**：`frontend-pc` 无测试运行器（`package.json` 仅 dev/build/lint/preview），故 R1 用 `node --test` 覆盖纯函数，R2–R7 用源码结构断言（`evidence_type: static`，Owner 豁免 D3）；真实渲染/交互走部署侧人工走查（Out of Scope 明确）。
- **影响面预检**：`Alert`/`Table` 仅在本 Modal 内使用；`orphanColumns`、`orphanSelected`、`purgeOrphans` 不改；其它页面不引用该文件内部状态。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 空列表**不渲染 Table**（含表头与全选），改为居中 `Empty`（Owner DOM 回执要求） | Architect | 本计划锁定 | 阻塞实现 |
| D2 | 告警条件：仅 `orphans.length > 0` 渲染 warning；文案精简为「清理会物理删除 IAM 账号，不可撤销」 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | R2–R7 为纯前端结构断言（无测试运行器）→ `evidence_type: static` + **Owner 豁免**（Architect 授予） | Architect | 本计划锁定 | 若审计要求 render → 另立前端测试基建 Issue |
| D4 | 加载中 `Spin`；请求失败显示错误占位且**不得**出现「未发现孤儿账号」 | Architect | 本计划锁定 | 阻塞实现 |
| D5 | 清理语义（二次确认、批量禁用/计数、`purge` 不可撤销）保持不变 | Architect | 本计划锁定 | 阻塞实现 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R2 | static | 空态/条件渲染的源码结构断言，Owner 豁免见 D3 |
| R3 | static | 有数据分支的源码结构断言，Owner 豁免见 D3 |
| R4 | static | 字段接住的源码结构断言，Owner 豁免见 D3 |
| R5 | static | 加载/失败分支的源码结构断言，Owner 豁免见 D3 |
| R6 | static | 判定条件 Tooltip 化的源码结构断言，Owner 豁免见 D3 |
| R7 | static | 清理语义不变的源码结构断言，Owner 豁免见 D3 |

## Out of Scope

- 后端 `GET /admin/iam-orphans` 契约与扫描逻辑（含 `scanned`/`min_age_minute` 计算）。
- 「孤儿判定只能扫到有 active 组织关系的 pending 用户」的可见性缺口（Issue 已写明另立 Issue）。
- 真实浏览器渲染/交互走查与预生产截图（部署侧，人工验收）。
- 前端测试基建（`frontend-pc` 无测试运行器）的引入。
- 孤儿清理的运维 SOP（`user_task_queues` 核对）。
- 全量 `go test ./...`（本单无后端改动；gate 以前端 build 为准）。

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|---|---|---|---|
| R1 | ①`[]` ②`[{id:'a'}]` ③`{loading:true,list:[]}` ④`{error:'x',list:[]}` ⑤`scanned=12,minAge=60` | `frontend-pc/src/utils/orphanPanel.js`（node --test） | `docs/evidence/2469/R1-pure.txt` |
| R2 | 修后 `UserManagement.jsx`（空态分支代码段） | PC 账户管理 Modal（空列表） | `docs/evidence/2469/R2-empty-static.txt` |
| R3 | 同上（有数据分支代码段） | PC 同上（1 条） | `docs/evidence/2469/R3-data-static.txt` |
| R4 | 同上（`fetchOrphans` 代码段） | `GET /admin/iam-orphans` | `docs/evidence/2469/R4-fields-static.txt` |
| R5 | 同上（loading / error 分支代码段） | PC 同上 | `docs/evidence/2469/R5-loading-static.txt` |
| R6 | 同上（Tooltip 代码段） | PC 同上 | `docs/evidence/2469/R6-tooltip-static.txt` |
| R7 | 同上（确认/批量代码段） | PC 同上 | `docs/evidence/2469/R7-purge-static.txt` |
| R8 | 修后 `frontend-pc` | `npm run build` + eslint | `docs/evidence/2469/R8-build.txt` |
| R9 | 修后干净树 + gate 报告 | 读校验（`plan-source.sh`） | `docs/evidence/2469/R9-gate.txt` |

### Assertions

- R1：`node --test` exit 0；`SHOW_EMPTY=false`、`SHOW_ONE=true`、`ISEMPTY_LOADING=false`、`ISEMPTY_ERROR=false`、`EMPTY_TEXT_HAS_SCANNED=1`、`TITLE_HIT=命中 1 个`；负例：`isEmptyState({loading:true})` 为 true → FAIL。
- R2：命中 `shouldShowWarning` 条件渲染、`Empty` 组件、空态内「刷新」；且空态分支**不含** `rowSelection`；负例：空态仍渲染 `<Table` → FAIL。
- R3：命中 `orphanTitle` 使用、`不可撤销` 文案、`Table` 正常分支；负例：warning 仍含四条判定长文 → FAIL。
- R4：命中 `setOrphanScanned(`、`setOrphanMinAge(`、`min_age_minute`；负例：只取 `data.list` → FAIL。
- R5：命中 `Spin` 与 `orphanError` 分支；负例：空态文案在 error 分支出现 → FAIL。
- R6：命中 `Tooltip` + `QuestionCircleOutlined`；负例：判定四条仍以 Alert description 呈现 → FAIL。
- R7：命中 `confirmPurgeSelected` 与 `orphanSelected.length`；负例：确认弹窗被移除 → FAIL。
- R8：`PC_BUILD=0`、`LINT_EXIT=0`；负例：任一非 0 → FAIL。
- R9：`GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS`；负例：`P5=NA` 或 sha 不一致 → FAIL。
- 通用 provenance：每个 artifact 首行 `# build=1.0.<fix> # commit=<sha> # dirty=false`。
- 负例（跨行）：任何 artifact 出现 `空列表仍渲染 Table` 或 `未发现孤儿账号` 出现在 error 分支 → FAIL。

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2469/R1-pure.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2469/R2-empty-static.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2469/R3-data-static.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2469/R4-fields-static.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2469/R5-loading-static.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2469/R6-tooltip-static.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2469/R7-purge-static.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2469/R8-build.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2469/R9-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| symptom | trigger | root cause | test blind spot | same-class paths | fix layer | red-green | prevention |
|---|---|---|---|---|---|---|---|
| 空列表仍常驻红色告警 | 打开孤儿面板（0 条） | `Alert` 无条件渲染 | 无空态结构断言 | 其它「有风险」面板（如警告配置） | 前端 | R2 红绿 | D2 条件化 |
| 空表格骨架 + 禁用全选误导 | 同上 | `Table` 恒渲染，仅 `locale.emptyText` 兜底 | 无空态分支断言 | 其它列表空态（同类） | 前端 | R2 红绿 | D1 空态换 `Empty` |
| 扫描数不可见 | 同上 | `fetchOrphans` 只取 `data.list` | 无字段接住断言 | 其它「扫描/统计」接口 | 前端 | R4 红绿 | 接住 `scanned`/`min_age_minute` |
| 加载中先闪「未发现」 | 打开面板瞬间 | 空态与 loading 未互斥 | 无 loading 断言 | 全局列表 loading 态 | 前端 | R5 红绿 | `isEmptyState` 互斥 |
| 请求失败仍显示「未发现」 | 网络失败 | 失败未落状态 | 无失败断言 | 其它接口失败态 | 前端 | R5 红绿 | `orphanError` 分支 |
| 四条判定条件以红条长文呈现 | 打开面板 | 文案位置设计 | 无 Tooltip 断言 | 其它说明性文案 | 前端 | R6 红绿 | 判定条件收进 Tooltip |
| 空态标题与命中数缺失 | 有数据 | 标题固定 | 无标题断言 | 面板标题 | 前端 | R3 红绿 | `orphanTitle` |
| 改动误伤清理语义 | 重构 Modal | 结构改动大 | 无回归断言 | 批量操作面板 | 前端 | R7 红绿 | D5 不变式断言 |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2469.sh r1` | `docs/evidence/2469/R1-pure.txt` | exit 0；grep `SHOW_EMPTY=false`、`ISEMPTY_ERROR=false`、`EMPTY_TEXT_HAS_SCANNED=1` | PASS | `ISEMPTY_LOADING=true` → FAIL | dev |
| R2 | `bash scripts/verify-2469.sh r2` | `docs/evidence/2469/R2-empty-static.txt` | exit 0；grep `ALERT_CONDITIONAL=1`、`EMPTY_BLOCK=1`、`EMPTY_TABLE_ABSENT=1` | PASS | 空态仍含 `rowSelection` → FAIL | dev |
| R3 | `bash scripts/verify-2469.sh r3` | `docs/evidence/2469/R3-data-static.txt` | exit 0；grep `TITLE_USES_helper=1`、`WARNING_ONE_LINE=1` | PASS | warning 仍含四条长文 → FAIL | dev |
| R4 | `bash scripts/verify-2469.sh r4` | `docs/evidence/2469/R4-fields-static.txt` | exit 0；grep `SET_SCANNED=1`、`SET_MIN_AGE=1` | PASS | 仅取 `data.list` → FAIL | dev |
| R5 | `bash scripts/verify-2469.sh r5` | `docs/evidence/2469/R5-loading-static.txt` | exit 0；grep `SPIN_BRANCH=1`、`ERROR_BRANCH=1`、`EMPTY_TEXT_ABSENT_IN_ERROR=1` | PASS | error 分支出现空态文案 → FAIL | dev |
| R6 | `bash scripts/verify-2469.sh r6` | `docs/evidence/2469/R6-tooltip-static.txt` | exit 0；grep `TOOLTIP=1`、`CONDITIONS_IN_TOOLTIP=1` | PASS | 判定四条仍在 Alert 正文 → FAIL | dev |
| R7 | `bash scripts/verify-2469.sh r7` | `docs/evidence/2469/R7-purge-static.txt` | exit 0；grep `CONFIRM_KEPT=1`、`BATCH_DISABLED_KEPT=1` | PASS | 确认弹窗被移除 → FAIL | dev |
| R8 | `bash scripts/verify-2469.sh r8` | `docs/evidence/2469/R8-build.txt` | exit 0；grep `PC_BUILD=0`、`LINT_EXIT=0` | PASS | 任一非 0 → FAIL | dev |
| R9 | `bash scripts/verify-2469.sh r9` | `docs/evidence/2469/R9-gate.txt` | exit 0；grep `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | PASS | `P5=NA` 或 sha 不一致 → FAIL | dev |

### Workload

- Size: S
- Basis: 1 个新纯函数模块 + 1 个页面文件（Modal 结构）+ 1 个纯函数测试 + 1 个 verify 脚本 + 9 条证据行；无后端、无 migration
- Confidence: high

### Files（变更文件清单）

- `frontend-pc/src/utils/orphanPanel.js`
- `frontend-pc/src/pages/System/UserManagement.jsx`
- `frontend-pc/tests/orphanPanel.test.mjs`
- `scripts/verify-2469.sh`
- `docs/evidence/2469/plan.md`
- `docs/evidence/2469/pre-ready-2469.json`
- `docs/evidence/2469/R1-pure.txt`
- `docs/evidence/2469/R2-empty-static.txt`
- `docs/evidence/2469/R3-data-static.txt`
- `docs/evidence/2469/R4-fields-static.txt`
- `docs/evidence/2469/R5-loading-static.txt`
- `docs/evidence/2469/R6-tooltip-static.txt`
- `docs/evidence/2469/R7-purge-static.txt`
- `docs/evidence/2469/R8-build.txt`
- `docs/evidence/2469/R9-gate.txt`
- `docs/evidence/2469/evidence.json`

## Dependencies

- 无

### 依赖与关联说明（非硬依赖）

- **#2369**（孤儿面板来源）：本单只改展示层，不改扫描契约。
- **#2382**（预生产核对回执）：本项为其非阻塞 UX 观察，独立关闭。
- 部署侧：真实浏览器/预生产走查（空态无红条、有数据红条 + 命中数、清理后回空态）属人工验收 → 关闭时创建 `[Ops-Followup of #2469]`（`waitForConfirm`，无 `status:*`）。

*Model: DeepSeek V4.1 Flash 1017*

