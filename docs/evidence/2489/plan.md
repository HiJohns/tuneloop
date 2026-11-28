## Accepted Plan — #2489 v4（真实契约 fixture + 传输层 ApiError + 分支级 R1）｜Rev.5｜Rev.7｜Rev.8（R8 语义定死：SHARED_REUSE=2）
> **Rev.8 说明（采纳 Investigation Packet O1）**：Rev.7 把 R2/R3/R5 逐行裁定为 **keep**（不接入常驻提示），但 R8 的 `SHARED_REUSE=5` 是上一轮的**修订残留**，与 `PERSIST_ALERT_UNUSED=1` 互斥（实测只有 `TransitCenter.jsx`(R4) 与 `MerchantManagement.jsx`(R6) 两页消费共享件）。
> 本版把 `SHARED_REUSE` **语义定死为「实际消费共享件的页面数」**，取 **2**；并新增 `KEEP_PAGES_NO_ALERT=3`，让 keep 裁定也变成可机器判定（3 个 keep 页确实未 import 共享件、无 `conflictText`/`isBusinessCorrectableConflict` 调用）。
> 不采纳 O2（token 名与语义不符，审计易按字面判 FAIL）与 O3（恢复三页 convert 会打挂 `FALSE_CONVERT=0`，正是本单要避免的反向缺陷）。

> **v4 说明（修 Round 2 审计 A–F / 1–7）**：① Files 增补 **`frontend-pc/src/services/api.js`** 并新增 **D9 传输层 `ApiError{code,message,data}`**（否则 400 族拿不到 code，本单全部 R2–R7 接线不可达）；② R2/R3/R4 改用**真实可失败契约**（商户重名 `409+40900`、定价 `400+40002`、中转 `400+40002`），删除虚构 fixture（权限页 40000/40300、会员门槛、`template not found` 误用）；③ **删除 R7（租户页）** —— `POST /system/tenants` 路由不存在 → **#2516**，本单仅保留「未判 `res.code` 不得假成功」防御断言；④ R1 改**按分支块**归类（禁 12 行窗口启发式）；⑤ 新增 **R11 传输层可执行断言**（400/409/500 三态）；⑥ D8 清空点补 `MemberPermissionEditModal`。

> **v2 说明（回应 Investigation Packet：R10 与子任务 gate 语义互斥）**：本单正文首行 `Parent: #2486` → `pre-ready-check.sh` 判定为 **sub-task**，P5 恒为 `N/A`（`sub-task: full gate belongs to the parent/batch gate`），因此 R10 原写的 `FULL_SUITE_EXIT=0` 与负例 `P5=NA → FAIL` **不可能同时成立**（计划/harness 冲突，非实现缺陷）。
> v2 裁定：**R10 改为「本单直接执行全量 PC 构建并记录退出码」**（Option B），负例改为 `FULL_GATE_DIRECT_EXIT!=0 → FAIL`；P5 的 `N/A` 由 D6 显式承接（父批次 #2486 收口时再跑一次批次 gate）。实现侧仅需扩展 `scripts/verify-2489.sh r10`，无需改业务代码。

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 共享件与 rubric 由 #2480 交付（消费，不重写）：`FormConflictAlert`、`isBusinessCorrectableConflict` / `conflictText` | `frontend-pc/src/components/FormConflictAlert.jsx`、`frontend-pc/src/utils/formConflict.js` |
| 父任务拆分与拓扑 | #2486（accepted）`## Sub-tasks` |
| 本批 5 文件与 toast 数（Rev.5：#2518 已删除租户页） | `PermissionManage/index.jsx`(8)、`pricing/MerchantPricingConfig.jsx`(7)、`transit/TransitCenter.jsx`(5)、`System/MembershipLevelsPage.jsx`(4)、`MerchantManagement.jsx`(4)；~~`TenantManagement.jsx`(2)~~ 已由 #2518 删除 |
| 多数调用点为 catch-all（`err.message`）或「先 `message.error(resp.message)` 丢弃 `code`」→ 无法按 rubric 区分 | 同批文件（`grep -n "message.error" -B2`） |

