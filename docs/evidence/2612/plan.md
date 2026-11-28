## Accepted Plan — #2612 [Gap] PC 其余 117 处 catch 直出 `e.message` + 7 处 state→UI 直出（网络错误散出 ASCII）收口 `userFacingErrorMessage`｜Rev.3（R4 计数口径改为两族并集 + `PRE_REF` 锁定 + R6 规则机器化）

> **Rev.3 说明（回应 `/work` 的 needMore：Rev.2 的 R4 与「B5 必须落地」互斥）**：
> ① **R4 diff 口径改为两族并集**：`GIT_DIFF_FILES` 37 → **39**（= `CATCH_FILES=37` + `STATE_FILES=7` − `FAMILY_OVERLAP_FILES=5`），`FILES_MATCH_TABLE` 的比较集合写死为「catch 判定表 `FILES=` ∪ state 族 7 文件」；负例明确「少改任一 state 文件 → READY 前 FAIL」。
> ② **新增可分离断言**：`CATCH_DIFF_FILES=37` / `STATE_DIFF_FILES=7` / `STATE_ONLY_FILES=2` / `FAMILY_OVERLAP_FILES=5` / `UNION_FILES=39`，使「漏改」能定位到族，而不是只表现为一个总数对不上。
> ③ **锁定 `PRE_REF`**（= **最早** `fix(issue-2612)` 提交的父提交）为唯一「修前快照 + diff 基座」，与 `FIX_REF`（最新 fix，provenance 用）**分离**。Rev.2 的 `BASE_REF=${FIX_SHA}^` 默认锚点在**第二个** fix 提交（B5）之后会退化为「B5 的父提交」，使 `PRE_SITES=117` 变 0、`GIT_DIFF_FILES` 变 7 —— 本次一并修掉，避免下一轮再报 needMore。
> ④ **R6 扫描规则机器化**（唯一 owner 仍 = `scripts/scan-catch-guard-2609.sh`，新增 `--state-table`）：setter 名必须含 `Error|Err|Message|Msg`；排除响应体（`res.`/`resp.`/`result.`）、排除比较式（`.message ===`）、排除 `setLoading(...)` 型行；同族 7 处按 S1/S2/S3 分层 = **2/4/1**（可复算，不靠人工点名）。
> ⑤ **`gate` 模式（pre-ready P4）同步并集口径**：禁止再断言 `GIT_DIFF_FILES=37` / `TABLE_FILES=37`（旧口径留在脚本里会让 P4 永远失败）。
> ⑥ 范围不变：仍 **117 catch + 7 state / 39 文件**；B2（`WarehouseManagement` 扫码）继续取 **E2**，跟踪 Issue = **#2634**（已建，`status:todo`）。

Tags: bug, code:frontend

### 背景与现状核对（代码为据）

| # | 事实 | 依据 |
|---|---|---|
| 1 | 唯一出口已由 **#2609** 落地：`frontend-pc/src/services/apiError.js` 的 `userFacingErrorMessage(err, fallback)` —— 仅 `name === 'ApiError'` 透传（message 已中文化），网络/未知错误回落中文兜底 | #2609 变更点 1 |
| 2 | catch 族范围 = **117 行 / 37 文件**（不含 #2609 的三页），按目录 4 批：B1 `pages/System/*` 14 行 / 4 文件；B2 `pages/admin/instrument|property|category/*` 38 行 / 6 文件；B3 `pages/admin/*` 其余 24 行 / 10 文件；B4 顶层 `pages/*` + `components/*` 41 行 / 17 文件 | #2612 正文清单（同口径复算：14+38+24+41=117，4+6+10+17=37） |
| 3 | 形态分布（catch 族）：`'中文前缀' + err.message` 74 行；含 `\|\| '<中文>'` 兜底 75 行；裸 `err.message` 无兜底 1 行；`'…' + (err.message \|\| e)` 2 行；`'…' + (err.response?.data?.message \|\| err.message)` 1 行 | `git grep` 归类结果（见「判定表规则」） |
| 4 | 断网时 `fetch` reject 的 `TypeError.message` 是浏览器 ASCII（`Failed to fetch` / `NetworkError when attempting to fetch resource.` / `Load failed`），不经 `resolveHttpFailure`，无 `code`、`name !== 'ApiError'` | `frontend-pc/src/services/api.js:236`（无 try/catch）；#2609 事实 3/4 |
| 5 | 部分写法还会把对象拼进文案：`'清理失败: ' + (err.message \|\| err)`（`err` 为对象时渲染 `[object Object]`） | `frontend-pc/src/pages/System/UserManagement.jsx` 等处 |
| 6 | 1 处使用 `err.response?.data?.message`（axios 形态）：PC 全量已迁 fetch（`api.js` 唯一 transport），该分支**不可达**，实际只会取 `err.message` | `git grep -n 'err.response' frontend-pc/src` → 仅 1 命中；判定表 `reachable` 列必须标注 |
| 7 | 本单无新增运行时语义：`userFacingErrorMessage` 的语义已由 #2609 的 `frontend-pc/tests/userFacingErrorMessage.test.mjs` 钉死 | #2609 R2 |
| 8 | 响应体三方（`res.message` / `resp.message` / `result.message`）不属于本类，不在 117 内 | 扫描器排除口径 |
| 9 | **state→UI 第二族 = 7 行 / 7 文件**（把异常 `message` 塞进 error state 再渲染）：`App.jsx:958`（`setErrorMsg`）、`pages/AssetAuditDashboard.jsx:39`、`pages/DepositFlow.jsx:58`、`pages/FinanceConfig.jsx:35`、`pages/LeaseLedger.jsx:67`、`pages/StaffEdit.jsx:58`、`pages/System/UserManagement.jsx:194` | `git grep` 规则复算（见「state 族扫描规则」），`PRE_STATE_SITES=7` / `PRE_STATE_FILES=7` |
| 10 | 两族文件集**有 5 处重叠**（StaffEdit、UserManagement、DepositFlow、FinanceConfig、LeaseLedger），state 族**独有 2 个**（`App.jsx`、`AssetAuditDashboard.jsx`）→ 并集 **39** | `CATCH_FILES=37` + `STATE_FILES=7` − `FAMILY_OVERLAP_FILES=5` = `UNION_FILES=39`（脚本逐集合计算，不写死） |
| 11 | 修前是**跨两个 fix 提交**的堆叠：catch 族已落 `fce5ce63a`（父 = `2aa3fb317`，即本单修前基线）；B5（state 族）尚未落地 | `git log origin/issue-2612`（`601fda8df`） |

