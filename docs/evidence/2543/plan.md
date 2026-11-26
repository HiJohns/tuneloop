## Accepted Plan — #2543 确认执行主体域收敛｜Rev.1（B1：移除带病 IAM 调用）

> Tags: bug, code:go, api-contract
> 背景：`executeAction` 把本地 `users.id` 当 IAM 主体传给 `BindUserToOrganization` / `SetUserCustomerPermissions`；且 `confirmation_sessions` 表在预生产与生产**均不存在**（`backend/database/db.go:730` 注释：`ConfirmationSession, Label — no table ever created (non-persisted)`），整条会话链路在目标环境不可用。

### Owner 裁定与假设（必须显式）

原始 Issue 要求「先由 Owner 在 保留修正(A) / 废弃(B1,B2) 中选一个」。本轮 `/analyze` 为非交互执行，按 Issue 评论（`2026-11-26T09:47:36Z`）的**默认建议 B1** 落计划：

- **D1 = B1**：仅删除 `merchant_admin` 分支中的 IAM 调用（`bind` + `customer-permissions`），保留路由与 `site_manager`/`site_staff` 分支（只写本地），并在代码与文档中写明「该特性无持久化表，禁止在此新增 IAM 调用」。
- 若 Owner 改选 **A**（保留并修正主体为 `user.IAMSub`）或 **B2**（整特性删除），本计划作废，需回 Architect 重发 Rev.2；本计划**不含**补建表、不含任何 IAM 写入。

### 代码事实核对（与 Issue 正文的差异，按代码为准）

| 项 | Issue 正文 | 代码实际 | 结论 |
|---|---|---|---|
| IAM 调用点 | `bind` / `customer-permissions` / `unbind` 三处 | 只有两处：`BindUserToOrganization(session.UserID, merchant.OrgID, toIAMRole("merchant_admin"), "")`（`confirmation_session.go:363`）与 `SetUserCustomerPermissions(merchant.OrgID, session.UserID, …)`（`:368`）；**无 unbind**（`Reject` 只改状态，`grep -n 'iamClient\.' ` 全文件仅 2 处） | 正文的 unbind 已不存在；本计划按实际两处删除 |
| set-perm 失败处理 | 「#2542 修的静默吞错分支」 | 现为 `log.Printf` 后继续（`:369`） | 随两处一起删除，问题一并消失 |
| 可触达性 | 疑似不可达 | `confirmation_sessions` 表在预生产/生产均不存在；`POST /api/confirmation-sessions` 写库即失败；前端 `grep -rn "confirmation-sessions" frontend-pc/src frontend-mobile/src` = 0 | 目标环境不可达；但带病分支一旦被未来复用即写错主体，必须移除 |
| 既有测试覆盖 | — | `confirmation_session_test.go` 的 `ConfirmConfirmationSession_Success` 使用 `action_type=site_staff`；`merchant_admin` 仅出现在 invalid-token/expired 两个在 `Confirm` 前置即返回的用例中 → **无任何断言覆盖 merchant_admin 的 IAM 分支** | 本单**不翻转任何既有断言**；新增用例只做「0 次 IAM 请求」负例 |

### 方案（变更点）

1. **`executeAction` 的 `merchant_admin` 分支降级为纯本地校验 + 显式禁令注释**
   - 删除 `iamClient := services.NewIAMClient()`、`BindUserToOrganization(...)`、`ComputeCusPermBitmapExt`/`SetUserCustomerPermissions(...)` 整段（`confirmation_session.go:355-373`）。
   - 保留：`merchant.ID` 查询与 `merchant not found` 校验（数据错误仍需显式失败）、`user.Status` 激活逻辑（对所有 `ActionType` 共用）。
   - 新增注释（固定措辞，供 R5 断言）：
     `// #2543：本特性无持久化表（db.go 豁免），禁止在此新增 IAM 调用；`
     `// 所有权与角色写入的唯一路径是商户创建/转让（merchant.go、merchant_member.go）。`
