## Accepted Plan — #2612 [Gap] PC 其余 117 处 catch 直出 `e.message`（网络错误散出 ASCII）收口 `userFacingErrorMessage`

Tags: bug, code:frontend

### 背景与现状核对（代码为据）

| # | 事实 | 依据 |
|---|---|---|
| 1 | 唯一出口已由 **#2609** 落地：`frontend-pc/src/services/apiError.js` 的 `userFacingErrorMessage(err, fallback)` —— 仅 `name === 'ApiError'` 透传（message 已中文化），网络/未知错误回落中文兜底 | #2609 变更点 1 |
| 2 | 本单范围 = **117 行 / 37 文件**（不含 #2609 的三页），按目录 4 批：B1 `pages/System/*` 14 行 / 4 文件；B2 `pages/admin/instrument|property|category/*` 38 行 / 6 文件；B3 `pages/admin/*` 其余 24 行 / 10 文件；B4 顶层 `pages/*` + `components/*` 41 行 / 17 文件 | #2612 正文清单（同口径 `git grep` 复算：14+38+24+41=117，4+6+10+17=37） |
| 3 | 形态分布（同批 `git grep` 归类）：`'中文前缀' + err.message` 74 行；含 `&#124;&#124; '<中文>'` 兜底 75 行；裸 `err.message` 无兜底 1 行；`'…' + (err.message &#124;&#124; e)` 2 行；`'…' + (err.response?.data?.message &#124;&#124; err.message)` 1 行 | `git grep` 归类结果（见「判定表规则」） |
| 4 | 断网时 `fetch` reject 的 `TypeError.message` 是浏览器 ASCII（`Failed to fetch` / `NetworkError when attempting to fetch resource.` / `Load failed`），不经 `resolveHttpFailure`，无 `code`、`name !== 'ApiError'` | `frontend-pc/src/services/api.js:236`（无 try/catch）；#2609 事实 3/4 |
| 5 | 部分写法还会把对象拼进文案：`'清理失败: ' + (err.message &#124;&#124; err)`（`err` 为对象时渲染 `[object Object]`） | `frontend-pc/src/pages/System/UserManagement.jsx` 等处 |
| 6 | 1 处使用 `err.response?.data?.message`（axios 形态）：PC 全量已迁 fetch（`api.js` 唯一 transport），该分支**不可达**，实际只会取 `err.message` | `git grep -n 'err.response' frontend-pc/src` → 仅 1 命中；判定表 `reachable` 列必须标注 |
| 7 | 本单无新增运行时语义：`userFacingErrorMessage` 的语义已由 #2609 的 `frontend-pc/tests/userFacingErrorMessage.test.mjs` 钉死 | #2609 R2 |
| 8 | 当前三方（`res.message` / `resp.message` / `result.message`）为**响应体**字段，不属于本类，不在 117 内 | 扫描器排除口径 |

**结论**：这是**机械批处理**（117 行同一形态族），但每行「兜底中文」需按所在操作命名。因此计划的第一步不是改代码，而是**先产出判定表**（tier + reachable + fallback，`UNCLASSIFIED_ROWS=0`），再按批替换；断言只钉「结构 + 计数 + 零残留」，不逐行钉文案，避免「计划期望值与实现措辞不一致」类返工。

### 判定表规则（本单锁定）

| tier | 修前形态 | 统一改法 | 兜底文案来源 |
|---|---|---|---|
| T1 | `message.X('中文前缀: ' + err.message)` | `message.X(userFacingErrorMessage(err, '<兜底>'))` | 前缀去尾部 `:` `：`（如 `'删除失败: '` → `删除失败`） |
| T2 | `message.X('中文前缀: ' + (err.message &#124;&#124; '中文兜底'))` | 同上 | 沿用**原兜底字面量**（丢弃前缀，避免 ApiError 中文被前缀重复） |
| T3 | `message.X(err.message &#124;&#124; '中文兜底')` | 同上 | 沿用原兜底字面量 |
| T4 | `message.X(err.message)`（无兜底） | 同上 | 判定表按所在操作补一条中文短语（如 `加载失败`） |
| T5 | `message.X('中文前缀: ' + (err.message &#124;&#124; err))` | 同上（**禁止**把对象拼进文案） | 前缀去冒号 |
| T6 | `message.X('中文前缀: ' + (err.response?.data?.message &#124;&#124; err.message))` | 同上（axios 分支不可达，等价 T1） | 前缀去冒号；判定表 `reachable=no`（axios 形态） |