**结论**：与 Batch 1 同病 —— 先拿回 `code` 再分类转换；本单只覆盖上述 5 文件（与 #2485/#2487/#2488 文件集合互斥）。

### 变更点

0. **传输层（新增）**：`frontend-pc/src/services/api.js` 非 2xx 抛出 `ApiError{code,message,data}`（404/409 既有对象返回不变），否则 400+40002 的业务冲突在页面侧不可达。

1. **判定表（只读）**：`scripts/scan-form-toasts-2489.sh` → `docs/evidence/2489/R1-inventory.txt`，逐条列出本批 5 文件的每个 `message.error(`：`file:line`、所在函数、是否读 `code`、tier（`business-correctable` / `keep:instant` / `needs-code-first`）、`reason`；`READONLY=1`。
2. **code 判定回填**：`needs-code-first` 项先判 `resp.code`：
   - `isBusinessCorrectableConflict(resp)` → `setFormConflict(conflictText(resp, '<页面兜底>'))`（持久）；
   - 其它 → 维持 `message.error(...)`（瞬时）。
3. **接入共享件**：6 文件各自渲染 `<FormConflictAlert text={formConflict} />`，清理约定沿用 #2480 D8（字段编辑 / 提交开始 / 取消 / 成功）。
4. **商户重名对齐 #2459**：`MerchantManagement.jsx` 的 409 语义以 #2459（IAM 优先 + namespace 唯一 + 502 默认映射）为准；不得在页面里重写 code 白名单。
5. `scripts/verify-2489.sh`：10 行证据生成器。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 判定表（按**分支块**归类） | `scripts/scan-form-toasts-2489.sh` | `bash scripts/scan-form-toasts-2489.sh` | 修后树 | PC | 每个 `if/catch` **分支** 1 行（`file:line + branch + tier + reason + fn`）；`UNCLASSIFIED=0`、`WINDOW_HEURISTIC=0`、`READONLY=1` | R1 |
| R2 权限模板页（改为 keep 判定） | `frontend-pc/src/pages/admin/PermissionManage/index.jsx` | `/admin/permissions` 保存模板 | 实测仅 `40000/40300/50000`（参数/权限/5xx） | PC | 全部 `keep:instant`（非用户可纠正冲突）；`KEEP_ROWS=3`、`FALSE_CONVERT=0` | R2 |
| R3 价格配置页 | `frontend-pc/src/pages/admin/pricing/MerchantPricingConfig.jsx` | 商户价格配置保存 | `40001` / `40002`（**pricing template not found**）/ `40300` / `50000` | PC | 全部 `keep:instant`；`KEEP_ROWS=1`、`FALSE_CONVERT=0`、`PERSIST_ALERT_UNUSED=1` | R3 |
| R4 中转网点页 | `frontend-pc/src/pages/admin/transit/TransitCenter.jsx` | 中转网点新增/编辑 | `409 + 40900`（同名组织，真实可达） | PC | `PERSIST_ALERT=1`、`CODE_FIRST=1` | R4 |
| R5 会员等级页（改为 keep 判定） | `frontend-pc/src/pages/System/MembershipLevelsPage.jsx` | 会员等级新增/编辑 | 门槛冲突为**前端校验**（后端仅 JSON binding 40002） | PC | `keep:instant`；`FRONTEND_ONLY_RULE=1` | R5 |
| R6 商户管理页（真实契约） | `frontend-pc/src/pages/MerchantManagement.jsx` | 商户**新增**重名 | **HTTP 409 + 40900**（`merchant.go:348`） | PC | 停留表单 + `FormConflictAlert`；**不得出现「商户创建成功」**（`NO_FALSE_SUCCESS=1`）；改名行随 #2459 语义，标注 `DEFER_2459=1` | R6 |
| R8 共享件复用与残留 | 6 文件 + `frontend-pc/src/components/FormConflictAlert.jsx` | `bash scripts/verify-2489.sh r8` | 修后树 | PC | `SHARED_REUSE=2`、`KEEP_PAGES_NO_ALERT=3`、`LOCAL_ALERT_DUP=0`、`LOCAL_CODE_LIST=0`、`BUSINESS_TOAST_RESIDUAL=0` | R8 |
| R9 构建与 lint | `frontend-pc` | `cd frontend-pc && npm run build` + `npx eslint` 改动文件 | 修后树 | PC | `PC_BUILD=0`、`LINT_EXIT=0` | R9 |
| R11 传输层可执行断言（新增） | `frontend-pc/src/services/api.js` | `bash scripts/verify-2489.sh r11`（mock `fetch`） | `400+{code:40002}` / `409+{code:40900}` / 网络与 500 | PC | ① 400 → 抛出 error 携带 `code=40002`；② 409 → 不得进成功分支；③ 网络/500 → 仍走 toast 路径；`API_ERROR_CODE=1` | R11 |
| R10 gate 四源 + 全量 PC 构建（子任务直跑） | `docs/evidence/2489/pre-ready-2489.json`、`docs/evidence/2489/plan.md`、`scripts/verify-2489.sh` | `bash scripts/verify-2489.sh r10`（内部直接 `cd frontend-pc && npm run build` 并记录退出码）+ `plan-source.sh --issue 2489` | 修后树（干净）；子任务 P5 按 harness 为 N/A | PC | `GATE_RESULT=PASS`、`PLAN_SOURCE=PASS`、`FULL_GATE_DIRECT_EXIT=0`（全量构建真跑）；P5 的 N/A 由 D6 承接 | R10 |