**结论**：本单是**两族机械批处理**（catch 族 117 行 + state 族 7 行），但「修前基线」跨了两个 fix 提交，因此计划必须同时锁死**站点口径**（两族各自的扫描规则）和**计数基座**（`PRE_REF`），否则第二轮 fix 会让 R1/R4 的期望值自动失效。断言只钉「结构 + 计数 + 零残留」，不逐行钉文案。

### 判定表规则（catch 族 T1–T6，本单锁定）

| tier | 修前形态 | 统一改法 | 兜底文案来源 |
|---|---|---|---|
| T1 | `message.X('中文前缀: ' + err.message)` | `message.X(userFacingErrorMessage(err, '<兜底>'))` | 前缀去尾部 `:` `：`（如 `'删除失败: '` → `删除失败`） |
| T2 | `message.X('中文前缀: ' + (err.message \|\| '中文兜底'))` | 同上 | 沿用**原兜底字面量**（丢弃前缀，避免 ApiError 中文被前缀重复） |
| T3 | `message.X(err.message \|\| '中文兜底')` | 同上 | 沿用原兜底字面量 |
| T4 | `message.X(err.message)`（无兜底） | 同上 | 判定表按所在操作补一条中文短语（如 `加载失败`） |
| T5 | `message.X('中文前缀: ' + (err.message \|\| err))` | 同上（**禁止**把对象拼进文案） | 前缀去冒号 |
| T6 | `message.X('中文前缀: ' + (err.response?.data?.message \|\| err.message))` | 同上（axios 分支不可达，等价 T1） | 前缀去冒号；判定表 `reachable=no`（axios 形态） |

### state 族扫描规则（S1–S3，Rev.3 机器化锁定）

**站点定义（`scripts/scan-catch-guard-2609.sh --state-table`，与 catch 族互不混算）**：

1. 行内必须含 **error/msg 语义的 state setter 调用**：`set[A-Za-z0-9_]*(Error|Err|Message|Msg)[A-Za-z0-9_]*\(`；
2. 且满足其一：**(i) 裸透传** = 含异常接收者 `.message`（接收者限 `e|err|error|ex|exc`，可带 `?.`）；**(ii) 守卫透传** = 含 `userFacingErrorMessage(`；
3. **排除**：接收者为响应体（`res.` / `resp.` / `result.` / `data.`）；`.message` 后紧跟比较运算（`===` / `!==` / `==` / `!=`，如 `pages/admin/instrument/List.jsx:143`）；setter 名不含 `Error|Err|Message|Msg` 的行（如 `setLoading(...)`、`setSortBoundary(...)`）。

| tier | 修前形态 | 统一改法 | 兜底文案来源 |
|---|---|---|---|
| S1 | `setX(err?.message \|\| '中文')` | `setX(userFacingErrorMessage(err, '<原兜底>'))` | 沿用原兜底 |
| S2 | `setX(err.message)`（无兜底） | `setX(userFacingErrorMessage(err, '<按操作补>'))` | 按文件补：`AssetAuditDashboard` → `加载仪表盘失败`；`DepositFlow` → `加载试算数据失败`；`FinanceConfig` → `加载财务配置失败`；`LeaseLedger` → `加载租借台账失败` |
| S3 | `setErrorMsg(error.message \|\| '中文')` | `setErrorMsg(userFacingErrorMessage(error, '<原兜底>'))` | 沿用原兜底（`App.jsx` → `认证失败`） |

- 分层期望：`PRE_STATE_TIER_S1=2`（`StaffEdit`、`UserManagement`）/ `PRE_STATE_TIER_S2=4`（`AssetAuditDashboard`、`DepositFlow`、`FinanceConfig`、`LeaseLedger`）/ `PRE_STATE_TIER_S3=1`（`App.jsx`），合计 `PRE_STATE_SITES=7`。
- 兜底文案**必须是含 CJK 的单引号字面量**（`STATE_FALLBACK_ASCII_LEAK=0`、`STATE_FALLBACK_MISSING=0` 硬断言）。

### 通用约束（两族共同）

- 只替换**文案表达式**：`isBusinessCorrectableConflict` 前置、`errorFields` 早退、`res.message` 分支、`code`/HTTP/分支顺序、`catch` 边界、state 的其它赋值全部不变。
- 禁止在页面内联 `err?.message ||`（`docs/spec/api/api.md` §1.3，唯一出口 = #2609 的 helper）；禁止 import `ApiError` 类；禁止为 state 族另写第二份判定。

