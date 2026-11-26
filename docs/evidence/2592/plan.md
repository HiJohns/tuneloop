## Accepted Plan — #2592（后端 11 处 5xx 收敛为固定字面量 + 诊断仅进日志）

Tags: bug

### 背景与裁定（Architect）

Issue 给出二选一（① 后端收敛 11 处为固定文案；② 前端把「中文前缀 + 冒号 + CJK 尾串」也纳入 #2560 D2 形态表）。

**裁定 D1：采用 ①（后端收敛）**。

1. 根因在后端：`docs/spec/api/api.md` 的 5xx 口径第 3 条（由 #2560 分支持有）明确「5xx `message` 必须是**固定字面量**，**禁止**拼接 `err.Error()`（诊断只能进日志）」；前端形态判定只是二次网。改前端只会扩大误伤面（合法中文 5xx 文案被吞），且无法阻止未来新增拼接点。
2. 现状实测（`origin/develop` = `2ce72b62c`）：`CJK_ERR_CONCAT=7`（`"message": "中文..." + x.Error()` 形态），另有 **4 处** 同一泄漏面的变体形态不被 #2497 扫描器覆盖（见下），合计 11 处，与 Issue 清单逐条一致。

### 变更点（含验证方式）

1. **7 处 `gin.H` 形态** —— `"message": "<中文>" + <err>.Error()` → 固定字面量 + 上一行 `log.Printf`（原始 error 只进日志）：
   `backend/handlers/merchant.go:621`（UpdateMerchant owner 解析失败，50000）、`:632`（IAM 转让失败，50200）、`backend/handlers/merchant_member.go:55`（ListMembers owner 解析失败）、`:62`（ListMembers viewer 解析失败）、`:497`（UpdateMemberRole owner 解析失败）、`:614`（RemoveMember owner 解析失败）、`:960`（TransferOwnership 兜底，50200）。
   验证：R1（全仓 CJK 拼接归零）+ R3（站点级日志）+ R4（运行时分支）。
2. **3 处 `ownershipTransferError.Message` 结构体字段形态** —— `Message: "<中文>" + <err>.Error()` → 固定字面量 + `log.Printf`：
   `backend/handlers/merchant_member.go:811`（转让内 owner 解析失败）、`:854`（目标成员查询失败）、`:879`（IAM 传输失败非类型化，50200）。
   **注意**：此形态**不被 #2497 的扫描正则覆盖**（该正则只匹配 `"message":` 带引号的键），所以「`CJK_ERR_CONCAT=0`」**不足以**证明本单达成；必须由 R2 的变体扫描 + 固定文案清点补齐。
   验证：R2 + R3 + R4。
3. **1 处 `fmt.Sprintf` 形态** —— `Message: fmt.Sprintf("IAM 已转让，但本地同步失败（旧所有人：%s，新所有人：%s），请联系平台人工核对：%v", prev, target, syncErr)` → 固定字面量（**去掉两个 UUID 与 `%v`**），完整细节进 `log.Printf`：`backend/handlers/merchant_member.go:916`。
   验证：R2（`Message: fmt.Sprintf` 形态归零）+ R4（含 UUID 不回显负例）。
4. **既有断言加强（语义变更必须翻转/加强）** —— `backend/handlers/merchant_ownership_2546_test.go::TestTransferOwnershipLocalSyncFailure_2546` 现仅 `assert.Contains(resp.Message, "人工核对")`（容忍 UUID/诊断回显）。改为：`assert.Equal` 固定字面量 + 断言响应体**不含**注入错误串 `/ 不含 owner UUID / 不含 target UUID`。旧期望 → 新期望记录见「Bug Reflection Matrix」。
   验证：R4（`SITE=merchant_member.go:916` + `LEAK=0`）。
5. **新增回归测试** `backend/handlers/merchant_error_sanitize_2592_test.go`（复用 #2546 夹具/路由助手），逐分支断言「响应只有固定文案 + 日志含原始 error」；GORM 回调注入确定性失败（`Query`/`Update` 的 `tx.AddError`），IAM 传输失败用 `services.SetIAMInternalURLForTesting` 指向已关闭端口。
   验证：R4。
