## Accepted Plan — #2542 `executeAction`（merchant_admin）禁止静默吞掉 `SetUserCustomerPermissions` 失败 [Core Update Required]

> 同函数前置：**#2540**（Rev.2，`status:wip`）正在改同一 `case "merchant_admin"` 块的第 360 行 bind 关系角色（`"admin"` → `toIAMRole("merchant_admin")` = `OWNER`），并新增同路径 fake IAM 测试文件。本单在其**之后**落地，避免同函数文本冲突与测试夹具符号冲突。
> 邻居登记：`/analyze` 扫描中发现的主体域问题（本地 `users.id` 当 IAM 主体）另立 **#2543**（本单 Out of Scope，不静默扩张）。

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 商户管理员确认动作：本地置 `active` → IAM bind → 写客户权限位 | `backend/handlers/confirmation_session.go:344-371` |
| bind 失败会 `return fmt.Errorf("IAM bind failed: %w", err)`（错误语义正确） | `backend/handlers/confirmation_session.go:360-362` |
| `SetUserCustomerPermissions` 失败**只 `log.Printf` 后继续**，函数返回 `nil` | `backend/handlers/confirmation_session.go:365-367` |
| 因此 `Confirm` 返回 200「successfully confirmed」；会话已先被写成 `confirmed` | `backend/handlers/confirmation_session.go:192-206` |
| `IAMConfirmationCallback` 仅在 `executeAction` **返回 error** 时才置 `failed`（吞错后走不到） | `backend/handlers/confirmation_session.go:301-308` |
| `SMSCallback` 同样只在返回 error 时回 500，且不落库失败态 | `backend/handlers/confirmation_session.go:431-434` |
| 同类调用点（共 6 处，本单只改第 1 处）：`permission_manage.go:136` 返回 500、`user_staff.go:693-700` 解绑+清理后返回 500、`user_staff.go:806`/`:863` 走 `compensateSiteTransfer` 并回滚、`services/bulk_import_accounts.go:322-330` 解绑+删用户+该行记失败 | `backend/handlers/permission_manage.go`、`backend/handlers/user_staff.go`、`backend/services/bulk_import_accounts.go` |
| 参照实现（激活路径的「绑定失败→解绑+清理→返回 500」） | `backend/handlers/user_staff.go:682-701` |
| `merchant_admin` 模板的客户权限位非空（`instrument:create`=bit0、`order:read`=bit7 等） | `backend/services/role_templates.go:28-45`、`backend/services/permission_registry.go:90-97` |
| 既有确认会话用例只覆盖 `site_staff`（不经 IAM），`merchant_admin` 的失败分支零覆盖 | `backend/handlers/confirmation_session_test.go:242-330` |
| fake IAM 夹具可直接复用既有能力（`SetIAMInternalURLForTesting`） | `backend/handlers/member_role_iam_fail_2461_test.go:52-152`、`backend/services/iam_client.go`（`SetIAMInternalURLForTesting`） |

**根因**：外部 IAM 写失败被降级为日志，`executeAction` 返回 `nil` → 上层按成功回执，会话落 `confirmed`。商户管理员因此「绑定成功但客户权限位为空」，PC/H5 大量按 `cus_perm` 位判定的能力全部不可用，且**无任何错误提示**（违反 AGENTS.md 红线「API 调用链中静默吞错」）。

### 变更点

**A. `backend/handlers/confirmation_session.go`（单文件）**

1. `executeAction` → `case "merchant_admin"`：写客户权限位失败时**回滚绑定并回传错误**（禁止只日志）：

```go
if err := iamClient.SetUserCustomerPermissions(merchant.OrgID, session.UserID, cusPerm, cusPermExt); err != nil {
    // #2542: 客户权限位写入失败 = 管理员权限不可用 → 与 bind 失败同等处理：
    // 回滚绑定（不 purge：主体非本流程创建）并回传错误，禁止返回成功。
    if unbindErr := iamClient.UnbindUserFromOrganization(session.UserID, merchant.OrgID, ""); unbindErr != nil {
        log.Printf("[Confirm] CRITICAL: rollback unbind failed user=%s org=%s: %v (action error: %v)",
            session.UserID, merchant.OrgID, unbindErr, err)
        return fmt.Errorf("IAM set customer permissions failed: %w (rollback unbind also failed: %v)", err, unbindErr)
    }
    return fmt.Errorf("IAM set customer permissions failed: %w", err)
}
```

