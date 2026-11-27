## Accepted Plan — #2564（分类页 40002 重名恢复表单内持久提示 + 提交路径判码去假成功）｜Rev.3

### 背景与现状核对（仓库 HEAD 只读核对）

| 事实 | 位置 |
|---|---|
| 分类新增/改名重名：`HTTP 400 + code 40002` + 中文 message「分类名称已存在」 | `backend/handlers/api.go:874-879`（CreateCategory）、`backend/handlers/api.go:953-958`（UpdateCategory） |
| 创建成功码是 **`20100`**（`http.StatusCreated`），不是 `20000` | `backend/handlers/api.go:907-911`（`c.JSON(http.StatusCreated, gin.H{"code": 20100, ...})`） |
| 更新成功码是 `20000` | `backend/handlers/api.go:995-999` |
| 删除失败返回 `400 + 40003`（有子分类），与重名无关（不在本单） | `backend/handlers/api.go:1011-1016` |
| 「业务可纠正冲突」白名单唯一 owner = `40002/40004/40005/40900` | `frontend-pc/src/utils/formConflict.js:8-14` |
| 持久提示共享件（唯一 owner，禁止各页自写 Alert） | `frontend-pc/src/components/FormConflictAlert.jsx` |
| 分类页当前**未消费** `formConflict.js` / `FormConflictAlert`（无 import、无 state） | `frontend-pc/src/pages/admin/category/List.jsx:1-10` |
| 提交路径：`update` 不判码直接 `message.success('更新成功')`；`create` 只判 `20000` → 成功分支永不执行（死分支） | `frontend-pc/src/pages/admin/category/List.jsx:250-262` |
| catch 只做通用 toast（HTTP 400 的 40002 落此处，code 被丢） | `frontend-pc/src/pages/admin/category/List.jsx:265-268` |
| #2563 已 accepted：非 404/409 的非 2xx 抛出**带 `code`/`status`/`data` 的 Error**（唯一 owner `frontend-pc/src/services/apiError.js`） | `docs/evidence/2563/plan.md`（D1） |
| PC 无组件测试运行器 → UI 用静态结构断言（Owner 豁免，沿用 #2480 D6 / #2516 D5） | `frontend-pc/package.json`（无 test 脚本） |

### 变更点

1. `frontend-pc/src/pages/admin/category/List.jsx`：
   - 新增 `formConflict` state，import `isBusinessCorrectableConflict` / `conflictText`（`utils/formConflict.js`）与 `FormConflictAlert`（`components/FormConflictAlert.jsx`）；
   - `handleFormSubmit` 的 `catch`：先 `if (error.errorFields) return`（保留表单校验路径），再 `if (isBusinessCorrectableConflict(error)) { setFormConflict(conflictText(error, '请修改后重新提交')); return }`，最后保留 `message.error` 瞬时时兜底；
   - **更新分支**：改为 `const res = await categoriesApi.update(...)`，`res.code !== 20000` 时先判业务可纠正冲突 → 置持久提示并 `return`；否则 `message.error(res.message || '更新失败')` 并 `return`（**不得**再在失败时关闭弹窗/报「更新成功」）；
   - **创建分支**：成功判定改为 `response.code === 20000 || response.code === 20100`（使既有 `message.success('创建成功')` 与 `setSelectedParentId` 死分支真正生效）；非成功时同样先判冲突置持久提示、否则 `message.error` 并 `return`；
   - Modal 内渲染 `<FormConflictAlert text={formConflict} testId="category-form-conflict" />`（表单上方），`onValuesChange` 清空陈旧提示（#2480 D8），提交开始与成功后亦清空。
2. 新增 `scripts/verify-2564.sh`（r1–r3 证据生成器，每个 artifact 首行写 provenance）。
3. 不改后端、不改 `formConflict.js` / `FormConflictAlert.jsx`（只消费）、不改 `handleDelete` / 排序 / 隐藏路径。

- Tags: bug, scope:pc, scope:frontend

## Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 分类重名（新建或改名）只播 3 秒 toast，用户改名前提示已消失；创建成功无任何成功反馈 | `frontend-pc/src/pages/admin/category/List.jsx:250-268` |
| 2 | 触发条件（trigger） | 后端返回 `HTTP 400 + 40002`「分类名称已存在」；或创建返回 `HTTP 201 + 20100` | `backend/handlers/api.go:874-879`、`907-911` |
| 3 | 根因（root cause） | 提交路径把「HTTP 状态」「业务 code」混用：update 完全不判码即报成功；create 只判 `20000` 漏 `20100`；catch 无 code 判定入口（#2563 之前 code 已被丢） | `List.jsx:250-268` + #2563 D1 |
| 4 | 测试盲点（test blind spot） | PC 无组件测试运行器，分类页无任何自动化用例；`formConflict` 纯函数测试造的是对象，从不验证真实客户端产出的 Error | `frontend-pc/package.json`、`frontend-pc/tests/` |
| 5 | 同类扫描（same-class paths） | 同文件 `handleDelete`（40003）/`handleHide`/`sortSingle` 仍是通用 toast（删除有子分类属 40003 即时域，D2 明确不动）；Batch 2 其余页面由 #2486 承接 | `List.jsx:154-165`、`#2486` |
| 6 | 修复层（fix layer） | 页面层消费共享件（`formConflict.js` + `FormConflictAlert`），不自写判定/Alert；传输层已在 #2563 收敛 | 本计划 R1/R2 |
| 7 | 红绿实证（red-green） | 红：`git show <fix>^:List.jsx` 只有 `response.code === 20000` 且 catch 无 code 判定；绿：修后 `20100` 被接受、catch 有冲突分支、update 失败不再报成功 | 本计划 R2 |
| 8 | 防复发（prevention） | 静态断言锁死：`CREATE_ACCEPTS_20100=1`、`UPDATE_CODE_GUARD=1`、`CONFLICT_BRANCH_IN_CATCH=1`、`LOCAL_ALERT_DUP=0`；共享件白名单仍唯一 owner | 本计划 R1/R2 |
| 9 | 影响面（blast radius） | 仅分类页提交路径（新建/改名的弹窗）；列表、拖拽排序、隐藏、删除路径不改 | `List.jsx` diff 范围 |
| 10 | 未修边界（deferred） | 删除有子分类（40003）是否也改持久提示 → 归 #2486 判定；`handleHide` 的并发 404 是否需判码 → 不在本单（D2） | 本计划 Out of Scope |

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 持久提示接线（catch code-first + 共享件 + 改输入清空） | `frontend-pc/src/pages/admin/category/List.jsx` | 分类新建/改名弹窗提交 → `handleFormSubmit` 的 catch | catch 收到 40002/40004/40005 或 40900 | PC | `CONFLICT_BRANCH_IN_CATCH=1`、`CONFLICT_TEXT_FALLBACK=1`、`ALERT_RENDERED=1`、`CLEAR_ON_CHANGE=1`、`SHARED_REUSE=1`、`LOCAL_ALERT_DUP=0`、`FALLBACK_TOAST=1`；负例：catch 无 code 判定、自写 Alert、或删掉兜底 toast → FAIL | R1 |
| R2 提交路径判码（create 接 20100 + update 失败不假成功）与红绿对照 | `frontend-pc/src/pages/admin/category/List.jsx` | 分类新建（`POST /api/categories`）、改名（`PUT /api/categories/:id`） | 成功 20100 / 20000；失败 400/404 | PC | `CREATE_ACCEPTS_20100=1`、`UPDATE_CODE_GUARD=1`、`NONOK_RETURN_BEFORE_CLOSE=2`、`RED_SINGLE_CODE_CHECK=1`、`RED_NO_CATCH_BRANCH=1`、`GREEN_CODE_FIRST=1`、`AUTOSELECT_TOPLEVEL_ONLY=1`、`CREATED_SUBCATEGORY_KEEPS_PARENT=1`、`GUARD_MUTATION_DETECTED=1`、`RED_NO_AUTOSELECT_GUARD=1`；负例：仍只判 20000、失败分支落到 `setModalVisible(false)`、或新建子分类后父级被改成顶级 → FAIL | R2 |
| R3 构建 + ESLint | `frontend-pc` | `npm run build`、`npx eslint` | 修后干净树 | PC | `BUILD_EXIT=0`、`ESLINT_NO_UNDEF=0`；负例：任一非 0 或 `no-undef` 计数 > 0 → FAIL | R3 |
| R4 pre-ready gate（四源一致 + 证据完整 + P13 已推送） | `docs/evidence/2564/pre-ready-2564.json` | `pre-ready-check.sh 2564` | 修后树 + 最新 Accepted Plan | PC | `GATE_RESULT=PASS`、`jq -e '.overall == "PASS"'`、evidence 每行 `pass=true`、`dirty=false`；负例：任一 P 项失败或 evidence 缺行 → FAIL | R4 |