## Input Truth & Tool Preflight

- **Source of truth**：`#2480` rubric（`business-correctable` = `40002/40004/40005/40900` 且指向用户可改输入；`keep:instant` = 成功/网络/权限/5xx/`40901/40902`）。
- **差异核对**：本批 5 文件与 `#2480`（`SiteManagement.jsx`/`StaffManagement.jsx`）、`#2485`（4 文件）、`#2487`/`#2488` 清单交集必须为空（`sort` + `comm`）。
- **损坏输入拒绝**：`resp` 非对象 → 按瞬时处理；`message` 非字符串 → 页面兜底；空 `text` → 组件返回 `null`。
- **工具盲区**：PC 无组件测试运行器 → 静态接线断言；真实截图由 `[Ops-Followup of #2489]` 承载。扫描脚本文本级分类，`ambiguous` 行必须人工落 `reason`。
- **影响面预检**：不改 `40901/40902` 的既有 `Modal` 分支；不改 API 调用与权限门控；不改 `message.success`。
- **gate 语义基线（D6）**：`pre-ready-check.sh:1092-1098` 对 `Parent:` 子任务固定 `na P5`（full gate 归父批次），故本单 gate 行的可执行断言是 `GATE_RESULT=PASS` + `PLAN_SOURCE=PASS` + **本单直跑的** `FULL_GATE_DIRECT_EXIT=0`，不得再写 `FULL_SUITE_EXIT` / `P5=NA → FAIL`。

## Decision Lock