6. **新增证据生成器** `scripts/verify-2592.sh`（r1/r2/r3/r4/r5），首行 provenance；**不修改** `scripts/verify-2497.sh` 或任何共享验收工具（避免「同单改验收规则」）。
   验证：R1–R5 自身即证据。

### 固定文案清单（唯一口径，D5）

| 站点 | HTTP/code | 新固定字面量 | 出现次数 |
|---|---|---|---|
| `merchant.go:621`、`merchant_member.go:55`、`:497`、`:614`、`:811` | 500/50000 | `读取商户所有人信息失败，请稍后重试` | 5 |
| `merchant_member.go:62` | 500/50000 | `读取当前登录账号失败，请稍后重试` | 1 |
| `merchant_member.go:854` | 500/50000 | `查询目标成员失败，请稍后重试` | 1 |
| `merchant.go:632`、`merchant_member.go:879`、`:960` | 502/50200 | `转让所有权失败（IAM），请稍后重试` | 3 |
| `merchant_member.go:916` | 500/50000 | `IAM 已转让，但本地同步失败，请联系平台人工核对` | 1 |

合计 11 处。**不得引入常量/helper 替换这些字面量**（站点级可审计 + 验收清点依赖字面量，见 D5）；**不得保留 `err.Error()` / `%v` / UUID**。`mapTransferOwnershipIAMError`（40303/40400/40900/40002/50200 的 `iamErr.Message` 分支）**语义与文案不动**。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| 全仓 5xx「中文 + `.Error()`」拼接归零（覆盖 7 处 gin.H 站点） | `backend/handlers/merchant.go` + `backend/handlers/merchant_member.go` | 静态扫描 `backend/handlers/*.go` | 修复后工作树 | 后端 | `CJK_ERR_CONCAT=0`，且基线 `BASELINE_HITS=7` 的每一行在交付树中不再存在 | R1 |
| 变体形态归零 + 固定文案清点（覆盖 4 处 `Message:` / `Sprintf` 站点） | `backend/handlers/merchant.go` + `backend/handlers/merchant_member.go` | 静态扫描两个 handler 文件 | 修复后工作树 | 后端 | `MSG_FIELD_CONCAT=0`、`MSG_SPRINTF_CONCAT=0`；文案计数 `LIT_OWNER=5 LIT_VIEWER=1 LIT_TARGET=1 LIT_TRANSFER=3 LIT_SYNC=1` | R2 |
| 诊断仍进日志（11 站点逐一，禁止文件级 grep 兜底） | `backend/handlers/merchant.go` + `backend/handlers/merchant_member.go` | 站点级扫描：固定文案行上方 ≤6 行内同一 error 接收者的 `log.Printf` | 修复后工作树 | 后端 | `LOG_PER_SITE=11`、`LOST_DETAIL=0` | R3 |
| 运行时分支只回固定文案（9 个可达站点）+ 既有 R14 断言加强 | `backend/handlers/merchant_error_sanitize_2592_test.go` + `backend/handlers/merchant_ownership_2546_test.go` | `POST /merchants/:id/transfer-ownership`、`PUT /merchants/:id`、`GET/PUT/DELETE /admin/merchants/:id/members[/:uid]` | 注入失败态（GORM 回调 / IAM 不可达） | 后端（PC/H5/weapp 共用 API） | `RUNTIME_SITES=9`、`LEAK=0`、`# fail 0`；916 分支不含任何 UUID | R4 |
| 编译烟测 | `backend` 模块 | `cd backend && go build ./...` | 修复后工作树 | 后端 | `BUILD_OK=1`、exit 0 | R5 |

## Input Truth & Tool Preflight