2. 新增失败态落库helper（三个入口共用，禁止各写一份）：

```go
// failSession persists the terminal "failed" state; the surface receipt keeps
// the original error (JSON 500 / redirect status=failed / SMS 500).
func failSession(db *gorm.DB, session *models.ConfirmationSession, actionErr error) {
    session.Status = "failed"
    session.Message = fmt.Sprintf("Action execution failed: %v", actionErr)
    if err := db.Save(session).Error; err != nil {
        log.Printf("[Confirm] CRITICAL: failed to persist failure state for session %s: %v (action error: %v)",
            session.ID, err, actionErr)
    }
}
```

3. 三个调用点在 `executeAction` 返回 error 时统一调用 `failSession`：
   - `Confirm`（`:203-206`）：保留 HTTP 500 `code:50000`（回执不变），会话由 `confirmed` 覆盖为 `failed`；
   - `IAMConfirmationCallback`（`:301-308`）：会话置 `failed`，重定向改为 `/confirmation-result?status=failed&action=<action_type>`（Issue 期望 1「回执 status=failed」；该 `status=failed` 分支后端已在使用，属既有消费者契约）；
   - `SMSCallback`（`:431-434`）：会话置 `failed`，HTTP 500 文本不变。

4. 不动：`case "site_manager", "site_staff"`、`Reject`、`Create`/`Get`、`executeAction` 的主体取值（`session.UserID`，见 `## Out of Scope` → #2543）、bind 的关系角色（#2540 owner）。

**B. 测试（新增 `backend/handlers/confirmation_session_2542_test.go`，helper 一律加 `2542` 后缀，禁止与 #2540 的 `merchant_admin_confirm_role_2540_test.go` 抢符号）**

5. fake IAM（`httptest.NewServer` + `services.SetIAMInternalURLForTesting` + `t.Setenv("IAM_SECRET"/"IAM_NAMESPACE")`）记录 `bind`/`unbind`（`body.action` 区分）、`customer-permissions`、`perm-version`、`token` 调用次数与状态：
   - bind 恒 200；`customer-permissions` 与 `unbind` 状态可注入（默认 200）；
   - 统计 `DELETE /api/v1/users/` 请求数作为 `PURGE_CALLS`（本单要求为 0）。
6. 夹具：`db := testfixtures.SetupTestDB(t)` + `testutil.SetDB(t, db)`（handler 用 `database.GetDB()` 全局，必须切换，参照 `member_role_iam_fail_2461_test.go:169-186`）；`tenant T`、`org O`、`merchant M(tenant=T, org=O)`、`user U(id=uuid, iam_sub=uuid, tenant=T, org=O, status=pending)`、`session S(user_id=U.id, merchant_id=M.id, action_type=merchant_admin, status=waiting, token, expires=+24h)`；`middleware.PermissionRegistry = services.NewPermissionRegistry()`（测试内替换并恢复，参照 `gift_policy_test.go:165-166`），否则 cus_perm 位恒 0。
7. 用例：
   - `TestConfirmationSession2542_CusPermFailureRollsBack`：`customer-permissions`=500 → `POST /api/confirmation-sessions/:id/confirm` 返回 500/`50000`；会话 `failed`；`BIND_CALLS=1`、`UNBIND_CALLS=1`、`PURGE_CALLS=0`；
   - `TestConfirmationSession2542_SuccessWritesCusPerm`：全 200 → 200/`20000`；会话 `confirmed`；`UNBIND_CALLS=0`；`cus_perm` 含 bit0 与 bit7；
   - `TestConfirmationSession2542_CompensationFailureExplicit`：`customer-permissions`=500 且 `unbind`=500 → 仍返回 500，会话 `failed`，响应 message 含回滚失败信息，日志含 `CRITICAL`（`log.SetOutput` 捕获后恢复）；
   - `TestConfirmationSession2542_IAMCallbackFailureReceipt`：`GET /api/iam/confirmation-callback?session=<id>&result=accept&confirm_type=email`（`customer-permissions`=500）→ 302 且 `Location` 含 `status=failed`，会话 `failed`。

**C. 证据与文档**