| D9b | **传输层基线 = #2563 的 `frontend-pc/src/services/apiError.js`**（400 族 throw 带 `code`；404/409 返回对象；500/网络 throw 无 code）；`api.js` 只消费，**不新增第二个 `ApiError` 实现**（D9 的「新增 ApiError」表述作废） | Architect | 本计划锁定（Rev.7，采纳 Packet §4） | 若要求 `api.js` 自建实现 → 与 #2563 双 owner，拒绝 |
| D9 | 传输层契约 | `api.js` 非 2xx 抛 `ApiError{code,message,data}`（404/409 对象返回不变）；页面按 `code` 接 `FormConflictAlert` |
| **D8** | **`SHARED_REUSE` 语义 = 「实际消费共享件（`FormConflictAlert` / `formConflict`）的页面数」**，不是「本批文件数」。本批 5 文件 = 2 convert（R4/R6）+ 3 keep（R2/R3/R5）；keep 页必须**不** import 共享件 → 由 `KEEP_PAGES_NO_ALERT=3` 反向锁死。改语义需重开分析。 | 本计划锁定（Rev.8） |
| D10 | 路由缺陷边界 | `POST /system/tenants` 缺失 → **#2516**；本单 R7 只保留「未判 code 不得假成功」防御断言 |

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 范围 = 本批 5 文件（配置/定价/权限/商户/租户），不跨批 | Architect | 本计划锁定 | 阻塞实现 |
| D2 | 先拿回 `code` 再分类（禁止按文案猜） | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 共享件唯一 owner（#2480 交付），禁止自写 Alert / 复制 code 白名单 | Architect | 本计划锁定 | 阻塞实现 |
| D4 | 商户 409 语义对齐 #2459（IAM 优先 + 502 默认映射） | Architect | 本计划锁定 | 若 #2459 返工改变 409/502 语义 → 回 Architect |
| D5 | UI 行证据 = 静态接线断言；真实渲染由 Ops 截图回评（**Owner 豁免**，`evidence_type: static`） | Architect | owner_approved（沿用本仓前端豁免惯例） | 若审计要求 render → 另立前端测试基建 Issue |
| D6 | **子任务 gate 语义**：本单正文含 `Parent: #2486` → `pre-ready-check.sh` 将 P5 判为 `N/A`（`sub-task: full gate belongs to the parent/batch gate`）。因此 R10 **不得**要求 `P5=PASS`/`FULL_SUITE_EXIT=0`；全量 PC 构建由 `scripts/verify-2489.sh r10` **直接执行并记录** `FULL_GATE_DIRECT_EXIT`，批次全量由父任务 **#2486** 收口时复核 | Architect | owner_approved（与 harness 子任务规则一致） | 若 Owner 要求子任务也必须有 P5=PASS → 需先改 harness（另立 Issue），本单阻塞 |
| D7 | 本单**不做** `### Batch Gate Reference` 登记：P15 需要父任务 gate 产物存在，而 #2486 的批次 gate 在其收口时才产出；改由 D6 的「本单直跑 + 父任务复核」双轨覆盖 | Architect | 本计划锁定 | 若审计坚持 P15 引用 → 等 #2486 收口后再补登 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R2 | static | 权限模板页接线结构断言，Owner 豁免见 D5 |
| R3 | static | 价格配置页接线结构断言，Owner 豁免见 D5 |
| R4 | static | 中转网点页接线结构断言，Owner 豁免见 D5 |
| R5 | static | 会员等级页接线结构断言，Owner 豁免见 D5 |
| R6 | static | 商户管理页接线结构断言，Owner 豁免见 D5 |
| R8 | static | 复用/残留度量（grep），Owner 豁免见 D5 |

## Out of Scope