- **Source of truth**：Issue #2592 正文的 11 行清单（file:line + 5xx code + 形态）为唯一权威；`docs/spec/api/api.md:6926`（转让本地同步失败必须 500/50000 + 「IAM 已转让，但本地同步失败…请联系平台人工核对」）与 `docs/cases/organization.md:226`（「回滚 + 500/50000（请联系平台人工核对）」）是本单固定文案的上位口径，二者**不要求** UUID/诊断回显。
- **修复前复测（红）**：`origin/develop` 上 `CJK_ERR_CONCAT=7`，变体形态 `Message:` 3 处 + `Message: fmt.Sprintf` 1 处 → 合计 11。红测输出写入 Work Summary（inline key output）；红色运行必然 exit 1，**不作为**合同行产物。
- **差异核对**：修复后 R1 必须重新实测 `BASELINE_HITS`（来自 `origin/develop` 的 `git grep -nP`），且交付树对每一基线行按内容匹配确认「旧拼接行已消失」；基线数字由脚本实测写入 artifact，**计划不硬编码 develop 漂移值**。
- **损坏输入拒绝**：`verify-2592.sh` 对缺失 artifact 目录、缺失 handler 文件、基线 ref 解析失败一律 exit 2（基础设施错误），不得以 0 退出冒充 PASS。
- **工具盲区（必须人工核对）**：
  1. 扫描器是**正则形态匹配**，不解析 AST：动态拼接（变量先拼再赋给 `message`）、`strings.Join`、跨函数构造的文案不在覆盖内 → 本单以「站点级清单（11 行 file:line）+ 固定文案计数」双重锁定，任何一处遗漏都会让计数不等于 5/1/1/3/1。
  2. 运行时行无法覆盖 2 个**防御性不可达**站点（`merchant.go:632`、`merchant_member.go:960`）：`transferMerchantOwnership` 的所有 error 返回都是 `*ownershipTransferError`，`errors.As` 恒真，故这两处的兜底分支在现有类型流下不可达 → 由 R2/R3 以 static 方式断言（见 D6），**不**冒充运行时证据。
  3. 测试依赖本机测试库（`POSTGRES_HOST=127.0.0.1` / `POSTGRES_PORT=15433` / `TUNELOOP_TEST_DB`，同 #2497 口径）；**禁止**设置 `TUNELOOP_TEST_ALLOW_SKIP=1`（会把缺库变成静默 skip）。缺库 = 基础设施错误，须修复环境后重跑。
- **重命令边界**：`go build` / `go test` 经 `heavy-run.sh` 取 host 锁（`VERIFY_2592_NO_LOCK=1` 供 gate 内重入跳过二次取锁）；analyze 阶段不执行任何重命令。

## Decision Lock

| ID | 项 | 裁定 / 锁定条件 |
|---|---|---|
| D1 | 收敛方式 | 采用 Issue 选项 ①（后端固定文案）；**不做**选项 ②（前端形态加严）。理由：根因在后端；api.md 5xx 口径第 3 条要求后端静态扫描为主保证 |
| D2 | 诊断可观测性 | 每处 5xx 必须保留 `log.Printf`，参数含原始 error 与定位标识（merchant id / member id / previous-target owner id）；禁止丢弃诊断 |
| D3 | 核心文档 | **无需 Core Update**：HTTP 状态码、业务码、`mapTransferOwnershipIAMError` 语义均不变；api.md:6926 与 cases/organization.md:226 的既有口径被本实现满足；通用「5xx 固定文案」规则由 #2560 分支持有，本单**不重复写入** api.md（避免与 #2560 合入时同一行冲突） |
| D4 | 范围 | 仅 Issue 列出的 11 处；不改错误码表、不改前端 PC/mobile 解析器、不改 `verify-2497.sh` / `plan-lint.sh` 等共享验收工具 |
| D5 | 文案唯一口径 | 固定字面量必须是「固定文案清单」表中的 5 个字面量原文（内联字面量，不抽常量/helper）；清点计数 5/1/1/3/1 由 R2 断言 |
| D6 | 防御性不可达分支 | `merchant.go:632` 与 `merchant_member.go:960` 在当前类型流下不可达（`transferMerchantOwnership` 恒返回 `*ownershipTransferError`）；**保留**分支（防御未来新增非类型化返回），由 R2/R3 static 断言其文案与日志，计划不声称运行时覆盖 |
| D7 | 前端二次网 | #2560 的 `isInternalDiagnosticMessage` 形态判定继续保持（不回退、不扩表）；本单不新增/修改任何 marker 或 fixture |
| D8 | 验收工具独立性 | 新增 `scripts/verify-2592.sh`，不复用/不改动 #2497 的 `scripts/verify-2497.sh`（其基线口径只覆盖 `"message":` 形态，且其 artifact 目录属 #2497） |
| D9 | 语义权威 | 本计划所有期望值 = `target_semantics`（修复后口径）；`origin/develop` 的 11 处拼接是 `historical_baseline`，仅用于红测与基线覆盖断言，不得作为期望值 |

