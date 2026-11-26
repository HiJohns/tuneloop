## Work Summary — #2564（分类页 40002 重名恢复表单内持久提示 + 提交路径判码去假成功）

### 修改内容

#### Round 2（审计 Bug 1 修复）

- `frontend-pc/src/pages/admin/category/List.jsx`（`handleFormSubmit` 创建分支）
  - 创建成功后**仅顶级创建**自动选中新分类：`if (response.data?.id && !finalParentId)`。
  - 子分类创建（`finalParentId` = 父分类 id）保持父分类选中——新子分类随即出现在父的子分类列表。
  - 修复前把 level-2 id 写入 `selectedParentId`：该 id 不在 `level1Categories`（`GET /api/categories` 只返回一级 + 嵌套 `sub_categories`）→ 右侧面板清空、父分类名渲染为空串、点击「编辑」时传入 `undefined` 触发 TypeError，且「创建子分类」会以该 level-2 id 再建一层不可见的孙级节点。
- `scripts/verify-2564.sh`
  - r2 新增可执行断言：`AUTOSELECT_TOPLEVEL_ONLY=1`、`CREATED_SUBCATEGORY_KEEPS_PARENT=1`、`GUARD_MUTATION_DETECTED=1`（变异负例：删掉 `&& !finalParentId` 后守卫断言必须归零）、`RED_NO_AUTOSELECT_GUARD=1`（红样无该守卫）。
  - 红样锚点改为「本 Issue 第一个页面 fix commit 的父提交」（`git log --grep="^fix(issue-2564" --reverse | head -1`），仍为动态推导；沿用 `git log -1 -- $SRC` 会在第二次 fix 后把红样前移到上一版绿样，红绿对照退化为「绿 vs 绿」。

#### Round 1（首轮实现）

- `frontend-pc/src/pages/admin/category/List.jsx`
  - 新增 `formConflict` state，import 共享件 `frontend-pc/src/utils/formConflict.js`（`isBusinessCorrectableConflict` / `conflictText`）与 `frontend-pc/src/components/FormConflictAlert.jsx`，**未自写 Alert / 未复制白名单**（D3）。
  - `handleFormSubmit` 的 `catch`：`errorFields` 保持原行为（表单自身展示，不发 toast）；命中 `isBusinessCorrectableConflict(error)` → `setFormConflict(conflictText(error, '请修改后重新提交'))` 并 `return`；其余仍走 message.error 瞬时兜底。
  - **更新分支**：`updated.code !== 20000` 时先判业务可纠正冲突（对象返回的 404/409 兜底）→ 置持久提示；否则 message.error；两种情况都 `return`，**不再报「更新成功」/不关闭弹窗**。
  - **创建分支**：成功判定改为 `response.code === 20000 || response.code === 20100`（D1：`CreateCategory` 实际返回 20100），使既有 message.success('创建成功') 与 `setSelectedParentId` 死分支恢复；失败时同样先判冲突置持久提示，否则 message.error 并 `return`。
  - Modal 表单上方渲染 `<FormConflictAlert text={formConflict} testId="category-form-conflict" />`；`onValuesChange` 清空陈旧提示（#2480 D8），打开弹窗/提交开始/成功亦清空。
- `scripts/verify-2564.sh`（新增）：r1–r3 证据生成器，artifact 首行写 provenance（build/commit/dirty）。

未改后端、未改共享件本体、未动 `handleDelete`（40003）/`handleHide`/排序（D2）。

### 涉及文件

- `frontend-pc/src/pages/admin/category/List.jsx`
- `scripts/verify-2564.sh`（新增）
- `docs/evidence/2564/plan.md`
- `docs/evidence/2564/summary.md`
- `docs/evidence/2564/changed-files.txt`
- `docs/evidence/2564/evidence.json`
- `docs/evidence/2564/R1-consumer-wiring.txt`
- `docs/evidence/2564/R2-code-first.txt`
- `docs/evidence/2564/R3-build-lint.txt`
- `docs/evidence/2564/pre-ready-2564.json`

### Commit Hash

- fix（页面接线）：`5cd3c900a`
- fix（证据脚本落盘 markers）：`5463d3d98`
- fix（红样锚点）：`869bd336c`
- fix（Round 1 Bug 1 子分类选中语义 + r2 守卫断言）：`a30c6e250`
- evidence：见 Remote 行（`docs(evidence): issue-2564`）

### Rev.2 复跑（本次 /work 恢复）

