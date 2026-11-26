## Accepted Plan — #2543 确认会话建表 + IAM 主体改用 `iam_sub`（Owner 2026-11-26 裁定：可以建表）

> 裁定依据：Owner 2026-11-26 —— 「用户/组织/权限/角色一律以 IAM 为事实来源」「确认会话可以建表」。
> 结论：**不做废弃（B1/B2），采用 A** —— 建表让特性真正可用，并把主体修正为 IAM subject。

### 变更点

1. **建表迁移**（`backend/database/migrations/20261126002_confirmation_sessions.up.sql` / `.down.sql`）
   - 列与 `models.ConfirmationSession` 对齐：`id uuid pk default gen_random_uuid()`、`tenant_id`、`org_id`、`user_id`、`confirm_type`、`confirm_target`、`merchant_id`、`action_type`、`action_target_id`、`iam_session_id`、`callback_url`、`status`（默认 `waiting`）、`message`、`token`（唯一）、`expires_at`、`confirmed_at`、`created_at`、`updated_at`；
   - 索引：`user_id`、`tenant_id`、`org_id`、`merchant_id`、`status`、`iam_session_id`、`token` unique；
   - `modelsToValidate` 不再把 `ConfirmationSession` 列为「无表」豁免（`backend/database/db.go:730` 注释同步删除该模型）。
2. **IAM 主体修正**（`backend/handlers/confirmation_session.go` 的 `executeAction`）
   - 先按 `session.UserID` 查本地 `users` 行，取 `iam_sub`；
   - `iam_sub == ""` → 返回显式错误（确认失败、`status=failed`、`message` 含「账号尚未激活，无法绑定组织」），**不得**发任何 IAM 请求；
   - `merchant_admin` 分支：`BindUserToOrganization(iamSub, merchant.OrgID, toIAMRole("merchant_admin"), "")`、`SetUserCustomerPermissions(merchant.OrgID, iamSub, …)`；
   - `site_manager` / `site_staff` 分支只写本地，本身不发 IAM（保持不变），但同样要求本地用户存在。
3. **创建会话的输入校验**（`Create`）：`user_id` 必须命中本地 `users`；不要求此刻有 `iam_sub`（激活可能晚于创建），但响应里带 `iam_ready` 布尔，便于调用方提前提示。
4. **文档**
   - `docs/topics/iam/iam-notes.md` 新增「事实来源」章节：用户/组织/权限/角色一律以 IAM 为唯一事实源；本地 `users`/`merchant_members`/`site_members` 仅为缓存与展示，任何写操作先 IAM 后本地；
   - `docs/spec/permissions.md` 同步一句同义表述 + 「只有命名空间管理员可真正删除用户，商户管理员只能解绑」（与 #2568 D2 一致）；
   - `docs/spec/api/api.md` 补 `POST /api/confirmation-sessions` 系列端点的存在与 `iam_ready` 字段。

### Files

- `backend/database/migrations/20261126002_confirmation_sessions.up.sql`
- `backend/database/migrations/20261126002_confirmation_sessions.down.sql`
- `backend/database/db.go`
- `backend/handlers/confirmation_session.go`
- `backend/handlers/confirmation_session_2543_test.go`（新增）
- `docs/topics/iam/iam-notes.md`
- `docs/spec/permissions.md`
- `docs/spec/api/api.md`
- `scripts/verify-2543.sh`（新增）
- `docs/evidence/2543/plan.md`
- `docs/evidence/2543/evidence.json`
- `docs/evidence/2543/pre-ready-2543.json`
- `docs/evidence/2543/R1-schema.txt`
- `docs/evidence/2543/R2-iam-subject.txt`
- `docs/evidence/2543/R3-empty-sub-guard.txt`
- `docs/evidence/2543/R4-docs.txt`
- `docs/evidence/2543/R5-build.txt`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 表存在且列齐全 | migration + `db.go` | 迁移执行后 | 干净库 | backend | `TABLE_EXISTS=1`、模型列全命中、`EXEMPT_REMOVED=1` | R1 |
| 绑定用 `iam_sub` | `confirmation_session.go` | `POST /api/confirmation-sessions/:id/confirm`（`merchant_admin`） | 本地用户有 `iam_sub` | backend（fake IAM 录音） | `BIND_USER_ARG=iam_sub`、`IAM_LOCAL_ID_SENT=0`、`ROLE=OWNER` | R2 |
| 无 `iam_sub` 显式失败 | 同上 | 同上 | 本地用户 `iam_sub=""` | backend | `CONFIRM_STATUS=failed`、`IAM_CALLS=0`、`MESSAGE_HAS_ACTIVATE=1` | R3 |
| 文档口径 | 三份文档 | grep | — | 文档 | `DOC_FACT_SOURCE=1`、`DOC_DELETE_SYSADMIN_ONLY=1`、`DOC_IAM_READY=1` | R4 |
| 构建/测试 | backend | 定向测试 + `go build` + 迁移升降级 | 修后树 | backend | 全绿 | R5 |

### Input Truth & Tool Preflight

