## Accepted Plan — #2485 v3（Tier 2 Batch 1：成员/对象管理域业务可纠正错误改持久提示）

> **v3 说明（仅修 v2 `### Files` 的漏项，矩阵/合同/范围不变）**
> ① **needMore 来源**：/work 按 v2 完成实现（fix `52f1ed229`）与证据（`4af374ae5`）后，仅卡在 `pre-ready-check.sh` 的 P5。核对 `pre-ready-check.sh` 源码确认三项**互斥**：P17 要求 `docs/evidence/2485/plan.md` 快照存在且与最新 Accepted Plan 同 hash（`P17_SNAP_SHA != PLAN_SHA256 → fail`）；P5 要求工作树干净（快照必须**已提交**）；P6 要求已提交文件必须在 `### Files` 中声明（`fail P6: changed files not declared`）。v2 重写 Files 时漏掉该快照 → 三者无解。v1 原本含此项（`#2480`/`#2540` 等同类证据包的 Files 均含自己的 `plan.md`）。
> ② **本版唯一变更**：`### Files（变更文件清单）` 增加 `docs/evidence/2485/plan.md`；并把 `FormConflictAlert.jsx` 标注为**消费不修改**（避免被误读为可在本单改动共享件）。
> ③ **依赖现状更新（事实核对，非范围变更）**：`git ls-tree origin/develop frontend-pc/src/utils/formConflict.js frontend-pc/src/components/FormConflictAlert.jsx` → 两个 blob 均存在（#2480 已随 `origin/develop 58cb3a54a` 落地）→ 硬前置**已满足**，v2 中「未合入前 HOLD」的历史条件已失效。
> ④ v2 保留：R2「码来源可追溯」、R5 可证伪度量（含检测器自测）、D3 `defer` 策略（无 code/码不可达 → defer + reason）、D4 批次边界（4 文件）。

### 背景

`#2480`（Tier 1，已 PASS 并合入 develop）交付共享件 `frontend-pc/src/utils/formConflict.js`（rubric）+ `frontend-pc/src/components/FormConflictAlert.jsx`（持久提示），并明确 Owner 要求：**业务可纠正错误不用 3 秒 toast；瞬时/网络/权限/5xx 保持 toast**。

本单是 **Batch 1**（成员/对象管理域），与 Batch 2（#2486 及其子任务 #2487/#2488/#2489）平行。

### 变更点

1. 对本批 4 个文件逐条 `message.error(` 分类：`business-correctable`（转持久）/ `keep:instant` / `defer`，产出判定表（`file:line` + 结论 + 理由 + `code_source`）；
2. `business-correctable` 行：先补 `code` 判定，再改为 `<FormConflictAlert>` 持久提示；**引用的 code 必须能在 `backend/handlers` 中定位到导出点**；
3. `keep:instant` 行：保留 toast + 理由（成功/网络/权限/5xx/Modal 引导流）；
4. 无 code 可判、或码存在但**传输层不可观测**（`frontend-pc/src/services/api.js` 对非 404/409 非 2xx 直接 throw，response body 丢失）的行标 `defer` + `reason`，不计入残留但登记，并另立跟进 Issue（当前：`#2563`）。

### 范围文件（判定对象）

