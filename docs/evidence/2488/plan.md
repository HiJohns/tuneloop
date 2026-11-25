## Accepted Plan — #2488（内容/展示类页面：业务可纠正错误 → 持久提示）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 共享件与 rubric 由 #2480 交付（消费，不重写） | `frontend-pc/src/components/FormConflictAlert.jsx`、`frontend-pc/src/utils/formConflict.js` |
| 父任务拆分 | #2486（accepted）`## Sub-tasks` |
| 本批 3 文件与 toast 数（初扫） | `System/BannerManagePage.jsx`(11)、`System/MallImageManageCard.jsx`(11)、`InstrumentStock.jsx`(11) |
| 多数为 catch-all（`err.message`）或丢弃 `code` | 同批文件（`grep -n "message.error" -B2`） |
| `InstrumentStock.jsx` 的「库存状态冲突」需先确认后端返回码（可能含 `40005` 状态不允许） | 后端 `handlers/instrument*.go` 的 40005/40900 使用点 |

**结论**：与 Batch 1 同法 —— 先拿回 `code` 再分类；本批只覆盖 3 文件（与 #2485/#2487/#2489 互斥）。

### 变更点

1. **判定表（只读）**：`scripts/scan-form-toasts-2488.sh` → `docs/evidence/2488/R1-inventory.txt`（逐条 `file:line` + tier + reason；`READONLY=1`）。
2. **code 判定回填**：`needs-code-first` 项先判 `resp.code`；命中 rubric → `setFormConflict(conflictText(...))` 持久；其它保持 toast。
3. **接入共享件**：3 文件渲染 `<FormConflictAlert text={formConflict} />`，清理沿用 #2480 D8 四处约定。
4. `scripts/verify-2488.sh`：7 行证据生成器。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 判定表（只读） | `scripts/scan-form-toasts-2488.sh` | `bash scripts/scan-form-toasts-2488.sh` | 修后树 | PC | 3 文件每个 `message.error(` 1 行含 tier/reason；`UNCLASSIFIED=0`、`READONLY=1` | R1 |
| R2 轮播配置页 | `frontend-pc/src/pages/System/BannerManagePage.jsx` | 轮播新增/编辑保存 | `40002/40004/40900` | PC | 命中 rubric → 持久提示；`CODE_FIRST=1` | R2 |
| R3 商城图片页 | `frontend-pc/src/pages/System/MallImageManageCard.jsx` | 商城图片保存/上传 | `40002/40004/40900` | PC | 同上 | R3 |
| R4 库存操作页 | `frontend-pc/src/pages/InstrumentStock.jsx` | 库存调整/状态变更 | `40005/40900`（状态不允许/冲突） | PC | 同上；状态冲突不得被误判为瞬时（`STATE_CONFLICT=1`） | R4 |
| R5 共享件复用与残留 | 3 文件 + `frontend-pc/src/components/FormConflictAlert.jsx` | `bash scripts/verify-2488.sh r5` | 修后树 | PC | `SHARED_REUSE=3`、`LOCAL_ALERT_DUP=0`、`LOCAL_CODE_LIST=0`、`BUSINESS_TOAST_RESIDUAL=0` | R5 |
| R6 构建与 lint | `frontend-pc` | `cd frontend-pc && npm run build` + `npx eslint` 改动文件 | 修后树 | PC | `PC_BUILD=0`、`LINT_EXIT=0` | R6 |
| R7 gate 四源 + 全量 PC 构建 | `docs/evidence/2488/pre-ready-2488.json`、`docs/evidence/2488/plan.md` | `--full-gate-cmd "cd frontend-pc && npm run build"` + `plan-source.sh --issue 2488` | 修后树（干净） | PC | `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | R7 |

## Input Truth & Tool Preflight

- **Source of truth**：#2480 rubric；`InstrumentStock` 的 40005 语义以 `backend/handlers/instrument*.go` 实际返回码为准（实现前 `grep -n "40005\|40900" backend/handlers/instrument*.go` 核对）。
- **差异核对**：本批 3 文件与 #2480/#2485/#2487/#2489 清单交集为空。
- **损坏输入拒绝**：`resp` 非对象 → 瞬时；`message` 非字符串 → 页面兜底；空 `text` → 组件返回 `null`。
- **工具盲区**：PC 无组件测试运行器 → 静态接线断言；真实截图由 `[Ops-Followup of #2488]` 承载；扫描脚本文本级分类，`ambiguous` 必须人工落 `reason`。
- **影响面预检**：不改 `40901/40902` 的 `Modal` 分支；不改上传/库存 API 与权限门控；不改 `message.success`。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 范围 = 本批 3 文件，不跨批 | Architect | 本计划锁定 | 阻塞实现 |
| D2 | 先拿回 `code` 再分类 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 共享件唯一 owner（#2480），禁止自写 Alert / 复制白名单 | Architect | 本计划锁定 | 阻塞实现 |
| D4 | `InstrumentStock` 的「状态不允许（40005）」属**业务可纠正**（用户可改数量/先归还再操作），必须持久提示 | Architect | 本计划锁定 | 阻塞实现 |
| D5 | UI 行证据 = 静态接线断言；真实渲染由 Ops 截图回评（**Owner 豁免**，`evidence_type: static`） | Architect | owner_approved（沿用本仓前端豁免惯例） | 若审计要求 render → 另立前端测试基建 Issue |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R2 | static | 轮播配置页接线结构断言，Owner 豁免见 D5 |
| R3 | static | 商城图片页接线结构断言，Owner 豁免见 D5 |
| R4 | static | 库存操作页接线结构断言，Owner 豁免见 D5 |
| R5 | static | 复用/残留度量（grep），Owner 豁免见 D5 |

## Out of Scope

- 其余批量（#2485、#2487、#2489）。
- 移动端 `dialog.alert` 形态。
- 后端返回码/文案本身；上传组件行为。
- 全局 toast 时长配置；PC 组件测试基建。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 修后 3 文件 + 扫描脚本 | `bash scripts/scan-form-toasts-2488.sh` | `docs/evidence/2488/R1-inventory.txt` | 每处 1 行；`UNCLASSIFIED=0`；`READONLY=1` | 未分类 → FAIL |
| R2 | fake `40900`（轮播重名） | 轮播保存 | `docs/evidence/2488/R2-banner.txt` | `PERSIST_ALERT=1`、`CODE_FIRST=1` | 仍 toast → FAIL |
| R3 | fake `40004`（重复图片配置） | 商城图片保存 | `docs/evidence/2488/R3-mall-image.txt` | 同上 | 同上 → FAIL |
| R4 | fake `40005`（状态不允许） | 库存操作 | `docs/evidence/2488/R4-stock.txt` | `PERSIST_ALERT=1`、`STATE_CONFLICT=1` | 40005 仍 toast → FAIL |
| R5 | 修后源码 | `bash scripts/verify-2488.sh r5` | `docs/evidence/2488/R5-residual.txt` | `SHARED_REUSE=3`、`LOCAL_ALERT_DUP=0`、`BUSINESS_TOAST_RESIDUAL=0` | 残留 ≥1 → FAIL |
| R6 | 修后 `frontend-pc` | `npm run build` + `npx eslint` | `docs/evidence/2488/R6-build-lint.txt` | `PC_BUILD=0`、`LINT_EXIT=0` | 任一非 0 → FAIL |
| R7 | 修后干净树 + gate | 读校验 | `docs/evidence/2488/R7-gate.txt` | `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | `P5=NA` → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2488.sh r1` | `docs/evidence/2488/R1-inventory.txt` | exit 0；grep `UNCLASSIFIED=0`、`READONLY=1` | PASS | 未分类 → FAIL | dev |
| R2 | `bash scripts/verify-2488.sh r2` | `docs/evidence/2488/R2-banner.txt` | exit 0；grep `PERSIST_ALERT=1`、`CODE_FIRST=1` | PASS | 仍 toast → FAIL | dev |
| R3 | `bash scripts/verify-2488.sh r3` | `docs/evidence/2488/R3-mall-image.txt` | exit 0；grep `PERSIST_ALERT=1`、`CODE_FIRST=1` | PASS | 仍 toast → FAIL | dev |
| R4 | `bash scripts/verify-2488.sh r4` | `docs/evidence/2488/R4-stock.txt` | exit 0；grep `PERSIST_ALERT=1`、`STATE_CONFLICT=1` | PASS | 40005 仍 toast → FAIL | dev |
| R5 | `bash scripts/verify-2488.sh r5` | `docs/evidence/2488/R5-residual.txt` | exit 0；grep `SHARED_REUSE=3`、`BUSINESS_TOAST_RESIDUAL=0` | PASS | 残留 ≥1 → FAIL | dev |
| R6 | `bash scripts/verify-2488.sh r6` | `docs/evidence/2488/R6-build-lint.txt` | exit 0；grep `PC_BUILD=0`、`LINT_EXIT=0` | PASS | 任一非 0 → FAIL | dev |
| R7 | `bash scripts/verify-2488.sh r7` | `docs/evidence/2488/R7-gate.txt` | exit 0；grep `GATE_RESULT=PASS`、`PLAN_SOURCE=PASS` | PASS | `P5=NA` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2488/R1-inventory.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2488/R2-banner.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2488/R3-mall-image.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2488/R4-stock.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2488/R5-residual.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2488/R6-build-lint.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2488/R7-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 | 触发条件 | 根因 | 修复点 | 覆盖行 | 负例断言 |
|---|---|---|---|---|---|---|
| B1 | 内容配置冲突只显示 3 秒 toast | 重名/重复配置 | 瞬时提示承载可纠正错误 | 变更点 2/3 | R2/R3 | 仍 toast → 失败 |
| B2 | 库存状态冲突被当瞬时错误 | 状态前置不满足 | 未读 code | 变更点 2 + D4 | R4 | 40005 仍 toast → R4 失败 |
| B3 | 丢弃 `code` 无法分类 | catch-all | 未保留响应码 | 变更点 2 | R1 | `UNCLASSIFIED>0` → R1 失败 |
| B4 | 各自实现 Alert | 多页面 | 无共享件 | 变更点 3 | R5 | `LOCAL_ALERT_DUP>0` → R5 失败 |
| B5 | 提示残留 | 改输入后 | 无清理约定 | 变更点 3 | R2–R4 | 残留 → 失败 |
| B6 | 瞬时错误常驻 | 网络/权限 | 分类过宽 | 变更点 2 | R1/R5 | 网络错误走 Alert → 失败 |
| B7 | 跨批重复转换 | 并行批次 | 清单未互斥 | Input Truth | R5 | 交集非空 → R5 失败 |
| B8 | 上传失败被当业务冲突 | 文件过大/类型非法 | 40002 与上传错误混用 | 变更点 2 | R3 | 上传失败走 Alert → R3 失败 |

### Workload

- Size: M
- Basis: 3 个前端文件接线改造 + 1 个扫描脚本 + 1 个 verify 脚本 + 7 条证据行；复用 #2480 共享件；无后端、无 migration
- Confidence: medium（库存页分类需核对后端码）

### Files（变更文件清单）

- `frontend-pc/src/components/FormConflictAlert.jsx`
- `frontend-pc/src/utils/formConflict.js`
- `frontend-pc/src/pages/System/BannerManagePage.jsx`
- `frontend-pc/src/pages/System/MallImageManageCard.jsx`
- `frontend-pc/src/pages/InstrumentStock.jsx`
- `scripts/scan-form-toasts-2488.sh`
- `scripts/verify-2488.sh`
- `docs/evidence/2488/plan.md`
- `docs/evidence/2488/pre-ready-2488.json`
- `docs/evidence/2488/R1-inventory.txt`
- `docs/evidence/2488/R2-banner.txt`
- `docs/evidence/2488/R3-mall-image.txt`
- `docs/evidence/2488/R4-stock.txt`
- `docs/evidence/2488/R5-residual.txt`
- `docs/evidence/2488/R6-build-lint.txt`
- `docs/evidence/2488/R7-gate.txt`
- `docs/evidence/2488/evidence.json`

## Dependencies

- #2480

### 依赖与关联说明（非硬依赖）

- **#2480（accepted，硬前置）**：共享件 + rubric；未合入前 `/work` HOLD。
- **#2486（accepted）**：父任务；本单为其子任务。
- **部署侧**：预生产截图回评由 `[Ops-Followup of #2488]` 承载。

*Model: DeepSeek V4.1 Flash 1017*