- 现状：`confirmation_session.go:355-372` 已按 #2540 D4 改为 `toIAMRole("merchant_admin")`（OWNER），但主体仍是 `session.UserID`（本地 `users.id`），且 `confirmation_sessions` 表在预生产与生产**都不存在**（2026-11-26 只读核对）。
- IAM 端：`PUT /api/v1/organizations/{org}/users/{iamUserID}/bind`（`backend/services/iam_client.go:1256`）要求 IAM 用户 id。
- 迁移纪律：up/down 成对；`schema_migrations` 版本递增；生产库执行需走发布流程（本单只提交迁移文件，不手动执行）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 采用 A（建表 + 修正主体），不废弃该特性 | **Owner 2026-11-26「确认会话可以建表」** |
| D2 | IAM 主体一律 `users.iam_sub`；本地 `users.id` 只用于本地查询 | Owner 2026-11-26「IAM 为事实来源」 |
| D3 | `iam_sub` 为空 → 确认失败且 0 次 IAM 请求（禁止把本地 id 发给 IAM） | 本计划锁定（红线：不静默） |
| D4 | `site_manager`/`site_staff` 仅写本地（现状），不新增 IAM 调用 | 本计划锁定 |
| D5 | 迁移只提交、不自动执行到生产；预生产随发布 bootstrap 执行 | 本计划锁定 |

### Out of Scope

- 为确认会话新增前端页面/邮件模板（沿用现有确认页）
- `merchant_admin` 之外的审批动作扩展
- 存量会话数据（表此前不存在，无存量）

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 空库执行 up 迁移 | `bash scripts/verify-2543.sh r1` | `docs/evidence/2543/R1-schema.txt` | `TABLE_EXISTS=1`、`COLUMN_COUNT>=17`、`TOKEN_UNIQUE=1`、`EXEMPT_REMOVED=1`；down 后 `TABLE_EXISTS=0` | 缺列或无法回滚 → FAIL |
| R2 | 会话（`merchant_admin`）+ 本地用户 `iam_sub=S1` | `POST /api/confirmation-sessions/:id/confirm` | `docs/evidence/2543/R2-iam-subject.txt` | `BIND_USER_ARG=S1`、`IAM_LOCAL_ID_SENT=0`、`ROLE=OWNER`、`PERM_USER_ARG=S1` | 请求路径含本地 id → FAIL |
| R3 | 同上但 `iam_sub=""` | 同上 | `docs/evidence/2543/R3-empty-sub-guard.txt` | `CONFIRM_STATUS=failed`、`IAM_CALLS=0`、`MESSAGE_HAS_ACTIVATE=1` | 仍发 IAM 请求 → FAIL |
| R4 | 三份文档 | `bash scripts/verify-2543.sh r4` | `docs/evidence/2543/R4-docs.txt` | `DOC_FACT_SOURCE=1`、`DOC_DELETE_SYSADMIN_ONLY=1`、`DOC_IAM_READY=1` | 缺项 → FAIL |
| R5 | 修后树 | `bash scripts/verify-2543.sh r5` | `docs/evidence/2543/R5-build.txt` | `GO_TEST_OK=1`、`BUILD_OK=1`、`MIGRATION_ROUNDTRIP=1` | 任一失败 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2543.sh r1` | `docs/evidence/2543/R1-schema.txt` | exit 0；含 `TABLE_EXISTS=1`、`EXEMPT_REMOVED=1` | PASS | 表缺失 → FAIL | dev |
| R2 | `bash scripts/verify-2543.sh r2` | `docs/evidence/2543/R2-iam-subject.txt` | exit 0；含 `IAM_LOCAL_ID_SENT=0`、`ROLE=OWNER` | PASS | 发本地 id → FAIL | dev |
| R3 | `bash scripts/verify-2543.sh r3` | `docs/evidence/2543/R3-empty-sub-guard.txt` | exit 0；含 `IAM_CALLS=0`、`MESSAGE_HAS_ACTIVATE=1` | PASS | 仍请求 IAM → FAIL | dev |
| R4 | `bash scripts/verify-2543.sh r4` | `docs/evidence/2543/R4-docs.txt` | exit 0；含 `DOC_FACT_SOURCE=1` | PASS | 缺文档 → FAIL | dev |
| R5 | `bash scripts/verify-2543.sh r5` | `docs/evidence/2543/R5-build.txt` | exit 0；含 `GO_TEST_OK=1`、`MIGRATION_ROUNDTRIP=1` | PASS | 任一失败 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2543.sh`（`r1`–`r5`），每个文本 artifact 首行为 `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`，另起一行 `GENERATED_AT_HEAD=<40 位 sha>`。
- 清单（全部位于 `docs/evidence/2543/`）：
  - `docs/evidence/2543/R1-schema.txt`
  - `docs/evidence/2543/R2-iam-subject.txt`
  - `docs/evidence/2543/R3-empty-sub-guard.txt`
  - `docs/evidence/2543/R4-docs.txt`
  - `docs/evidence/2543/R5-build.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2543 --summary-file … --changed-files … --evidence-file docs/evidence/2543/evidence.json --report docs/evidence/2543/pre-ready-2543.json`；`evidence.json` 的 build 字段须匹配 evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Dependencies

- 无

### Workload

- Size: M（1 个迁移 up/down + 1 处主体修正 + 1 个测试文件 + 3 份文档 + 5 条证据）
- Confidence: high