- 其余批量（#2485 Batch 1、#2487 人员、#2488 内容）。
- 移动端 `dialog.alert` 形态。
- 后端返回码/文案本身。
- 全局 toast 时长配置。
- PC 组件测试基建。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 修后 5 文件 + 扫描脚本 | `bash scripts/scan-form-toasts-2489.sh` | `docs/evidence/2489/R1-inventory.txt` | 每处 1 行；`UNCLASSIFIED=0`；`READONLY=1` | 未分类行 → FAIL |
| R2 | 权限模板页三类真实错误（`40000` 参数 / `40300` 权限 / `50000` 5xx） | 权限模板保存 | `docs/evidence/2489/R2-perm.txt` | 全部 `keep:instant`（非用户可纠正）；`KEEP_ROWS=3`、`FALSE_CONVERT=0`、`PERSIST_ALERT_UNUSED=1` | 把不可纠正错误改成常驻提示（误分类）→ FAIL |
| R3 | 价格配置页四类真实错误（`40001` binding / `40002` template not found / `40300` / `50000`） | 价格配置保存 | `docs/evidence/2489/R3-pricing.txt` | 全部 `keep:instant`；`KEEP_ROWS=1`、`FALSE_CONVERT=0`、`PERSIST_ALERT_UNUSED=1` | 把 `40002(template not found)` 转成常驻提示 → FAIL |
| R4 | fake `40900` | 中转网点保存 | `docs/evidence/2489/R4-transit.txt` | 同上 | 同上 → FAIL |
| R5 | fake `40002` | 会员等级保存 | `docs/evidence/2489/R5-membership.txt` | 同上 | 同上 → FAIL |
| R6 | fake `40900`（商户重名） | 商户新增/改名 | `docs/evidence/2489/R6-merchant.txt` | `PERSIST_ALERT=1`、`THEME_2459=1`、`CODE_FIRST=1` | 自写 code 白名单 → FAIL |
| R11 | mock `fetch`：`400+{code:40002}` / `409+{code:40900}` / 网络与 500 | `bash scripts/verify-2489.sh r11` | `docs/evidence/2489/R11-transport.txt` | `API_ERROR_CODE=1`、`NO_SUCCESS_ON_409=1`、`TOAST_ON_500=1` | 400 无 code 或 409 进成功分支 → FAIL |
| R8 | 修后源码 | `bash scripts/verify-2489.sh r8` | `docs/evidence/2489/R8-residual.txt` | `SHARED_REUSE=2`、`KEEP_PAGES_NO_ALERT=3`、`LOCAL_ALERT_DUP=0`、`BUSINESS_TOAST_RESIDUAL=0` | 残留 ≥1 → FAIL |
| R9 | 修后 `frontend-pc` | `npm run build` + `npx eslint` | `docs/evidence/2489/R9-build-lint.txt` | `PC_BUILD=0`、`LINT_EXIT=0` | 任一非 0 → FAIL |
| R10 | 修后干净树 + 本单直跑全量 PC 构建 + gate 报告 | `bash scripts/verify-2489.sh r10` | `docs/evidence/2489/R10-gate.txt` | `GATE_RESULT=PASS`、`PLAN_SOURCE=PASS`、`FULL_GATE_DIRECT_EXIT=0` | `FULL_GATE_DIRECT_EXIT!=0` 或 `GATE_RESULT!=PASS` → FAIL |
| `docs/evidence/2489/R11-transport.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2489.sh r1` | `docs/evidence/2489/R1-inventory.txt` | exit 0；grep `UNCLASSIFIED=0`、`READONLY=1` | PASS | 未分类 → FAIL | dev |
| R2 | `bash scripts/verify-2489.sh r2` | `docs/evidence/2489/R2-perm.txt` | exit 0；grep `KEEP_ROWS=3`、`FALSE_CONVERT=0`、`PERSIST_ALERT_UNUSED=1` | PASS | 误把 keep 类转成常驻提示 → FAIL | dev |
| R3 | `bash scripts/verify-2489.sh r3` | `docs/evidence/2489/R3-pricing.txt` | exit 0；grep `KEEP_ROWS=1`、`FALSE_CONVERT=0` | PASS | 误转常驻提示 → FAIL | dev |
| R4 | `bash scripts/verify-2489.sh r4` | `docs/evidence/2489/R4-transit.txt` | exit 0；grep `PERSIST_ALERT=1`、`CODE_FIRST=1` | PASS | 仍 toast → FAIL | dev |
| R5 | `bash scripts/verify-2489.sh r5` | `docs/evidence/2489/R5-membership.txt` | exit 0；grep `KEEP_ROWS=2`、`FRONTEND_ONLY_RULE=1` | PASS | 误转常驻提示 → FAIL | dev |
| R6 | `bash scripts/verify-2489.sh r6` | `docs/evidence/2489/R6-merchant.txt` | exit 0；grep `PERSIST_ALERT=1`、`THEME_2459=1` | PASS | 自写白名单 → FAIL | dev |
| R8 | `bash scripts/verify-2489.sh r8` | `docs/evidence/2489/R8-residual.txt` | exit 0；grep `SHARED_REUSE=2`、`KEEP_PAGES_NO_ALERT=3`、`BUSINESS_TOAST_RESIDUAL=0` | PASS | 残留 ≥1 → FAIL | dev |
| R9 | `bash scripts/verify-2489.sh r9` | `docs/evidence/2489/R9-build-lint.txt` | exit 0；grep `PC_BUILD=0`、`LINT_EXIT=0` | PASS | 任一非 0 → FAIL | dev |
| R10 | `bash scripts/verify-2489.sh r10` | `docs/evidence/2489/R10-gate.txt` | exit 0；grep `GATE_RESULT=PASS`、`PLAN_SOURCE=PASS`、`FULL_GATE_DIRECT_EXIT=0` | PASS | `FULL_GATE_DIRECT_EXIT!=0` → FAIL | dev |
| R11 | `bash scripts/verify-2489.sh r11` | docs/evidence/2489/R11-transport.txt | exit 0；含 `API_ERROR_CODE=1`、`NO_SUCCESS_ON_409=1`、`TOAST_ON_500=1` | PASS | 400 无 code 或 409 进成功分支 → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2489/R1-inventory.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R2-perm.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R3-pricing.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R4-transit.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R5-membership.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R6-merchant.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R8-residual.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R9-build-lint.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R10-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R11-transport.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 | 触发条件 | 根因 | 修复点 | 覆盖行 | 负例断言 |
|---|---|---|---|---|---|---|
| B1 | 配置页业务冲突只显示 3 秒 toast | 重名/字段冲突 | 瞬时提示承载可纠正错误 | 变更点 2/3 | R2–R7 | 仍 toast → 对应行失败 |
| B2 | 丢弃 `code` 无法分类 | catch-all | 前端未保留响应码 | 变更点 2 | R1 | `UNCLASSIFIED>0` → R1 失败 |
| B3 | 各自实现 Alert 形态 | 多页面 | 无共享件 | 变更点 3 | R8 | `LOCAL_ALERT_DUP>0` → R8 失败 |
| B4 | 商户重名语义与后端不一致 | 商户改名 | 页面自写 code 判定 | 变更点 4 | R6 | `THEME_2459=0` → R6 失败 |
| B5 | 提示残留 | 用户改输入 | 无清理约定 | 变更点 3 | R2–R7 | 残留 → 对应行失败 |
| B6 | 瞬时错误被改成常驻 | 网络/权限 | 分类过宽 | 变更点 2 | R1/R8 | 网络错误走 Alert → 失败 |
| B7 | 跨批重复转换同一文件 | 并行批次 | 清单未互斥 | Input Truth 差异核对 | R8 | 交集非空 → R8 失败 |
| B8 | 权限模板页把 403 当可纠正 | 权限不足 | rubric 误用 | 变更点 1 | R2 | 403 走 Alert → R2 失败 |