### 计数基座（Rev.3 锁定，解决 Rev.2 自相矛盾）

| 符号 | 定义 | 用途 |
|---|---|---|
| `PRE_REF` | **最早** `fix(issue-2612)` 提交的父提交（当前 = `2aa3fb317`）；脚本口径：`git rev-parse "$(git log --format=%H --grep='^fix(issue-2612)' | tail -1)^"`，可用同名环境变量覆盖 | 修前快照（R1/R2/R3/R6 的 `--base`）+ R4 的 diff 基座 |
| `FIX_REF` | **最新** `fix(issue-2612)` 提交 | provenance（`# commit=`）与「证据必须在 fix 之后生成」的锚点 |
| 约束 | `PRE_REF` 必须是 `HEAD` 祖先；且修前快照必须非空（`PRE_SITES=117`、`PRE_STATE_SITES=7` 即为其自证；任一为 0 → FAIL，禁止「空基线假绿」） | — |

**R4 计数口径（两族并集）**：

| token | 口径 | 期望 |
|---|---|---|
| `CATCH_FILES` | 修前 catch 判定表 `FILES=` 计数 | 37 |
| `STATE_FILES` | 修前 state 族文件计数 | 7 |
| `FAMILY_OVERLAP_FILES` | 两族交集计数 | 5 |
| `CATCH_DIFF_FILES` | `git diff PRE_REF..HEAD -- frontend-pc/src` ∩ catch 文件集 | 37 |
| `STATE_DIFF_FILES` | 同上 ∩ state 文件集 | 7 |
| `STATE_ONLY_FILES` | state 族独有文件（`App.jsx`、`AssetAuditDashboard.jsx`）在 diff 中的数量 | 2 |
| `UNION_FILES` | 两族并集计数 | 39 |
| `GIT_DIFF_FILES` | `git diff PRE_REF..HEAD -- frontend-pc/src` 的去重文件数 | 39 |
| `FILES_MATCH_TABLE` | 排序后 `diff` 集 == 排序后并集 | 1 |

> R4 仍只承载 **catch 族**残留（`RESIDUAL_PC_RAW`）；**state 族残留由 R6 单独承载**（`RESIDUAL_STATE_MSG`），两族不得互相顶替。

### 变更点

1. **判定表先行**（catch 族）：扩展 `scripts/scan-catch-guard-2609.sh` 的 `--table` 模式（tier / file:line / reachable / fallback / before），#2612 用它产 `docs/evidence/2612/R1-triage-table.txt`；`UNCLASSIFIED_ROWS` 必须为 0 才允许进入改码步骤。（已落地，Rev.3 不改判定核心）
2. **B1–B4 分 4 批替换**（catch 族目录批次见事实 2），每批以「同口径扫描该批文件 → 该批残留 0」收口；批次之间独立提交。（已落地：`fce5ce63a`）
3. **判定表复算差异**：若 `--table` 复算的 `PRE_SITES` ≠ 117（口径漂移），**不得**为凑数调整口径，按 `## Investigation Packet` 回报（`status:analysis`）。
4. **不新增**运行时测试：#2609 的 `userFacingErrorMessage.test.mjs` 已覆盖语义；本单只做接线。
5. **B5（state 族 7 处 / 7 文件）**：按「state 族扫描规则」S1/S2/S3 替换；单独一个 fix 提交，message 必须以 `fix(issue-2612)` 开头（保证 `FIX_REF`/`PRE_REF` 解析稳定），**禁止 amend / rebase 已推送的 `fce5ce63a`**。
6. **扫描器扩展 `--state-table`**（与 `--table` 并列，不改 catch 判定核心）：输出 `STATE_ROW ...` 逐行 + 汇总 token（pre: `PRE_STATE_*`；post: `STATE_*` / `RESIDUAL_STATE_MSG`）。`List.jsx:143` 的比较式必须被排除（规则 3），否则 `PRE_STATE_SITES` 会变 8 → FAIL。
7. **`verify-2612.sh` 对齐**：`PRE_REF`/`FIX_REF` 分离；新增 `r6`；`r4` 与 `gate` 改并集口径（输出上表全部 token，并打印 `PRE_REF <sha>` / `FIX_REF <sha>` 供审计）。

### 既有断言影响（0 翻转）

| 检查 | 结果 | 依据 |
|---|---|---|
| 既有测试断言这两族的文案 | **0 处** | `grep -rn "Failed to fetch\|删除失败\|加载失败" frontend-pc/tests` → 无命中该批页面 |
| `isBusinessCorrectableConflict` 相关断言（#2488/#2569/#2581） | **不变** | 本单不碰冲突持久化分支 |
| `#2609` 的三页 | **不变** | 本单文件清单明确排除（`INSCOPE_EXCLUDE_2609=3`） |
| `CATCH_SITES=125`（= #2609 的 8 + 本单 117） | **不变** | state 族用独立 token，不参与 `CATCH_*` 求和 |
| `OUT_OF_SCOPE_SITES`（catch 域外） | 本单后归零 | R4 断言 `RESIDUAL_PC_RAW=0` |

### 批次文件清单（catch 37 + state 7，并集 39）