- 只替换**文案表达式**：`isBusinessCorrectableConflict` 前置、`errorFields` 早退、`res.message` 分支、`code`/HTTP/分支顺序、`catch` 边界全部不变。
- 兜底文案**必须是含 CJK 的单引号字面量**（扫描器硬断言，禁止 ASCII 兜底）。
- 禁止在页面内联 `err?.message &#124;&#124;`（`api.md` §1.3，唯一出口 = #2609 的 helper）；禁止 import `ApiError` 类。

### 变更点

1. **判定表先行**：扩展 `scripts/scan-catch-guard-2609.sh` 增加 `--table` 模式（输出 tier / file:line / reachable / fallback / before），#2612 用它产 `docs/evidence/2612/R1-triage-table.txt`；`UNCLASSIFIED_ROWS` 必须为 0 才允许进入改码步骤。
2. **B1–B4 分 4 批替换**（目录批次见事实 2），每批以「同口径扫描该批文件 → 该批残留 0」收口；批次之间一次性提交，避免跨批混杂。
3. **判定表复算差异**：若 `--table` 复算的 `PRE_SITES` ≠ 117（口径漂移），**不得**为凑数调整口径，按 `## Investigation Packet` 回报（`status:analysis`），本计划期望值以复算值为准前必须先由 Architect 回填。
4. **不新增**运行时测试：#2609 的 `userFacingErrorMessage.test.mjs` 已覆盖语义；本单只做接线，新增测试会重复 owner。

### 既有断言影响（0 翻转）

| 检查 | 结果 | 依据 |
|---|---|---|
| 既有测试断言这些 117 处的文案 | **0 处** | `grep -rn "Failed to fetch&#124;删除失败&#124;加载失败" frontend-pc/tests` → 无命中该批页面 |
| `isBusinessCorrectableConflict` 相关断言（#2488/#2569/#2581） | **不变** | 本单不碰冲突持久化分支 |
| `#2609` 的三页 | **不变** | 本单文件清单明确排除（`IN_SCOPE_EXCLUDE`） |
| `OUT_OF_SCOPE` 计数 | 本单后归零 | R4 断言 `RESIDUAL_PC_RAW=0` |

### 批次文件清单（37 文件，与判定表 `FILES=` 互校）

| 批 | 行数 | 文件 |
|---|---|---|
| B1 | 14 | `frontend-pc/src/pages/System/UserManagement.jsx`、`frontend-pc/src/pages/System/GiftPolicies.jsx`、`frontend-pc/src/pages/System/AuditLogPage.jsx`、`frontend-pc/src/pages/System/TechnicianProfiles.jsx` |
| B2 | 38 | `frontend-pc/src/pages/admin/instrument/Detail.jsx`、`frontend-pc/src/pages/admin/instrument/List.jsx`、`frontend-pc/src/pages/admin/instrument/BatchImport.jsx`、`frontend-pc/src/pages/admin/instrument/Form.jsx`、`frontend-pc/src/pages/admin/property/List.jsx`、`frontend-pc/src/pages/admin/category/List.jsx` |
| B3 | 24 | `frontend-pc/src/pages/admin/inventory/RentSetting.jsx`、`frontend-pc/src/pages/admin/face-review/index.jsx`、`frontend-pc/src/pages/admin/warnings/WarningSettings.jsx`、`frontend-pc/src/pages/admin/warnings/SmtpSettings.jsx`、`frontend-pc/src/pages/admin/platform-staff/index.jsx`、`frontend-pc/src/pages/admin/pricing/MerchantPricingConfig.jsx`、`frontend-pc/src/pages/admin/audit/DamageAssessment.jsx`、`frontend-pc/src/pages/admin/transit/TransitCenter.jsx`、`frontend-pc/src/pages/admin/payments/PaymentList.jsx`、`frontend-pc/src/pages/admin/billing/BillingDashboard.jsx` |
| B4 | 41 | `frontend-pc/src/pages/StaffManagement.jsx`、`frontend-pc/src/pages/StaffBulkImport.jsx`、`frontend-pc/src/pages/SiteManagement.jsx`、`frontend-pc/src/pages/SiteBulkImport.jsx`、`frontend-pc/src/pages/LeaseLedger.jsx`、`frontend-pc/src/pages/InviteManagement.jsx`、`frontend-pc/src/pages/DirectMemberManagement.jsx`、`frontend-pc/src/pages/DepositFlow.jsx`、`frontend-pc/src/pages/WarehouseManagement.jsx`、`frontend-pc/src/pages/merchant/NotificationSettings.jsx`、`frontend-pc/src/pages/StaffResetPassword.jsx`、`frontend-pc/src/pages/StaffEdit.jsx`、`frontend-pc/src/pages/MerchantManagement.jsx`、`frontend-pc/src/pages/IAMSyncPage.jsx`、`frontend-pc/src/pages/FinanceConfig.jsx`、`frontend-pc/src/components/IdPhotoDisplay.jsx`、`frontend-pc/src/components/MerchantMemberManagement.jsx` |

