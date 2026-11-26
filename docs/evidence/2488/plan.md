## Accepted Plan — #2488 v4（内容/展示类页面 + **传输层 ApiError** + 真实契约 fixture）｜Rev.5

> **v4 说明（采纳审计 O1）**：① `### Files` 增补 **`frontend-pc/src/services/api.js`**（传输层）与 `docs/evidence/2488/plan.md`；② 新增 **D7 `ApiError{code,message,data}`** 契约——非 2xx（除 404/409 的既有对象返回）抛出携带 `code` 的错误，否则 banner/mall 的 `HTTP 400 + 40002` 永远无法被页面判定（R2/R3 不可达）；③ R2/R3 fixture 改为**真实契约**（`HTTP 400 + 40002`），R4 改为 `HTTP 409 + 40900`（丢失登记），**删除不存在的 `40005` fixture**；④ `/instruments/batch/price` 路由错误移出本单 → **#2514**。

> **v2 说明（与 #2489 同一计划缺口）**：本单正文首行 `Parent: #2486` → `pre-ready-check.sh` 判定为 **sub-task**，P5 恒为 `N/A`，故 R7 原写的 `FULL_SUITE_EXIT=0` 与负例 `P5=NA → FAIL` 互斥（计划/harness 冲突，非实现缺陷）。
> v2 裁定：R7 改为「本单直接执行全量 PC 构建并记录退出码」→ 断言 `GATE_RESULT=PASS` + `PLAN_SOURCE=PASS` + `FULL_GATE_DIRECT_EXIT=0`；负例改为 `FULL_GATE_DIRECT_EXIT!=0 → FAIL`；P5 的 N/A 由 D6 承接（父 #2486 收口复核）。

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

0. **传输层（新增）**：`frontend-pc/src/services/api.js` 的 `!response.ok` 分支——除既有 404/409 返回对象外，抛出的错误必须携带 `{ code, message, data }`（`ApiError`），保证页面能按 `code` 判定；不改 404/409 既有返回形状。

1. **判定表（只读）**：`scripts/scan-form-toasts-2488.sh` → `docs/evidence/2488/R1-inventory.txt`（逐条 `file:line` + tier + reason；`READONLY=1`）。
2. **code 判定回填**：`needs-code-first` 项先判 `resp.code`；命中 rubric → `setFormConflict(conflictText(...))` 持久；其它保持 toast。
3. **接入共享件**：3 文件渲染 `<FormConflictAlert text={formConflict} />`，清理沿用 #2480 D8 四处约定。
4. `scripts/verify-2488.sh`：7 行证据生成器。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 判定表（只读）+ **可达性列**（`evidence_type: static`，Owner 豁免见 D5） | `scripts/scan-form-toasts-2488.sh` | `bash scripts/scan-form-toasts-2488.sh` | 修后树 | PC | 3 文件每个 `message.error(` 1 行含 tier/reason **且 `reachable=` 列（该错误路径是否由页面真实控件触发：`yes/no`）**；`UNCLASSIFIED=0`、`READONLY=1`、`UNREACHABLE_CLAIMED=0` | R1 |
| R2 轮播配置页 | `frontend-pc/src/pages/System/BannerManagePage.jsx`（+ `services/api.js`） | 轮播新增/编辑保存 | **HTTP 400 + 40002**（经 ApiError 携带 code） | PC | 命中 rubric → 持久提示；`CODE_FIRST=1`、`API_ERROR_CODE=1`、`CATCH_MESSAGE=1`（catch 分支保留服务端 message 原文）、`CLEAR_FIELDS=1`（成功/切换行时清空上次持久提示） | R2 |
| R3 商城图片页 | `frontend-pc/src/pages/System/MallImageManageCard.jsx`（+ `services/api.js`） | 商城图片保存/上传 | **HTTP 400 + 40002** | PC | 同上；`API_ERROR_CODE=1`、`CATCH_MESSAGE=1`、`CLEAR_FIELDS=1` | R3 |
| R4 库存操作页 | `frontend-pc/src/pages/InstrumentStock.jsx` | 丢失登记/库存冲突 | **HTTP 409 + 40900** | PC | 同上；冲突不得被误判为瞬时（`STATE_CONFLICT=1`）、`CLEAR_FIELDS=1`；`/instruments/batch/price` 路由问题移至 #2514，不在本行断言 | R4 |
| R5 共享件复用与残留 | 3 文件 + `frontend-pc/src/components/FormConflictAlert.jsx` | `bash scripts/verify-2488.sh r5` | 修后树 | PC | `SHARED_REUSE=3`、`LOCAL_ALERT_DUP=0`、`LOCAL_CODE_LIST=0`、`BUSINESS_TOAST_RESIDUAL=0`、`RESIDUAL_INJECT=1`（向 3 文件注入一条旧式 `message.error` 后，探针必须报 `BUSINESS_TOAST_RESIDUAL=1`，证明探针可证伪）、`RESIDUAL_PREDICATE_SPLIT=1`（rubric 分类与 toast 残留两类谓词分开） | R5 |
| R6 构建与 lint | `frontend-pc` | `cd frontend-pc && npm run build` + `npx eslint` 改动文件 | 修后树 | PC | `PC_BUILD=0`、`LINT_EXIT=0` | R6 |
| R7 gate 四源 + 全量 PC 构建（子任务直跑） | `docs/evidence/2488/pre-ready-2488.json`、`docs/evidence/2488/plan.md`、`scripts/verify-2488.sh` | `bash scripts/verify-2488.sh r7`（内部直接 `cd frontend-pc && npm run build` 并记录退出码）+ `plan-source.sh --issue 2488` | 修后树（干净）；子任务 P5 按 harness 为 N/A | PC | `GATE_RESULT=PASS`、`PLAN_SOURCE=PASS`、`FULL_GATE_DIRECT_EXIT=0`；P5 的 N/A 由 D6 承接 | R7 |