| 批 | 行数 | 文件 |
|---|---|---|
| B1 | 14 | `frontend-pc/src/pages/System/UserManagement.jsx`、`frontend-pc/src/pages/System/GiftPolicies.jsx`、`frontend-pc/src/pages/System/AuditLogPage.jsx`、`frontend-pc/src/pages/System/TechnicianProfiles.jsx` |
| B2 | 38 | `frontend-pc/src/pages/admin/instrument/Detail.jsx`、`frontend-pc/src/pages/admin/instrument/List.jsx`、`frontend-pc/src/pages/admin/instrument/BatchImport.jsx`、`frontend-pc/src/pages/admin/instrument/Form.jsx`、`frontend-pc/src/pages/admin/property/List.jsx`、`frontend-pc/src/pages/admin/category/List.jsx` |
| B3 | 24 | `frontend-pc/src/pages/admin/inventory/RentSetting.jsx`、`frontend-pc/src/pages/admin/face-review/index.jsx`、`frontend-pc/src/pages/admin/warnings/WarningSettings.jsx`、`frontend-pc/src/pages/admin/warnings/SmtpSettings.jsx`、`frontend-pc/src/pages/admin/platform-staff/index.jsx`、`frontend-pc/src/pages/admin/pricing/MerchantPricingConfig.jsx`、`frontend-pc/src/pages/admin/audit/DamageAssessment.jsx`、`frontend-pc/src/pages/admin/transit/TransitCenter.jsx`、`frontend-pc/src/pages/admin/payments/PaymentList.jsx`、`frontend-pc/src/pages/admin/billing/BillingDashboard.jsx` |
| B4 | 41 | `frontend-pc/src/pages/StaffManagement.jsx`、`frontend-pc/src/pages/StaffBulkImport.jsx`、`frontend-pc/src/pages/SiteManagement.jsx`、`frontend-pc/src/pages/SiteBulkImport.jsx`、`frontend-pc/src/pages/LeaseLedger.jsx`、`frontend-pc/src/pages/InviteManagement.jsx`、`frontend-pc/src/pages/DirectMemberManagement.jsx`、`frontend-pc/src/pages/DepositFlow.jsx`、`frontend-pc/src/pages/WarehouseManagement.jsx`、`frontend-pc/src/pages/merchant/NotificationSettings.jsx`、`frontend-pc/src/pages/StaffResetPassword.jsx`、`frontend-pc/src/pages/StaffEdit.jsx`、`frontend-pc/src/pages/MerchantManagement.jsx`、`frontend-pc/src/pages/IAMSyncPage.jsx`、`frontend-pc/src/pages/FinanceConfig.jsx`、`frontend-pc/src/components/IdPhotoDisplay.jsx`、`frontend-pc/src/components/MerchantMemberManagement.jsx` |
| **B5（state 族）** | **7** | `frontend-pc/src/App.jsx`、`frontend-pc/src/pages/AssetAuditDashboard.jsx`、`frontend-pc/src/pages/StaffEdit.jsx`、`frontend-pc/src/pages/System/UserManagement.jsx`、`frontend-pc/src/pages/DepositFlow.jsx`、`frontend-pc/src/pages/FinanceConfig.jsx`、`frontend-pc/src/pages/LeaseLedger.jsx`（**独立计数，不与 B1–B4 混算**） |

### Files（变更文件清单）

- `scripts/scan-catch-guard-2609.sh`
- `scripts/verify-2612.sh`
- `docs/evidence/2612/plan.md`
- `docs/evidence/2612/R1-triage-table.txt`
- `docs/evidence/2612/R2-batch-b1b2.txt`
- `docs/evidence/2612/R3-batch-b3b4.txt`
- `docs/evidence/2612/R4-residual-scan.txt`
- `docs/evidence/2612/R5-build-lint.txt`
- `docs/evidence/2612/R6-state-family.txt`
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
- `frontend-pc/src/App.jsx`
- `frontend-pc/src/pages/AssetAuditDashboard.jsx`
- `frontend-pc/src/components/MerchantMemberManagement.jsx`