2. **清理不再需要的引用**：删除后若 `services`/`middleware` 在本文件仅被该段使用，则移出 import（`go build` 必须通过；若 `middleware` 仍被 `Create` 使用则保留）。
3. **模型注释**：`backend/models/models.go` 的 `ConfirmationSession` 注释追加「非持久化（无迁移建表）；`merchant_admin` 动作不写 IAM（#2543）」。
4. **契约文档**：`docs/spec/api/api.md:6720` 现写「审批确认路径（`ConfirmationSession`，`action_type=merchant_admin`）同样绑定 `OWNER`」—— 与新行为相反，改写为「该路径不写 IAM（无持久化表，legacy 分支）；商户管理员的 `OWNER` 绑定只发生在商户创建（`POST /api/merchants`）与所有权转让（`POST /api/merchants/:id/transfer-ownership`）」。
5. **回归测试（新增，不翻转既有）**：`backend/handlers/confirmation_session_iam_subject_2543_test.go`
   - `TestExecuteAction_MerchantAdmin_NoIAMWrite_2543`：`session.UserID` = 本地 users.id（`iam_sub` 非空）→ `executeAction` 返回 nil；用测试内 IAM 客户端替身/计数记录，断言 **0 次** IAM 调用、0 次以本地 id 为主体的请求。
   - `TestExecuteAction_SiteStaff_Regression_2543`：沿用 `site_staff` 语义，断言 `site_members` 行创建且 0 次 IAM 调用。
   - 负例断言（同一用例内）：若出现任一 IAM 调用 → 直接 FAIL，并打印被调函数与主体。
6. **不做**：不补建 `confirmation_sessions` 表、不恢复该特性、不改路由、不改前端（无调用方）、不改 `site_manager`/`site_staff` 行为。

### Bug Reflection Matrix

