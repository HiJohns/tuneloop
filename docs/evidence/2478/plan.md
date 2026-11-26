## Accepted Plan — #2478（直属成员管理「创建员工」单击即进创建表单）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 直属成员管理「创建员工」按钮只做**裸路由跳转** `navigate('/staff')`，不带任何「打开创建表单」的意图 | `frontend-pc/src/pages/DirectMemberManagement.jsx:228` |
| 人员管理页 `viewMode` 默认 `'list'`，**没有任何外部入口能置为 `'create'`**；只有页内按钮 `setViewMode('create')` | `frontend-pc/src/pages/StaffManagement.jsx:25`、`:654` |
| 该页查询参数是**本地 state**（`useState({})`，#2427 刻意把 URL 与查询解耦），因此不能靠 URL query 传意图 | `StaffManagement.jsx:23` |
| 页面已 `useLocation()`（有 `[location]` 触发刷新），可安全消费 `location.state` | `StaffManagement.jsx:45`、`:73-81` |

**结论**：这是「跨页创建意图缺失」——第一次点击只是到达 `/staff` 列表视图，用户必须再点页内「创建员工」才进表单，故表现为「要点两次」。修复 = 传递**一次性意图**并在落地页消费。

### 变更点

1. 新增纯函数模块 `frontend-pc/src/utils/staffCreateIntent.js`（便于 `node --test` 覆盖，PC 无测试运行器）：
   - `CREATE_INTENT_OPEN = 'openCreate'`
   - `isCreateIntent(state)` → `Boolean(state && state[CREATE_INTENT_OPEN] === true)`
   - `createIntentState()` → `{ [CREATE_INTENT_OPEN]: true, userType: 'merchant_direct' }`（当前页仅支持直属员工）
   - `isConsumableIntent(state)` → 与 `isCreateIntent` 同义但**幂等**（`null/undefined/{}/false` 均 false），保证消费后不会再次触发表单
2. `frontend-pc/src/pages/DirectMemberManagement.jsx`：按钮改为
   `navigate('/staff', { state: createIntentState() })`（保留原有图标/文案）。
3. `frontend-pc/src/pages/StaffManagement.jsx`：新增一次性消费 effect
   - `useEffect(() => { if (!isCreateIntent(location.state)) return; resetCreateState(); setViewMode('create'); navigate(location.pathname, { replace: true, state: null }) }, [location.state])`
   - 目的：①首帧即进入创建视图；②`replace` 清掉 state → **刷新/前进后退不重复弹表单**；③返回列表后再从直属成员管理点击仍生效（新一次 `navigate` 重新带 state）。
   - 不修改既有 `[location]` 刷新 effect 与创建/取消逻辑。