- Accepted Plan 已更新为 **Rev.2**（`sha256 e85e6ecca2d4…`）：R2 矩阵/G0 登记 4 个判定键、R4 断言键由 result 改为报告根键 overall。
- 处理：刷新 `docs/evidence/2564/plan.md` 快照到 Rev.2 → evidence.json 的 `plan_sha256` 与 R4 断言对齐 → 重跑 r1/r2/r3 证据 → 重跑 pre-ready gate。
- 产品代码未改（Round 2 审计已确认正确），本轮只做计划/证据对齐与复验。

### 验证层级

- 独立 Issue（body 无 `Parent:`，但为 #2563 的子任务 Issue —— 本单按计划 R4 执行自己的 gate）：`node --test` 纯函数由 #2563 提供且未改；本单为 PC 页面层，证据为**静态结构断言**（D4 Owner 豁免）+ `npm run build` + 改动文件 eslint。
- 无后端 / 无 migration / 无新增依赖。

### Remote

- remote: `origin/issue-2564`
- fix: `5cd3c900a` / `5463d3d98` / `869bd336c` / `a30c6e250`（pushed=yes）
- evidence（含本次 Rev.2 快照刷新与 R4 断言对齐）: `7b3d5449c`（pushed=yes）
- gate 报告: `58e6cb8da` → `docs/evidence/2564/pre-ready-2564.json`（pushed=yes）
- remote tip: `origin/issue-2564@58e6cb8dab699e1da8778ac7cc9fd9196b38ccdd`

### 红绿实证

- 红样锚点 = 本 Issue 第一个页面 fix commit 的父提交（`git log --format=%H --grep="^fix(issue-2564" --reverse -- frontend-pc/src/pages/admin/category/List.jsx | head -1`，自动取，不手写 sha）→ `5cd3c900a^`：只有 `response.code === 20000`（无 `20100`）、`catch` 无任何 code 判定、`setSelectedParentId` 无 `finalParentId` 守卫。
- 绿样（当前工作树）：`CREATE_ACCEPTS_20100=1`、`UPDATE_CODE_GUARD=1`、`NONOK_RETURN_BEFORE_CLOSE=2`、`GREEN_CODE_FIRST=1`、`RED_SINGLE_CODE_CHECK=1`、`RED_NO_CATCH_BRANCH=1`、`AUTOSELECT_TOPLEVEL_ONLY=1`、`CREATED_SUBCATEGORY_KEEPS_PARENT=1`、`GUARD_MUTATION_DETECTED=1`、`RED_NO_AUTOSELECT_GUARD=1`。
- 复现：`bash scripts/verify-2564.sh r2`；红绿对照样本路径见产物 `RED_SAMPLE=` 行，计数见 `CREATE_BLOCK_AUTOSELECT:` 行（`total=1 guarded=1 mutation_guarded=0 red_total=1 red_guarded=0`）。

### 审计 Bug 1 对照（Round 1 → 修复）

| 审计项 | 矩阵行 | 修复后证据 |
|---|---|---|
| 子分类创建成功后把 level-2 id 写入 `selectedParentId`（右侧面板清空 + 「编辑」TypeError + 可建不可见孙级） | R2 | 创建分支增加 `!finalParentId` 守卫；`AUTOSELECT_TOPLEVEL_ONLY=1`、`CREATED_SUBCATEGORY_KEEPS_PARENT=1`、`GUARD_MUTATION_DETECTED=1`、`RED_NO_AUTOSELECT_GUARD=1`（`docs/evidence/2564/R2-code-first.txt`） |
| R2 静态断言锁不住「选中目标是否合法」 | R2 | r2 新增守卫结构断言 + 变异负例（删守卫 → 断言归零） |
| 全部证据行需在新 fix commit 之后重跑并重写 provenance | R1/R2/R3/R4 | 四份产物 provenance 均为 `commit=a30c6e250…`、`dirty=false` |

### Accepted Plan 对照（4/4 变更点）

1. catch code-first + 共享 Alert + 改输入清空 → 完成（R1 七项标记全绿）。
2. 更新分支判码、创建分支接受 20100、失败不假成功、创建成功后仅顶级自动选中 → 完成（R2 十项标记 + 红绿/变异对照）。
3. `scripts/verify-2564.sh` 证据生成器 → 完成。
4. 构建 + eslint + gate → 完成（R3 / R4）。

### Evidence Contract（4/4 PASS）

| 行 | 结果 | 产物 |
|---|---|---|
| R1 | PASS | `docs/evidence/2564/R1-consumer-wiring.txt` |
| R2 | PASS | `docs/evidence/2564/R2-code-first.txt` |
| R3 | PASS | `docs/evidence/2564/R3-build-lint.txt` |
| R4 | PASS | `docs/evidence/2564/pre-ready-2564.json` |

