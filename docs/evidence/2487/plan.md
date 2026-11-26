## Accepted Plan — #2487（Tier 2 批次：人员/成员类业务可纠正错误改持久提示）｜Rev.2

### 背景

`#2480`（Tier 1，Owner 判定 PASS）已提供共享件与 rubric：`frontend-pc/src/utils/formConflict.js`（`isBusinessCorrectable(result)` / `conflictMessage`）+ `frontend-pc/src/components/FormConflictAlert.jsx`（持久提示），并要求「业务可纠正错误不用 3 秒 toast；瞬时/网络/权限/5xx 保持 toast」。

本单承接 **Batch 2**（父 #2486）的人员/成员类页面。

### 变更点

1. 对 `## 范围` 内 4 个文件逐条 `message.error(` 分类：`business-correctable` / `keep:instant` / `defer`，输出判定表（每行含 `file:line`、当前 code 判定、结论、理由）；
2. `business-correctable` 行：先补 `code` 判定（`40002/40004/40005/40900`），再改为 `<FormConflictAlert>` 持久提示（复用共享件，禁止本地复制 Alert）；
3. `keep:instant` 行：保留 toast，并在判定表写理由（成功/网络/权限/5xx/Modal 引导流）；
4. `TechnicianProfiles.jsx` 若受 #2465 收敛影响，相关行标 `defer` 并写明 `reason`。

### 范围文件（判定对象）

- `frontend-pc/src/pages/System/UserManagement.jsx`（约 15 处 toast）
- `frontend-pc/src/pages/admin/platform-staff/index.jsx`（约 6 处）
- `frontend-pc/src/pages/System/TechnicianProfiles.jsx`（约 5 处）

### 本批特殊约束

- `UserManagement.jsx` 的孤儿面板/账户操作提示属**状态冲突**（409/40005 等）可判 `business-correctable`；清理成功类保持 toast；
- 人员身份语义以 **#2462 / #2465** 为准（`users.role` 与 `merchant_members.role` 域一致）；本批只改错误呈现，不改身份逻辑；
- `TechnicianProfiles.jsx` 若在 #2465 中被收敛为 `/staff?staff_type=technician`，相关 toast 转换须在 #2465 合入后复核（记 `defer` + reason）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 逐条分类判定表 | `scripts/verify-2487.sh` | 静态扫描 | 全量 `message.error(` | PC | 6 文件全部行有结论，`UNCLASSIFIED=0` | R1 |
| 业务可纠正行改持久提示 | 6 个页面 | 表单提交 409/40002/40004/40005 | 后端返回对应 code | PC | 命中行改用 `FormConflictAlert`，且**先判 code** | R2 |
| 共享件复用 | `frontend-pc/src/components/FormConflictAlert.jsx` | 静态引用 | — | PC | `SHARED_REUSE>=1`、`LOCAL_ALERT_DUP=0` | R3 |
| 残留计数 | 6 个页面 | 静态扫描 | — | PC | `BUSINESS_TOAST_RESIDUAL=0`（或每条有 `keep` 理由） | R4 |
| 构建与 lint | `frontend-pc` | `npm run build` + `eslint` | — | PC | 两者 exit 0 | R5 |

## Input Truth & Tool Preflight

- **Source of truth**：`#2480` 的 rubric（`frontend-pc/src/utils/formConflict.js`）与共享件 `FormConflictAlert.jsx`；扫描基线 `docs/evidence/2480/R11-scan.txt`（364 行，`tier=defer:tier2` 为待判定项）。
- **差异核对**：本单 6 个文件的 toast 点在扫描表中的 `tier` 与本次判定结果必须一一对应（行数不得减少，只允许改结论）。
- **损坏输入拒绝**：无法判定 code 的行一律进 `defer` 并写明 `reason`，不得默认转成持久提示。
- **工具盲区**：静态 grep 无法区分「同一行内多分支」；判定表以 `file:line` + 手写理由为准，脚本只做计数与一致性断言。

## Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 判定口径 | 以 #2480 rubric 为准：`40002/40004/40005/40900` 且指向用户可改输入 → `business-correctable`；其余 `keep:instant` |
| D2 | 持久形态 | 只用共享 `FormConflictAlert`；禁止在本单页面新增自定义 Alert 组件 |
| D3 | defer 策略 | 后端未给 code 的点标 `defer` + `reason`，不计入 `BUSINESS_TOAST_RESIDUAL`，但必须登记供后续批次收口 |

## Out of Scope