| # | 维度 | 记录 |
|---|---|---|
| B1 | 现象 | `executeAction` 把本地 `users.id` 当 IAM 主体（bind / customer-permissions），对端按 `iamUserID` 查主体 → 404/400 |
| B2 | 触发条件 | `session.ActionType == "merchant_admin"` 且 `merchant.OrgID != ""`；`session.UserID` 只能是本地 `users.id`（`Create` 校验本地存在） |
| B3 | 根因 | 本地主键与 IAM 主体是两列（`users.id` / `users.iam_sub`），同一变量 `session.UserID` 被两种语义复用；无命名/类型区分 |
| B4 | 为何未被发现 | 目标环境表不存在 → 路径不可达；既有测试只覆盖 `site_staff`；无「IAM 主体」断言；set-perm 失败仅 log |
| B5 | 修复点 | 删除该分支的两处 IAM 调用（不改主体、不降级为 `iam_sub`），保留本地校验；用注释固化禁令 |
| B6 | 同类扫描 | `grep -rn 'iamClient\.' backend/handlers/confirmation_session.go` → 修后 0 处；`grep -rn 'session.UserID' backend/handlers/confirmation_session.go` → 仅剩本地 DB 用途（user/site_members 查询），无 IAM 传参 |
| B7 | 回归与负例 | `IAM_CALLS=0`、`LOCAL_ID_AS_IAM_SUBJECT=0`；若任何 IAM 调用出现 → FAIL；`site_staff` 既有行为不变 |
| B8 | 防复发 | 代码注释 + `api.md` 口径纠正 + 单测负例；未来若复活该特性（选项 A/B2 重立）必须走「建表迁移 + `iam_sub` 主体 + 双列命名区分」设计评审 |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志） |
|---|---|---|---|---|---|---|
| R1 `merchant_admin` 动作零 IAM 写入且本地成功 | `backend/handlers/confirmation_session.go` | `executeAction`（经 `POST /confirmation-sessions/:id/confirm`） | 本地 user（`iam_sub` 非空）+ `action_type=merchant_admin` + merchant 有 `org_id` | backend（测试库 + IAM 替身计数） | `CONFIRM_STATUS=200`、`SESSION_STATUS=confirmed`、`USER_STATUS=active`、`IAM_CALLS=0` | R1 |
| R2 负例：不得出现以本地 `users.id` 为主体的 IAM 请求 | `backend/handlers/confirmation_session.go` | 同上 + 源码断言 | 修后源码与运行时 | backend | `LOCAL_ID_AS_IAM_SUBJECT=0`、`BIND_SESSION_USERID_HITS=0`、`IAM_CLIENT_REFS=0` | R2 |
| R3 `site_manager`/`site_staff` 回归（未受影响） | `backend/handlers/confirmation_session.go` | 同上（`action_type=site_staff`） | `site_staff` + `action_target_id` | backend | `SITE_MEMBER_CREATED=1`、`SITE_MEMBER_ROLE=Staff`、`IAM_CALLS=0` | R3 |
| R4 构建 + 定向回归（既有确认会话用例不动） | `backend/handlers/confirmation_session.go` | `cd backend && go build ./...` + `go test ./handlers/ -run 'ConfirmationSession'` | 修后工作树 | backend | `BUILD_RC=0`、`REGRESSION_RC=0`、`REGRESSION_PASS=1` | R4 |
| R5 文档与注释口径纠正 | `docs/spec/api/api.md`、`backend/models/models.go` | grep 结构断言 | — | 文档 + 代码注释 | `DOC_API_NO_IAM_ON_CONFIRM=1`、`DOC_API_OWNER_PATHS=1`、`MODEL_COMMENT_NONPERSISTED=1`、`FORBIDDEN_IAM_COMMENT=1` | R5 |

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 本地 user（`id`=uuid，`iam_sub='sub-343'`，`status=pending`）+ merchant（`org_id` 非空）+ session（`action_type=merchant_admin`，`status=waiting`，token 有效） | `bash scripts/verify-2543.sh r1` | `docs/evidence/2543/R1-no-iam-write.txt` | `CONFIRM_STATUS=200`、`SESSION_STATUS=confirmed`、`USER_STATUS=active`、`IAM_CALLS=0` | `IAM_CALLS` > 0，或 `CONFIRM_STATUS` ≠ 200 → FAIL |
| R2 | 修后源码 + R1 运行时调用日志 | `bash scripts/verify-2543.sh r2` | `docs/evidence/2543/R2-subject-negative.txt` | `LOCAL_ID_AS_IAM_SUBJECT=0`、`BIND_SESSION_USERID_HITS=0`、`IAM_CLIENT_REFS=0` | 任一 `=1`（出现本地 id 作 IAM 主体，或分支仍持有 IAM 客户端）→ FAIL |
| R3 | 同 R1，`action_type=site_staff`，`action_target_id` 为网点 uuid | `bash scripts/verify-2543.sh r3` | `docs/evidence/2543/R3-site-branch-regression.txt` | `SITE_MEMBER_CREATED=1`、`SITE_MEMBER_ROLE=Staff`、`IAM_CALLS=0` | `SITE_MEMBER_CREATED=0`（回归破坏）→ FAIL |
| R4 | 修后工作树 | `bash scripts/verify-2543.sh r4` | `docs/evidence/2543/R4-build-regression.txt` | `BUILD_RC=0`、`REGRESSION_RC=0`、`REGRESSION_PASS=1` | `BUILD_RC` ≠ 0，或既有 `ConfirmationSession` 用例 FAIL → FAIL |
| R5 | 修后 `api.md` 与模型注释 | `bash scripts/verify-2543.sh r5` | `docs/evidence/2543/R5-doc-comment.txt` | `DOC_API_NO_IAM_ON_CONFIRM=1`、`DOC_API_OWNER_PATHS=1`、`MODEL_COMMENT_NONPERSISTED=1`、`FORBIDDEN_IAM_COMMENT=1` | 任一 `=0`（文档仍写「同样绑定 OWNER」）→ FAIL |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | runtime | 测试库 + IAM 替身计数（真实 handler 入口） |
| R2 | runtime | R1 运行的 IAM 调用记录 + 源码 grep 断言 |
| R3 | runtime | `site_members` 行读回 + IAM 计数 |
| R4 | runtime | `go build` + 定向 `go test` |
| R5 | static | 文档/注释 grep 结构断言 |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2543.sh r1` | `docs/evidence/2543/R1-no-iam-write.txt` | exit 0；grep `CONFIRM_STATUS=200`、`SESSION_STATUS=confirmed`、`USER_STATUS=active`、`IAM_CALLS=0` | PASS | `IAM_CALLS` 大于 0 或 `CONFIRM_STATUS` 非 200 → FAIL | dev |
| R2 | `bash scripts/verify-2543.sh r2` | `docs/evidence/2543/R2-subject-negative.txt` | exit 0；grep `LOCAL_ID_AS_IAM_SUBJECT=0`、`BIND_SESSION_USERID_HITS=0`、`IAM_CLIENT_REFS=0` | PASS | 任一为 1 → FAIL | dev |
| R3 | `bash scripts/verify-2543.sh r3` | `docs/evidence/2543/R3-site-branch-regression.txt` | exit 0；grep `SITE_MEMBER_CREATED=1`、`SITE_MEMBER_ROLE=Staff`、`IAM_CALLS=0` | PASS | `SITE_MEMBER_CREATED=0` → FAIL | dev |
| R4 | `bash scripts/verify-2543.sh r4` | `docs/evidence/2543/R4-build-regression.txt` | exit 0；grep `BUILD_RC=0`、`REGRESSION_RC=0`、`REGRESSION_PASS=1` | PASS | `BUILD_RC` 非 0 或回归 FAIL → FAIL | dev |
| R5 | `bash scripts/verify-2543.sh r5` | `docs/evidence/2543/R5-doc-comment.txt` | exit 0；grep `DOC_API_NO_IAM_ON_CONFIRM=1`、`DOC_API_OWNER_PATHS=1`、`MODEL_COMMENT_NONPERSISTED=1`、`FORBIDDEN_IAM_COMMENT=1` | PASS | 任一为 0 → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2543/R1-no-iam-write.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2543/R2-subject-negative.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2543/R3-site-branch-regression.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2543/R4-build-regression.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2543/R5-doc-comment.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

逐份另需第二行 `GENERATED_AT_HEAD=<40 位 sha>`；生成器 `scripts/verify-2543.sh`（`r1`–`r5`）在 fix commit 之后运行。

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Workload

- Size: S
- Basis: 1 个后端实现文件（删除 2 处 IAM 调用 + 注释）+ 1 个新增测试文件 + 1 个验证脚本 + 1 份契约文档 + 1 处模型注释 + 5 条证据行；无 migration、无路由变更、无前端变更
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/confirmation_session.go`
- `backend/handlers/confirmation_session_iam_subject_2543_test.go`
- `backend/models/models.go`
- `scripts/verify-2543.sh`
- `docs/spec/api/api.md`
- `docs/evidence/2543/plan.md`
- `docs/evidence/2543/evidence.json`
- `docs/evidence/2543/pre-ready-2543.json`
- `docs/evidence/2543/R1-no-iam-write.txt`
- `docs/evidence/2543/R2-subject-negative.txt`
- `docs/evidence/2543/R3-site-branch-regression.txt`
- `docs/evidence/2543/R4-build-regression.txt`
- `docs/evidence/2543/R5-doc-comment.txt`

