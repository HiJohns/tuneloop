## Accepted Plan — #2552 中转网点成员把分支组织 ID 当租户写库（D2 违反）+ #2547 计划项未落地

> Tags: bug, code:go, api-contract, data-replay
> 来源：#2547（`status:done`，Accepted Plan + Work Summary + Round 1 审计）+ PR #2549（merged `e83ee8bd7`，随 `c37655eb0` 上预生产 1.0.100）
> 本单是 #2547 计划在集成分支上的**差异收敛**：D2 反例（critical）+ 变更点 1/4/5/6 未落地

### 现状核对（develop @ `c37655eb0`，代码为据）

| # | 事实 | 位置 |
|---|---|---|
| 1 | `AddTransitSiteMember` 把 `site.OrgID`（中转子组织）当 `addMembersCore` 的第 4 参数（**本地租户作用域**）传入，并带 legacy 回退 `site.TenantID` | `backend/handlers/transit_site_admin.go:290-294` |
| 2 | 该参数三处写/判定：新建用户 `TenantID`、幂等判定 `WHERE tenant_id = ? AND site_id = ? AND user_id = ?`、`models.SiteMember{TenantID:…}` | `backend/handlers/site_member_common.go:146`（签名）、`:95`、`:157`、`:217` |
| 3 | IAM 绑定目标与第 4 参数无关：`addMembersCore` 固定用 `site.OrgID`；`Update`/`Remove` 也已用 `site.OrgID` | `backend/handlers/site_member_common.go:330`；`transit_site_admin.go:343-350`、`:385-390` |
| 4 | 建站：`parentOrgID := tenantID`（未解析命名空间主组织）；请求体无 `type`；`tenantID` 空无前置拒绝；IAM 失败一律 `502/50200`；本地落库失败无 `DeleteOrganization` 补偿 | `backend/handlers/transit_site_admin.go:88-118` |
| 5 | `CreateOrganizationRequest` 无 `Type` 字段；`Organization` 无 `Type` 字段（响应侧无法断言 `kind=branch`） | `backend/services/iam_client.go:276-287`、`:397-407` |
| 6 | 中转成员识别仍按 `(s.tenant_id = ? OR s.org_id = ?)` 且参数是 `rootOrgID`（列表视图与 by-id 两处） | `backend/handlers/personnel.go:312`、`:638-639`；调用点 `:99`、`:559` |
| 7 | 回填服务与 CLI 在 develop **不存在**（`backend/services/transit_site_org_backfill.go`、`backend/tools/backfill_transit_site_orgs/`） | `ls` 实测 |
| 8 | 文档仍半旧：`docs/cases/transit.md:61`「平台顶层组织（租户=平台根）」；`docs/ops/tenant-id-audit.md:433`、`:462`「tenant=org=平台租户」；`docs/spec/api/api.md:6503` 中转判据仍 `sites.tenant_id/org_id = oid` | 见左列 |

**根因（两条，互相独立）**

1. **D2 违反**：把「IAM 绑定目标组织（`site.OrgID`）」误当作「本地租户作用域」，导致 `site_members.tenant_id` / `users.tenant_id` 写成分支组织 ID；唯一键含 `tenant_id`，幂等判定同源 → **重复成员行**（同一用户在同一站点两条记录：列表重复、移除只删一条、角色更新歧义）。
2. **计划项漏做**：建站的组织类型/父组织/错误映射/补偿、人员判据站点维度、回填工具、四份文档均未随集成落地。

### 变更点（分阶段 gate：P1 必须全绿后才做 P2/P3/P4）

**P1 — 正确性（R1/R2/R3）**

1. **D2 修复（成员租户作用域）**：`backend/handlers/transit_site_admin.go`
   - `AddTransitSiteMember` 恢复 `addMembersCore(c, db, site, site.TenantID, siteID, input)`；**删除** `bindOrgID` 及其 legacy 回退（回退本身也是把 org 当 tenant）。
   - 注释锁定语义：「第 4 参数只写本地租户列与幂等判定；IAM 绑定目标由 core 固定取 `site.OrgID`（#2547 D2）」。
   - `UpdateTransitSiteMemberRole` / `RemoveTransitSiteMember` 不动实现（已用 `site.OrgID`），补断言锁定 bind/role/unbind 三路径 org 段 = `sites.org_id`。
   - 负例来源（#2547 审计 R2b）：站点已有 `site_members` 行（`tenant_id = 平台租户`）时，重复添加同一用户 → `site_members` 行数保持 1。