### Workload

- Size: M
- Basis: 6 个前端文件接线改造 + 1 个扫描脚本 + 1 个 verify 脚本（10 行）+ 10 条证据行；复用 #2480 共享件；无后端、无 migration
- Confidence: medium（分类含人工判定，`ambiguous` 行可能小返工）

### Files（变更文件清单）

- `frontend-pc/src/services/api.js`

- `frontend-pc/src/components/FormConflictAlert.jsx`
- `frontend-pc/src/utils/formConflict.js`
- `frontend-pc/src/pages/admin/PermissionManage/index.jsx`
- `frontend-pc/src/pages/admin/pricing/MerchantPricingConfig.jsx`
- `frontend-pc/src/pages/admin/transit/TransitCenter.jsx`
- `frontend-pc/src/pages/System/MembershipLevelsPage.jsx`
- `frontend-pc/src/pages/MerchantManagement.jsx`
- `scripts/scan-form-toasts-2489.sh`
- `scripts/verify-2489.sh`
- `docs/evidence/2489/plan.md`
- `docs/evidence/2489/pre-ready-2489.json`
- `docs/evidence/2489/R1-inventory.txt`
- `docs/evidence/2489/R2-perm.txt`
- `docs/evidence/2489/R3-pricing.txt`
- `docs/evidence/2489/R4-transit.txt`
- `docs/evidence/2489/R5-membership.txt`
- `docs/evidence/2489/R6-merchant.txt`
- `docs/evidence/2489/R8-residual.txt`
- `docs/evidence/2489/R9-build-lint.txt`
- `docs/evidence/2489/R10-gate.txt`
- `docs/evidence/2489/R11-transport.txt`
- `docs/evidence/2489/evidence.json`

## Dependencies

- #2480

### 依赖与关联说明（非硬依赖）

- **#2480（accepted，硬前置）**：共享件 + rubric；未合入前 `/work` HOLD。
- **#2486（accepted）**：父任务（拆分与验收口径）；本单为其子任务之一。
- **#2459（accepted）**：`MerchantManagement.jsx` 的 409/502 语义来源。
- **部署侧**：预生产截图回评由 `[Ops-Followup of #2489]` 承载。

*Model: DeepSeek V4.1 Flash 1017*