## Input Truth & Tool Preflight

- **Source of truth**：#2480 rubric；`InstrumentStock` 的 40005 语义以 `backend/handlers/instrument*.go` 实际返回码为准（实现前 `grep -n "40005\|40900" backend/handlers/instrument*.go` 核对）。
- **差异核对**：本批 3 文件与 #2480/#2485/#2487/#2489 清单交集为空。
- **损坏输入拒绝**：`resp` 非对象 → 瞬时；`message` 非字符串 → 页面兜底；空 `text` → 组件返回 `null`。
- **工具盲区**：PC 无组件测试运行器 → 静态接线断言；真实截图由 `[Ops-Followup of #2488]` 承载；扫描脚本文本级分类，`ambiguous` 必须人工落 `reason`。
- **影响面预检**：不改 `40901/40902` 的 `Modal` 分支；不改上传/库存 API 与权限门控；不改 `message.success`。
- **gate 语义基线（D6）**：`pre-ready-check.sh:1092-1098` 对 `Parent:` 子任务固定 `na P5`，故 R7 的可执行断言为 `GATE_RESULT=PASS` + `PLAN_SOURCE=PASS` + 本单直跑 `FULL_GATE_DIRECT_EXIT=0`。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 范围 = 本批 3 文件，不跨批 | Architect | 本计划锁定 | 阻塞实现 |
| D2 | 先拿回 `code` 再分类 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 共享件唯一 owner（#2480），禁止自写 Alert / 复制白名单 | Architect | 本计划锁定 | 阻塞实现 |
| D4 | `InstrumentStock` 的可纠正冲突以**真实可失败入口**为准：`HTTP 409 + 40900`（丢失登记/冲突）；**不再使用不存在的 `40005` fixture** | Architect | 本计划锁定（v4 修正） | 阻塞实现 |
| D7 | 传输层契约 | `api.js` 非 2xx 抛出 `ApiError{code,message,data}`（404/409 既有对象返回不变）；页面按 `code` 判定后才能接 `FormConflictAlert` | Architect | 本计划锁定（v4 新增） | 阻塞实现 |
| D8 | 路由缺陷边界 | `/instruments/batch/price` 恒 404 属独立缺陷 → **#2514**，不在本单 R4 断言 | Architect | 本计划锁定 | — |
| D5 | UI 行证据 = 静态接线断言；真实渲染由 Ops 截图回评（**Owner 豁免**，`evidence_type: static`） | Architect | owner_approved（沿用本仓前端豁免惯例） | 若审计要求 render → 另立前端测试基建 Issue |
| D6 | **子任务 gate 语义**：`Parent: #2486` → P5 判为 `N/A`；R7 不得要求 `P5=PASS`/`FULL_SUITE_EXIT=0`，全量 PC 构建由 `scripts/verify-2488.sh r7` 直接执行并记录 `FULL_GATE_DIRECT_EXIT`，批次全量由父任务 #2486 收口复核 | Architect | owner_approved（与 harness 子任务规则一致） | 若 Owner 要求子任务也必须有 P5=PASS → 需先改 harness（另立 Issue），本单阻塞 |
| D7 | 不做 `### Batch Gate Reference` 登记：P15 需父任务 gate 产物存在（#2486 收口时才产出），改由 D6 双轨覆盖 | Architect | 本计划锁定 | 若审计坚持 P15 引用 → 等 #2486 收口后补登 |

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
| R2 | fake `HTTP 400 + {code:40002}`（轮播参数/重名） | 轮播保存 | `docs/evidence/2488/R2-banner.txt` | `PERSIST_ALERT=1`、`CODE_FIRST=1`、`API_ERROR_CODE=1`、`CATCH_MESSAGE=1`、`CLEAR_FIELDS=1` | 仍 toast、拿不到 code、丢失服务端 message 或未清空旧提示 → FAIL |
| R3 | fake `40004`（重复图片配置） | 商城图片保存 | `docs/evidence/2488/R3-mall-image.txt` | `PERSIST_ALERT=1`、`CODE_FIRST=1`、`API_ERROR_CODE=1`、`CATCH_MESSAGE=1`、`CLEAR_FIELDS=1` | 同上 → FAIL |
| R4 | fake `HTTP 409 + {code:40900}`（丢失登记/冲突） | 库存操作 | `docs/evidence/2488/R4-stock.txt` | `PERSIST_ALERT=1`、`STATE_CONFLICT=1` | 409 仍 toast → FAIL |
| R5 | 修后源码 | `bash scripts/verify-2488.sh r5` | `docs/evidence/2488/R5-residual.txt` | `SHARED_REUSE=3`、`LOCAL_ALERT_DUP=0`、`BUSINESS_TOAST_RESIDUAL=0` | 残留 ≥1 → FAIL |
| R6 | 修后 `frontend-pc` | `npm run build` + `npx eslint` | `docs/evidence/2488/R6-build-lint.txt` | `PC_BUILD=0`、`LINT_EXIT=0` | 任一非 0 → FAIL |
| R7 | 修后干净树 + 本单直跑全量 PC 构建 + gate 报告 | `bash scripts/verify-2488.sh r7` | `docs/evidence/2488/R7-gate.txt` | `GATE_RESULT=PASS`、`PLAN_SOURCE=PASS`、`FULL_GATE_DIRECT_EXIT=0` | `FULL_GATE_DIRECT_EXIT!=0` 或 `GATE_RESULT!=PASS` → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2488.sh r1` | `docs/evidence/2488/R1-inventory.txt` | exit 0；grep `UNCLASSIFIED=0`、`READONLY=1` | PASS | 未分类 → FAIL | dev |
| R2 | `bash scripts/verify-2488.sh r2` | `docs/evidence/2488/R2-banner.txt` | exit 0；grep `PERSIST_ALERT=1`、`CODE_FIRST=1` | PASS | 仍 toast → FAIL | dev |
| R3 | `bash scripts/verify-2488.sh r3` | `docs/evidence/2488/R3-mall-image.txt` | exit 0；grep `PERSIST_ALERT=1`、`CODE_FIRST=1` | PASS | 仍 toast → FAIL | dev |
| R4 | `bash scripts/verify-2488.sh r4` | `docs/evidence/2488/R4-stock.txt` | exit 0；grep `PERSIST_ALERT=1`、`STATE_CONFLICT=1` | PASS | 409 仍 toast → FAIL | dev |
| R5 | `bash scripts/verify-2488.sh r5` | `docs/evidence/2488/R5-residual.txt` | exit 0；grep `SHARED_REUSE=3`、`BUSINESS_TOAST_RESIDUAL=0` | PASS | 残留 ≥1 → FAIL | dev |
| R6 | `bash scripts/verify-2488.sh r6` | `docs/evidence/2488/R6-build-lint.txt` | exit 0；grep `PC_BUILD=0`、`LINT_EXIT=0` | PASS | 任一非 0 → FAIL | dev |
| R7 | `bash scripts/verify-2488.sh r7` | `docs/evidence/2488/R7-gate.txt` | exit 0；grep `GATE_RESULT=PASS`、`PLAN_SOURCE=PASS`、`FULL_GATE_DIRECT_EXIT=0` | PASS | `FULL_GATE_DIRECT_EXIT!=0` → FAIL | dev |

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

- `frontend-pc/src/services/api.js`
- `docs/evidence/2488/plan.md`

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




