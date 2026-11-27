## Work Summary — #2564（分类页 40002 重名恢复表单内持久提示 + 提交路径判码去假成功）

### 修改内容

#### Round 3（审计「计划层」驳回 → Accepted Plan Rev.4 逐字对齐）

Round 3 审计的 4 条 delta 全部属于**验收文本/证据记录层**（产品代码已被确认正确），本轮按 Rev.4 **逐字落地**，`frontend-pc/src/pages/admin/category/List.jsx` **零改动**：

- **delta 1（红样锚点）**：`scripts/verify-2564.sh` 的 r2 红样仍为**动态推导**——「本 Issue 第一个 `fix(issue-2564)` 且触及 List.jsx 的 commit 的父提交」（`git log --format=%H --grep="^fix(issue-2564" --reverse -- "$SRC" | head -1` 取 `^`），**不手写 sha**，也**不再**使用 `git log -1 -- "$SRC"` 口径（第二次 fix 后该口径会前移到上一版绿样，红绿对照退化为「绿 vs 绿」）。产物输出 RED_SAMPLE 行（`frontend-pc/src/pages/admin/category/List.jsx` 的红样）。
- **delta 2（夹具锚点）**：G0 R2 的夹具改为可执行锚点——`finalParentId` 计算块 + `if (response.data?.id && !finalParentId)` 守卫；脚本按正则定位并把**实际行号**写入产物（`ANCHOR_FINAL_PARENT_BLOCK=245-256`、`ANCHOR_CREATE_GUARD_LINE=281`），并新增 `ANCHOR_FINAL_PARENT_PRESENT=1` / `ANCHOR_CREATE_GUARD_PRESENT=1`（锚点缺失即 R2_FAIL）。不再引用「上层分类选择器默认值」这类页面上不存在的控件。
- **delta 3（合同三处一致）**：`docs/evidence/2564/evidence.json` 的 R2 `expected` 逐字补登 `AUTOSELECT_TOPLEVEL_ONLY=1`、`CREATED_SUBCATEGORY_KEEPS_PARENT=1`、`GUARD_MUTATION_DETECTED=1`（含变异负例说明）、`RED_NO_AUTOSELECT_GUARD=1`，与矩阵/G0/合同三处一致。
- **delta 4（R4 命令与实跑一致）**：R4 合同命令登记 `--affected-cmd "cd frontend-pc && npx eslint src/pages/admin/category/List.jsx"`，本报告 Pre-ready 段落的命令即该登记命令逐字；断言键同步为 `jq -e '.overall == "PASS"'`（旧 .result 键已删除）。

#### Round 2（审计 Bug 1 修复）

- `frontend-pc/src/pages/admin/category/List.jsx`（`handleFormSubmit` 创建分支）
  - 创建成功后**仅顶级创建**自动选中新分类：`if (response.data?.id && !finalParentId)`。
  - 子分类创建（`finalParentId` = 父分类 id）保持父分类选中——新子分类随即出现在父的子分类列表。
  - 修复前把 level-2 id 写入 `selectedParentId`：该 id 不在 `level1Categories`（`GET /api/categories` 只返回一级 + 嵌套 `sub_categories`）→ 右侧面板清空、父分类名渲染为空串、点击「编辑」时传入 `undefined` 触发 TypeError，且「创建子分类」会以该 level-2 id 再建一层不可见的孙级节点。
- `scripts/verify-2564.sh`
  - r2 新增可执行断言：`AUTOSELECT_TOPLEVEL_ONLY=1`、`CREATED_SUBCATEGORY_KEEPS_PARENT=1`、`GUARD_MUTATION_DETECTED=1`（变异负例：删掉 `&& !finalParentId` 后守卫断言必须归零）、`RED_NO_AUTOSELECT_GUARD=1`（红样无该守卫）。
  - 红样锚点改为「本 Issue 第一个页面 fix commit 的父提交」，仍为动态推导。

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
- `scripts/verify-2564.sh`
- `docs/evidence/2564/plan.md`（Rev.4 快照）
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
- fix（Round 3 delta 2：r2 夹具锚点行号写入产物）：`ed9ffd279`
- evidence：见 Remote 行（`docs(evidence): issue-2564`）

### 验证层级

- **子任务**（Issue body 含 `Parent: #2563`）：本单只跑**定向**命令——P4 `cd frontend-pc && npx eslint src/pages/admin/category/List.jsx`（改动文件静态检查）+ R3 `cd frontend-pc && npm run build`（PC 构建）；**全量 gate 由父任务 #2563 / 批次承担**（父任务 #2563 的 gate 报告已 PASS 且已合入 `origin/develop`），P5 因此为 `N/A (sub-task)`。
- 证据形态：PC 无组件测试运行器，R1/R2 为**静态结构断言**（D4 Owner 豁免），真实点击/渲染回执由跟踪 Issue #2572 承载。
- 无后端 / 无 migration / 无新增依赖。