2. **中转成员识别切站点维度**：`backend/handlers/personnel.go`
   - 新增 `platformTenantIDFor(ctx) string`：`middleware.GetTenantID`，空时回落 `middleware.GetOrgID`（与 `sites.tenant_id` 建站来源同源）。
   - `personnelSystemView(db, rootOrgID, platformTenantID, keyword)`（新增形参）：中转段判据改为 `s.type = 'transit' AND s.tenant_id = ? AND sm.status = 'active'`（成员仍经 `JOIN sites s ON s.id = sm.site_id`）。
   - `hasTransitSiteMembership(db, ctx, userID, platformTenantID)`（参数由 org 改为平台租户）：同判据；两处调用点（`:99`、`:559`）改传 `platformTenantIDFor(ctx)`。
   - 负例：同租户 `type='store'` 站点成员不得进系统视图；他租户 `type='transit'` 站点成员不得进系统视图；by-id 对中转成员返回 200、对上述两类负例返回 404。
3. **建站契约（#2547 变更点 1/2 未落地部分）**：`backend/services/iam_client.go` + `backend/handlers/transit_site_admin.go`
   - `CreateOrganizationRequest` 增 `Type string \`json:"type,omitempty"\``；`Organization` 增 `Type string \`json:"type"\``（既有调用方不传 → 请求体不变）。
   - `CreateOrganization` / `CreateOrganizationWithToken` 的 409 改为**哨兵错误**：`services.ErrOrganizationNameConflict`（`fmt.Errorf("%w: %s", ErrOrganizationNameConflict, body)`），handler 用 `errors.Is` 映射，禁止字符串匹配。
   - 新增 `services.ResolveNamespacePrimaryOrg(client) (string, error)`：`ListOrganizations()` 中 `IsPrimary=true` **恰好 1 个**；0 个或多个 → 显式错误（不猜、不回落租户）。
   - `CreateAdminTransitSite` 顺序改为：`tenantID` 空 → `400/40002`（**不调 IAM**）→ 解析主组织（失败 → `500/50000` 显式）→ `CreateOrganization{Name, ParentID: primaryOrg, Type: "branch", Address, ContactPhone: req.Phone, NotificationLang: middleware.GetCulture(c), OperatorID: middleware.GetUserID(ctx), AdminInfo: nil}`（保持 service 凭据，见 D3）→ 失败：`ErrOrganizationNameConflict` → `409/40900`「同名组织已存在」，其余 → `502/50200` → 写 `sites` 行（`OrgID=createdOrg.OrgID`，`TenantID=tenantID`）→ 本地失败 → `DeleteOrganization(createdOrg.OrgID)` best-effort + `log`，返回 `500/50000`（不留孤儿组织）。
   - 断言：IAM 请求体 `parent_id` = 主组织、`type=branch`、无 `admin_info`；IAM 失败 / 无唯一主组织 / `tenant_id` 空 → **无 `sites` 行**。

**P2 — 存量工具（R4）**

4. **回填服务 + CLI（默认只读，不挂启动路径）**
   - `backend/services/transit_site_org_backfill.go`：
     - `PlanTransitSiteOrgBackfill(db) ([]TransitSiteBackfillRow, error)`：列出 `type='transit'` 站点，`NeedsBackfill = org_id 为空 OR org_id = tenant_id`；行含 `site_id/name/tenant_id/org_id/status/member_count`。
     - `ApplyTransitSiteOrgBackfill(db, iam, opts) (TransitBackfillResult, error)`：逐站解析主组织 → 建 branch 子组织（`Name=site.Name`，无 owner）→ **条件更新** `UPDATE sites SET org_id=? WHERE id=? AND (org_id IS NULL OR org_id = tenant_id)`（幂等，已回填跳过）；返回 `created/updated/skipped`。
     - `RebindTransitSiteMembers(db, iam, site, opts)`：仅 `--phase=members` 时调用；对每个 active 成员 `BindUserToOrganization(iam_sub, site.OrgID, toIAMRole(role), "backfill")` → 成功后 best-effort `UnbindUserFromOrganization(iam_sub, oldOrgID, "backfill")`；失败显式返回（不静默）。
   - `backend/tools/backfill_transit_site_orgs/main.go` 薄 CLI：默认 `--dry-run`（打印清单与 `writes=0 iam_creates=0`）；`--apply` 才写；`--phase=orgs|members` 默认 `orgs`；输出机器可读计数（`PLAN_ROWS=`, `APPLY_CREATED=`, `APPLY_UPDATED=`, `APPLY_SKIPPED=`, `MEMBERS_REBIND=`）。
   - **不挂任何启动路径**（`iam_bootstrap` / migration / main 均不调用）。

**P3 — 文档（R5）**

