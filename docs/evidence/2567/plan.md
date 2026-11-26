## Accepted Plan — #2567 扫码绑定=账户合并：C 废止 / A 继承组织身份 / 合并后设密码 / 写审计｜Rev.2

> 依据：Owner + Ken 2026-11-26 裁定（三条）。基线：`account_merge.go`（#2445）已实现合并与成员迁移，缺「缓存镜像 / 列表可见性 / 审计 / 确认页摘要 / 密码步骤时机」。

### 变更点

1. **统一缓存收口**（新增 `backend/handlers/user_cache.go`）
   - `reconcileUserCache(tx *gorm.DB, userID string) error`：按成员关系重算并写回 `users.role` / `tenant_id` / `org_id`：
     - 有 `merchant_members` active 行 → `tenant_id = 该行 tenant_id`、`org_id = 该行 tenant_id`（商户组织）、`role = 该行 role`
     - 否则有 `site_members` active 行 → `tenant_id = sites.tenant_id`、`org_id = sites.org_id`、`role = site_members.roles[0]`（空则 `site_member`）
     - 否则（纯顾客）→ `role = USER`、`tenant_id`/`org_id` 置 nil UUID
   - 合并、解绑、移除成员三条路径（#2568 落地）都调用同一函数；本单先接合并路径。
2. **合并流程补齐**（`backend/handlers/account_merge.go`）
   - 迁移成员/档案后在**同一事务**内调用 `reconcileUserCache(tx, toUser.ID)`，并对 fromUser 同样调用（→ 顾客态）；
   - 清理 C 的**本地**待办/邀请（Rev.2：IAM 的 `user_task_queues` 属运维侧，本仓红线禁止直连 IAM 库）：
     - `staff_invites`（`invitee_user_id = C` 且 `status='pending'`）→ `status='rejected'`、`reject_reason='account_merged'`（复用既有列）；
     - `confirmation_sessions`（`user_id = C` 且 `status='waiting'`）→ `status='cancelled'`（`status` 为 varchar(20)，无需迁移；表由 #2543 建立，缺失时本条跳过并记日志）；
     - 语义取「关闭」而非「转挂」：C 已废止，其待办不应以新身份继续流转。
   - 写审计 `audit_logs`：`action=ACCOUNT_MERGE`、`resource=user`、`details={from_local, from_iam_sub, to_local, to_iam_sub, org_id, source:"qr_bind"}`。
3. **成员列表可见性**（`backend/handlers/user_staff.go` 的 `ListStaff`、`personnel.go` 三视图）
   - 一律过滤 `users.status IN ('active')`（`obsolete`/`deleted`/`disabled` 不出现）；A 因 `tenant_id/org_id` 已镜像而自然可见。
4. **PC/小程序确认页**（`frontend-mobile/src/pages-weapp/bind/Bind.jsx` + PC 人员列表）
   - `needs_merge` 态展示摘要：「将绑定到 **{组织名}**（原员工：**{C 姓名}**），原员工账户将被**废止**」+ 二次确认按钮；
   - 合并成功后：若当前账号无密码 → 进入「设置登录密码 + 确认密码」步骤（复用 #2446 的表单与校验），有密码 → 直接成功页；
   - 文案固定「废止」，不使用「删除」（C 行仍保留可追溯）。
5. **文档**：`docs/cases/account-select.md`（合并语义与密码步骤）、`docs/spec/api/api.md`（`POST /wechat-bind/confirm` 的 merge 分支响应与审计）、`docs/topics/iam/iam-notes.md`（账户生命周期：绑定→废止→顾客回退）。

### Files