### Files（变更文件清单）

- `scripts/scan-catch-guard-2609.sh`
- `scripts/verify-2612.sh`
- `docs/evidence/2612/plan.md`
- `docs/evidence/2612/R1-triage-table.txt`
- `docs/evidence/2612/R2-batch-b1b2.txt`
- `docs/evidence/2612/R3-batch-b3b4.txt`
- `docs/evidence/2612/R4-residual-scan.txt`
- `docs/evidence/2612/R5-build-lint.txt`
- `docs/evidence/2612/evidence.json`
- `docs/evidence/2612/pre-ready-2612.json`
- `frontend-pc/src/pages/System/UserManagement.jsx`
- `frontend-pc/src/pages/System/GiftPolicies.jsx`
- `frontend-pc/src/pages/System/AuditLogPage.jsx`
- `frontend-pc/src/pages/System/TechnicianProfiles.jsx`
- `frontend-pc/src/pages/admin/instrument/Detail.jsx`
- `frontend-pc/src/pages/admin/instrument/List.jsx`
- `frontend-pc/src/pages/admin/instrument/BatchImport.jsx`
- `frontend-pc/src/pages/admin/instrument/Form.jsx`
- `frontend-pc/src/pages/admin/property/List.jsx`
- `frontend-pc/src/pages/admin/category/List.jsx`
- `frontend-pc/src/pages/admin/inventory/RentSetting.jsx`
- `frontend-pc/src/pages/admin/face-review/index.jsx`
- `frontend-pc/src/pages/admin/warnings/WarningSettings.jsx`
- `frontend-pc/src/pages/admin/warnings/SmtpSettings.jsx`
- `frontend-pc/src/pages/admin/platform-staff/index.jsx`
- `frontend-pc/src/pages/admin/pricing/MerchantPricingConfig.jsx`
- `frontend-pc/src/pages/admin/audit/DamageAssessment.jsx`
- `frontend-pc/src/pages/admin/transit/TransitCenter.jsx`
- `frontend-pc/src/pages/admin/payments/PaymentList.jsx`
- `frontend-pc/src/pages/admin/billing/BillingDashboard.jsx`
- `frontend-pc/src/pages/StaffManagement.jsx`
- `frontend-pc/src/pages/StaffBulkImport.jsx`
- `frontend-pc/src/pages/SiteManagement.jsx`
- `frontend-pc/src/pages/SiteBulkImport.jsx`
- `frontend-pc/src/pages/LeaseLedger.jsx`
- `frontend-pc/src/pages/InviteManagement.jsx`
- `frontend-pc/src/pages/DirectMemberManagement.jsx`
- `frontend-pc/src/pages/DepositFlow.jsx`
- `frontend-pc/src/pages/WarehouseManagement.jsx`
- `frontend-pc/src/pages/merchant/NotificationSettings.jsx`
- `frontend-pc/src/pages/StaffResetPassword.jsx`
- `frontend-pc/src/pages/StaffEdit.jsx`
- `frontend-pc/src/pages/MerchantManagement.jsx`
- `frontend-pc/src/pages/IAMSyncPage.jsx`
- `frontend-pc/src/pages/FinanceConfig.jsx`
- `frontend-pc/src/components/IdPhotoDisplay.jsx`
- `frontend-pc/src/components/MerchantMemberManagement.jsx`

