## Accepted Plan — #2489（配置/定价/权限/商户类页面：业务可纠正错误 → 持久提示）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 共享件与 rubric 由 #2480 交付（消费，不重写）：`FormConflictAlert`、`isBusinessCorrectableConflict` / `conflictText` | `frontend-pc/src/components/FormConflictAlert.jsx`、`frontend-pc/src/utils/formConflict.js` |
| 父任务拆分与拓扑 | #2486（accepted）`## Sub-tasks` |
| 本批 6 文件与 toast 数（初扫） | `PermissionManage/index.jsx`(8)、`pricing/MerchantPricingConfig.jsx`(7)、`transit/TransitCenter.jsx`(5)、`System/MembershipLevelsPage.jsx`(4)、`MerchantManagement.jsx`(4)、`TenantManagement.jsx`(2) |
| 多数调用点为 catch-all（`err.message`）或「先 `message.error(resp.message)` 丢弃 `code`」→ 无法按 rubric 区分 | 同批文件（`grep -n "message.error" -B2`） |

**结论**：与 Batch 1 同病 —— 先拿回 `code` 再分类转换；本单只覆盖上述 6 文件（与 #2485/#2487/#2488 文件集合互斥）。

### 变更点

1. **判定表（只读）**：`scripts/scan-form-toasts-2489.sh` → `docs/evidence/2489/R1-inventory.txt`，逐条列出本批 6 文件的每个 `message.error(`：`file:line`、所在函数、是否读 `code`、tier（`business-correctable` / `keep:instant` / `needs-code-first`）、`reason`；`READONLY=1`。
2. **code 判定回填**：`needs-code-first` 项先判 `resp.code`：
   - `isBusinessCorrectableConflict(resp)` → `setFormConflict(conflictText(resp, '<页面兜底>'))`（持久）；
   - 其它 → 维持 `message.error(...)`（瞬时）。