- `frontend-pc/src/components/SiteMemberManagement.jsx`
- `frontend-pc/src/components/MerchantMemberManagement.jsx`
- `frontend-pc/src/pages/DirectMemberManagement.jsx`
- `frontend-pc/src/pages/admin/category/List.jsx`

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 逐条分类判定表 | `scripts/verify-2485.sh` | `bash scripts/verify-2485.sh r1` | 本批 4 文件全量 `message.error(` | PC | 每行有结论与理由，`UNCLASSIFIED=0`，行数 ≥ #2480 扫描基线 | R1 |
| R2 码来源可追溯 + 传输层可达性 | 4 个页面 + 后端 handler 导出点 | `bash scripts/verify-2485.sh r2` | 判定表引用的每个 code | PC | `CODE_SOURCE_MISSING=0`、`UNREACHABLE_CODES=0`、`TRANSPORT_UNOBSERVABLE_PERSISTED=0`、`TRANSPORT_OBSERVABLE_DEFERRED=0` | R2 |
| R3 业务可纠正行改持久提示 | 4 个页面 | `bash scripts/verify-2485.sh r3` | 命中可达 code 的表单提交 | PC | 命中行使用 `FormConflictAlert` 且先判 code（`CONVERTED_ROWS>=1`、`CODE_FIRST=1`） | R3 |
| R4 共享件复用、禁自写 | `frontend-pc/src/components/FormConflictAlert.jsx`（**消费不修改**） | `bash scripts/verify-2485.sh r4` | 静态引用 | PC | `SHARED_REUSE>=1`、`LOCAL_ALERT_DUP=0` | R4 |
| R5 残留计数（可证伪） | 4 个页面 | `bash scripts/verify-2485.sh r5` | 静态扫描 | PC | `BUSINESS_TOAST_RESIDUAL=0`；`KEEP_WITH_REASON` == keep 行数；`RESIDUAL_DETECTOR_SELF_TEST=1` | R5 |
| R6 构建与 lint | `frontend-pc` | `bash scripts/verify-2485.sh r6` | 修后树 | PC | `BUILD_OK` + `ESLINT_OK` | R6 |

## Input Truth & Tool Preflight

- **Source of truth**：`#2480` rubric 与共享件；后端码以 **`backend/handlers/*.go` 的实际导出为唯一依据**（禁止引用未导出/历史码）；前端可达性以 `frontend-pc/src/services/api.js` 的实际 throw 语义为唯一依据。
- **差异核对**：判定表行数必须 ≥ 扫描基线（`docs/evidence/2480/R11-scan.txt`）中本批 4 文件的行数；只允许改结论，不允许删行。
- **损坏输入拒绝**：无 `code_source` 的行一律 `defer`；码存在但传输层不返回 response body 的行同样 `defer`（不得凭文案猜分类）。
- **工具盲区**：grep 无法区分同函数多分支；`code_source` 由实现者填写并在 R2 校验存在性；扫描器自身必须带自测（`RESIDUAL_DETECTOR_SELF_TEST`），否则度量不可证伪。
- **证据包完整性**：`docs/evidence/<N>/plan.md` 是证据包标准组成（P17 快照 / P5 干净树 / P6 声明三者共同要求），必须随证据一并提交。

## Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 触发码来源 | 只接受 `backend/handlers` 实际导出的码；判定表必须写 `code_source`（文件:行）；引用不到即 `defer` |
| D2 | 持久形态 | 只用共享 `FormConflictAlert`，禁止本批自建 Alert 或复制码白名单 |
| D3 | defer 策略 | 无 code / 码不可达 / 传输层不可观测 → `defer` + `reason`，不计入残留但登记；被 defer 的用户可见缺陷必须同日另立 Issue 并在计划中记录单号 |
| D4 | 批次边界 | 本批仅 4 个文件；Batch 2 文件改动视为越界（P6 会拦截） |
| D5 | 证据包范围 | `docs/evidence/2485/plan.md` 属本单证据包，必须列入 `### Files` 并提交（否则 P5/P17/P6 互斥） |
| D6 | UI 行证据形态 | PC 无组件测试运行器 → UI 行以静态结构断言为主（`evidence_type: static`），**Owner 豁免**（沿用 #2480 / #2540 惯例）；真实渲染/点击截图由 `[Ops-Followup of #2485]` 回评 |

## Out of Scope