### Remote

- remote: `origin/issue-2564`；fix `5cd3c900a` / `5463d3d98` / `869bd336c` / `a30c6e250` / `ed9ffd279` pushed=yes；evidence `docs(evidence): issue-2564` pushed=yes（gate 前完成 push）。

### 红绿实证

- 红样锚点 = 本 Issue 第一个页面 fix commit 的父提交（`git log --format=%H --grep="^fix(issue-2564" --reverse -- frontend-pc/src/pages/admin/category/List.jsx | head -1`，自动取，不手写 sha）→ `5cd3c900a^`：只有 `response.code === 20000`（无 `20100`）、`catch` 无任何 code 判定、`setSelectedParentId` 无 `finalParentId` 守卫。
- 绿样（当前工作树）：`CREATE_ACCEPTS_20100=1`、`UPDATE_CODE_GUARD=1`、`NONOK_RETURN_BEFORE_CLOSE=2`、`GREEN_CODE_FIRST=1`、`RED_SINGLE_CODE_CHECK=1`、`RED_NO_CATCH_BRANCH=1`、`ANCHOR_FINAL_PARENT_PRESENT=1`、`ANCHOR_CREATE_GUARD_PRESENT=1`、`AUTOSELECT_TOPLEVEL_ONLY=1`、`CREATED_SUBCATEGORY_KEEPS_PARENT=1`、`GUARD_MUTATION_DETECTED=1`、`RED_NO_AUTOSELECT_GUARD=1`。
- 复现：`bash scripts/verify-2564.sh r2`；红绿对照样本路径见产物 `RED_SAMPLE=` 行，夹具行号见 `ANCHOR_*` 行，计数见 `CREATE_BLOCK_AUTOSELECT:` 行（`total=1 guarded=1 mutation_guarded=0 red_total=1 red_guarded=0`）。

### 审计对照（Round 1/2/3 → 修复）

| 审计项 | 矩阵行 | 修复后证据 |
|---|---|---|
| Round 1 Bug 1：子分类创建成功后把 level-2 id 写入 `selectedParentId`（右侧面板清空 + 「编辑」TypeError + 可建不可见孙级） | R2 | 创建分支增加 `!finalParentId` 守卫；`AUTOSELECT_TOPLEVEL_ONLY=1`、`CREATED_SUBCATEGORY_KEEPS_PARENT=1`、`GUARD_MUTATION_DETECTED=1`、`RED_NO_AUTOSELECT_GUARD=1` |
| Round 2 Bug 1：R4 断言引用不存在的 .result 键 | R4 | evidence.json R4 `assert` 改为 `jq -e '.overall == "PASS"'` + `GATE_RESULT=PASS`；报告实际键为 `overall` |
| Round 2 Bug 2 / Round 3 P1：R2 红样锚点按 `git log -1` 口径会取到上一版绿样 | R2 | 红样锚点改为「首个 `fix(issue-2564)` 命中该文件的 commit 的父提交」动态推导；产物 `RED_SAMPLE=5cd3c900a^` |
| Round 3 P2：G0 R2 夹具指向不存在的「上层分类选择器」 | R2 | 夹具改为可执行锚点：`finalParentId` 计算块 + `!finalParentId` 守卫；产物写入 `ANCHOR_FINAL_PARENT_BLOCK=245-256`、`ANCHOR_CREATE_GUARD_LINE=281` 与两项 `ANCHOR_*_PRESENT=1` |
| Round 3 P3：4 个新判定键未进 Evidence Contract / evidence.json | R2 | evidence.json R2 `expected` 逐字补登 4 键（含 `GUARD_MUTATION_DETECTED` 变异负例说明），矩阵/G0/合同三处一致 |
| Round 3 P4：R4 记录的命令与实际运行不一致（实际带 `--affected-cmd`） | R4 | R4 合同命令登记 `--affected-cmd "cd frontend-pc && npx eslint src/pages/admin/category/List.jsx"`；本节命令逐字一致，P4 实跑 PASS |
| Round 3 Low：`验证层级` 误写「独立 Issue」 | — | 本节改为「子任务（`Parent: #2563`）：定向 eslint + PC build，全量 gate 归父/批次」 |

### Accepted Plan 对照（4/4 变更点）

1. catch code-first + 共享 Alert + 改输入清空 → 完成（R1 七项标记全绿）。
2. 更新分支判码、创建分支接受 20100、失败不假成功、创建成功后仅顶级自动选中 → 完成（R2 十二项标记 + 锚点 + 红绿/变异对照）。
3. `scripts/verify-2564.sh` 证据生成器 → 完成（含 Rev.4 夹具锚点行号）。
4. 构建 + eslint + gate → 完成（R3 / R4）。