## Input Truth & Tool Preflight

- **Source of truth**：
  - 后端码 = `backend/handlers/api.go` 的实际导出（重名 `40002`、创建成功 `20100`、更新成功 `20000`、删除子分类 `40003`）；
  - 白名单与文案 = `frontend-pc/src/utils/formConflict.js` + `frontend-pc/src/services/errorMessages.js`（不复制）；
  - 传输契约 = `docs/evidence/2563/plan.md` D1（非 404/409 抛带 `code` 的 Error）。
- **差异核对（/analyze 只读实测）**：
  - `grep -n "response.code === 20000" frontend-pc/src/pages/admin/category/List.jsx` → 仅 1 处（第 255 行），无 `20100`；
  - `grep -n "categoriesApi.update" frontend-pc/src/pages/admin/category/List.jsx` → 第 154 行（隐藏，带 `visible` 参数）与第 251 行（提交，未判码）；
  - `grep -n "FormConflictAlert\|formConflict" frontend-pc/src/pages/admin/category/List.jsx` → 无命中（本单新增接线）；
  - `sed -n '907,911p' backend/handlers/api.go` → `StatusCreated` + `20100`。
- **损坏输入拒绝规则**：`isBusinessCorrectableConflict` 收到 `null`/非对象/非数字 code 必须返回 false 且不抛错（既有实现已满足，禁止本单另写判定）；`conflictText` 的空 message 必须回落到调用点 fallback（禁止渲染空白 Alert）。
- **工具盲区声明**：
  - `scripts/verify-2564.sh` 是**文本级**结构断言（grep 块边界/计数），无法证明运行时渲染；UI 回执由 `[Ops-Followup of #2564]` 承载（D4）。
  - 静态断言不能区分同名函数分支，故 R2 只锁「`handleFormSubmit` 函数体内、`setModalVisible(false)` 之前存在 ≥2 个失败 `return`」这一结构事实。
  - 红样依赖 `git log -1 --format=%H -- frontend-pc/src/pages/admin/category/List.jsx`（禁止手写 sha）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 成功码口径按端点实际返回：`CreateCategory` = `20100`、`UpdateCategory` = `20000`；创建分支接受两者 | Architect | 本计划锁定（代码为据 `api.go:907-911` / `995-999`） | 阻塞实现 |
| D2 | 本单只改「提交路径」（新建/改名）；`handleDelete`（40003 有子分类，即时域）、`handleHide`、拖拽/上下移排序不改 | Architect | 本计划锁定 | 若 Owner 要求扩面 → 回 Architect 另立 Issue |
| D3 | 判定与文案复用 #2480 共享件（`isBusinessCorrectableConflict` / `conflictText` / `FormConflictAlert`），禁止自写 Alert 或第二份白名单 | Architect | 本计划锁定 | 阻塞实现 |
| D4 | R1/R2 证据形态：静态结构断言；真实点击/渲染回执由 `[Ops-Followup of #2564]`（`waitForConfirm`）承载 —— **Owner 豁免（UI static exemption）**，沿用 #2480 D6 / #2516 D5 | Architect | Owner 豁免已记录 | 若审计要求 render → 另立前端测试基建 Issue（Out of Scope） |
| D5 | 前置 #2563 合入 develop 前不得开工（传输层不给 code 时 catch 分支不可达） | Architect | `## Dependencies` + `work_deps` 校验 | HOLD，禁止本地复制 `apiError.js` |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | static | 页面 catch 结构与共享件接线断言；真实渲染见 D4 Ops Follow-up |
| R2 | static+scan | 提交路径判码结构断言 + `git show` 红绿对照 |

## Out of Scope