3. **接入共享件**：6 文件各自渲染 `<FormConflictAlert text={formConflict} />`，清理约定沿用 #2480 D8（字段编辑 / 提交开始 / 取消 / 成功）。
4. **商户重名对齐 #2459**：`MerchantManagement.jsx` 的 409 语义以 #2459（IAM 优先 + namespace 唯一 + 502 默认映射）为准；不得在页面里重写 code 白名单。
5. `scripts/verify-2489.sh`：10 行证据生成器。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 判定表（只读） | `scripts/scan-form-toasts-2489.sh` | `bash scripts/scan-form-toasts-2489.sh` | 修后树 | PC | 6 文件每个 `message.error(` 各 1 行含 tier/reason；`UNCLASSIFIED=0`、`READONLY=1` | R1 |
| R2 权限模板页 | `frontend-pc/src/pages/admin/PermissionManage/index.jsx` | `/admin/permissions` 保存模板 | `40002/40004/40900`（模板重名/字段非法） | PC | 命中 rubric → 持久提示；`CODE_FIRST=1` | R2 |
| R3 价格配置页 | `frontend-pc/src/pages/admin/pricing/MerchantPricingConfig.jsx` | 商户价格配置保存 | `40002/40005/40900` | PC | 同上 | R3 |
| R4 中转网点页 | `frontend-pc/src/pages/admin/transit/TransitCenter.jsx` | 中转网点新增/编辑 | `40004/40900`（重名） | PC | 同上 | R4 |
| R5 会员等级页 | `frontend-pc/src/pages/System/MembershipLevelsPage.jsx` | 会员等级新增/编辑 | `40002/40900`（门槛冲突/重名） | PC | 同上 | R5 |
| R6 商户管理页 | `frontend-pc/src/pages/MerchantManagement.jsx` | 商户改名/新增 | `40900`（与 #2459 一致） | PC | 同上；`THEME_2459=1`（不重写 code 白名单） | R6 |
| R7 租户管理页 | `frontend-pc/src/pages/TenantManagement.jsx` | 租户新增/编辑 | `40004/40900` | PC | 同上 | R7 |
| R8 共享件复用与残留 | 6 文件 + `frontend-pc/src/components/FormConflictAlert.jsx` | `bash scripts/verify-2489.sh r8` | 修后树 | PC | `SHARED_REUSE=6`、`LOCAL_ALERT_DUP=0`、`LOCAL_CODE_LIST=0`、`BUSINESS_TOAST_RESIDUAL=0` | R8 |
| R9 构建与 lint | `frontend-pc` | `cd frontend-pc && npm run build` + `npx eslint` 改动文件 | 修后树 | PC | `PC_BUILD=0`、`LINT_EXIT=0` | R9 |
| R10 gate 四源 + 全量 PC 构建 | `docs/evidence/2489/pre-ready-2489.json`、`docs/evidence/2489/plan.md` | `--full-gate-cmd "cd frontend-pc && npm run build"` + `plan-source.sh --issue 2489` | 修后树（干净） | PC | `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | R10 |

## Input Truth & Tool Preflight

- **Source of truth**：`#2480` rubric（`business-correctable` = `40002/40004/40005/40900` 且指向用户可改输入；`keep:instant` = 成功/网络/权限/5xx/`40901/40902`）。
- **差异核对**：本批 6 文件与 `#2480`（`SiteManagement.jsx`/`StaffManagement.jsx`）、`#2485`（4 文件）、`#2487`/`#2488` 清单交集必须为空（`sort` + `comm`）。
- **损坏输入拒绝**：`resp` 非对象 → 按瞬时处理；`message` 非字符串 → 页面兜底；空 `text` → 组件返回 `null`。
- **工具盲区**：PC 无组件测试运行器 → 静态接线断言；真实截图由 `[Ops-Followup of #2489]` 承载。扫描脚本文本级分类，`ambiguous` 行必须人工落 `reason`。
- **影响面预检**：不改 `40901/40902` 的既有 `Modal` 分支；不改 API 调用与权限门控；不改 `message.success`。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 范围 = 本批 6 文件（配置/定价/权限/商户/租户），不跨批 | Architect | 本计划锁定 | 阻塞实现 |
| D2 | 先拿回 `code` 再分类（禁止按文案猜） | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 共享件唯一 owner（#2480 交付），禁止自写 Alert / 复制 code 白名单 | Architect | 本计划锁定 | 阻塞实现 |
| D4 | 商户 409 语义对齐 #2459（IAM 优先 + 502 默认映射） | Architect | 本计划锁定 | 若 #2459 返工改变 409/502 语义 → 回 Architect |
| D5 | UI 行证据 = 静态接线断言；真实渲染由 Ops 截图回评（**Owner 豁免**，`evidence_type: static`） | Architect | owner_approved（沿用本仓前端豁免惯例） | 若审计要求 render → 另立前端测试基建 Issue |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R2 | static | 权限模板页接线结构断言，Owner 豁免见 D5 |
| R3 | static | 价格配置页接线结构断言，Owner 豁免见 D5 |
| R4 | static | 中转网点页接线结构断言，Owner 豁免见 D5 |
| R5 | static | 会员等级页接线结构断言，Owner 豁免见 D5 |
| R6 | static | 商户管理页接线结构断言，Owner 豁免见 D5 |
| R7 | static | 租户管理页接线结构断言，Owner 豁免见 D5 |
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
| R1 | 修后 6 文件 + 扫描脚本 | `bash scripts/scan-form-toasts-2489.sh` | `docs/evidence/2489/R1-inventory.txt` | 每处 1 行；`UNCLASSIFIED=0`；`READONLY=1` | 未分类行 → FAIL |
| R2 | fake `40900`（模板重名） | 权限模板保存 | `docs/evidence/2489/R2-perm.txt` | `PERSIST_ALERT=1`、`CODE_FIRST=1` | 仍 toast → FAIL |
| R3 | fake `40002` | 价格配置保存 | `docs/evidence/2489/R3-pricing.txt` | 同上 | 同上 → FAIL |
| R4 | fake `40900` | 中转网点保存 | `docs/evidence/2489/R4-transit.txt` | 同上 | 同上 → FAIL |
| R5 | fake `40002` | 会员等级保存 | `docs/evidence/2489/R5-membership.txt` | 同上 | 同上 → FAIL |
| R6 | fake `40900`（商户重名） | 商户新增/改名 | `docs/evidence/2489/R6-merchant.txt` | `PERSIST_ALERT=1`、`THEME_2459=1`、`CODE_FIRST=1` | 自写 code 白名单 → FAIL |
| R7 | fake `40004` | 租户保存 | `docs/evidence/2489/R7-tenant.txt` | `PERSIST_ALERT=1`、`CODE_FIRST=1` | 仍 toast → FAIL |
| R8 | 修后源码 | `bash scripts/verify-2489.sh r8` | `docs/evidence/2489/R8-residual.txt` | `SHARED_REUSE=6`、`LOCAL_ALERT_DUP=0`、`BUSINESS_TOAST_RESIDUAL=0` | 残留 ≥1 → FAIL |
| R9 | 修后 `frontend-pc` | `npm run build` + `npx eslint` | `docs/evidence/2489/R9-build-lint.txt` | `PC_BUILD=0`、`LINT_EXIT=0` | 任一非 0 → FAIL |
| R10 | 修后干净树 + gate | 读校验 | `docs/evidence/2489/R10-gate.txt` | `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | `P5=NA` → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2489.sh r1` | `docs/evidence/2489/R1-inventory.txt` | exit 0；grep `UNCLASSIFIED=0`、`READONLY=1` | PASS | 未分类 → FAIL | dev |
| R2 | `bash scripts/verify-2489.sh r2` | `docs/evidence/2489/R2-perm.txt` | exit 0；grep `PERSIST_ALERT=1`、`CODE_FIRST=1` | PASS | 仍 toast → FAIL | dev |
| R3 | `bash scripts/verify-2489.sh r3` | `docs/evidence/2489/R3-pricing.txt` | exit 0；grep `PERSIST_ALERT=1`、`CODE_FIRST=1` | PASS | 仍 toast → FAIL | dev |
| R4 | `bash scripts/verify-2489.sh r4` | `docs/evidence/2489/R4-transit.txt` | exit 0；grep `PERSIST_ALERT=1`、`CODE_FIRST=1` | PASS | 仍 toast → FAIL | dev |
| R5 | `bash scripts/verify-2489.sh r5` | `docs/evidence/2489/R5-membership.txt` | exit 0；grep `PERSIST_ALERT=1`、`CODE_FIRST=1` | PASS | 仍 toast → FAIL | dev |
| R6 | `bash scripts/verify-2489.sh r6` | `docs/evidence/2489/R6-merchant.txt` | exit 0；grep `PERSIST_ALERT=1`、`THEME_2459=1` | PASS | 自写白名单 → FAIL | dev |
| R7 | `bash scripts/verify-2489.sh r7` | `docs/evidence/2489/R7-tenant.txt` | exit 0；grep `PERSIST_ALERT=1`、`CODE_FIRST=1` | PASS | 仍 toast → FAIL | dev |
| R8 | `bash scripts/verify-2489.sh r8` | `docs/evidence/2489/R8-residual.txt` | exit 0；grep `SHARED_REUSE=6`、`BUSINESS_TOAST_RESIDUAL=0` | PASS | 残留 ≥1 → FAIL | dev |
| R9 | `bash scripts/verify-2489.sh r9` | `docs/evidence/2489/R9-build-lint.txt` | exit 0；grep `PC_BUILD=0`、`LINT_EXIT=0` | PASS | 任一非 0 → FAIL | dev |
| R10 | `bash scripts/verify-2489.sh r10` | `docs/evidence/2489/R10-gate.txt` | exit 0；grep `GATE_RESULT=PASS`、`PLAN_SOURCE=PASS` | PASS | `P5=NA` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2489/R1-inventory.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R2-perm.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R3-pricing.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R4-transit.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R5-membership.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R6-merchant.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R7-tenant.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R8-residual.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R9-build-lint.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2489/R10-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

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