## Out of Scope

- **不做**前端形态加严（Issue 选项 ②）：会扩大合法中文 5xx 文案的误伤面，需 Owner 单独裁定；本单后端收敛后前端无需变更。
- **ASCII 前缀**的 5xx 拼接站点（全仓 129 处，如 `"failed to create payment: " + err.Error()`）不在本单：`isHumanReadableMessage` 要求 CJK 才进 L2，ASCII 前缀串无回显路径，且不属于 #2546 引入的 CJK 形态；治理需另立 Issue 评估误伤面。
- `mapTransferOwnershipIAMError` 回显上游 `iamErr.Message` 的 3 处（40303/40400/40900/40002/50200）：文案来自 beaconiam 响应体，属跨仓上游契约，不是 `err.Error()` 拼接；若确认上游携带诊断，须在 beaconiam 侧准入收敛（另立跨仓 Issue）。
- 不改错误码 / HTTP 状态 / 审计路由 / 前端文案表；不重构错误处理中间件；不新增 DB 迁移。
- 不修改 `scripts/verify-2497.sh`、`scripts/plan-lint.sh`、`scripts/pre-ready-check.sh` 等共享验收工具。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative | provenance |
|---|---|---|---|---|---|---|
| R1 | `origin/develop` 基线（`git grep -nP` 实测 `BASELINE_HITS`）+ 交付树 | `bash scripts/verify-2592.sh r1` | `docs/evidence/2592/R1-scan.txt` | exit 0；含 `CJK_ERR_CONCAT=0`、`BASELINE_HITS=7`、`MISSING_SITE=` 空 | 交付树任一处 `"message": "中文"+x.Error()` → `CJK_ERR_CONCAT>=1` 且 exit 1 | 首行 `# build=1.0.<短码> # commit=<40sha> # dirty=false` |
| R2 | 两个 handler 文件（`merchant.go` / `merchant_member.go`） | `bash scripts/verify-2592.sh r2` | `docs/evidence/2592/R2-forms.txt` | exit 0；`MSG_FIELD_CONCAT=0`、`MSG_SPRINTF_CONCAT=0`；`LIT_OWNER=5`、`LIT_VIEWER=1`、`LIT_TARGET=1`、`LIT_TRANSFER=3`、`LIT_SYNC=1`；`GIN_CONCAT_DETAILS=` 空 | 任一 `Message: "中文"+x.Error()` 或 `Message: fmt.Sprintf("中文…%v", …)` 存在 → 计数 >0 且 exit 1；文案计数不等 → exit 1 | 同上 |
| R3 | 11 个站点（9 运行时 + 2 防御性） | `bash scripts/verify-2592.sh r3` | `docs/evidence/2592/R3-logs.txt` | exit 0；`LOG_PER_SITE=11`、`LOST_DETAIL=0`；每个 `SITE <file>:<site> covered=1` | 任一固定文案行上方 ≤6 行无引用同一 error 接收者的 `log.Printf` → `LOST_DETAIL>=1` 且 exit 1 | 同上 |
| R4 | #2546 夹具（owner/admin/staff 三成员 + 注入失败）；IAM 指向已关闭端口 | `bash scripts/verify-2592.sh r4` | `docs/evidence/2592/R4-runtime.txt` | exit 0；`RUNTIME_SITES=9`、`LEAK=0`、`# fail 0`、无 `--- FAIL`；每站点 `SITE=<file>:<line>` 标记齐全；916 分支 `UUID_LEAK=0` | 任一分支响应体含注入哨兵（`sentinel2592` / `pq:` / `connection refused`）或 UUID → `LEAK=1` 且 exit 1 | 同上 |
| R5 | 交付树 `backend` 模块 | `bash scripts/verify-2592.sh r5` | `docs/evidence/2592/R5-build.txt` | exit 0；`BUILD_OK=1` | 编译失败 → `BUILD_OK=0` 且 exit 1 | 同上 |