### Evidence Contract（4/4 PASS）

| 行 | 结果 | 产物 |
|---|---|---|
| R1 | PASS | `docs/evidence/2564/R1-consumer-wiring.txt` |
| R2 | PASS | `docs/evidence/2564/R2-code-first.txt` |
| R3 | PASS | `docs/evidence/2564/R3-build-lint.txt` |
| R4 | PASS | `docs/evidence/2564/pre-ready-2564.json` |

账本：`docs/evidence/2564/evidence.json`（schema v1，`dirty=false`，`commit=ed9ffd279…`，`plan_sha256=d78df1b8…`，四行各一条记录，`pass=true`）。

### G0 Golden Acceptance

| 行 | artifact | 关键断言（可重放） |
|---|---|---|
| R1 | `docs/evidence/2564/R1-consumer-wiring.txt` | `CONFLICT_BRANCH_IN_CATCH=1`、`CONFLICT_TEXT_FALLBACK=1`、`ALERT_RENDERED=1`、`CLEAR_ON_CHANGE=1`、`SHARED_REUSE=1`、`LOCAL_ALERT_DUP=0`、`FALLBACK_TOAST=1`、`R1_OK` |
| R2 | `docs/evidence/2564/R2-code-first.txt` | `CREATE_ACCEPTS_20100=1`、`UPDATE_CODE_GUARD=1`、`NONOK_RETURN_BEFORE_CLOSE=2`、`GREEN_CODE_FIRST=1`、`RED_SINGLE_CODE_CHECK=1`、`RED_NO_CATCH_BRANCH=1`、`ANCHOR_FINAL_PARENT_BLOCK=245-256`、`ANCHOR_CREATE_GUARD_LINE=281`、`ANCHOR_FINAL_PARENT_PRESENT=1`、`ANCHOR_CREATE_GUARD_PRESENT=1`、`AUTOSELECT_TOPLEVEL_ONLY=1`、`CREATED_SUBCATEGORY_KEEPS_PARENT=1`、`GUARD_MUTATION_DETECTED=1`、`RED_NO_AUTOSELECT_GUARD=1`、`R2_OK` |
| R3 | `docs/evidence/2564/R3-build-lint.txt` | `BUILD_EXIT=0`、`ESLINT_NO_UNDEF=0`、`R3_OK` |
| R4 | `docs/evidence/2564/pre-ready-2564.json` | `RESULT: PASS`、`jq -e '.overall == "PASS"'`、P12 四行、P13 已推送 |

### 矩阵 n/n

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 持久提示接线（catch code-first + 共享件 + 改输入清空） | PASS | `docs/evidence/2564/R1-consumer-wiring.txt` |
| R2 提交路径判码（create 接 20100 + update 失败不假成功）与红绿对照 | PASS | `docs/evidence/2564/R2-code-first.txt`（含 `AUTOSELECT_TOPLEVEL_ONLY=1` / `CREATED_SUBCATEGORY_KEEPS_PARENT=1` / `GUARD_MUTATION_DETECTED=1` / `RED_NO_AUTOSELECT_GUARD=1` + 锚点行号） |
| R3 构建 + ESLint | PASS | `docs/evidence/2564/R3-build-lint.txt` |
| R4 pre-ready gate（四源一致 + 证据完整 + P13 已推送） | PASS | `docs/evidence/2564/pre-ready-2564.json` |

### 依赖 / 边界

- 前置 #2563 已 `status:done` 且合入 `origin/develop`（本 worktree 基线已含 `frontend-pc/src/services/apiError.js` 的 code 透传；D5 满足）。
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
    --affected-cmd "cd frontend-pc && npx eslint src/pages/admin/category/List.jsx" \
    --full-gate-cmd "cd frontend-pc && npm run build" \
    --report docs/evidence/2564/pre-ready-2564.json
  ```
- 报告：`docs/evidence/2564/pre-ready-2564.json`；`RESULT: PASS`（P1/P2/P3/P4/P6/P11/P12/P13/P14/P16/P17/P18 全 PASS；P15/P7/P8/P9/P10 为 N/A）。
- P4 为 `PASS: all 1 affected command(s) passed`（`cd frontend-pc && npx eslint src/pages/admin/category/List.jsx`，与登记命令逐字一致）。
- P5 为 `N/A (sub-task: full gate belongs to the parent/batch gate)`：本 Issue body 含 `Parent: #2563`，按 Issue #70 分层口径，全量 gate 由父任务 #2563 承担（其 gate 已 PASS 并合入 `origin/develop`）。

*Model: DeepSeek V4.1 Flash 1017*