- `frontend-pc/src/components/FormConflictAlert.jsx`
- `frontend-pc/src/utils/formConflict.js`
- `frontend-pc/src/pages/admin/PermissionManage/index.jsx`
- `frontend-pc/src/pages/admin/pricing/MerchantPricingConfig.jsx`
- `frontend-pc/src/pages/admin/transit/TransitCenter.jsx`
- `frontend-pc/src/pages/System/MembershipLevelsPage.jsx`
- `frontend-pc/src/pages/MerchantManagement.jsx`
- `frontend-pc/src/pages/TenantManagement.jsx`
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
- `docs/evidence/2489/R7-tenant.txt`
- `docs/evidence/2489/R8-residual.txt`
- `docs/evidence/2489/R9-build-lint.txt`
- `docs/evidence/2489/R10-gate.txt`
- `docs/evidence/2489/evidence.json`

## Dependencies

- #2480

### 依赖与关联说明（非硬依赖）

- **#2480（accepted，硬前置）**：共享件 + rubric；未合入前 `/work` HOLD。
- **#2486（accepted）**：父任务（拆分与验收口径）；本单为其子任务之一。
- **#2459（accepted）**：`MerchantManagement.jsx` 的 409/502 语义来源。
- **部署侧**：预生产截图回评由 `[Ops-Followup of #2489]` 承载。

*Model: DeepSeek V4.1 Flash 1017*