## Input Truth & Tool Preflight

- **Source of truth**：
  1. **主体域**：本地 `users.id`（uuid，DB 默认生成）与 IAM 主体 `users.iam_sub` 是两列；任何 IAM 调用必须以 `iam_sub` 为主体（`backend/models/models.go`、`handlers/auth.go:200`）。
  2. **可触达性**：`confirmation_sessions` 表在预生产 `tuneloop_pre_snapshot` 与生产 `tuneloop` 均不存在（Issue 评论 `2026-11-26T09:47:36Z` 只读核对）；`backend/database/db.go:730` 将其列为非持久化豁免。
  3. **无调用方**：前端 `grep -rn "confirmation-sessions" frontend-pc/src frontend-mobile/src` = 0；后端除本文件外无 `ActionType: "merchant_admin"` 创建点。
- **差异核对（实施前先跑，结果入 R2/R4 artifact）**：
  - `grep -n 'iamClient\.' backend/handlers/confirmation_session.go` → 修后 0 处（当前 2 处：`:363` bind、`:368` set-perm）。
  - `grep -n 'session.UserID' backend/handlers/confirmation_session.go` → 修后仅剩本地 DB 用途（`users` / `site_members` 查询与写入），不得出现在任何 IAM 实参位置。
  - `grep -c 'ConfirmationSession' backend/database/db.go` → 豁免清单条目保留（本单不建表）。