5. 四份文档同步（见 `## Core Update`）。

**P4 — 验证（R6/R7）**

6. `scripts/verify-2552.sh`（r1–r7）+ 证据产物；`go build`、`gofmt`、定向回归。

### Bug Reflection Matrix

| # | 缺陷面 symptom | 触发条件 trigger | 根因 root cause | 测试盲区 test blind spot | 同类路径 same-class | 修复层 fix layer | 红绿 red-green | 预防 prevention |
|---|---|---|---|---|---|---|---|---|
| B1 | 中转成员 `site_members.tenant_id` 被写成分支组织 ID | 站点已有 `org_id`（回填/新建）且新增成员 | 把「IAM 目标组织」当「本地租户作用域」传参 | 无「tenant 列值」断言，只断言 IAM org | 任何复用 `addMembersCore` 的调用点 | handler 传参 + 注释锁定 | R1 `MEMBER_TENANT_IS_LOCAL_TENANT=1`（修前 0） | 合同行断言 tenant 列值，不只断言 IAM 请求 |
| B2 | 重复添加同一用户产生第二条成员行 | 成员行仍是旧 tenant（平台租户）、站点已回填 org | 幂等判定与唯一键都用含 tenant 的列 | 无重复添加幂等用例 | 所有 `(tenant_id, site_id, user_id)` 唯一键表 | 恢复本地租户口径（判定与写入同源） | R1 `MEMBER_ROWS=1`（修前 2） | 幂等负例固定进合同 |
| B3 | 建站未发 `type=branch`，父组织 = 平台租户而非命名空间主组织 | 平台管理员创建中转网点 | IAM 客户端缺 `Type` 字段 + 未解析 `is_primary` | mock 只校验返回 org_id | `merchant.go` / `site.go` 建组织调用 | service/handler 契约补齐 | R3 `CREATE_TYPE_BRANCH=1`、`CREATE_PARENT_IS_NAMESPACE=1` | 请求体字段逐项断言 |
| B4 | 同名组织返回 502 而非 409 | IAM 返回 409 | handler 未识别冲突错误 | 无名冲突用例 | 所有 `CreateOrganization` 调用方 | 哨兵错误 + `errors.Is` 映射 | R3 `DUPLICATE_NAME_409=1` | 错误映射表进合同 |
| B5 | `tenant_id` 空仍建 IAM 组织，本地落库失败留孤儿组织 | 平台上下文缺 `tid` | 无前置校验 + 无补偿 | 无「空租户」用例 | 其他「先外部后本地」写入 | 前置 400 + 失败补偿 | R3 `NO_SITE_ON_EMPTY_TENANT=1`、`ORPHAN_ORG_COMPENSATED=1` | 前置校验 + 补偿断言 |
| B6 | 建子组织后既有中转成员从系统视图消失 | 判据用 `sites.org_id = oid` | 判据维度与建站写入维度不一致（org vs tenant） | 无「org ≠ tenant」夹具 | 人员三视图 / by-id 可见性 | 判据改站点+平台租户维度 | R2 `SYSTEM_VIEW_TRANSIT=1` | 夹具固定 `org_id ≠ tenant_id` |
| B7 | 判据放宽后同租户普通网点成员/他租户中转成员越权可见 | 判据只留 `type='transit'` 或漏租户条件 | 只改一半口径 | 无负例 | `personnelUserVisible` 其他分支 | 双负例固定进合同 | R2 `NEG_STORE_SAME_TENANT=0`、`NEG_OTHER_TENANT_TRANSIT=0` | 每个放宽都要配负例 |
| B8 | 存量站无可用回填路径（只能手写 UPDATE） | 其他环境有历史中转站 | 计划项 5 未落地 | 无 dry-run/幂等证据 | 任何一次性数据修复 | 服务 + CLI（默认 dry-run） | R4 `DRYRUN_WRITES=0`、`SECOND_APPLY_UPDATED=0` | 数据工具必须先证明 0 写入 |
| B9 | 文档仍写旧归属/旧判据 → 下一轮实现再分裂 | 文档未随代码落字 | 验收只覆盖代码 | 四面文档无断言 | 其他口径类改动 | 文档结构断言 | R5 四项 `=1` | 文档 grep 进合同 |
| B10 | 修复只在 issue 分支，集成分支无证据可读 | 直接按分支交付、未合并 | 验收未覆盖集成可读性 | 无四源/跟踪断言 | 所有长链条修复 | R7 + #2554 | R7 `PLAN_SOURCE_FRESH=PASS`、`PLAN_MD_TRACKED=1` | 合并后 Ops 回评（#2554） |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 D2：成员 tenant 列 = 本地租户；IAM org = 站点 org；重复添加不产生第二行 | `backend/handlers/transit_site_admin.go` | `POST /api/admin/transit-sites/:id/members` | 站点 `tenant_id=平台租户`、`org_id=分支组织`（两者不等）；先添加一次，再重复添加同一用户 | backend（Postgres 测试库 + fake IAM） | `MEMBER_TENANT_IS_LOCAL_TENANT=1`、`MEMBER_IAM_ORG_IS_SITE_ORG=1`、`NO_DUPLICATE_MEMBER_ROW=1`、`MEMBER_ROWS=1`；`users.tenant_id = 平台租户` | R1 |
| R2 中转成员识别改为站点维度（列表 + by-id） | `backend/handlers/personnel.go` | `GET /api/admin/personnel`、`GET /api/admin/staff/:id` | 中转站 `org_id=分支组织`、`tenant_id=平台租户`；另有同租户 `type=store` 站、他租户 `type=transit` 站 | backend | `SYSTEM_VIEW_TRANSIT=1`、`BYID_TRANSIT=200`、`NEG_STORE_SAME_TENANT=0`、`NEG_OTHER_TENANT_TRANSIT=0`、`OLD_ORG_EQUALS_TENANT_RULE=0` | R2 |
| R3 建站：`type=branch` + 父组织 = 命名空间主组织 + 错误映射 + 补偿 | `backend/handlers/transit_site_admin.go`、`backend/services/iam_client.go` | `POST /api/admin/transit-sites` | fake IAM：`is_primary` 恰 1；另测名冲突 409、`tenant_id` 空、无/多主组织、本地落库失败 | backend | `CREATE_TYPE_BRANCH=1`、`CREATE_PARENT_IS_NAMESPACE=1`、`DUPLICATE_NAME_409=1`、`NO_SITE_ON_EMPTY_TENANT=1`、`PRIMARY_ORG_UNRESOLVED=50000`、`ORPHAN_ORG_COMPENSATED=1` | R3 |
| R4 回填工具：dry-run 0 写入 + apply 幂等（含 members 阶段） | `backend/services/transit_site_org_backfill.go`、`backend/tools/backfill_transit_site_orgs/main.go` | 服务函数 + CLI | 2 个存量站（`org_id=tenant_id`）+ 1 个已回填站 + 1 个有成员的站 | backend | `DRYRUN_WRITES=0`、`DRYRUN_IAM_CREATES=0`、`APPLY_UPDATED=2`、`APPLY_CREATED=2`、`SECOND_APPLY_UPDATED=0`、`MEMBERS_REBIND=1` | R4 |
| R5 四份文档口径同步 | `docs/spec/api/api.md`、`docs/cases/transit.md`、`docs/ops/tenant-id-audit.md`、`docs/spec/database/database.md` | grep 结构断言 | 修后文档 | 文档 | `DOC_API_TRANSIT_TENANT=1`、`DOC_CASE_TRANSIT_ORG=1`、`DOC_OPS_AUDIT_ROW=1`、`DOC_DB_SITES_ORG=1` | R5 |
| R6 构建 + 定向回归 | `backend/handlers/transit_site_admin.go`、`backend/handlers/personnel.go`、`backend/services/transit_site_org_backfill.go` | `go build` + `gofmt -l` + 定向套件 | 修后工作树 | backend | `BUILD_RC=0`、`GOFMT_DIFF=0`、`REGRESSION_RC=0`、`REGRESSION_PASS=1` | R6 |
| R7 计划/证据四源与分支可读 | `docs/evidence/2552/plan.md`、`scripts/verify-2552.sh` | `plan-source.sh --issue 2552 --fresh` + `git ls-files` | 推送后的 issue 分支 | backend/证据 | `PLAN_SOURCE_FRESH=PASS`、`PLAN_MD_TRACKED=1`（`docs/evidence/2552/plan.md` 被 git 跟踪） | R7 |

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 站点 T（`type=transit`，`tenant_id=tid`，`org_id=branchOrg`）+ 用户 U（无成员行） | `bash scripts/verify-2552.sh r1` | `docs/evidence/2552/R1-member-tenant.txt` | `MEMBER_TENANT_IS_LOCAL_TENANT=1`、`MEMBER_IAM_ORG_IS_SITE_ORG=1`、`NO_DUPLICATE_MEMBER_ROW=1`、`MEMBER_ROWS=1` | `MEMBER_TENANT_IS_LOCAL_TENANT=0`（tenant 列被写成 org）或 `MEMBER_ROWS=2`（重复行）→ FAIL |
| R2 | 中转成员 M（T + branchOrg）；负例：同租户 `type=store` 成员 S、他租户 transit 成员 X | `bash scripts/verify-2552.sh r2` | `docs/evidence/2552/R2-personnel-predicate.txt` | `SYSTEM_VIEW_TRANSIT=1`、`BYID_TRANSIT=200`、`NEG_STORE_SAME_TENANT=0`、`NEG_OTHER_TENANT_TRANSIT=0`、`OLD_ORG_EQUALS_TENANT_RULE=0` | `SYSTEM_VIEW_TRANSIT=0`（改口径后既有成员消失）或任一负例 =1（越权可见）→ FAIL |
| R3 | fake IAM：`organizations` 列表含 1 个 `is_primary`；create 返回 `newOrg`；另测 409/空 tenant/0 或 2 主组织/本地落库失败 | `bash scripts/verify-2552.sh r3` | `docs/evidence/2552/R3-create-branch.txt` | `CREATE_TYPE_BRANCH=1`、`CREATE_PARENT_IS_NAMESPACE=1`、`DUPLICATE_NAME_409=1`、`NO_SITE_ON_EMPTY_TENANT=1`、`PRIMARY_ORG_UNRESOLVED=50000`、`ORPHAN_ORG_COMPENSATED=1` | `CREATE_TYPE_BRANCH=0` 或 `NO_SITE_ON_EMPTY_TENANT=0`（空 tenant 仍建站/建组织）或 `ORPHAN_ORG_COMPENSATED=0` → FAIL |
| R4 | 站 A/B（`org_id=tenant_id`）、站 C（已回填）、站 D（1 成员，测 members 阶段） | `bash scripts/verify-2552.sh r4` | `docs/evidence/2552/R4-backfill.txt` | `DRYRUN_WRITES=0`、`DRYRUN_IAM_CREATES=0`、`APPLY_UPDATED=2`、`APPLY_CREATED=2`、`SECOND_APPLY_UPDATED=0`、`MEMBERS_REBIND=1` | `DRYRUN_WRITES>0`（dry-run 写了）或 `SECOND_APPLY_UPDATED>0`（不幂等）→ FAIL |
| R5 | 四份文档修后内容 | `bash scripts/verify-2552.sh r5` | `docs/evidence/2552/R5-docs.txt` | `DOC_API_TRANSIT_TENANT=1`、`DOC_CASE_TRANSIT_ORG=1`、`DOC_OPS_AUDIT_ROW=1`、`DOC_DB_SITES_ORG=1` | 任一 `=0` → FAIL |
| R6 | 修后工作树 | `bash scripts/verify-2552.sh r6` | `docs/evidence/2552/R6-build-regression.txt` | `BUILD_RC=0`、`GOFMT_DIFF=0`、`REGRESSION_RC=0`、`REGRESSION_PASS=1` | 任一 `≠0` → FAIL |
| R7 | 推送后的 issue 分支 + Accepted Plan 评论 | `bash scripts/verify-2552.sh r7` | `docs/evidence/2552/R7-plan-source.txt` | `PLAN_SOURCE_FRESH=PASS`、`PLAN_MD_TRACKED=1` | `PLAN_SOURCE_FRESH=E_PLAN_STALE`（快照与评论不一致）或 `PLAN_MD_TRACKED=0` → FAIL |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | runtime | 本地 DB 读回 + fake IAM 录音 |
| R2 | runtime | 三视图/by-id HTTP 断言（同 actor） |
| R3 | runtime | fake IAM 录音 + `sites` 行数 |
| R4 | runtime | 服务函数 + CLI 计数（dry-run/apply/二次 apply） |
| R5 | static | 文档 grep 结构断言 |
| R6 | runtime | `go build` + `gofmt -l` + 定向套件 |
| R7 | runtime | `plan-source.sh` + `git ls-files` |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2552.sh r1` | `docs/evidence/2552/R1-member-tenant.txt` | exit 0；grep `MEMBER_TENANT_IS_LOCAL_TENANT=1`、`MEMBER_IAM_ORG_IS_SITE_ORG=1`、`NO_DUPLICATE_MEMBER_ROW=1`、`MEMBER_ROWS=1` | PASS | `MEMBER_TENANT_IS_LOCAL_TENANT=0` 或 `MEMBER_ROWS=2` → FAIL | dev |
| R2 | `bash scripts/verify-2552.sh r2` | `docs/evidence/2552/R2-personnel-predicate.txt` | exit 0；grep `SYSTEM_VIEW_TRANSIT=1`、`BYID_TRANSIT=200`、`NEG_STORE_SAME_TENANT=0`、`NEG_OTHER_TENANT_TRANSIT=0`、`OLD_ORG_EQUALS_TENANT_RULE=0` | PASS | 任一负例 `=1` 或 `SYSTEM_VIEW_TRANSIT=0` → FAIL | dev |
| R3 | `bash scripts/verify-2552.sh r3` | `docs/evidence/2552/R3-create-branch.txt` | exit 0；grep `CREATE_TYPE_BRANCH=1`、`CREATE_PARENT_IS_NAMESPACE=1`、`DUPLICATE_NAME_409=1`、`NO_SITE_ON_EMPTY_TENANT=1`、`PRIMARY_ORG_UNRESOLVED=50000`、`ORPHAN_ORG_COMPENSATED=1` | PASS | `NO_SITE_ON_EMPTY_TENANT=0` 或 `ORPHAN_ORG_COMPENSATED=0` → FAIL | dev |
| R4 | `bash scripts/verify-2552.sh r4` | `docs/evidence/2552/R4-backfill.txt` | exit 0；grep `DRYRUN_WRITES=0`、`DRYRUN_IAM_CREATES=0`、`APPLY_UPDATED=2`、`APPLY_CREATED=2`、`SECOND_APPLY_UPDATED=0`、`MEMBERS_REBIND=1` | PASS | `DRYRUN_WRITES>0` 或 `SECOND_APPLY_UPDATED>0` → FAIL | dev |
| R5 | `bash scripts/verify-2552.sh r5` | `docs/evidence/2552/R5-docs.txt` | exit 0；grep `DOC_API_TRANSIT_TENANT=1`、`DOC_CASE_TRANSIT_ORG=1`、`DOC_OPS_AUDIT_ROW=1`、`DOC_DB_SITES_ORG=1` | PASS | 任一 `=0` → FAIL | dev |
| R6 | `bash scripts/verify-2552.sh r6` | `docs/evidence/2552/R6-build-regression.txt` | exit 0；grep `BUILD_RC=0`、`GOFMT_DIFF=0`、`REGRESSION_RC=0`、`REGRESSION_PASS=1` | PASS | 任一 `≠0` → FAIL | dev |
| R7 | `bash scripts/verify-2552.sh r7` | `docs/evidence/2552/R7-plan-source.txt` | exit 0；grep `PLAN_SOURCE_FRESH=PASS`、`PLAN_MD_TRACKED=1` | PASS | `PLAN_SOURCE_FRESH=E_PLAN_STALE` 或 `PLAN_MD_TRACKED=0` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2552/R1-member-tenant.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2552/R2-personnel-predicate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2552/R3-create-branch.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2552/R4-backfill.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2552/R5-docs.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2552/R6-build-regression.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2552/R7-plan-source.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Workload

- Size: L
- Basis: 3 个后端实现文件（`transit_site_admin.go`、`personnel.go`、`iam_client.go`）+ 2 个新增（回填服务、CLI）+ 4 个测试文件（新增 3、既有 mock 扩展 1）+ 1 个验证脚本 + 4 份文档 + 7 条证据行；含跨层（handler↔service↔CLI）与数据只读工具；无 migration、无 DB 结构变更
- Confidence: medium（回填工具为新增代码，CLI 行为需 dry-run 实证；members 阶段改写 IAM 关系属高风险操作，默认不执行）

### Files（变更文件清单）

- `backend/handlers/transit_site_admin.go`
- `backend/handlers/personnel.go`
- `backend/services/iam_client.go`
- `backend/services/transit_site_org_backfill.go`
- `backend/tools/backfill_transit_site_orgs/main.go`
- `backend/handlers/transit_site_admin_test.go`
- `backend/handlers/transit_site_admin_2552_test.go`
- `backend/handlers/personnel_transit_2552_test.go`
- `backend/services/transit_site_org_backfill_2552_test.go`
- `scripts/verify-2552.sh`
- `docs/spec/api/api.md`
- `docs/cases/transit.md`
- `docs/ops/tenant-id-audit.md`
- `docs/spec/database/database.md`
- `docs/evidence/2552/plan.md`
- `docs/evidence/2552/evidence.json`
- `docs/evidence/2552/pre-ready-2552.json`
- `docs/evidence/2552/R1-member-tenant.txt`
- `docs/evidence/2552/R2-personnel-predicate.txt`
- `docs/evidence/2552/R3-create-branch.txt`
- `docs/evidence/2552/R4-backfill.txt`
- `docs/evidence/2552/R5-docs.txt`
- `docs/evidence/2552/R6-build-regression.txt`
- `docs/evidence/2552/R7-plan-source.txt`

## Input Truth & Tool Preflight

- **Source of truth（三份，按优先级）**：
  1. `#2547` Accepted Plan（Decision Lock D2/D3 + 变更点 1/4/5/6）——本单的验收基准；Round 1 审计（08:20Z）给出集成分支实测差异。
  2. IAM 组织模型：beaconiam#509（`kind=branch`、`parent_id = 命名空间组织`、无 owner）；`ListOrganizations` 返回 `is_primary`（`backend/services/iam_client.go:461-490`）。
  3. 本地归属：`sites.tenant_id` = 创建者 `tid`（平台租户）；`sites.org_id` = 该站组织。二者**不同域**（#2508/#2274），禁止互相替代。