8. 新增 `scripts/verify-2542.sh`（`r1`–`r6` + `gate` + `red` + `ledger`，每行首行 provenance），`r5` 同时做邻居静态扫描 + `go build ./...` + 定向回归；`red` 用 `${FIX_SHA}^` 的旧 `confirmation_session.go` 重跑 R1 用例并要求 FAIL（红绿实证）。
9. `docs/spec/api/api.md` §2.8（确认会话）新增失败语义小节（`[Core Update Required]`，见 `## Core Update`）。

- Tags: bug, code:go, scope:backend

### 语义变更（必须翻转的既有断言）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `backend/handlers/confirmation_session.go:203-206`（`Confirm` 失败分支，**无既有断言**） | `executeAction` 失败仍返回 500，但会话保持 `confirmed`（先写后执行） | 会话置 `failed` + `Message` 记录原因；`failed` 不再被 `:187` 的「already confirmed」短路 → 允许修复后重试 | Issue 期望 1：不得按成功留痕 |
| `backend/handlers/confirmation_session.go:301-308`（IAM 回调失败重定向，**无既有断言**） | `status=error&message=<err>` | `status=failed&action=<action_type>`（原因写入 `session.Message`） | Issue 期望 1「回执 status=failed」；`status=failed` 分支后端原本就在产出 |
| `backend/handlers/confirmation_session.go:431-434`（SMS 回调失败，**无既有断言**） | 返回 500，会话保持 `confirmed` | 返回 500 文本不变 + 会话 `failed` | 同 `Confirm`：不得按成功留痕 |
| `backend/handlers/confirmation_session_test.go:242`（`TestConfirmationSessionHandler_Confirm/Success`，`site_staff` 不经 IAM） | 200 + `confirmed` | **保持不变**（纳入 R5 定向回归） | 本单只改失败分支 |

> 旧行为未被任何用例锁定，故无「必须翻绿」的断言；按「语义变更必须新增负例行」纪律，本单强制 `CONFIRM_LOG_ONLY=0`、`SESSION_STATUS=failed`、`UNBIND_CALLS=0`（成功路径）三条「旧行为不再发生」断言。

### Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §2.8「确认会话」只有 `GET /api/confirmation-sessions/:id`，无执行/失败语义 | 新增 `### 2.8.2 确认执行与失败语义`：`POST /confirmation-sessions/:id/confirm` 执行 `merchant_admin` 时 IAM bind 成功但 `customer-permissions` 失败 → 补偿 `unbind`（不 purge）+ HTTP 500 `code 50000` + 会话 `status=failed`（`message` 记原因）；补偿失败另记 CRITICAL 并在错误信息中体现；`GET /api/iam/confirmation-callback` 执行失败 → 302 `status=failed&action=<type>`；成功 → 会话 `confirmed` | 契约文档缺失失败语义，前端/联调无法判断失败回执形态；Issue 期望 1 要求回执 `status=failed` |

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状 | 商户管理员确认「成功」（200 / 成功页），但客户权限位为空 → 大量按钮不可用/列表空，无任何提示 | `confirmation_session.go:365-367`、`:203-206` |
| 2 | 触发条件 | `case "merchant_admin"` 且 IAM bind 成功、`customer-permissions` 返回非 200（IAM 短暂故障 / 主体无权限 / 网关错误） | 同上 |
| 3 | 根因 | 外部写失败被降级为 `log.Printf`，`executeAction` 返回 `nil`；且会话在执行**前**已写 `confirmed`，失败无落库位 | `:192-206`、`:365-367` |
| 4 | 测试盲区 | 既有用例只走 `site_staff`（无 IAM 调用），`merchant_admin` 失败分支零覆盖；无 fake IAM | `confirmation_session_test.go:242-330` |
| 5 | 同类扫描 | 6 处调用点中 5 处已正确（返回 500 / 解绑+清理 / 补偿 / 该行记失败），仅本处吞错 | R5 `NEIGHBOR_CALL_SITES=5`、`NEIGHBOR_UNHANDLED=0` |
| 6 | 修复层 | 调用点（本 handler）+ 统一失败落库 helper，不改 IAM client、不改上层路由 | 变更点 1–3 |
| 7 | 红绿实证 | `red` 用 `${FIX_SHA}^` 旧源码跑 R1 → 必须 FAIL（返回 200/会话 confirmed）；修后 R1 必须 PASS | `scripts/verify-2542.sh red`、R1 |
| 8 | 假阴性反例 | 若仍只 `log.Printf` → R5 `CONFIRM_LOG_ONLY>0`；若失败不落库 → R1/R3/R4 `SESSION_STATUS=confirmed`；若成功路径误解绑 → R2 `UNBIND_CALLS>0`；若补偿失败被吞 → R3 `COMP_ERROR_PRESENT=0`；若回执仍 success → R1 `CODE=20000`、R4 `LOCATION_HAS_STATUS_FAILED=0` | R1/R2/R3/R4/R5 负例 |
| 9 | 回归面 | `TestConfirmationSessionHandler_Create/Get/Confirm`、`TestConfirmationSessionHandler_Confirm/Success`、`site_manager`/`site_staff` 分支、`Reject`；定向回归纳入 R5 | R5 `REG_FAIL=0` |
| 10 | 未修边界 | 主体域（`session.UserID` 当 IAM 主体）→ #2543；bind 关系角色 → #2540；`user.Status` 在失败时已被置 `active`（本流程不改，语义待 Owner）；`IncrementPermVersion` 错误被忽略（部署侧非阻断） | `## Out of Scope`、`## Decision Lock` D6 |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志） |
|---|---|---|---|---|---|---|
| R1 客户权限位失败 → 回滚绑定 + 回执失败 + 会话 failed（红绿） | `backend/handlers/confirmation_session.go`、`backend/handlers/confirmation_session_2542_test.go` | `POST /api/confirmation-sessions/:id/confirm` | `action_type=merchant_admin`；fake IAM `bind=200`、`customer-permissions=500` | backend（Postgres 测试库 + fake IAM） | `STATUS=500`、`CODE=50000`、`SESSION_STATUS=failed`、`BIND_CALLS=1`、`CUSPERM_CALLS=1`、`UNBIND_CALLS=1`、`PURGE_CALLS=0` | R1 |
| R2 成功路径不受影响（负例） | 同上 | 同上 | 全 200 | backend | `STATUS=200`、`CODE=20000`、`SESSION_STATUS=confirmed`、`UNBIND_CALLS=0`、`BIND_CALLS=1`、`CUS_PERM_BIT0=1`、`CUS_PERM_BIT7=1` | R2 |
| R3 补偿本身失败必须显式 | 同上 | 同上 | `customer-permissions=500` 且 `unbind=500` | backend | `STATUS=500`、`SESSION_STATUS=failed`、`UNBIND_CALLS=1`、`UNBIND_FAILED=1`、`COMP_ERROR_PRESENT=1`、`CRITICAL_LOGGED=1` | R3 |
| R4 IAM 回调回执 `status=failed` | 同上 | `GET /api/iam/confirmation-callback` | `result=accept`、`customer-permissions=500` | backend | `REDIRECT_CODE=302`、`LOCATION_HAS_STATUS_FAILED=1`、`SESSION_STATUS=failed` | R4 |
| R5 邻居静态扫描 + 构建 + 定向回归 | `backend/handlers/confirmation_session.go`、`scripts/verify-2542.sh` | `bash scripts/verify-2542.sh r5` | 修后工作树（#2540 已合入） | backend | `CONFIRM_CUSPERM_SITES=1`、`CONFIRM_LOG_ONLY=0`、`CONFIRM_UNBIND_AFTER=1`、`CONFIRM_RETURN_ERR=1`、`NEIGHBOR_CALL_SITES=5`、`NEIGHBOR_UNHANDLED=0`、`BUILD_RC=0`、`REG_FAIL=0` | R5 |
| R6 契约文档同步 | `docs/spec/api/api.md` | grep | 修后文档 | 文档 | `DOC_SESSION_FAILED=1`、`DOC_UNBIND_COMPENSATION=1`、`DOC_CALLBACK_FAILED=1` | R6 |
| R7 pre-ready gate（四源 + 定向测试 + 构建） | `docs/evidence/2542/pre-ready-2542.json` | `pre-ready-check.sh 2542` | 修后干净树 + 最新 Accepted Plan | backend | `overall=PASS`、每行 `pass=true`、`dirty=false`、P13 在 `origin/issue-2542` | R7 |

### Input Truth & Tool Preflight