> `git diff --name-only PRE_REF..HEAD -- frontend-pc/src` 必须与「catch 判定表 `FILES=` ∪ state 族 7 文件」逐字相等（R4 的 `FILES_MATCH_TABLE=1`；`GIT_DIFF_FILES=39`）。

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 catch 判定表（tier + reachable + fallback 全覆盖） | `scripts/scan-catch-guard-2609.sh` | `bash scripts/verify-2612.sh r1` | 修前快照 `PRE_REF` | PC（静态） | `SCAN_EXIT=0`、`PRE_SITES=117`、`PRE_FILES=37`、`TIER_ROWS=117`、`UNCLASSIFIED_ROWS=0`、`FALLBACK_ASCII_LEAK=0`、`NO_FALLBACK_ROWS=0`、`TIER_T1_T6_SUM=117` | R1 |
| R2 B1+B2 批（52 行 / 10 文件） | B1+B2 共 10 文件（见「批次文件清单」） | `bash scripts/verify-2612.sh r2` | 修后工作树 | PC（静态） | `B1_SITES=14`、`B2_SITES=38`、`B1B2_GUARDED=52`、`B1B2_RESIDUAL=0` | R2 |
| R3 B3+B4 批（65 行 / 27 文件） | B3+B4 共 27 文件（见「批次文件清单」） | `bash scripts/verify-2612.sh r3` | 修后工作树 | PC（静态） | `B3_SITES=24`、`B4_SITES=41`、`B3B4_GUARDED=65`、`B3B4_RESIDUAL=0` | R3 |
| R4 catch 族残留 + 两族并集计数锁定 | `scripts/scan-catch-guard-2609.sh` | `bash scripts/verify-2612.sh r4` | 修后 `frontend-pc/src` 全目录 | PC（静态） | `RESIDUAL_PC_RAW=0`、`OUT_OF_SCOPE_SITES=0`、`INSCOPE_EXCLUDE_2609=3`、`CATCH_FILES=37`、`STATE_FILES=7`、`FAMILY_OVERLAP_FILES=5`、`UNION_FILES=39`、`CATCH_DIFF_FILES=37`、`STATE_DIFF_FILES=7`、`STATE_ONLY_FILES=2`、`GIT_DIFF_FILES=39`、`FILES_MATCH_TABLE=1` | R4 |
| R5 PC 构建 + ESLint（39 文件 + 扫描器） | 39 文件全量（见「批次文件清单」） | `bash scripts/verify-2612.sh r5` | 修后工作树 | PC 构建产物 | `PC_BUILD=0`、`LINT_EXIT=0`、`BUILD_LINT_OK=1` | R5 |
| R6 state→UI 第二族（S1–S3，7 处 / 7 文件） | `frontend-pc/src/App.jsx`、`frontend-pc/src/pages/AssetAuditDashboard.jsx`、`frontend-pc/src/pages/StaffEdit.jsx`、`frontend-pc/src/pages/System/UserManagement.jsx`、`frontend-pc/src/pages/DepositFlow.jsx`、`frontend-pc/src/pages/FinanceConfig.jsx`、`frontend-pc/src/pages/LeaseLedger.jsx`（B5） | `bash scripts/verify-2612.sh r6` | 修前 `PRE_REF` + 修后工作树 | PC（静态） | `PRE_STATE_SITES=7`、`PRE_STATE_FILES=7`、`PRE_STATE_TIER_S1=2`、`PRE_STATE_TIER_S2=4`、`PRE_STATE_TIER_S3=1`、`STATE_SITES=7`、`STATE_GUARDED=7`、`RESIDUAL_STATE_MSG=0`、`STATE_FALLBACK_ASCII_LEAK=0`、`STATE_FALLBACK_MISSING=0`、`CATCH_RAW=0`、`CATCH_GUARDED=125` | R6 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `PRE_REF`（修前，117 行 / 37 文件） | `bash scripts/verify-2612.sh r1` | `docs/evidence/2612/R1-triage-table.txt` | `SCAN_EXIT=0`、`PRE_SITES=117`、`PRE_FILES=37`、`TIER_ROWS=117`、`UNCLASSIFIED_ROWS=0`、`FALLBACK_ASCII_LEAK=0`、`NO_FALLBACK_ROWS=0`、`TIER_T1_T6_SUM=117` | 任一行未分类、兜底含 ASCII 字母、缺兜底（T4 未补文案）、行数≠117、`PRE_REF` 取错（`PRE_SITES=0`）→ 任一即 FAIL |
| R2 | 修后 B1+B2 文件集（10 个） | `bash scripts/verify-2612.sh r2` | `docs/evidence/2612/R2-batch-b1b2.txt` | `B1_SITES=14`、`B2_SITES=38`、`B1B2_GUARDED=52`、`B1B2_RESIDUAL=0` | 该批任一站点仍裸 `.message`、站点数被顺手删减、兜底非 CJK → 任一即 FAIL |
| R3 | 修后 B3+B4 文件集（27 个） | `bash scripts/verify-2612.sh r3` | `docs/evidence/2612/R3-batch-b3b4.txt` | `B3_SITES=24`、`B4_SITES=41`、`B3B4_GUARDED=65`、`B3B4_RESIDUAL=0` | 同上 → FAIL |
| R4 | 修后 `frontend-pc/src` 全目录 + 两族文件集 | `bash scripts/verify-2612.sh r4` | `docs/evidence/2612/R4-residual-scan.txt` | `RESIDUAL_PC_RAW=0`、`OUT_OF_SCOPE_SITES=0`、`INSCOPE_EXCLUDE_2609=3`、`CATCH_FILES=37`、`STATE_FILES=7`、`FAMILY_OVERLAP_FILES=5`、`UNION_FILES=39`、`CATCH_DIFF_FILES=37`、`STATE_DIFF_FILES=7`、`STATE_ONLY_FILES=2`、`GIT_DIFF_FILES=39`、`FILES_MATCH_TABLE=1` | 全仓仍有裸 `.message`、改动文件集≠两族并集（越界改文件）、`GIT_DIFF_FILES`≠39（漏改 1 个 catch 或 state 文件，例：`App.jsx` / `AssetAuditDashboard.jsx` 未改 → `STATE_ONLY_FILES=1`）、#2609 三页被改 → 任一即 FAIL |
| R5 | 修后工作树（`frontend-pc` 依赖已装） | `bash scripts/verify-2612.sh r5` | `docs/evidence/2612/R5-build-lint.txt` | `PC_BUILD=0`、`LINT_EXIT=0`、`BUILD_LINT_OK=1` | 构建失败 / ESLint 非 0 → FAIL |
| R6 | `PRE_REF` 的 state 族 7 处 + 修后同 7 处 | `bash scripts/verify-2612.sh r6` | `docs/evidence/2612/R6-state-family.txt` | `PRE_STATE_SITES=7`、`PRE_STATE_FILES=7`、`PRE_STATE_TIER_S1=2`、`PRE_STATE_TIER_S2=4`、`PRE_STATE_TIER_S3=1`、`STATE_SITES=7`、`STATE_GUARDED=7`、`RESIDUAL_STATE_MSG=0`、`STATE_FALLBACK_ASCII_LEAK=0`、`STATE_FALLBACK_MISSING=0`、`CATCH_RAW=0`、`CATCH_GUARDED=125` | 任一 state 站点仍裸 `.message`、比较式（`List.jsx:143`）被误计/误改、兜底非 CJK 或缺兜底、响应体分支被误迁、`CATCH_*` 被第二族污染 → 任一即 FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2612.sh r1 > docs/evidence/2612/R1-triage-table.txt` | `docs/evidence/2612/R1-triage-table.txt` | exit 0 且含 `SCAN_EXIT=0` `PRE_SITES=117` `PRE_FILES=37` `TIER_ROWS=117` `UNCLASSIFIED_ROWS=0` `FALLBACK_ASCII_LEAK=0` `NO_FALLBACK_ROWS=0` `TIER_T1_T6_SUM=117` | PASS（8 项 token 全为期望值） | 未分类行、兜底含 ASCII、缺兜底、行数≠117、`PRE_REF` 取错 → FAIL | dev |
| R2 | `bash scripts/verify-2612.sh r2 > docs/evidence/2612/R2-batch-b1b2.txt` | `docs/evidence/2612/R2-batch-b1b2.txt` | exit 0 且含 `B1_SITES=14` `B2_SITES=38` `B1B2_GUARDED=52` `B1B2_RESIDUAL=0` | PASS（4 项 token 全为期望值） | 该批残留裸 `.message`、站点数被删减、兜底非 CJK → FAIL | dev |
| R3 | `bash scripts/verify-2612.sh r3 > docs/evidence/2612/R3-batch-b3b4.txt` | `docs/evidence/2612/R3-batch-b3b4.txt` | exit 0 且含 `B3_SITES=24` `B4_SITES=41` `B3B4_GUARDED=65` `B3B4_RESIDUAL=0` | PASS（4 项 token 全为期望值） | 同上 → FAIL | dev |
| R4 | `bash scripts/verify-2612.sh r4 > docs/evidence/2612/R4-residual-scan.txt` | `docs/evidence/2612/R4-residual-scan.txt` | exit 0 且含 `RESIDUAL_PC_RAW=0` `OUT_OF_SCOPE_SITES=0` `INSCOPE_EXCLUDE_2609=3` `CATCH_FILES=37` `STATE_FILES=7` `FAMILY_OVERLAP_FILES=5` `UNION_FILES=39` `CATCH_DIFF_FILES=37` `STATE_DIFF_FILES=7` `STATE_ONLY_FILES=2` `GIT_DIFF_FILES=39` `FILES_MATCH_TABLE=1` | PASS（12 项 token 全为期望值） | 全仓仍有裸 `.message`、改动文件集≠两族并集、`GIT_DIFF_FILES`≠39、state 独有文件漏改、#2609 三页被改 → FAIL | dev |
| R5 | `bash scripts/verify-2612.sh r5 > docs/evidence/2612/R5-build-lint.txt` | `docs/evidence/2612/R5-build-lint.txt` | exit 0 且含 `PC_BUILD=0` `LINT_EXIT=0` `BUILD_LINT_OK=1` | PASS（3 项 token 全为期望值） | 构建失败 / ESLint 非 0 → FAIL | dev |
| R6 | `bash scripts/verify-2612.sh r6 > docs/evidence/2612/R6-state-family.txt` | `docs/evidence/2612/R6-state-family.txt` | exit 0 且含 `PRE_STATE_SITES=7` `PRE_STATE_FILES=7` `PRE_STATE_TIER_S1=2` `PRE_STATE_TIER_S2=4` `PRE_STATE_TIER_S3=1` `STATE_SITES=7` `STATE_GUARDED=7` `RESIDUAL_STATE_MSG=0` `STATE_FALLBACK_ASCII_LEAK=0` `STATE_FALLBACK_MISSING=0` `CATCH_RAW=0` `CATCH_GUARDED=125` | PASS（12 项 token 全为期望值） | state 站残留裸 `.message`、兜底含 ASCII/缺失、比较式误计、响应体分支误迁、`CATCH_*` 被污染 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2612.sh`（`r1`–`r6`；沿用 `scripts/verify-2609.sh` 结构：provenance 首行、`heavy-run.sh` 取锁）。
- **两个锚点分离**（Rev.3）：
  - `PRE_REF` = 最早 `fix(issue-2612)` 提交的父提交（修前快照 + R4 diff 基座），当前 = `2aa3fb317`；
  - `FIX_REF` = 最新 `fix(issue-2612)` 提交（provenance 的 `# commit=`）。
  - 每个 artifact 首行：`# build=1.0.<fix短码> # commit=<FIX_REF 40 位> # dirty=false`，次行 `# tree=<HEAD sha>`，第三行 `# pre_ref=<PRE_REF sha>`。