- **差异核对（实施前先跑，结果入 R1/R2 artifact）**：
  - `grep -n "addMembersCore(c, db, site," backend/handlers/transit_site_admin.go` → 修后第 4 参数为 `site.TenantID`（无 `bindOrgID`）。
  - `grep -rn "s.org_id = ?" backend/handlers/personnel.go` → 修后中转判据 0 处（`OLD_ORG_EQUALS_TENANT_RULE=0`）。
  - `grep -n "Type " backend/services/iam_client.go` → `CreateOrganizationRequest.Type` 与 `Organization.Type` 各 1 处（`omitempty`）。
  - `ls backend/services/transit_site_org_backfill.go backend/tools/backfill_transit_site_orgs/main.go` → 两文件存在；`grep -rn "ApplyTransitSiteOrgBackfill" backend --include=*.go | grep -v _test` → **仅** CLI 调用（不挂启动路径）。
- **损坏输入拒绝**：`tenant_id` 空 → `400/40002` 且 `IAM_CREATES=0`；主组织 0 个或多个 → `500/50000` 且 `SITES_INSERTED=0`；IAM 名冲突 → `409/40900`；`site_members` 重复添加 → 幂等命中（行数不增）；`--phase=members` 未显式传 → 默认不重绑成员。
- **数据安全**：回填工具默认 dry-run；`--apply` 才写；`--phase=members` 才动 IAM 关系；**不挂任何启动路径**；存量数据修复（重复行清理、已污染 tenant 回写）不在本单 → #2554（需 Owner 授权）。
- **工具盲区**：fake IAM 只证明请求体/状态码映射，不证明 beaconiam 侧 `kind=branch` 落库；生产/预生产关系以 beaconiam#509 + #2550/#2554 的人工核对为准。`--dry-run` 默认语义必须由 CLI 自身输出证明（`writes=0`），不能靠代码阅读。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | `addMembersCore` 第 4 参数 = **本地租户作用域**（`site.TenantID`）；IAM 绑定/角色/解绑目标 = `site.OrgID`；删除 legacy `bindOrgID` 回退 | Architect | 本计划锁定（#2547 D2） | 若允许「org 作 tenant」→ 直接回退 #2547 语义，须回 Owner |
| D2 | 中转成员判据 = `sites.type='transit' AND sites.tenant_id = platformTenant(tid 空回落 oid)`，成员经 `site_members.site_id → sites`；**不使用** `sites.org_id` | Architect | 本计划锁定（#2547 变更点 4） | 若改用 `org_id` → 建子组织后既有成员会从系统视图消失（#2547 审计语义挑战 2） |
| D3 | 建站建组织**沿用 service 凭据**（`CreateOrganization`，现状不改 `WithToken`），新增 `Type="branch"` / `ParentID=主组织` / 补偿与错误映射 | Architect | 本计划锁定（最小风险；预生产已验证 service 凭据可建组织） | 若要求用户令牌 → 需 403 降级/测试矩阵扩展，回 Architect |
| D4 | 回填工具：默认 dry-run；`--apply` 才写；`--phase=members` 才改 IAM 关系；**不挂启动路径**；条件更新保证幂等 | Architect / ops | 本计划锁定（#2547 变更点 5） | 若要求自动执行 → 需 Owner 授权 + 预生产演练，另立 |
| D5 | 错误码沿用既有约定：名冲突 `409/40900`、参数 `400/40002`、IAM 其他失败 `502/50200`、主组织不可解析 `500/50000` | Architect | 本计划锁定（#2547 变更点 1） | 若统一 500 → 与 #2457/#2540 口径冲突 |
| D6 | “集成分支可读性”拆两段：R7 断言**推送分支**四源 FRESH + plan.md 被跟踪；`origin/develop` 可读性 + 合并后四源 PASS 归 #2554（合并后 Ops 回评） | Architect / ops | 本计划锁定（合并前不可能满足） | 若审计要求合同行覆盖 develop → 会永久 FAIL，须由 Owner 改验收口径 |
| D7 | 分阶段 gate（P1 正确性 → P2 工具 → P3 文档 → P4 验证），**不拆子任务** | Architect | 本计划锁定（同一验收链；拆分会造成“判据已换、工具未上”的中间态） | 若 Owner 要求拆分 → 按 P1/P2 切两单，P3 随 P2 |
| D8 | 存量重复成员行/已污染 tenant 的回写**不在本单**（数据修复需授权）→ #2554 | Owner / ops | #2554 已建（`waitForConfirm`） | 未授权 → 只修「不再产生新重复」，存量行由 #2554 清单决定 |