- **Source of truth（口径唯一）**：Issue #2542「期望」1–5；AGENTS.md 红线「API 调用链中静默吞错」；参照实现的补偿顺序 `backend/handlers/user_staff.go:682-701`（bind 失败 → purge；set-perm 失败 → unbind + purge）。本单**不 purge**（主体非本流程创建，见 D2）。
- **差异核对（只读，已于 /analyze 执行）**：
  - `grep -rn "SetUserCustomerPermissions" backend/` → 6 个调用点（`confirmation_session.go:365`、`permission_manage.go:136`、`user_staff.go:693/806/863`、`bulk_import_accounts.go:322`）+ `iam_client.go` 两处定义；仅 `confirmation_session.go:365` 是「log 后成功返回」；
  - `grep -n "Status = \"failed\"" backend/handlers/confirmation_session.go` → 仅 `:303`（IAM 回调），`Confirm`/`SMSCallback` 缺失；
  - `grep -n "already confirmed" backend/handlers/confirmation_session.go` → `:187`（失败置 `failed` 后不再被短路，语义已推演）。
- **Reachability（Step 3.5 预判）**：入口 `POST /api/confirmation-sessions/:id/confirm`（`backend/main.go:309`）与 `GET /api/iam/confirmation-callback`（`main.go:306` 邻近段）均已注册；`case "merchant_admin"` 命中前置 = 会话 `action_type=merchant_admin` 且 IAM bind 返回 200。**已知风险**：仓库内无 `merchant_admin` 会话创建方（`grep -rn "ActionType: \"merchant_admin\"" backend/` → 0），且 `executeAction` 用本地 `users.id` 作 IAM 主体（→ #2543），故「bind 成功」在线上是否可达**未验证**；本单把该分支修成「失败必回传」属防御性正确化，不改变可达性结论，`/work` 不得以「疑似不可达」为由跳过证据行。
- **损坏输入拒绝**：会话不存在 → 404（不变）；`merchant.OrgID == ""` → 直接跳过 IAM（现有语义，不变）；`AllRoleTemplates["merchant_admin"]` 缺失或权限码为空 → 不写权限位（现有语义，不变，不属本单）。
- **工具盲区声明**：
  - fake IAM 只能证明**请求形态与失败传导**，不能证明对端真实权限位；对端语义以 `permission_calc`（直授 ∪ 角色按位或）为准，本单不改；
  - `grep` 为文本级：R5 以「本文件 1 处调用点 + 0 处 log-only + 错误分支含 unbind/return + 每个邻居调用点向后 6 行命中 `return`/`continue`/`errs = append`/`failed`」作为最小防漂移断言；
  - 全量套件由 pre-ready P5 baseline 差分承担；合同行只跑定向用例 + `go build ./...`。
- **证据时序**：所有 `docs/evidence/2542/R*.txt` 必须在 fix commit **之后**重跑生成；`evidence.json` 的 `build/commit/dirty` 与 R7 `actual` 只由 `bash scripts/verify-2542.sh ledger` 从最新 `fix(issue-2542)` commit 与已 PASS 的 gate 报告派生，禁止手改。

### Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 失败语义 = 回滚 `unbind` + 返回 error；**不 purge**（主体非本流程创建，purge 会删既有账号与其他组织关系） | Architect（Issue 期望 1 允许「视语义决定」） | 本计划锁定 | Owner 若要 purge → 回 Architect 补授权与影响面（属破坏性操作） |
| D2 | 补偿动作失败必须**显式**：`CRITICAL` 日志 + 错误信息携带回滚失败 | Architect | 本计划锁定（Issue 期望 2） | 不允许改成只日志 |
| D3 | 失败态落库 = 三入口共用 `failSession`，`Message` 记录原因；`Confirm`/SMS 的 HTTP 形态不变 | Architect | 本计划锁定 | 若要连 HTTP 码也改 → 属对端契约变更，需 Owner 裁定 |
| D4 | IAM 回调失败重定向由 `status=error&message=` 改为 `status=failed&action=` | Architect（依据 Issue 期望 1 原文） | 本计划锁定 | 若 Owner 要求保留 `message` 查询参数 → 回 Architect，最小改回 `status=error` |
| D5 | 本单在 **#2540 合入后**落地；测试 helper 一律带 `2542` 后缀，禁止与 #2540 的同路径测试抢符号 | Architect | 本计划锁定（前置未满足 `/work` 必须 HOLD） | 若 Owner 要求并行 → 需先去重测试夹具 owner |
| D6 | 主体域（`session.UserID` vs `user.IAMSub`）与「该路径是否废弃」不在本单 | Owner | 已另立 #2543 | 若 #2543 结论为「废弃路径」→ 本单可 abort（当前按 Issue 契约实施防御性错误语义） |