- **全部 6 份 artifact 在 B5 的 fix 提交之后重新生成**（旧 `820d12ea3` 的 R1–R5 被本 Rev.3 的证据取代；禁止只改 `evidence.json` 的 commit 冒充新证据）。
- 重命令（`npm run build`）一律经 `~/.config/opencode/scripts/heavy-run.sh` 取锁。
- 逐条清单（全部位于 `docs/evidence/2612/`）：
  - `docs/evidence/2612/R1-triage-table.txt`
  - `docs/evidence/2612/R2-batch-b1b2.txt`
  - `docs/evidence/2612/R3-batch-b3b4.txt`
  - `docs/evidence/2612/R4-residual-scan.txt`
  - `docs/evidence/2612/R5-build-lint.txt`
  - `docs/evidence/2612/R6-state-family.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2612 --summary-file tmp/pre-ready-2612-summary.md --changed-files tmp/pre-ready-2612-changed.txt --evidence-file docs/evidence/2612/evidence.json --report docs/evidence/2612/pre-ready-2612.json`
- evidence.json：schema v1，**6 行**（R1–R6）全部 `pass=true`、`dirty=false`、`commit=<FIX_REF>`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - catch 站点口径 = `scripts/scan-catch-guard-2609.sh --table`（#2609 交付；本单只加 `--state-table`，不改 catch 判定核心）；
  - state 站点口径 = 同脚本 `--state-table`（规则见「state 族扫描规则」，Rev.3 首次机器化）；
  - 文案出口 = `frontend-pc/src/services/apiError.js` 的 `userFacingErrorMessage`（#2609）；
  - 传输层契约 = `docs/spec/api/api.md` §1.3；
  - 文件清单 = 本计划「批次文件清单」（catch 37 + state 7，并集 39）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `bash scripts/scan-catch-guard-2609.sh --table --base "$PRE_REF"` → `PRE_SITES=117` / `PRE_FILES=37`；
  - `bash scripts/scan-catch-guard-2609.sh --state-table --base "$PRE_REF"` → `PRE_STATE_SITES=7` / `PRE_STATE_FILES=7` / tier 2/4/1；
  - `git diff --name-only "$PRE_REF"..HEAD -- frontend-pc/src | wc -l` → 当前应为 **37**（B5 尚未落地），B5 落地后必须为 39；
  - `git merge-base --is-ancestor origin/issue-2609 HEAD` → 必须成立（基座 = `origin/issue-2609`）；
  - `git grep -c "err.response" frontend-pc/src` → 1（axios 不可达分支，判定表须标 `reachable=no`）。