## Out of Scope

- 存量数据修复：重复 `site_members` 行清理、已写入分支组织 ID 的 `site_members.tenant_id` / `users.tenant_id` 回写 → #2554（需 Owner 授权）。
- `organizations.type` 存量回填（`kind=branch` 标注）→ beaconiam#509。
- 预生产人工核对（建站 → IAM 子组织 → 成员绑定）→ #2550（`waitForConfirm`）。
- 中转物流路由/费用/中转单状态机（`docs/cases/transit.md` §4 语义不变）。
- 直营店/加盟店类型与表单（#2545）。
- 商户网点（`type=store/direct`）的建站/成员路径（`site.go` / `site_member_common.go` 的商户分支不改）。
- `transit_order.go` 的租户锚（#1935 平台域，另单）。
- 工单/订单跨租户归属复核（#509 P5 后另立）。

## Core Update

[Core Update Required]

| 文档路径 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §10.18.0 `system_admin` 行只写「`sites.type='transit'`」；§10.18.x by-id 判据写 `sites.tenant_id/org_id = oid` | 两处判据统一为「`sites.type='transit'` **且** `sites.tenant_id = 平台租户（`tid`，空回落 `oid`）`；成员经 `site_members.site_id → sites`」；补一句「中转网点持有自己的 branch 子组织（`sites.org_id`），成员 IAM 绑定目标 = 该组织」 | 判据换维度是可见性契约，不落字会导致下一轮再分裂 |
| `docs/cases/transit.md` | §4 归属行仍「平台顶层组织（租户=平台根）」 | 改为「命名空间组织下的直营分支子组织（`kind=branch`，无 OWNER）；成员 IAM 绑定 = 该子组织；本地 `sites.tenant_id` 仍是平台租户」 | 用例是验收权威，必须与实现同口径 |
| `docs/ops/tenant-id-audit.md` | `:433`「tenant=org=平台租户」、`:462`「#1935 平台顶层组织」 | `transit_site_admin.go` 行改为「tenant=平台租户；org=中转网点 branch 子组织（#2547/#2552）；成员行 tenant 必须 = 平台租户（D2）」 | 审计表是隔离口径的登记簿，旧结论会误导后续改造 |
| `docs/spec/database/database.md` | §2.10 `sites.org_id` 说明只说「网点自身的 IAM 组织 ID」 | 补：「`type='transit'` 时 = 命名空间组织下的 `branch` 子组织（无 owner）；`tenant_id` 仍为平台租户，两列不同域，禁止互相替代」 | 字段语义升级必须写明 |