- 不改后端错误码与文案（仅消费）；
- 不改 `frontend-mobile`（Tier 2 仅 PC；移动端另立 Issue）；
- 不改 `#2485` Batch 1 已转换的 `SiteManagement.jsx` / `StaffManagement.jsx`；
- 不改成功路径/瞬时提示的现有行为；不做 UI 布局重构。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 6 个文件全量 `message.error(`（脚本枚举） | `bash scripts/verify-2487.sh r1` | `docs/evidence/2487/R1-classify.txt` | 每行 `file:line` 有结论；`UNCLASSIFIED=0`；总行数 ≥ 扫描表同文件行数 | 出现无结论行 → FAIL |
| R2 | 判定为 `business-correctable` 的行 | `bash scripts/verify-2487.sh r2` | `docs/evidence/2487/R2-converted.txt` | 每行命中文件含 `FormConflictAlert` 使用且同函数内有 `code ===` 判定 | 仅换文案未加 code 判定 → FAIL |
| R3 | 转换后的引用关系 | `bash scripts/verify-2487.sh r3` | `docs/evidence/2487/R3-shared.txt` | `SHARED_REUSE>=1`；`LOCAL_ALERT_DUP=0`（无本单自建 Alert 组件） | 自建 Alert → FAIL |
| R4 | 6 个文件残留业务 toast | `bash scripts/verify-2487.sh r4` | `docs/evidence/2487/R4-residual.txt` | `BUSINESS_TOAST_RESIDUAL=0`；`KEEP_WITH_REASON` 行数 == `keep:instant` 行数 | 残留 >0 且无理由 → FAIL |
| R5 | HEAD 源码 | `bash scripts/verify-2487.sh r5` | `docs/evidence/2487/R5-build.txt` | `npm run build` exit 0；`npx eslint` 目标文件 0 error | 任一非 0 → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2487.sh r1` | docs/evidence/2487/R1-classify.txt | exit 0；含 `UNCLASSIFIED=0` | PASS | 出现无结论行 → FAIL | dev |
| R2 | `bash scripts/verify-2487.sh r2` | docs/evidence/2487/R2-converted.txt | exit 0；含 `CONVERTED_ROWS>=1` 与 `CODE_FIRST=1` | PASS | 缺 code 判定 → FAIL | dev |
| R3 | `bash scripts/verify-2487.sh r3` | docs/evidence/2487/R3-shared.txt | exit 0；含 `SHARED_REUSE>=1` 与 `LOCAL_ALERT_DUP=0` | PASS | 自建 Alert → FAIL | dev |
| R4 | `bash scripts/verify-2487.sh r4` | docs/evidence/2487/R4-residual.txt | exit 0；含 `BUSINESS_TOAST_RESIDUAL=0` | PASS | 残留 >0 → FAIL | dev |
| R5 | `bash scripts/verify-2487.sh r5` | docs/evidence/2487/R5-build.txt | exit 0；含 `BUILD_OK` 与 `ESLINT_OK` | PASS | 构建失败 → FAIL | dev |

## Artifact Provenance

- R1–R5（脚本生成，首行 provenance）：写 `# build=1.0.<短码>`、`# commit=<40 位 sha>`、`# dirty=false`。
  - docs/evidence/2487/R1-classify.txt
  - docs/evidence/2487/R2-converted.txt
  - docs/evidence/2487/R3-shared.txt
  - docs/evidence/2487/R4-residual.txt
  - docs/evidence/2487/R5-build.txt
- gate（非合同行，由 /work 执行）：`~/.config/opencode/scripts/pre-ready-check.sh 2487 --summary-file … --changed-files … --evidence-file docs/evidence/2487/evidence.json --report docs/evidence/2487/pre-ready-2487.json`；报告的 `build` 字段须匹配 `evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$`。

## Dependencies

- #2480

### 依赖说明（非声明）

- #2480 提供 Tier 1 共享件与 rubric（`frontend-pc/src/utils/formConflict.js`、`frontend-pc/src/components/FormConflictAlert.jsx`）与扫描基线 `docs/evidence/2480/R11-scan.txt`；其提交 `origin/issue-2480`（5 commits）**尚未合入 develop**，合入前本单 `/work` 必须 HOLD，禁止本地复制共享件。
- 平行批次：#2487（人员/成员）、#2488（内容/展示）；下游：#2486（Batch 2 父）。

### Workload

- Size: M
- Basis: 6 个页面（约 30 处 toast 判定）+ 1 个验证脚本 + 6 条证据行；无后端改动、无 migration。
- Confidence: medium（判定依赖逐行人工归类）

### Files（变更文件清单）

- `frontend-pc/src/components/FormConflictAlert.jsx`
- `frontend-pc/src/pages/System/UserManagement.jsx`
- `frontend-pc/src/pages/admin/platform-staff/index.jsx`
- `frontend-pc/src/pages/System/TechnicianProfiles.jsx`
- `scripts/verify-2487.sh`
- `docs/evidence/2487/R1-classify.txt`
- `docs/evidence/2487/R2-converted.txt`
- `docs/evidence/2487/R3-shared.txt`
- `docs/evidence/2487/R4-residual.txt`
- `docs/evidence/2487/R5-build.txt`
- `docs/evidence/2487/pre-ready-2487.json`
- `docs/evidence/2487/evidence.json`

*Model: DeepSeek V4.1 Flash 1017*