- **损坏输入拒绝规则**：
  - `PRE_REF` 解析失败 / 不等于 `HEAD` 祖先 / 修前计数为 0 → 立即判 FAIL（禁止空基线假绿）；
  - 无中文兜底的行（T4、S2）→ 必须补文案，否则 `NO_FALLBACK_ROWS>0` / `STATE_FALLBACK_MISSING>0` → FAIL；
  - `+ (err.message || err)` 形态 → 禁止把对象拼进文案；
  - `err.response?.data?.message` → 标 `reachable=no` 并按 T1 处理（不得保留不可达分支的取值优先级）；
  - 兜底字面量含 ASCII 字母 → FAIL；
  - 比较式（`error.message === '…'`）不是文案出口 → 不计入、不修改。
- **工具盲区（Work Summary 必须记录）**：
  - 静态扫描只证明源码结构；真实断网/真机表现归部署侧 `[Ops-Followup of #2612]`；
  - 逐行文案是否「最贴切」属人工/评审判断（rubric：动词 + 宾语，与所在操作一致）；机器只断言「有 CJK 兜底」；
  - state 族的 `S2` 兜底由 `--state-table` 的文件级映射表给出（4 条），措辞由评审 rubric 判定；
  - `### Files` 是提交边界，`/work` 必须在提交前把实际改动文件与之对齐（多一个/少一个都算 FAIL）。
- **契约影响**：本单零契约变更（§1.3 已由 #2609 定义）→ Work Summary 仍需 `### 契约三向核对`（pre-ready P7），说明「零契约变更」。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | **基座 = `origin/issue-2609`**（含 #2488 + #2609 的 helper）：`userFacingErrorMessage` 不存在于 develop。集成顺序 2488 → 2609 → 2612 | 本计划锁定 |
| D2 | 唯一出口 = `userFacingErrorMessage`；禁止内联 `err?.message \|\|`、禁止 import `ApiError` 类、禁止自写第二份判定 | 本计划锁定 |
| D3 | 兜底文案规则见「判定表规则」/「state 族扫描规则」（T1/T5/T6 取前缀去冒号；T2/T3/S1/S3 沿用原兜底；T4/S2 按操作补短语）；机器只断言 CJK 存在，措辞由评审 rubric 判定 | 本计划锁定 |
| D4 | 分批 = B1/B2（R2）/ B3/B4（R3）/ B5（R6）；批内一次提交；禁止跨批混提交 | 本计划锁定 |
| D5 | 判定表 `PRE_SITES` 与 117 不一致 → **Investigation Packet + status:analysis**，不得就地改口径或改期望值 | 本计划锁定 |
| D6 | 不新增运行时测试（语义 owner = #2609 的 `userFacingErrorMessage.test.mjs`）；本单只做接线 | 本计划锁定 |
| D7 | UI 行证据 = 源码结构断言 + 全仓残留计数；真实断网/真机表现由 Ops 截图回评（**Owner 豁免**，`evidence_type: static`） | Architect |
| D8 | 重命令经 `heavy-run.sh`；`npx eslint` 只覆盖改动页面（范围 = 39 文件，来自 `git diff PRE_REF..HEAD`） | 本计划锁定 |
| D9 | 全量回归由 pre-ready P5 baseline 差分承担，合同不写全量 `npm test`/`go test` 作为 exit 0 断言 | 本计划锁定 |
| D10 | **R4 计数口径 = 两族并集**（`UNION_FILES=39`），`FILES_MATCH_TABLE` 比较集合 =「catch 判定表 `FILES=` ∪ state 族 7 文件」；`GIT_DIFF_FILES=37` / `TABLE_FILES=37` 为**已废止**口径，不得再出现在任何断言里 | 本计划锁定（Rev.3） |
| D11 | **`PRE_REF` 与 `FIX_REF` 分离**：`PRE_REF` = 最早 fix 提交的父提交（修前快照 + R4 diff 基座），`FIX_REF` = 最新 fix（provenance）；B5 必须是**新增** `fix(issue-2612)` 提交，禁止 amend/rebase 已推送的 `fce5ce63a` | 本计划锁定（Rev.3） |
| D12 | `List.jsx:143` 的 `error.message === '已在最前'` 是**字符串相等比较**（非文案出口），只登记不修改；`--state-table` 必须按规则排除它 | 本计划锁定（Rev.3） |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | static | 判定表（tier/reachable/fallback 全覆盖，0 未分类）；Owner 豁免见 D7 |
| R2 | static | B1+B2 接线结构断言；Owner 豁免见 D7 |
| R3 | static | B3+B4 接线结构断言；Owner 豁免见 D7 |
| R4 | static | catch 族全仓残留 + 两族并集计数一致性；Owner 豁免见 D7 |
| R5 | build | `npm run build` + `npx eslint`（改动文件） |
| R6 | static | state 族结构断言（修前 7 / 修后 7 guarded）；Owner 豁免见 D7 |