4. 文档：`docs/spec/ui/ui.md` 在「直属成员管理 / 人员管理」入口处补一句「『创建员工』单击直达创建表单（一次性意图，刷新不重复打开）」。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 意图判定纯函数 | `frontend-pc/src/utils/staffCreateIntent.js` | `node --test` | state 为 `null`/`{}`/`{openCreate:true}`/`{openCreate:'true'}` | JS | `null→false`、`{}→false`、`{openCreate:true}→true`、`{openCreate:'true'}→false`；`createIntentState()` 含 `openCreate:true` 与 `userType:'merchant_direct'` | R1 |
| R2 入口按钮携带意图 | `frontend-pc/src/pages/DirectMemberManagement.jsx` | `/staff/direct` →「创建员工」 | 单击一次 | PC | 源码结构断言：`navigate('/staff', { state: createIntentState() })`，不再出现裸 `navigate('/staff')` | R2 |
| R3 落地页消费意图 | `frontend-pc/src/pages/StaffManagement.jsx` | `/staff`（带 `location.state`） | state 含 `openCreate:true` | PC | 源码结构断言：命中 `isCreateIntent(location.state)`、`setViewMode('create')`、`navigate(location.pathname, { replace: true, state: null })` | R3 |
| R4 刷新/返回不重复打开 | `frontend-pc/src/pages/StaffManagement.jsx`、`frontend-pc/src/utils/staffCreateIntent.js` | `/staff` 消费后刷新 | 消费后 state 为 `null` | PC | 源码结构断言：消费使用 `replace: true`；纯函数 `isConsumableIntent(null)=false` | R4 |
| R5 取消/返回列表回归 | `frontend-pc/src/pages/StaffManagement.jsx` | 创建视图 →「取消」 | 点击取消 | PC | 源码结构断言：取消分支仍 `resetCreateState()` + `setViewMode('list')`；列表视图默认 `viewMode='list'` 不变 | R5 |
| R6 构建与 lint | `frontend-pc` | `npm run build`、`eslint` 改动文件 | 修后树 | PC | build exit 0；lint 0 `no-undef` | R6 |
| R7 gate 四源一致 + 全量 build | `docs/evidence/2478/pre-ready-2478.json`、`docs/evidence/2478/plan.md` | `/work` 以 `--full-gate-cmd` 跑一次 pre-ready（前端-only → `cd frontend-pc && npm run build`），随后 `plan-source.sh --issue 2478` | 修后树（干净） | PC | gate `result=PASS` 且含 `FULL_SUITE_EXIT=0`（不得为 N/A）；`PLAN_SOURCE: PASS` | R7 |

## Input Truth & Tool Preflight

- **Source of truth**：`DirectMemberManagement` 的意图（要「创建员工」）与 `StaffManagement` 的视图状态（`viewMode`）分属两个组件；`StaffManagement` 的查询参数是本地 state（#2427），**不能**用 URL query 传意图 → 采用 `navigate(..., { state })` 一次性意图。
- **差异核对**：`grep -rn "navigate('/staff'" frontend-pc/src` 共 8 处，其余均为「返回列表/取消」语义（`StaffEdit`、`StaffResetPassword`、`StaffBulkImport`），**不应**改为携带创建意图；仅 `DirectMemberManagement.jsx:228` 需要。
- **损坏输入拒绝**：`location.state` 为 `null`/`undefined`/`{}`/非布尔 `openCreate` 一律不触发（防止字符串 `'true'` 误判）；消费后立即 `replace` 清 state，禁止残留导致刷新重复打开。
- **工具盲区**：`frontend-pc` 无测试运行器（`package.json` 仅 dev/build/lint/preview）→ 纯函数用 `node --test`，交互接线用源码结构断言（`evidence_type: static`，Owner 豁免 D4）；真实浏览器单击走查属部署侧。
- **影响面预检**：#2465（维修师创建入口内联到人员管理）会重构该入口，本单修复集中在「意图传递 + 一次性消费」，不与 #2465 的创建表单/筛选改动冲突；本单不触碰 `handleCreateUser` 与权限门控。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 意图传递方式：`navigate('/staff', { state: { openCreate: true, userType: 'merchant_direct' } })`；**不用** URL query（该页查询参数为本地 state，#2427） | Architect | 本计划锁定 | 若要求可分享 URL（`?create=1`）→ 回 Architect（需重新引入 URL 读取与清理） |
| D2 | 消费语义：落地即 `resetCreateState()` + `setViewMode('create')`，随后 `navigate(pathname, { replace: true, state: null })`；刷新/后退不得重复打开 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 不改权限门控与创建逻辑；仅「第一次点击」的视图切换 | Architect | 本计划锁定 | 阻塞实现 |
| D4 | R2–R5 为纯前端接线（无测试运行器）→ `evidence_type: static` 源码结构断言 + 构建，**Owner 豁免**（Architect 授予） | Architect | 本计划锁定 | 若审计要求 render → 另立前端测试基建 Issue |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R2 | static | 入口按钮携带 intent 的源码结构断言，Owner 豁免见 D4 |
| R3 | static | 落地页消费 intent 的源码结构断言，Owner 豁免见 D4 |
| R4 | static | `replace: true` 与幂等判定的源码结构断言，Owner 豁免见 D4 |
| R5 | static | 取消/列表回归的源码结构断言，Owner 豁免见 D4 |