## Dependencies

无

### 关联（非硬依赖）

- `#2547`（`status:done`）：本单的验收基准（Accepted Plan + Round 1 审计）。其 `origin/issue-2547` 分支（`81d2e2bd3`）上有已写好的等价实现与证据，可作为参考，但**不得直接 cherry-pick 无审查**（该分支同样含 D2 违反）。
- `#2549`（PR，merged `e83ee8bd7`）：本次缺陷引入点与建站基础实现。
- `#2550`（`waitForConfirm`）：预生产人工核对（建站 → 子组织 → 成员绑定）。
- `#2554`（本计划创建，`waitForConfirm`）：预生产 tenant 污染/重复行只读核对 + 合并后 `origin/develop` 证据可读性回评。
- `beaconiam#509`：`kind=branch` / `parent_id` 语义定义来源。
- `#2544`（done）：用例修订 C1（中转网点归属口径）。
- `#2508` / `#2274`：`sites.org_id` 与 `sites.id` 不同域的先例（本单沿用）。

### Deployment-side Follow-ups

- #2554：预生产 tenant 污染/重复行只读清单（含 SQL）、Owner 授权后的收敛、`origin/develop` 上 `docs/evidence/2552/` 可读性 + `plan-source.sh --issue 2552` 四源 PASS（合并后回评）。

*Model: DeepSeek V4.1 Flash 1017*