### Out of Scope

- #2609 的三页（`BannerManagePage` / `MallImageManageCard` / `InstrumentStock`）——已由 #2609 收口。
- `frontend-mobile`（H5 / weapp）的错误文案路径。
- 修改 `resolveHttpFailure` / `fetchBlob` 的失败分类、给 `fetchBlob` 抛 `ApiError`。
- 为两族逐处新增运行时组件测试（无 `@testing-library` 依赖，另立前端测试基建 Issue 才可做）。
- 真实断网/真机复验 → `[Ops-Followup of #2612]`（closing 时建，`waitForConfirm`，无 `status:*`）。
- **`WarehouseManagement` 扫码弹窗（B2 取 E2）**：基线（`fce5ce63a^`）打开弹窗即 `ReferenceError`（`QrScanner` import 已注释），本次删除 `handleScanResult`/`scannerRef` 属清理而非回归；当前仅剩相机预览、**无结果产出控件**。恢复自动扫码需新增前端依赖 `qr-scanner`（打包体积/许可/离线可用性需评估）+ 可用录入控件 → 由 **#2634** 承接（`Parent: #2612`），本单不改。
- `frontend-pc/src/pages/admin/instrument/List.jsx:143` 的 `error.message === '已在最前'` 是**字符串相等比较**（非文案出口），只登记不修改。
- 顺手重构被改页面的其它逻辑（只换文案表达式）。

### Dependencies

- #2609

### 依赖说明（非机器依赖，供审计阅读）

- `#2609`：唯一的阻塞依赖（helper + 扫描器 + 单测，已 `done`；`origin/issue-2609` 为本单分支基座）。
- `#2634`：**非阻塞**关联项（`WarehouseManagement` 扫码控件，`status:todo`），不阻塞本单收口，也不构成 `## Dependencies` 行。

### Workload

- Size: L
- Basis: 两族 124 处接线（catch 117 + state 7）/ 39 个页面文件 + 扫描器扩展（`--table` 已落地、本单加 `--state-table`）+ 证据脚本（`r1`–`r6` + `gate`）+ 6 条证据行；无 migration、无新端点、无新契约、无运行时语义变更；基座为 `origin/issue-2609`（堆叠分支，两个 fix 提交）
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | PC 各管理页断网时弹出英文技术串（`Failed to fetch`）或 `[object Object]`（`+ err` 拼接）；部分页把技术串塞进错误提示区常驻显示 | 事实 3/4/5/9 |
| 2 | 触发条件（trigger） | `fetch` reject（断网/DNS/CORS/代理中断）或未知异常进入 `catch` 的 toast `message` / error-state 分支 | 事实 4、9 |
| 3 | 根因（root cause） | 把异常对象 `message` 当用户文案；未区分 `ApiError`（已中文化）与浏览器 `TypeError` | 事实 4、`api.js:236` |
| 4 | 测试盲点（test blind spot） | 这两族页面无任何错误文案测试；#2488 的探针只覆盖三页 toast | 事实 7、#2609 事实 6 |
| 5 | 同类扫描（same-class paths） | 本单就是 #2609 的域外同类扫描产物：catch 117 + state 7（Rev.2 补齐第二族） | 事实 2/9/10 |
| 6 | 修复层（fix layer） | 提示层批量接线到唯一出口；不碰业务分支与传输层 | 变更点 1–6 |
| 7 | 红绿实证（red-green） | 修前 `PRE_SITES=117` / `PRE_STATE_SITES=7` / 残留非 0；修后 `RESIDUAL_PC_RAW=0`、`RESIDUAL_STATE_MSG=0` 且分批残留 0；缺陷形态（漏改）→ 对应族 token 非 0 即 FAIL | R1–R4、R6 |
| 8 | 防复发（prevention） | ① 唯一出口 + `api.md` §1.3 禁令；② 两族全仓残留作为回归闸；③ 改动文件集必须等于两族并集（防漏改/越界）；④ `PRE_REF`/`FIX_REF` 分离，保证多 fix 提交后修前基线仍可复算 | R4、R6、D10、D11 |
| 9 | 影响面（blast radius） | 仅提示文案来源；`code`/HTTP/分支顺序/成功路径/冲突持久提示/state 其它赋值不变 | 既有断言影响表 |
| 10 | 未修边界（deferred） | 移动端同类路径、`fetchBlob` 分类、前端测试基建 → 另立；扫码控件 → #2634；真实断网/真机 → `[Ops-Followup of #2612]` | Out of Scope |

*Model: DeepSeek V4.1 Flash 1017*