### Out of Scope

- 主体域修正（bind/set-perm/unbind 用本地 `users.id` 而非 `users.iam_sub`）与「merchant_admin 确认路径是否废弃」→ **#2543**。
- bind 的关系角色（`"admin"` → `OWNER`）→ **#2540**（同函数前置）。
- `user.Status` 在动作失败时已被置 `active`（`:344-349`，先写后执行）→ 本单不改；如需回退 `pending` 需 Owner 裁定。
- `AllRoleTemplates["merchant_admin"]` 缺失时的显式告警、`IncrementPermVersion` 错误忽略 → 非本 Issue 范围。
- 其余 5 个 `SetUserCustomerPermissions` 调用点（已正确，仅纳入 R5 静态扫描）。
- 前端任何改动（`/confirmation-result` 页面不在本仓）。
- `site_manager` / `site_staff` 分支的 IAM 语义。
- 存量「确认成功但权限位为空」的商户管理员数据回填 → 只读扫描 + Owner 授权后另立 Ops。

## G0 Golden Acceptance

| 行 ID | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） | assertions（含负例） |
|---|---|---|---|---|
| R1 | T/O/M/U（`U.status=pending`）+ 会话（`merchant_admin`，`status=waiting`，有效期 +24h）；fake IAM：bind 200、customer-permissions 500、unbind 200 | `POST /api/confirmation-sessions/:id/confirm?token=<S.token>`（handler 入口 + tenant claim） | `docs/evidence/2542/R1-cusperm-fail.txt` | `STATUS=500`、`CODE=50000`、`SESSION_STATUS=failed`、`BIND_CALLS=1`、`CUSPERM_CALLS=1`、`UNBIND_CALLS=1`、`PURGE_CALLS=0`；负例：`CODE=20000` 或 `SESSION_STATUS=confirmed` 或 `UNBIND_CALLS=0` → FAIL |
| R2 | 同 R1，但 customer-permissions 200 | 同上 | `docs/evidence/2542/R2-cusperm-ok.txt` | `STATUS=200`、`CODE=20000`、`SESSION_STATUS=confirmed`、`UNBIND_CALLS=0`、`BIND_CALLS=1`、`CUS_PERM_BIT0=1`、`CUS_PERM_BIT7=1`；负例：成功路径发生 unbind 或权限位为 0 → FAIL |
| R3 | 同 R1，但 customer-permissions 500 且 unbind 500 | 同上 | `docs/evidence/2542/R3-compensation-fail.txt` | `STATUS=500`、`SESSION_STATUS=failed`、`UNBIND_CALLS=1`、`UNBIND_FAILED=1`、`COMP_ERROR_PRESENT=1`、`CRITICAL_LOGGED=1`；负例：回滚失败被吞（无 CRITICAL / message 不含回滚信息）→ FAIL |
| R4 | 同 R1（会话带 `iam_session_id` 或直接按 `id` 命中） | `GET /api/iam/confirmation-callback?session=<S.id>&result=accept&confirm_type=email` | `docs/evidence/2542/R4-callback-failed.txt` | `REDIRECT_CODE=302`、`LOCATION_HAS_STATUS_FAILED=1`、`SESSION_STATUS=failed`；负例：`Location` 含 `status=success` 或会话 `confirmed` → FAIL |
| R5 | 修后工作树（#2540 已合入） | `bash scripts/verify-2542.sh r5`（grep 扫描 + `go build ./...` + 定向 `go test ./handlers -run ...`） | `docs/evidence/2542/R5-neighbor-scan-build.txt` | `CONFIRM_CUSPERM_SITES=1`、`CONFIRM_LOG_ONLY=0`、`CONFIRM_UNBIND_AFTER=1`、`CONFIRM_RETURN_ERR=1`、`NEIGHBOR_CALL_SITES=5`、`NEIGHBOR_UNHANDLED=0`、`BUILD_RC=0`、`REG_FAIL=0`；负例：任一邻居点仍「log 后成功」或编译/回归失败 → FAIL |
| R6 | 修后 `docs/spec/api/api.md` | grep | `docs/evidence/2542/R6-docs.txt` | `DOC_SESSION_FAILED=1`、`DOC_UNBIND_COMPENSATION=1`、`DOC_CALLBACK_FAILED=1`；负例：缺任一语义条目 → FAIL |
| R7 | 干净工作树 + 最新 Accepted Plan + `docs/evidence/2542/evidence.json` | `pre-ready-check.sh 2542` | `docs/evidence/2542/pre-ready-2542.json` | `overall=PASS`、每行 `pass=true`、`dirty=false`、P13 在 `origin/issue-2542`；负例：任一 P 项失败 / evidence 缺行 → 禁止 ready |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2542.sh r1 > docs/evidence/2542/R1-cusperm-fail.txt` | `docs/evidence/2542/R1-cusperm-fail.txt` | exit 0；`grep -qx 'R1_OK'` 且 `grep -qx 'SESSION_STATUS=failed'` 且 `grep -qx 'UNBIND_CALLS=1'` | `STATUS=500`、`CODE=50000`、`BIND_CALLS=1`、`CUSPERM_CALLS=1`、`PURGE_CALLS=0` | 仍返回 `CODE=20000` 或会话 `confirmed` 或 `UNBIND_CALLS=0` → `R1_FAIL` 非 0 退出 | dev |
| R2 | `bash scripts/verify-2542.sh r2 > docs/evidence/2542/R2-cusperm-ok.txt` | `docs/evidence/2542/R2-cusperm-ok.txt` | exit 0；`grep -qx 'R2_OK'` 且 `grep -qx 'CUS_PERM_BIT0=1'` 且 `grep -qx 'UNBIND_CALLS=0'` | `STATUS=200`、`CODE=20000`、`SESSION_STATUS=confirmed`、`BIND_CALLS=1`、`CUS_PERM_BIT7=1` | 成功路径出现 unbind 或 `cus_perm` 位为 0 → `R2_FAIL` 非 0 退出 | dev |
| R3 | `bash scripts/verify-2542.sh r3 > docs/evidence/2542/R3-compensation-fail.txt` | `docs/evidence/2542/R3-compensation-fail.txt` | exit 0；`grep -qx 'R3_OK'` 且 `grep -qx 'COMP_ERROR_PRESENT=1'` 且 `grep -qx 'CRITICAL_LOGGED=1'` | `STATUS=500`、`SESSION_STATUS=failed`、`UNBIND_CALLS=1`、`UNBIND_FAILED=1` | 回滚失败被静默（无 CRITICAL / 无回滚信息）→ `R3_FAIL` 非 0 退出 | dev |
| R4 | `bash scripts/verify-2542.sh r4 > docs/evidence/2542/R4-callback-failed.txt` | `docs/evidence/2542/R4-callback-failed.txt` | exit 0；`grep -qx 'R4_OK'` 且 `grep -qx 'LOCATION_HAS_STATUS_FAILED=1'` 且 `grep -qx 'SESSION_STATUS=failed'` | `REDIRECT_CODE=302` | `Location` 含 `status=success` 或会话 `confirmed` → `R4_FAIL` 非 0 退出 | dev |
| R5 | `bash scripts/verify-2542.sh r5 > docs/evidence/2542/R5-neighbor-scan-build.txt` | `docs/evidence/2542/R5-neighbor-scan-build.txt` | exit 0；`grep -qx 'R5_OK'` 且 `grep -qx 'NEIGHBOR_UNHANDLED=0'` 且 `grep -qx 'BUILD_RC=0'` | `CONFIRM_LOG_ONLY=0`、`CONFIRM_UNBIND_AFTER=1`、`NEIGHBOR_CALL_SITES=5`、`REG_FAIL=0` | 任一邻居调用点仍「log 后成功」或编译/定向回归失败 → `R5_FAIL` 非 0 退出 | dev |
| R6 | `bash scripts/verify-2542.sh r6 > docs/evidence/2542/R6-docs.txt` | `docs/evidence/2542/R6-docs.txt` | exit 0；`grep -qx 'R6_OK'` 且 `grep -qx 'DOC_UNBIND_COMPENSATION=1'` | `DOC_SESSION_FAILED=1`、`DOC_CALLBACK_FAILED=1` | 文档缺任一失败语义条目 → `R6_FAIL` 非 0 退出 | dev |
| R7 | `~/.config/opencode/scripts/pre-ready-check.sh 2542 --summary-file docs/evidence/2542/summary.md --changed-files docs/evidence/2542/changed-files.txt --evidence-file docs/evidence/2542/evidence.json --affected-cmd "bash scripts/verify-2542.sh r5" --full-gate-cmd "bash scripts/verify-2542.sh gate" --report docs/evidence/2542/pre-ready-2542.json` | `docs/evidence/2542/pre-ready-2542.json` | exit 0；`jq -e '.overall == "PASS"' docs/evidence/2542/pre-ready-2542.json` 且 evidence 每行 `pass=true` | `overall=PASS`、`dirty=false`、P13 提交在 `origin/issue-2542` | 任一 P 项失败 / evidence 缺行 / `dirty=true` → gate 非 0，禁止 ready | dev |

### Files（变更文件清单）

| 类别 | 路径 |
|---|---|
| Handler 修复 | `backend/handlers/confirmation_session.go` |
| 新增测试 | `backend/handlers/confirmation_session_2542_test.go` |
| 证据生成器（新增） | `scripts/verify-2542.sh` |
| 契约文档（Core Update） | `docs/spec/api/api.md` |
| 证据（计划快照） | `docs/evidence/2542/plan.md` |
| 证据（摘要） | `docs/evidence/2542/summary.md` |
| 证据（清单） | `docs/evidence/2542/changed-files.txt` |
| 证据（合同汇总） | `docs/evidence/2542/evidence.json` |
| 证据 R1 | `docs/evidence/2542/R1-cusperm-fail.txt` |
| 证据 R1（红，非合同产物） | `docs/evidence/2542/R1-red-prefix.txt` |
| 证据 R2 | `docs/evidence/2542/R2-cusperm-ok.txt` |
| 证据 R3 | `docs/evidence/2542/R3-compensation-fail.txt` |
| 证据 R4 | `docs/evidence/2542/R4-callback-failed.txt` |
| 证据 R5 | `docs/evidence/2542/R5-neighbor-scan-build.txt` |
| 证据 R6 | `docs/evidence/2542/R6-docs.txt` |
| Gate 报告 | `docs/evidence/2542/pre-ready-2542.json` |

## Artifact Provenance

- 生成器：`scripts/verify-2542.sh`（`r1`–`r6` + `gate` + `red` + `ledger`），每个文本 artifact 首行必须是
  `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`（锚定本单最新 `fix(issue-2542)` commit；证据收尾提交不改变锚点）。
- 红证据 `docs/evidence/2542/R1-red-prefix.txt` 非合同产物，首行为 pre-fix 锚点 `# build=1.0.<短码> # commit=<fix 的父提交 sha> # dirty=false`，并附 `RED_EXIT=<非 0>` 与 `RED_OK`。
- 逐条清单（全部位于 `docs/evidence/2542/`）：
  - `docs/evidence/2542/R1-cusperm-fail.txt`
  - `docs/evidence/2542/R2-cusperm-ok.txt`
  - `docs/evidence/2542/R3-compensation-fail.txt`
  - `docs/evidence/2542/R4-callback-failed.txt`
  - `docs/evidence/2542/R5-neighbor-scan-build.txt`
  - `docs/evidence/2542/R6-docs.txt`
  - `docs/evidence/2542/pre-ready-2542.json`（gate 报告，JSON 自带 `.provenance.{build,commit,dirty}`）