- `backend/handlers/user_cache.go`（新增）
- `backend/handlers/account_merge.go`
- `backend/handlers/user_staff.go`
- `backend/handlers/personnel.go`
- `backend/handlers/wechat_bind.go`
- `backend/handlers/account_merge_2567_test.go`（新增）
- `frontend-mobile/src/pages-weapp/bind/Bind.jsx`
- `docs/cases/account-select.md`
- `docs/spec/api/api.md`
- `docs/topics/iam/iam-notes.md`
- `scripts/verify-2567.sh`（新增）
- `docs/evidence/2567/plan.md`
- `docs/evidence/2567/evidence.json`
- `docs/evidence/2567/pre-ready-2567.json`
- `docs/evidence/2567/R1-merge-reconcile.txt`
- `docs/evidence/2567/R2-list-visibility.txt`
- `docs/evidence/2567/R3-audit.txt`
- `docs/evidence/2567/R4-password-step.txt`
- `docs/evidence/2567/R5-build.txt`
- `docs/evidence/2567/R6-local-cleanup.txt`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 合并后缓存镜像 | `account_merge.go` + `user_cache.go` | `POST /wechat-bind/confirm`（needs_merge 分支） | A 为纯顾客（USER/nil/nil）、C 为商户直属员工（merchant_members active） | backend（Postgres 测试库 + fake IAM） | `A_ROLE=merchant_staff`（或 C 的 role）、`A_TENANT=C 的 tenant_id`、`A_ORG=C 的商户 org`、`C_STATUS=obsolete`、`C_ROLE=USER` | R1 |
| 成员列表可见性 | `user_staff.go` / `personnel.go` | `GET /api/staff`、`GET /api/admin/personnel` | 合并后同 actor 查询 | backend | `A_IN_LIST=1`、`C_IN_LIST=0`、`OBSOLETE_ROWS=0` | R2 |
| 审计 | `account_merge.go` | 同上 | 合并成功 | backend | `AUDIT_ACTION=ACCOUNT_MERGE`、`AUDIT_HAS_FROM_TO=1`、`AUDIT_SOURCE=qr_bind` | R3 |
| 密码步骤 | `Bind.jsx` + 后端能力探测 | 合并成功页 | ①A 无密码 ②A 已有密码 | PC/H5 | ①出现「设置密码 + 确认密码」 ②直接成功；`PASSWORD_FIELD=1`、`SKIP_WHEN_EXISTS=1` | R4 |
| 构建/测试 | backend + frontend-mobile | 定向测试 + build + eslint | 修后树 | 双端 | 全绿 | R5 |
| C 的本地待办/邀请关闭（Rev.2） | `account_merge.go` | 合并事务 | C 有 1 条 pending 邀请 + 1 条 waiting 确认会话 | backend | `LOCAL_INVITES_CLOSED>=1`、`CONFIRM_SESSIONS_CLOSED>=1`、`IAM_DB_TOUCHED=0` | R6 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R4 | static | PC/H5 无组件测试运行器（Owner 豁免沿用 #2528/#2526）；以源码结构断言 + 后端能力字段为准，真机截图归 Ops |

### Input Truth & Tool Preflight

- 三宿字段语义（IAM 核查清单 E1，已确认）：`users.tenant_id`=商户 org、`users.org_id`=归属组织、`users.role`=域角色、顾客=nil/nil/USER。
- 合并现状：`account_merge.go:129-147`（成员迁移 + status）；**不写三宿字段**、**不写审计** = 本单修复目标。
- IAM 侧：合并只做关系迁移，不删除 IAM 用户（C 保留 `iam_sub`，仅本地 obsolete + 关系停用）——与 Owner 裁定 3 一致。
- 工具盲区：小程序真机走查不在本机能力内（Ops follow-up）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 合并后**不删除** C 的 IAM 用户，只停用组织关系 + 本地 `obsolete` | Owner 裁定 1 |
| D2 | 三宿字段由 `reconcileUserCache` 单一收口，禁止各处手写 | 本计划锁定（E6=B 方向的第一个落点） |
| D3 | 成员列表一律过滤非 active 状态 | 本计划锁定（杜绝「obsolete 仍显示禁用」） |
| D4 | 密码步骤仅在 A 无密码时出现，必须含「确认密码」 | Owner 裁定 2 |
| D5 | 审计必须记录 from/to `iam_sub` 与来源 `qr_bind` | 本计划锁定（清单 F1） |
| D6 | 文案统一「废止」，不出现「删除员工账号」 | Owner 裁定 1 措辞 |
| D7 | C 的待办/邀请**只做本地关闭**（`staff_invites`→rejected / `confirmation_sessions`→cancelled）；IAM `user_task_queues` 不在本单（红线：tuneloop 不直连 IAM 库） | 本计划锁定（Rev.2，采纳 Packet 选项 1） |

### Out of Scope