账本：`docs/evidence/2564/evidence.json`（schema v1，`dirty=false`，`commit=a30c6e250…`，四行各一条记录，`pass=true`）。

### G0 Golden Acceptance

| 行 | artifact | 关键断言（可重放） |
|---|---|---|
| R1 | `docs/evidence/2564/R1-consumer-wiring.txt` | `CONFLICT_BRANCH_IN_CATCH=1`、`CONFLICT_TEXT_FALLBACK=1`、`ALERT_RENDERED=1`、`CLEAR_ON_CHANGE=1`、`SHARED_REUSE=1`、`LOCAL_ALERT_DUP=0`、`FALLBACK_TOAST=1`、`R1_OK` |
| R2 | `docs/evidence/2564/R2-code-first.txt` | `CREATE_ACCEPTS_20100=1`、`UPDATE_CODE_GUARD=1`、`NONOK_RETURN_BEFORE_CLOSE=2`、`GREEN_CODE_FIRST=1`、`RED_SINGLE_CODE_CHECK=1`、`RED_NO_CATCH_BRANCH=1`、`AUTOSELECT_TOPLEVEL_ONLY=1`、`CREATED_SUBCATEGORY_KEEPS_PARENT=1`、`GUARD_MUTATION_DETECTED=1`、`RED_NO_AUTOSELECT_GUARD=1`、`R2_OK` |
| R3 | `docs/evidence/2564/R3-build-lint.txt` | `BUILD_EXIT=0`、`ESLINT_NO_UNDEF=0`、`R3_OK` |
| R4 | `docs/evidence/2564/pre-ready-2564.json` | `RESULT: PASS`、`jq -e '.overall == "PASS"'`、P12 四行、P13 已推送 |

### 矩阵 n/n

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 持久提示接线（catch code-first + 共享件 + 改输入清空） | PASS | `docs/evidence/2564/R1-consumer-wiring.txt` |
| R2 提交路径判码（create 接 20100 + update 失败不假成功）与红绿对照 | PASS | `docs/evidence/2564/R2-code-first.txt`（含 `AUTOSELECT_TOPLEVEL_ONLY=1` / `CREATED_SUBCATEGORY_KEEPS_PARENT=1` / `GUARD_MUTATION_DETECTED=1` / `RED_NO_AUTOSELECT_GUARD=1`） |
| R3 构建 + ESLint | PASS | `docs/evidence/2564/R3-build-lint.txt` |
| R4 pre-ready gate（四源一致 + 证据完整 + P13 已推送） | PASS | `docs/evidence/2564/pre-ready-2564.json` |

### 依赖 / 边界

- 前置 #2563 已 `status:done` 且合入 `origin/develop`（本 worktree 基线 `76a938bd0` 已含 `frontend-pc/src/services/apiError.js` 的 code 透传；D5 满足）。
- 本 Issue 未修改验收工具/规则；Out of Scope（传输层、Batch 2 其余页面 `#2486`、`handleDelete` 40003、后端错误码重构、PC 测试基建）保持未触碰。

### 人工/部署侧

- D4 的**真实渲染/点击回执**不在 Developer 可完成范围，已建跟踪 Issue #2572（`[Ops-Followup of #2564]`，label `waitForConfirm`，无 `status:*`）承载；主 Issue 验收独立判定。

### Pre-ready gate

- 命令（与计划 R4 合同逐字一致）：
  ```
  /home/coder/.config/opencode/scripts/pre-ready-check.sh 2564 \
    --summary-file docs/evidence/2564/summary.md \
    --changed-files docs/evidence/2564/changed-files.txt \
    --evidence-file docs/evidence/2564/evidence.json \
    --full-gate-cmd "cd frontend-pc && npm run build" \
    --report docs/evidence/2564/pre-ready-2564.json
  ```
- 报告：`docs/evidence/2564/pre-ready-2564.json`；`RESULT: PASS`（P1/P2/P3/P6/P11/P12/P13/P14/P16/P17/P18 全 PASS；P15/P7/P8/P9/P10 为 N/A）。
- P5 为 `N/A (sub-task: full gate belongs to the parent/batch gate)`：本 Issue body 含 `Parent: #2563`，按 Issue #70 分层口径，全量 gate 由父任务 #2563 承担（其 gate 已 PASS 并合入 `origin/develop`）。
- P4 为 `WARN (no --affected-cmd supplied)`：计划 R4 命令未包含 `--affected-cmd`（本单为静态接线 + 构建，无独立测试套件），定向构建/静态断言由 R1–R3 承担。

*Model: DeepSeek V4.1 Flash 1017*