- `docs/evidence/2542/evidence.json` 字段口径：`build` 形如 `1.0.<7-40 位短码>`，`commit` 为 40 位 sha，`dirty:false`；evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$
- 证据重生成纪律：fix commit 之后统一重跑 `bash scripts/verify-2542.sh r1` … `r6`，禁止只改 `build/commit` 冒充新证据。

### Workload

- Size: S
- Basis: 1 个 handler 文件（1 处错误分支 + 1 个失败落库 helper + 3 个调用点）+ 1 个新测试文件（4 用例）+ 1 个证据脚本 + 1 份契约文档 + 7 条证据行；无 migration、无前端、无新增依赖
- Confidence: high（补偿顺序有 `user_staff.go:682-701` 先例；fake IAM 有 `member_role_iam_fail_2461_test.go` 先例）

## Dependencies

- #2540

### 关联（非硬依赖）

- #2543：本单 `/analyze` 邻居扫描发现的主体域问题（`session.UserID` 当 IAM 主体 + 该路径是否废弃），属本单 Out of Scope。
- #646：`SetUserCustomerPermissions`（raw_bits）引入点，即本次吞错语句的来源。
- #2467 / #2466：#2540 的来源（业务角色当 IAM 角色）。

*Model: DeepSeek V4.1 Flash 1017*