## Bug Reflection Matrix

| # | 现象 | 根因 | 防复发机制 | 验证行 |
|---|---|---|---|---|
| 1 | 5xx 文案拼接 `err.Error()` 被回显给用户（内部 `rows=0` / ID / `pq:` 诊断外溢） | 后端把诊断串直接拼进用户可见 `message`（#2546 引入，晚于 #2497 扫描） | 11 处固定字面量 + 诊断只进日志；D2 强制保留日志 | R1、R2、R3 |
| 2 | 静态扫描「已归零」结论过期（#2497 `CJK_ERR_CONCAT=0` 不再成立） | 扫描只在 #2497 合入时跑一次；后续新增拼接点无回归 | 每单自带 `verify-2592.sh` 并对 `origin/develop` 重新实测基线 | R1 |
| 3 | 4 处变体形态（`Message:` 结构体字段 / `fmt.Sprintf`）逃过 #2497 正则 | #2497 正则只匹配带引号的 `"message":` 键，结构体字段与 `Sprintf` 不在覆盖内 | R2 扩展形态扫描（`MSG_FIELD_CONCAT` / `MSG_SPRINTF_CONCAT`）+ 文案计数 5/1/1/3/1 | R2 |
| 4 | 修文案时把诊断一起丢掉，线上无法排障 | 「固定文案」被误做成「删日志」 | D2：每处保留 `log.Printf`，R3 站点级断言（禁止文件级 grep 兜底） | R3 |
| 5 | 转让本地同步失败回显两个内部 UUID | `fmt.Sprintf` 把 `previousOwnerID` / `targetUserID` / `%v` 塞进用户文案 | 916 改固定字面量；日志保留 ID；测试断言响应体不含 UUID | R4 |
| 6 | 既有测试容忍泄漏（只 `Contains "人工核对"`） | 旧断言按 #2546 的半同步口径写成，未约束「不含诊断/UUID」 | 翻转/加强既有断言为 `Equal` 固定字面量 + 负例断言（本单同批修改） | R4 |
| 7 | 未来有人再加一处「中文 + err.Error()」，无人发现 | 无持续回归网 | R1 全仓目录级扫描（不硬编码文件清单）+ R2 形态扫描纳入本单证据链 | R1、R2 |
| 8 | 前端形态守卫被当作唯一防线，后端长期不合规 | #2560 是二次网，不能替代后端静态保证 | api.md 5xx 口径第 3 条（后端为主）+ 本单 11 处收敛；D7 不回退前端但不豁免后端 | R1、R4 |

## Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2592.sh r1` | docs/evidence/2592/R1-scan.txt | exit 0 且文件含 `CJK_ERR_CONCAT=0` 与 `BASELINE_HITS=7` 且无 `MISSING_SITE=` | PASS | 交付树任一 `"message": "中文"+x.Error()` → `CJK_ERR_CONCAT>=1` 且 exit 1 | dev |
| R2 | `bash scripts/verify-2592.sh r2` | docs/evidence/2592/R2-forms.txt | exit 0 且含 `MSG_FIELD_CONCAT=0`、`MSG_SPRINTF_CONCAT=0`、`LIT_OWNER=5`、`LIT_VIEWER=1`、`LIT_TARGET=1`、`LIT_TRANSFER=3`、`LIT_SYNC=1` | PASS | 任一 `Message: fmt.Sprintf("中文…%v", …)` 残留或文案计数不等 → exit 1 | dev |
| R3 | `bash scripts/verify-2592.sh r3` | docs/evidence/2592/R3-logs.txt | exit 0 且含 `LOG_PER_SITE=11`、`LOST_DETAIL=0` | PASS | 任一站点缺引用同一 error 的 `log.Printf` → `LOST_DETAIL>=1` 且 exit 1 | dev |
| R4 | `bash scripts/verify-2592.sh r4` | docs/evidence/2592/R4-runtime.txt | exit 0 且含 `RUNTIME_SITES=9`、`LEAK=0`、`# fail 0`，且无 `--- FAIL` 与 `UUID_LEAK=1` | PASS | 任一分支响应体含 `sentinel2592` 或 `pq:` 或 UUID → `LEAK=1` 且 exit 1 | dev |
| R5 | `bash scripts/verify-2592.sh r5` | docs/evidence/2592/R5-build.txt | exit 0 且含 `BUILD_OK=1` | PASS | 编译失败 → `BUILD_OK=0` 且 exit 1 | dev |

## Artifact Provenance

- R1–R5 均由 `scripts/verify-2592.sh` 生成，首行统一 provenance：`# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`（取自 fix commit，`dirty` 必须为 `false`）。
- docs/evidence/2592/R1-scan.txt
- docs/evidence/2592/R2-forms.txt
- docs/evidence/2592/R3-logs.txt
- docs/evidence/2592/R4-runtime.txt
- docs/evidence/2592/R5-build.txt
- docs/evidence/2592/plan.md（计划快照，与 Accepted Plan 评论一致；**不出现**在矩阵/合同 artifact 列）
- docs/evidence/2592/evidence.json（`/work` 按合同采集；`build` 字段须匹配 `evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$`）
- gate（由 `/work` 执行）：`~/.config/opencode/scripts/pre-ready-check.sh 2592 --summary-file … --changed-files … --evidence-file docs/evidence/2592/evidence.json --report docs/evidence/2592/pre-ready-2592.json`

## Dependencies

- 无

### 依赖关系说明（非声明，不参与依赖图解析）

- #2560（status:done，修复提交 `73ecb5517` 仅存在于 `origin/issue-2560`，**尚未合入 `origin/develop`**）：本单的「CJK 尾串会绕过形态判定」症状在 #2560 合入后才会真正暴露；但**后端收敛与该分支合入顺序无关**——无论前端是旧粗守卫还是 #2560 的形态判定，11 处固定字面量都不再携带诊断，因此本单不设阻塞依赖，也不改动 #2560 的前端/文档产物。
- #2546（已合入 `origin/develop`，`a5c4d3fc9`）：11 处拼接的引入方；本单只改 `message`/`Message` 文案与日志，不改其业务语义、ID 空间归一化与事务边界。
- #2497（已合入）：其 `scripts/verify-2497.sh` 的基线扫描只覆盖 `"message":` 形态；本单**不修改**它，另建 `scripts/verify-2592.sh` 覆盖变体形态（见 D8）。

### Workload

- Size: M
- Basis: 2 个 handler 文件 11 处站点 + 2 个测试文件（1 新增 / 1 加强）+ 1 个证据脚本 + 5 条证据行 + plan.md 与 gate artifact；无 migration、无前端改动、无跨仓改动
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/merchant.go`
- `backend/handlers/merchant_member.go`
- `backend/handlers/merchant_error_sanitize_2592_test.go`
- `backend/handlers/merchant_ownership_2546_test.go`
- `scripts/verify-2592.sh`
- `docs/evidence/2592/plan.md`
- `docs/evidence/2592/R1-scan.txt`
- `docs/evidence/2592/R2-forms.txt`
- `docs/evidence/2592/R3-logs.txt`
- `docs/evidence/2592/R4-runtime.txt`
- `docs/evidence/2592/R5-build.txt`
- `docs/evidence/2592/evidence.json`
- `docs/evidence/2592/pre-ready-2592.json`

*Model: deepseek/deepseek-flash*