- **损坏输入拒绝**：`merchant_admin` 且 `merchant_id` 查不到 → 仍返回 `merchant not found`（不静默吞）；`site_manager`/`site_staff` 分支不变。
- **工具盲区（显式声明）**：
  - IAM 替身只证明「本仓库不发请求」，不证明对端授权；本单已无 IAM 调用，对端无关。
  - 目标环境表不存在 → 本单证据只能用测试库（sqlite/in-memory）证明代码行为，**不得**宣称线上可用；这与 D1=B1（不复活特性）一致。
  - light 节点不查库、不跑构建；D1 判定依据为 Issue 评论中的只读核对结论（已写明 SQL 与结果）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | **B1：移除 `merchant_admin` 分支的 IAM 调用**，保留路由与 `site_*` 分支；不补建表、不改主体为 `iam_sub` | Owner（默认建议）/ Architect | 本计划锁定（Issue 评论 `2026-11-26T09:47:36Z` 建议 B1；不复活不可用特性） | Owner 选 A（修正主体 + 建表）或 B2（整特性删除）→ 本计划作废，回 Architect 重发 |
| D2 | 不为该控件补建 `confirmation_sessions` 表、不加迁移（无调用方，且属新特性立项） | Architect | 本计划锁定 | 若要求建表 → 需独立 Issue（迁移 + 校验 + 回填），本单不得夹带 |
| D3 | `site_manager`/`site_staff` 的本地写入行为保持不变 | Architect | 本计划锁定（回归由 R3 保护） | 若要统一主体口径 → 两者不调 IAM，无收益，回 Architect |
| D4 | 文档口径以「该路径不写 IAM」为准，`api.md:6720` 的「同样绑定 OWNER」必须改写 | Architect | 本计划锁定（代码为准） | 若坚持保留原句 → 与实现相反，审计应判 FAIL |
| D5 | 本单不翻转既有断言（现有 `ConfirmationSession` 用例不覆盖该分支） | Architect | 本计划锁定（见「代码事实核对」末行） | 若审计发现既有断言锁定了 IAM 绑定 → 回 Architect 登记翻转 |

## Out of Scope

- **选项 A**：保留该特性并把主体改成 `user.IAMSub`（含补建 `confirmation_sessions` 表迁移）—— 需 Owner 明确选择后另立 Issue。
- **选项 B2**：整条 confirmation-session 特性删除（handlers/models/routes/audit 映射）—— 需单独评审（可能是邀请模型 v2 脚手架）。
- 前端 `confirmation-sessions` 调用方接入（当前为 0，属新特性立项）。
- 邀请模型 v2 的复用设计（`site_manager`/`site_staff` 确认流）。
- #2542 已修的 `SetUserCustomerPermissions` 失败语义（本单删除该调用，其失败分支随之消失；#2542 的其它结论不受影响）。
- `db.go` 启动校验豁免清单调整（本单不建表，豁免继续有效）。

## Core Update

[Core Update Required]

| 文档路径 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §`10.x` `ConfirmationSession` 说明 + `:6720` 写「审批确认路径（`action_type=merchant_admin`）同样绑定 `OWNER`，不得使用字面量 admin」 | ① `:6720` 改为「该路径**不写 IAM**（`confirmation_sessions` 无持久化表，legacy 分支，禁止新增 IAM 调用，#2543）」；② 同段保留「商户管理员的 `OWNER` 绑定只发生在 `POST /api/merchants`（创建）与 `POST /api/merchants/:id/transfer-ownership`（转让）」；③ 在 confirmation-sessions 端点小节加一行「`action_type=merchant_admin` 仅做本地校验，无 IAM 副作用」 | 文档当前声明的行为与代码缺陷同源，且修正后与实现相反，必须同步 |

## Dependencies

无

### 关联（非硬依赖）

- `#2542`（相关）：`SetUserCustomerPermissions` 失败吞错修复；本单删除该调用，其失败分支在本路径消失。
- `#2540`（accepted）：`toIAMRole("merchant_admin")` 映射来源；本单不再调用 `toIAMRole`，映射本身不变。
- `#2544` / `beaconiam#508`：OWNER 唯一性语义；本单不触碰 IAM 写入路径。

*Model: DeepSeek V4.1 Flash 1017*