- 传输层（`services/api.js` / `services/apiError.js`）→ **#2563**（前置）。
- Batch 2 其余页面（用户管理、轮播、商城图片、库存、权限模板、价格配置、平台人员、中转网点、维修师档案、会员等级、商户、客户、租户）→ **#2486**。
- `handleDelete` 的 40003（有子分类）是否改持久提示、`handleHide`/排序的失败提示口径 → 由 #2486 判定，本单不改。
- 后端错误码/文案（`40002` 两义性、删除 `40003`）重构 → 另立 Issue。
- 分类列表/拖拽/隐藏的既有行为与布局（不做重构）。
- PC 组件测试基建（vitest/@testing-library）→ 另立 Issue。
- `frontend-mobile` 分类相关页面（无此 PC 页面对应实现）。

## G0 Golden Acceptance

| 行 ID | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） | assertions（含负例） |
|---|---|---|---|---|
| R1 | 修后 `List.jsx` 的 `handleFormSubmit` 与 Modal 段 | 分类新建/改名弹窗提交 | `docs/evidence/2564/R1-consumer-wiring.txt` | `CONFLICT_BRANCH_IN_CATCH=1`、`CONFLICT_TEXT_FALLBACK=1`、`ALERT_RENDERED=1`、`CLEAR_ON_CHANGE=1`、`SHARED_REUSE=1`、`LOCAL_ALERT_DUP=0`、`FALLBACK_TOAST=1`；负例：自写 `Alert` 或 catch 无 `isBusinessCorrectableConflict` → FAIL |
| R2 | 修前 `List.jsx`（`git log -1 -- ...List.jsx` 的父提交）+ 修后同文件；**夹具锚点（可执行）**：`frontend-pc/src/pages/admin/category/List.jsx` 的 `finalParentId` 计算块 + `if (response.data?.id && !finalParentId)` 守卫（脚本按正则定位并把实际行号写入 artifact，不写死行号） | 提交路径判码 + 分类选择器结构 | `docs/evidence/2564/R2-code-first.txt` | `RED_SINGLE_CODE_CHECK=1`、`RED_NO_CATCH_BRANCH=1`、`GREEN_CODE_FIRST=1`、`CREATE_ACCEPTS_20100=1`、`UPDATE_CODE_GUARD=1`、`NONOK_RETURN_BEFORE_CLOSE=2`、**`AUTOSELECT_TOPLEVEL_ONLY=1`**、**`CREATED_SUBCATEGORY_KEEPS_PARENT=1`**、**`GUARD_MUTATION_DETECTED=1`**、**`RED_NO_AUTOSELECT_GUARD=1`**；负例：仍只判 `20000`、失败分支仍关闭弹窗、或父分类被静默改成顶级 → FAIL |
| R3 | 修后干净树 | `cd frontend-pc` | `docs/evidence/2564/R3-build-lint.txt` | `BUILD_EXIT=0`、`ESLINT_NO_UNDEF=0`；负例：构建非 0 或 `no-undef` > 0 → FAIL |
| R4 | 修后树 + 最新 Accepted Plan + evidence.json | `pre-ready-check.sh 2564` | `docs/evidence/2564/pre-ready-2564.json` | `GATE_RESULT=PASS`、`jq -e '.overall == "PASS"'`、每行 `pass=true`、`dirty=false`；负例：任一 P 失败 / evidence 缺行 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2564.sh r1 > docs/evidence/2564/R1-consumer-wiring.txt` | `docs/evidence/2564/R1-consumer-wiring.txt` | exit 0；`grep -q '^R1_OK$'`；`grep -q 'CONFLICT_BRANCH_IN_CATCH=1'` | 7 个标记全部为期望值（含 `LOCAL_ALERT_DUP=0`、`CLEAR_ON_CHANGE=1`） | catch 无 code 判定、自写 Alert、或删掉兜底 toast → `R1_FAIL` 并非 0 退出 | dev |
| R2 | `bash scripts/verify-2564.sh r2 > docs/evidence/2564/R2-code-first.txt` | `docs/evidence/2564/R2-code-first.txt` | exit 0；`grep -q '^R2_OK$'`；`grep -q 'CREATE_ACCEPTS_20100=1'` | `UPDATE_CODE_GUARD=1`、`NONOK_RETURN_BEFORE_CLOSE=2`、`RED_SINGLE_CODE_CHECK=1`、`RED_NO_CATCH_BRANCH=1` | 仍只判 `20000`、失败分支仍 `setModalVisible(false)` 或仍报「更新成功」→ `R2_FAIL` 并非 0 退出 | dev |
| R3 | `bash scripts/verify-2564.sh r3 > docs/evidence/2564/R3-build-lint.txt` | `docs/evidence/2564/R3-build-lint.txt` | exit 0；`grep -q '^R3_OK$'`；`grep -q 'BUILD_EXIT=0'`；`grep -q 'ESLINT_NO_UNDEF=0'` | 构建 0 退出、`no-undef` 计数 0 | 构建非 0 或 `no-undef` > 0 → `R3_FAIL` 并非 0 退出 | dev |
| R4 | `/home/coder/.config/opencode/scripts/pre-ready-check.sh 2564 --summary-file docs/evidence/2564/summary.md --changed-files docs/evidence/2564/changed-files.txt --evidence-file docs/evidence/2564/evidence.json --full-gate-cmd "cd frontend-pc && npm run build" --report docs/evidence/2564/pre-ready-2564.json` | `docs/evidence/2564/pre-ready-2564.json` | exit 0；`GATE_RESULT=PASS`；`jq -e '.overall == "PASS"'` | 每条合同行 `pass=true`、`dirty=false`、P13 提交在 `origin/issue-2564` | 任一 P 项失败 / evidence.json 缺行 / `dirty=true` → gate 非 0，禁止 ready | dev |

### Files（变更文件清单）

| 类别 | 路径 |
|---|---|
| 页面接线 | `frontend-pc/src/pages/admin/category/List.jsx` |
| 证据生成器（新增） | `scripts/verify-2564.sh` |
| 证据（计划快照） | `docs/evidence/2564/plan.md` |
| 证据（摘要） | `docs/evidence/2564/summary.md` |
| 证据（改动清单） | `docs/evidence/2564/changed-files.txt` |
| 证据（合同汇总） | `docs/evidence/2564/evidence.json` |
| 证据 R1 | `docs/evidence/2564/R1-consumer-wiring.txt` |
| 证据 R2 | `docs/evidence/2564/R2-code-first.txt` |
| 证据 R3 | `docs/evidence/2564/R3-build-lint.txt` |
| Gate 报告 | `docs/evidence/2564/pre-ready-2564.json` |

## Artifact Provenance

- 生成器：`scripts/verify-2564.sh`（r1–r3）；每个文本 artifact 首行必须为
  `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`，锚定本单 fix commit（`FIX_REF` 传入）。
- 逐条清单（全部位于 `docs/evidence/2564/`，均需带上述首行）：
  - `docs/evidence/2564/R1-consumer-wiring.txt`（`List.jsx` 接线结构断言输出）
  - `docs/evidence/2564/R2-code-first.txt`（提交路径判码 + 红绿对照输出）
  - `docs/evidence/2564/R3-build-lint.txt`（`npm run build` + `npx eslint` 输出）
  - `docs/evidence/2564/pre-ready-2564.json`（gate 报告，JSON 自带 `.provenance.{build,commit,dirty}`）
- `docs/evidence/2564/evidence.json` 字段口径：`build` 形如 `1.0.<7-40 位短码>`，`commit` 为 40 位 sha，`dirty:false`。
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$
- 证据重生成纪律：fix commit 之后统一重跑 `scripts/verify-2564.sh r1`…`r3`，禁止只改 `build/commit` 冒充新证据。

## Dependencies

- #2563

### 依赖说明（非声明）

- #2563 提供传输契约（非 404/409 抛出带 `code`/`status`/`data` 的 `Error`）；其合入 develop 前本单 catch 分支不可达，`/work` 必须 HOLD。
- 平行关系：#2485 目前 wip 且其 Files 含本文件（其 D3 已把该行判为 `defer:tier2`）；本单承接该 defer，须在 #2485 之后或与其协调合并顺序，避免同文件冲突。

### Workload

- Size: M
- Basis: 1 个页面文件 + 1 个证据脚本 + 4 条证据行（含 plan/summary/changed-files/evidence.json）；无后端、无 migration、无新增依赖
- Confidence: high

*Model: DeepSeek V4.1 Flash 1017*