## Out of Scope

- 直属成员管理页面整体收敛 / 维修师创建入口内联（#2465）。
- 人员管理创建表单字段、校验、密码/邀请语义（#2372/#2402/#2400）。
- 其它 7 处 `navigate('/staff')`（返回列表/取消语义）不改。
- URL 可分享的创建入口（`?create=1`）。
- 真实浏览器单击走查（部署侧人工验收）。

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|---|---|---|---|
| R1 | 四态 state：`null`、`{}`、`{openCreate:true}`、`{openCreate:'true'}` | `frontend-pc/src/utils/staffCreateIntent.js`（node --test） | `docs/evidence/2478/R1-pure.txt` |
| R2 | 修后 `DirectMemberManagement.jsx` | `/staff/direct` 工具栏按钮 | `docs/evidence/2478/R2-intent-static.txt` |
| R3 | 修后 `StaffManagement.jsx` 消费 effect 片段 | `/staff` | `docs/evidence/2478/R3-consume-static.txt` |
| R4 | 修后消费语句 + helper | `/staff` 刷新 | `docs/evidence/2478/R4-replace-static.txt` |
| R5 | 修后取消分支 + 默认 `viewMode` | 创建视图「取消」 | `docs/evidence/2478/R5-cancel-static.txt` |
| R6 | 修后 `frontend-pc` | `npm run build` + eslint | `docs/evidence/2478/R6-build.txt` |
| R7 | 修后干净树 + gate 报告 | 读校验（`plan-source.sh`） | `docs/evidence/2478/R7-gate.txt` |

### Assertions

- R1：`node --test` exit 0；`NULL=false`、`EMPTY=false`、`TRUE=true`、`STRING=false`、`STATE_HAS_OPEN=1`、`STATE_TYPE=merchant_direct`；负例：`'true'` 被判 true → FAIL。
- R2：命中 `createIntentState()` 与 `navigate('/staff', { state:`；负例：仍为裸 `navigate('/staff')` → FAIL。
- R3：命中 `isCreateIntent(location.state)`、`setViewMode('create')`；负例：缺消费 effect → FAIL。
- R4：命中 `replace: true`、`state: null`；负例：用 `push` 保留 state → FAIL。
- R5：命中取消分支 `setViewMode('list')` 与 `useState('list')`；负例：取消后仍停留在 create → FAIL。
- R6：`PC_BUILD=0`、`LINT_EXIT=0`；负例：任一非 0 → FAIL。
- R7：`GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS`；负例：`P5=NA` 或 sha 不一致 → FAIL。
- 通用 provenance：每个 artifact 首行 `# build=1.0.<fix> # commit=<sha> # dirty=false`。
- 负例（跨行）：任何 artifact 出现「两次点击」「double click to create」语义 → FAIL。

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2478/R1-pure.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2478/R2-intent-static.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2478/R3-consume-static.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2478/R4-replace-static.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2478/R5-cancel-static.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2478/R6-build.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2478/R7-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| symptom | trigger | root cause | test blind spot | same-class paths | fix layer | red-green | prevention |
|---|---|---|---|---|---|---|---|
| 「创建员工」要点两次 | 直属成员管理 → 创建员工 | 入口只 `navigate('/staff')`，落地页无创建意图 | 无跨页入口用例 | 其它「跳转到列表并打开创建」入口 | 前端 | R2/R3 红绿 | D1/D2 一次性意图 |
| 刷新后表单又弹出（若实现不当） | 消费后刷新 | state 未清理 / 用 push | 无刷新语义用例 | 其它 `location.state` 消费 | 前端 | R4 红绿 | `replace: true` + `state: null` |
| 字符串 `'true'` 误触发 | 外部构造 state | 宽松真值判定 | 无类型用例 | 其它 intent 判定 | 前端 | R1 红绿 | 严格 `=== true` |
| 取消后仍停留创建视图 | 取消按钮 | 视图状态未回退 | 无回归用例 | 其它视图切换 | 前端 | R5 红绿 | 取消走 `resetCreateState` |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2478.sh r1` | `docs/evidence/2478/R1-pure.txt` | exit 0；grep `NULL=false`、`TRUE=true`、`STRING=false` | PASS | `STRING=true` → FAIL | dev |
| R2 | `bash scripts/verify-2478.sh r2` | `docs/evidence/2478/R2-intent-static.txt` | exit 0；grep `INTENT_NAV=1`、`BARE_NAV=0` | PASS | `BARE_NAV=1` → FAIL | dev |
| R3 | `bash scripts/verify-2478.sh r3` | `docs/evidence/2478/R3-consume-static.txt` | exit 0；grep `CONSUME_EFFECT=1`、`SET_CREATE=1` | PASS | `CONSUME_EFFECT=0` → FAIL | dev |
| R4 | `bash scripts/verify-2478.sh r4` | `docs/evidence/2478/R4-replace-static.txt` | exit 0；grep `REPLACE_TRUE=1`、`STATE_NULL=1` | PASS | `REPLACE_TRUE=0` → FAIL | dev |
| R5 | `bash scripts/verify-2478.sh r5` | `docs/evidence/2478/R5-cancel-static.txt` | exit 0；grep `CANCEL_TO_LIST=1`、`DEFAULT_LIST=1` | PASS | 取消仍停留 create → FAIL | dev |
| R6 | `bash scripts/verify-2478.sh r6` | `docs/evidence/2478/R6-build.txt` | exit 0；grep `PC_BUILD=0`、`LINT_EXIT=0` | PASS | 任一非 0 → FAIL | dev |
| R7 | `bash scripts/verify-2478.sh r7` | `docs/evidence/2478/R7-gate.txt` | exit 0；grep `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | PASS | `P5=NA` 或 sha 不一致 → FAIL | dev |