- 不改后端错误码与文案；不改 `frontend-mobile`；
- 不改 Batch 2 的 6+4+3 个文件（#2488/#2487/#2489）；
- 不改成功路径/瞬时提示行为；不做布局重构；
- 不改 `frontend-pc/src/services/api.js` 的传输层语义（40002/40004/40005 类冲突的丢码根因）→ **#2563**（已创建，含证据与验收）；
- 不改共享件 `FormConflictAlert.jsx` / `formConflict.js`（单一 owner = #2480）。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 本批 4 文件全量 `message.error(` | `bash scripts/verify-2485.sh r1` | `docs/evidence/2485/R1-classify.txt` | 每行有结论；`UNCLASSIFIED=0`；行数 ≥ 基线 | 出现无结论行 → FAIL |
| R2 | 判定表引用的全部 code + 4 个页面的响应处理形态 | `bash scripts/verify-2485.sh r2` | `docs/evidence/2485/R2-codes.txt` | `CODE_SOURCE_MISSING=0`；`UNREACHABLE_CODES=0`；`TRANSPORT_UNOBSERVABLE_PERSISTED=0`；`TRANSPORT_OBSERVABLE_DEFERRED=0` | 把传输层不可观测的 40002 行改回持久（RED 复现）→ FAIL |
| R3 | `business-correctable` 行 | `bash scripts/verify-2485.sh r3` | `docs/evidence/2485/R3-converted.txt` | `CONVERTED_ROWS>=1`；`CODE_FIRST=1` | 仅换文案 / 未判 code → FAIL |
| R4 | 共享件引用 | `bash scripts/verify-2485.sh r4` | `docs/evidence/2485/R4-shared.txt` | `SHARED_REUSE>=1`；`LOCAL_ALERT_DUP=0` | 自建 Alert → FAIL |
| R5 | 4 文件残留 | `bash scripts/verify-2485.sh r5` | `docs/evidence/2485/R5-residual.txt` | `BUSINESS_TOAST_RESIDUAL=0`；`KEEP_WITH_REASON` == keep 行数；`RESIDUAL_DETECTOR_SELF_TEST=1` | 缺陷形态下度量仍为 0（不可证伪）→ FAIL |
| R6 | 修后 `frontend-pc` | `bash scripts/verify-2485.sh r6` | `docs/evidence/2485/R6-build.txt` | `BUILD_OK` + `ESLINT_OK` | 任一失败 → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2485.sh r1` | `docs/evidence/2485/R1-classify.txt` | exit 0；含 `UNCLASSIFIED=0` | PASS | 无结论行 → FAIL | dev |
| R2 | `bash scripts/verify-2485.sh r2` | `docs/evidence/2485/R2-codes.txt` | exit 0；含 `CODE_SOURCE_MISSING=0` 与 `UNREACHABLE_CODES=0` | PASS | 引用未导出码 → FAIL | dev |
| R3 | `bash scripts/verify-2485.sh r3` | `docs/evidence/2485/R3-converted.txt` | exit 0；含 `CONVERTED_ROWS>=1` 与 `CODE_FIRST=1` | PASS | 缺 code 判定 → FAIL | dev |
| R4 | `bash scripts/verify-2485.sh r4` | `docs/evidence/2485/R4-shared.txt` | exit 0；含 `SHARED_REUSE>=1` 与 `LOCAL_ALERT_DUP=0` | PASS | 自建 Alert → FAIL | dev |
| R5 | `bash scripts/verify-2485.sh r5` | `docs/evidence/2485/R5-residual.txt` | exit 0；含 `BUSINESS_TOAST_RESIDUAL=0` | PASS | 残留 >0 → FAIL | dev |
| R6 | `bash scripts/verify-2485.sh r6` | `docs/evidence/2485/R6-build.txt` | exit 0；含 `BUILD_OK` 与 `ESLINT_OK` | PASS | 构建失败 → FAIL | dev |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | static | 静态扫描判定表（文本级分类 + 人工结论列） |
| R2 | static | 码来源存在性 + 传输层可达性结构断言 |
| R3 | static | 持久化接线结构断言，Owner 豁免见 D2/D6（PC 无组件测试运行器，沿用 #2480 惯例）；真实渲染截图由 `[Ops-Followup of #2485]` 承载 |
| R4 | static | 共享件复用与禁自写结构断言 |
| R5 | static | 残留计数 + 检测器自测（可证伪度量） |
| R6 | static | 构建/lint 退出码 |

## Artifact Provenance

- R1–R6（脚本生成，首行 provenance）：写 `# build=1.0.<短码>`、`# commit=<40 位 sha>`、`# dirty=false`。
  - `docs/evidence/2485/R1-classify.txt`
  - `docs/evidence/2485/R2-codes.txt`
  - `docs/evidence/2485/R3-converted.txt`
  - `docs/evidence/2485/R4-shared.txt`
  - `docs/evidence/2485/R5-residual.txt`
  - `docs/evidence/2485/R6-build.txt`
- 计划快照（P17）：`docs/evidence/2485/plan.md`，内容必须与本评论逐字一致（`scripts/plan-source.sh --issue 2485 --fresh` 生成），并随证据一起提交。
- gate（由 /work 执行）：`~/.config/opencode/scripts/pre-ready-check.sh 2485 --summary-file … --changed-files … --affected-cmd "node --test frontend-pc/tests/formConflict.test.mjs" --full-gate-cmd "cd frontend-pc && npm run build" --evidence-file docs/evidence/2485/evidence.json --report docs/evidence/2485/pre-ready-2485.json`；报告 `build` 须匹配 `evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$`。

## Dependencies

- #2480

### 依赖说明

- **#2480（已合入 develop，前置已满足）**：`git ls-tree origin/develop frontend-pc/src/utils/formConflict.js frontend-pc/src/components/FormConflictAlert.jsx` → 两个 blob 均存在；本单只消费，不修改、不复制。
- **#2563（本单 defer 项承接，`status:todo`）**：传输层丢码导致 40002/40004/40005 类冲突全链路不可观测；本单按 D3 判 `defer` 并在 #2563 闭环，不在本单改 `api.js`。
- 平行批次：#2487 / #2488 / #2489（Batch 2 子任务）。

### Workload

- Size: M
- Basis: 4 个前端文件（35 处消息调用点判定）+ 1 个验证脚本 + 6 条证据行 + 证据包快照；无后端改动、无 migration
- Confidence: medium（判定依赖逐行归类与后端码 + 传输层可达性核对）

### Files（变更文件清单）

- `frontend-pc/src/components/FormConflictAlert.jsx`（#2480 交付，本单**消费不修改**）
- `frontend-pc/src/components/SiteMemberManagement.jsx`
- `frontend-pc/src/components/MerchantMemberManagement.jsx`
- `frontend-pc/src/pages/DirectMemberManagement.jsx`
- `frontend-pc/src/pages/admin/category/List.jsx`
- `scripts/verify-2485.sh`
- `scripts/scan-member-form-toasts-2485.sh`（v1 扫描器；v3 起由 `scripts/verify-2485.sh` 的 r1 内联取代，**允许删除**）
- `docs/evidence/2485/plan.md`
- `docs/evidence/2485/R1-classify.txt`
- `docs/evidence/2485/R2-codes.txt`
- `docs/evidence/2485/R3-converted.txt`
- `docs/evidence/2485/R4-shared.txt`
- `docs/evidence/2485/R5-residual.txt`
- `docs/evidence/2485/R6-build.txt`
- `docs/evidence/2485/pre-ready-2485.json`
- `docs/evidence/2485/evidence.json`

### 本轮 needMore 处置（Architect）

- `needMore` 原因：v2 `### Files` 漏登 `docs/evidence/2485/plan.md`，与 P17（快照存在）+ P5（干净树，须已提交）+ P6（已提交文件须声明）互斥 → 不是实现缺陷，不需要返工代码。
- 处置：本版补登该路径（D5 固化），矩阵/合同/范围维持不变；/work 恢复后只需 3 步：刷新 `docs/evidence/2485/plan.md` 快照并同步 `evidence.json.plan_sha256` → 提交快照 → `pre-ready-check.sh 2485`（带 `--affected-cmd` / `--full-gate-cmd` / `--evidence-file` / `--report`）重跑。

*Model: DeepSeek V4.1 Flash 1017*