> `git diff --name-only` 必须与判定表 `FILES=` 集合逐字相等（R4 的 `FILES_MATCH_TABLE=1` 断言）。

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 判定表（tier + reachable + fallback 全覆盖） | `scripts/scan-catch-guard-2609.sh` | `bash scripts/scan-catch-guard-2609.sh --table` | 修前工作树 | PC（静态） | `PRE_SITES=117`、`PRE_FILES=37`、`TIER_ROWS=117`、`UNCLASSIFIED_ROWS=0`、`FALLBACK_ASCII_LEAK=0`、`NO_FALLBACK_ROWS=0` | R1 |
| R2 B1+B2 批（52 行 / 10 文件） | B1+B2 共 10 文件（见「批次文件清单」）；frontend-pc/src/pages/System/*、frontend-pc/src/pages/admin/instrument/* | `bash scripts/verify-2612.sh r2` | 修后工作树 | PC（静态） | `B1_SITES=14`、`B2_SITES=38`、`B1B2_RESIDUAL=0`、`B1B2_GUARDED=52` | R2 |
| R3 B3+B4 批（65 行 / 27 文件） | B3+B4 共 27 文件（见「批次文件清单」）；frontend-pc/src/pages/admin/*（其余）、frontend-pc/src/pages/*、frontend-pc/src/components/* | `bash scripts/verify-2612.sh r3` | 修后工作树 | PC（静态） | `B3_SITES=24`、`B4_SITES=41`、`B3B4_RESIDUAL=0`、`B3B4_GUARDED=65` | R3 |
| R4 全仓残留 + 计数锁定 | `scripts/scan-catch-guard-2609.sh` | `bash scripts/verify-2612.sh r4` | 修后 `frontend-pc/src` 全目录 | PC（静态） | `RESIDUAL_PC_RAW=0`、`OUT_OF_SCOPE_SITES=0`、`INSCOPE_EXCLUDE_2609=3` | R4 |
| R5 PC 构建 + ESLint（37 文件 + 扫描器） | 37 文件全量（见「批次文件清单」） | `cd frontend-pc && npm run build` 与 `npx eslint` 改动文件 | 修后工作树 | PC 构建产物 | `PC_BUILD=0`、`LINT_EXIT=0`、`BUILD_LINT_OK` | R5 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 修前 `frontend-pc/src` 全目录（117 行 / 37 文件） | `bash scripts/verify-2612.sh r1` | `docs/evidence/2612/R1-triage-table.txt` | `SCAN_EXIT=0`、`PRE_SITES=117`、`PRE_FILES=37`、`TIER_ROWS=117`、`UNCLASSIFIED_ROWS=0`、`FALLBACK_ASCII_LEAK=0`、`NO_FALLBACK_ROWS=0`、`TIER_T1_T6_SUM=117` | 任一行未分类、兜底含 ASCII 字母、缺兜底（T4 未补文案）、行数≠117 → 任一即 FAIL |
| R2 | 修后 B1+B2 文件集（判定表 `FILES=` 前 10 个） | `bash scripts/verify-2612.sh r2` | `docs/evidence/2612/R2-batch-b1b2.txt` | `B1_SITES=14`、`B2_SITES=38`、`B1B2_GUARDED=52`、`B1B2_RESIDUAL=0` | 该批任一站点仍裸 `.message`、站点数被顺手删减、兜底非 CJK → 任一即 FAIL |
| R3 | 修后 B3+B4 文件集（其余 27 个） | `bash scripts/verify-2612.sh r3` | `docs/evidence/2612/R3-batch-b3b4.txt` | `B3_SITES=24`、`B4_SITES=41`、`B3B4_GUARDED=65`、`B3B4_RESIDUAL=0` | 同上 → FAIL |
| R4 | 修后 `frontend-pc/src` 全目录 + #2609 三页排除集 | `bash scripts/verify-2612.sh r4` | `docs/evidence/2612/R4-residual-scan.txt` | `RESIDUAL_PC_RAW=0`、`OUT_OF_SCOPE_SITES=0`、`INSCOPE_EXCLUDE_2609=3`、`GIT_DIFF_FILES=37`、`FILES_MATCH_TABLE=1` | 全仓仍有裸 `.message`、`git diff --name-only` 与判定表 `FILES=` 不等（越界改文件/漏改）、#2609 三页被改 → 任一即 FAIL |
| R5 | 修后工作树（`frontend-pc` 依赖已装） | `bash scripts/verify-2612.sh r5` | `docs/evidence/2612/R5-build-lint.txt` | `PC_BUILD=0`、`LINT_EXIT=0`、`BUILD_LINT_OK` | 构建失败 / ESLint 非 0 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2612.sh r1 > docs/evidence/2612/R1-triage-table.txt` | `docs/evidence/2612/R1-triage-table.txt` | exit 0 且含 `SCAN_EXIT=0` `PRE_SITES=117` `PRE_FILES=37` `TIER_ROWS=117` `UNCLASSIFIED_ROWS=0` `FALLBACK_ASCII_LEAK=0` `NO_FALLBACK_ROWS=0` `TIER_T1_T6_SUM=117` | PASS（8 项 token 全为期望值） | 未分类行、兜底含 ASCII、缺兜底、行数≠117 → FAIL | dev |
| R2 | `bash scripts/verify-2612.sh r2 > docs/evidence/2612/R2-batch-b1b2.txt` | `docs/evidence/2612/R2-batch-b1b2.txt` | exit 0 且含 `B1_SITES=14` `B2_SITES=38` `B1B2_GUARDED=52` `B1B2_RESIDUAL=0` | PASS（4 项 token 全为期望值） | 该批残留裸 `.message`、站点数被删减、兜底非 CJK → FAIL | dev |
| R3 | `bash scripts/verify-2612.sh r3 > docs/evidence/2612/R3-batch-b3b4.txt` | `docs/evidence/2612/R3-batch-b3b4.txt` | exit 0 且含 `B3_SITES=24` `B4_SITES=41` `B3B4_GUARDED=65` `B3B4_RESIDUAL=0` | PASS（4 项 token 全为期望值） | 同上 → FAIL | dev |
| R4 | `bash scripts/verify-2612.sh r4 > docs/evidence/2612/R4-residual-scan.txt` | `docs/evidence/2612/R4-residual-scan.txt` | exit 0 且含 `RESIDUAL_PC_RAW=0` `OUT_OF_SCOPE_SITES=0` `INSCOPE_EXCLUDE_2609=3` `GIT_DIFF_FILES=37` `FILES_MATCH_TABLE=1` | PASS（5 项 token 全为期望值） | 全仓仍有裸 `.message`、改动文件集≠判定表、#2609 三页被改 → FAIL | dev |
| R5 | `bash scripts/verify-2612.sh r5 > docs/evidence/2612/R5-build-lint.txt` | `docs/evidence/2612/R5-build-lint.txt` | exit 0 且含 `PC_BUILD=0` `LINT_EXIT=0` `BUILD_LINT_OK` | PASS（3 项 token 全为期望值） | 构建失败 / ESLint 非 0 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2612.sh`（`r1`–`r5`；沿用 `scripts/verify-2609.sh` 结构：provenance 首行、`heavy-run.sh` 取锁、`FIX_REF` 可覆盖锚点）。
- 每个文本 artifact 首行：`# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `# tree=<HEAD sha>`；证据必须在 fix commit **之后**生成（默认锚点 `git log --grep="^fix(issue-2612)"`）。
- R1 是**修前**快照，但必须在 fix commit 之后用 `git show <base>:<path>` 从基座 ref 读取（不得用工作树脏文件），provenance 仍指向本单 fix commit。
- 重命令（`npm run build`）一律经 `~/.config/opencode/scripts/heavy-run.sh` 取锁。
- 逐条清单（全部位于 `docs/evidence/2612/`）：
  - `docs/evidence/2612/R1-triage-table.txt`
  - `docs/evidence/2612/R2-batch-b1b2.txt`
  - `docs/evidence/2612/R3-batch-b3b4.txt`
  - `docs/evidence/2612/R4-residual-scan.txt`
  - `docs/evidence/2612/R5-build-lint.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2612 --summary-file tmp/pre-ready-2612-summary.md --changed-files tmp/pre-ready-2612-changed.txt --evidence-file docs/evidence/2612/evidence.json --report docs/evidence/2612/pre-ready-2612.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - 站点/形态口径 = `scripts/scan-catch-guard-2609.sh`（#2609 交付，唯一实现；本单只加 `--table` 模式，不改判定核心）；
  - 文案出口 = `frontend-pc/src/services/apiError.js` 的 `userFacingErrorMessage`（#2609）；
  - 传输层契约 = `docs/spec/api/api.md` §1.3（#2609 增补「传输层网络错误」段）；
  - 文件清单 = #2612 正文 37 行表格（与判定表 `FILES=` 互校）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `bash scripts/scan-catch-guard-2609.sh --table` 复算 `PRE_SITES`/`PRE_FILES`，与 117/37 对齐（不一致 → Investigation Packet，不改口径）；
  - `git grep -c "err.response" frontend-pc/src` → 1（axios 不可达分支，判定表须标 `reachable=no`）；
  - `git merge-base --is-ancestor origin/issue-2609 HEAD` → 必须是祖先（基座 = `origin/issue-2609`）。
- **损坏输入拒绝规则**（判定表必须显式覆盖）：
  - 无中文兜底的行（T4）→ 必须补文案，否则 `NO_FALLBACK_ROWS>0` → FAIL；
  - `+ (err.message &#124;&#124; err)` 形态 → 禁止把对象拼进文案；
  - `err.response?.data?.message` → 标 `reachable=no` 并按 T1 处理（不得保留不可达分支的取值优先级）；
  - 兜底字面量含 ASCII 字母 → FAIL。
- **工具盲区（Work Summary 必须记录）**：
  - 静态扫描只证明源码结构；真实断网/真机表现归部署侧 `[Ops-Followup of #2612]`；
  - 逐行文案是否「最贴切」属人工/评审判断（rubric：动词 + 宾语，与所在操作一致）；机器只断言「有 CJK 兜底」；
  - 37 个文件的 `### Files` 由判定表回填，`/work` 必须在提交前补齐（`## Files` 是提交边界）。
- **契约影响**：本单不新增契约（§1.3 已由 #2609 定义）→ Work Summary 仍需 `### 契约三向核对`（pre-ready P7），说明「零契约变更」。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | **基座 = `origin/issue-2609`**（含 #2488 + #2609 的 helper）：`userFacingErrorMessage` 不存在于 develop。集成顺序 2488 → 2609 → 2612 | 本计划锁定 |
| D2 | 唯一出口 = `userFacingErrorMessage`；禁止内联 `err?.message &#124;&#124;`、禁止 import `ApiError` 类、禁止自写第二份判定 | 本计划锁定 |
| D3 | 兜底文案规则见「判定表规则」（T1/T5/T6 取前缀去冒号；T2/T3 沿用原兜底；T4 按操作补短语）；机器只断言 CJK 存在，措辞由评审 rubric 判定 | 本计划锁定 |
| D4 | 分批 = B1/B2（R2）/ B3/B4（R3）；批内一次提交；禁止跨批混提交 | 本计划锁定 |
| D5 | 判定表 `PRE_SITES` 与 117 不一致 → **Investigation Packet + status:analysis**，不得就地改口径或改期望值 | 本计划锁定 |
| D6 | 不新增运行时测试（语义 owner = #2609 的 `userFacingErrorMessage.test.mjs`）；本单只做接线 | 本计划锁定 |
| D7 | UI 行证据 = 源码结构断言 + 全仓残留计数；真实断网/真机表现由 Ops 截图回评（**Owner 豁免**，`evidence_type: static`） | Architect |
| D8 | 重命令经 `heavy-run.sh`；`npx eslint` 只覆盖改动页面（`tests/*.test.mjs` 不在 lint 范围，见 #2609 事实） | 本计划锁定 |
| D9 | 全量回归由 pre-ready P5 baseline 差分承担，合同不写全量 `npm test`/`go test` 作为 exit 0 断言 | 本计划锁定 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | static | 判定表（tier/reachable/fallback 全覆盖，0 未分类）；Owner 豁免见 D7 |
| R2 | static | B1+B2 接线结构断言；Owner 豁免见 D7 |
| R3 | static | B3+B4 接线结构断言；Owner 豁免见 D7 |
| R4 | static | 全仓残留 + 改动文件集一致性；Owner 豁免见 D7 |
| R5 | build | `npm run build` + `npx eslint`（改动文件） |

### Out of Scope

- #2609 的三页（`BannerManagePage` / `MallImageManageCard` / `InstrumentStock`）——已由 #2609 收口。
- `frontend-mobile`（H5 / weapp）的错误文案路径。
- 修改 `resolveHttpFailure` / `fetchBlob` 的失败分类、给 `fetchBlob` 抛 `ApiError`。
- 为 117 处逐一新增运行时组件测试（无 `@testing-library` 依赖，另立前端测试基建 Issue 才可做）。
- 真实断网/真机复验 → `[Ops-Followup of #2612]`（closing 时建，`waitForConfirm`，无 `status:*`）。
- 顺手重构被改页面的其它逻辑（只换文案表达式）。

### Dependencies

- #2609

### Workload

- Size: L
- Basis: 117 处接线 / 37 个页面文件 + 1 个扫描器扩展（`--table`）+ 1 个证据脚本 + 5 条证据行；无 migration、无新端点、无新契约、无运行时语义变更；基座为 `origin/issue-2609`（堆叠分支）
- Confidence: medium

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | PC 各管理页断网时弹出英文技术串（`Failed to fetch`）或 `[object Object]`（`+ err` 拼接） | 事实 3/4/5 |
| 2 | 触发条件（trigger） | `fetch` reject（断网/DNS/CORS/代理中断）或未知异常进入 `catch` 的提示分支 | 事实 4 |
| 3 | 根因（root cause） | 把异常对象 `message` 当用户文案；未区分 `ApiError`（已中文化）与浏览器 `TypeError` | 事实 4、`api.js:236` |
| 4 | 测试盲点（test blind spot） | 这批页面无任何错误文案测试；#2488 的探针只覆盖三页且原先不可证伪 | 事实 7、#2609 事实 6 |
| 5 | 同类扫描（same-class paths） | 本单就是 #2609 的域外同类扫描产物（117 行 / 37 文件） | #2612 正文清单 |
| 6 | 修复层（fix layer） | 提示层批量接线到唯一出口；不碰业务分支与传输层 | 变更点 1–2 |
| 7 | 红绿实证（red-green） | 修前 `PRE_SITES=117` / `RESIDUAL` 非 0；修后 `RESIDUAL_PC_RAW=0` 且分批残留 0；缺陷形态（漏改）→ 对应批 `*_RESIDUAL` 非 0 即 FAIL | R1–R4 |
| 8 | 防复发（prevention） | ① 唯一出口 + `api.md` §1.3 禁令；② 全仓扫描 `RESIDUAL_PC_RAW=0` 作为回归闸；③ 改动文件集必须等于判定表（防漏改/越界） | R4、D2 |
| 9 | 影响面（blast radius） | 仅提示文案来源；`code`/HTTP/分支顺序/成功路径/冲突持久提示不变 | 既有断言影响表 |
| 10 | 未修边界（deferred） | 移动端同类路径、`fetchBlob` 分类、前端测试基建 → 另立；真实断网/真机 → `[Ops-Followup of #2612]` | Out of Scope |

*Model: deepseek/deepseek-flash*