- 离职解绑与资产挂空（#2568）
- 微信解绑后自助清理（既有 #2016 S3）
- 多组织账号的身份选择页改造

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | A=纯顾客（USER/nil/nil）；C=商户直属（merchant_members active，role=merchant_staff）；IAM 两侧已有 sub | `POST /wechat-bind/confirm` | `docs/evidence/2567/R1-merge-reconcile.txt` | `A_ROLE=merchant_staff`、`A_TENANT_NON_NIL=1`、`A_ORG=merchant_org`、`C_STATUS=obsolete`、`C_ROLE=USER`、`MEMBERSHIP_ROWS=1` | A 仍 `USER/nil/nil` 或成员行重复 → FAIL |
| R2 | 同上 | `GET /api/staff` + `GET /api/admin/personnel` | `docs/evidence/2567/R2-list-visibility.txt` | `A_IN_LIST=1`、`C_IN_LIST=0`、`OBSOLETE_ROWS=0` | C 仍显示 / A 不可见 → FAIL |
| R3 | 同上 | 合并成功 | `docs/evidence/2567/R3-audit.txt` | `AUDIT_ACTION=ACCOUNT_MERGE`、`AUDIT_HAS_FROM_TO=1`、`AUDIT_SOURCE=qr_bind` | 无审计行 → FAIL |
| R4 | ①无密码 ②有密码 | `Bind.jsx` 静态接线 | `docs/evidence/2567/R4-password-step.txt` | `PASSWORD_FIELD=1`、`CONFIRM_FIELD=1`、`SKIP_WHEN_EXISTS=1`（`evidence_type: static`） | 有密码仍弹 / 无确认字段 → FAIL |
| R5 | 修后树 | `bash scripts/verify-2567.sh r5` | `docs/evidence/2567/R5-build.txt` | `GO_TEST_OK=1`、`GO_BUILD_OK=1`、`MOBILE_BUILD_OK=1`、`ESLINT_OK=1` | 任一失败 → FAIL |
| R6 | C 有 1 条 `staff_invites`(pending) + 1 条 `confirmation_sessions`(waiting) | `bash scripts/verify-2567.sh r6` | `docs/evidence/2567/R6-local-cleanup.txt` | `LOCAL_INVITES_CLOSED=1`、`CONFIRM_SESSIONS_CLOSED=1`、`IAM_DB_TOUCHED=0` | 仍为 pending/waiting，或出现直连 IAM 库的 SQL → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2567.sh r1` | `docs/evidence/2567/R1-merge-reconcile.txt` | exit 0；含 `C_STATUS=obsolete`、`MEMBERSHIP_ROWS=1` | PASS | 缓存未镜像 → FAIL | dev |
| R2 | `bash scripts/verify-2567.sh r2` | `docs/evidence/2567/R2-list-visibility.txt` | exit 0；含 `A_IN_LIST=1`、`OBSOLETE_ROWS=0` | PASS | obsolete 可见 → FAIL | dev |
| R3 | `bash scripts/verify-2567.sh r3` | `docs/evidence/2567/R3-audit.txt` | exit 0；含 `AUDIT_ACTION=ACCOUNT_MERGE` | PASS | 无审计 → FAIL | dev |
| R4 | `bash scripts/verify-2567.sh r4` | `docs/evidence/2567/R4-password-step.txt` | exit 0；含 `PASSWORD_FIELD=1`、`SKIP_WHEN_EXISTS=1` | PASS | 无条件弹窗 → FAIL | dev |
| R5 | `bash scripts/verify-2567.sh r5` | `docs/evidence/2567/R5-build.txt` | exit 0；含 `GO_TEST_OK=1`、`MOBILE_BUILD_OK=1` | PASS | 任一失败 → FAIL | dev |
| R6 | `bash scripts/verify-2567.sh r6` | `docs/evidence/2567/R6-local-cleanup.txt` | exit 0；含 `LOCAL_INVITES_CLOSED=1`、`IAM_DB_TOUCHED=0` | PASS | 待办未关闭 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2567.sh`（`r1`–`r5`），每个文本 artifact 首行为 `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`，另起一行 `GENERATED_AT_HEAD=<40 位 sha>`。
- 清单（全部位于 `docs/evidence/2567/`）：
  - `docs/evidence/2567/R1-merge-reconcile.txt`
  - `docs/evidence/2567/R2-list-visibility.txt`
  - `docs/evidence/2567/R3-audit.txt`
  - `docs/evidence/2567/R4-password-step.txt`
  - `docs/evidence/2567/R5-build.txt`
  - `docs/evidence/2567/R6-local-cleanup.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2567 --summary-file … --changed-files … --evidence-file docs/evidence/2567/evidence.json --report docs/evidence/2567/pre-ready-2567.json`；`evidence.json` 的 build 字段须匹配 evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Dependencies

- #2543

### Workload

- Size: M（2 处后端流程 + 1 个新收口函数 + 1 个客户端页面 + 3 份文档 + 5 条证据）
- Confidence: high