### Workload

- Size: S
- Basis: 1 个新纯函数模块 + 2 个页面接线 + 1 个纯函数测试 + 1 个 verify 脚本 + 1 篇 UI 文档 + 7 条证据行；无后端、无 migration
- Confidence: high

### Files（变更文件清单）

- `frontend-pc/src/utils/staffCreateIntent.js`
- `frontend-pc/src/pages/DirectMemberManagement.jsx`
- `frontend-pc/src/pages/StaffManagement.jsx`
- `frontend-pc/tests/staffCreateIntent.test.mjs`
- `scripts/verify-2478.sh`
- `docs/spec/ui/ui.md`
- `docs/evidence/2478/plan.md`
- `docs/evidence/2478/pre-ready-2478.json`
- `docs/evidence/2478/R1-pure.txt`
- `docs/evidence/2478/R2-intent-static.txt`
- `docs/evidence/2478/R3-consume-static.txt`
- `docs/evidence/2478/R4-replace-static.txt`
- `docs/evidence/2478/R5-cancel-static.txt`
- `docs/evidence/2478/R6-build.txt`
- `docs/evidence/2478/R7-gate.txt`
- `docs/evidence/2478/evidence.json`

## Dependencies

- 无

### 依赖与关联说明（非硬依赖）

- **#2465（accepted）**：将把维修师创建入口内联进人员管理并重构该页筛选；本单只修「一次性创建意图」，与其不冲突；#2465 实施时若删除 `/staff/direct` 入口，本单的 R2 行随该页一并下线（届时以 #2465 计划为准）。
- **#2122**（直属成员管理创建维修师入口）：同一页面的相邻入口，不改其行为。
- **部署侧**：真实浏览器单击走查（一次进入创建表单、刷新不回弹）由 `[Ops-Followup of #2478]`（`waitForConfirm`）承载。

*Model: DeepSeek V4.1 Flash 1017*


