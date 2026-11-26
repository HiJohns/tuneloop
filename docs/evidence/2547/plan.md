## Accepted Plan — #2547 中转网点建 IAM 子组织（命名空间组织下的直营分支），成员绑定到该子组织

> 需求来源：Owner 2026-11-26（「中转网点 = 平台直属直营，挂在命名空间组织的下属组织」）。
> 关联：beaconiam#509（组织独立性模型，Comment 2026-11-26）、#2544（用例修订 C1）、#2545（直营店/加盟店表单，本单不涉及）。

### 变更点

1. **建站时创建 IAM 子组织**（`backend/handlers/transit_site_admin.go` `CreateAdminTransitSite`）
   - 新增命名空间主组织解析：`ListOrganizations()` → 恰好一个 `IsPrimary=true` 的组织即 `parent_id`；0 个或多个 → `50000`（显式失败，不猜）。
   - 调 `CreateOrganizationWithToken`：`Name=req.Name`、`ParentID=<namespace primary org>`、`Type="branch"`、`Address`、`ContactPhone`、`NotificationLang=middleware.GetCulture(c)`、`OperatorID=middleware.GetUserID(ctx)`、**`AdminInfo=nil`**（分支无 owner）。
   - IAM 成功后把 `org_id` 写入 `sites.org_id`；`sites.tenant_id` 保持现有平台租户语义（`middleware.GetTenantID`）。
   - 失败语义：IAM 建组织失败 → 不写本地 `sites` 行（**不存在无 IAM 组织的站点**）；IAM 成功但本地落库失败 → 补偿 `DeleteOrganization(orgID)`（best-effort + `log.Printf`，仍返回 500）。
   - 错误映射：IAM `name conflict`/`already exists` → `409 / 40900`「同名组织已存在」；其余 → `50000` 带原始错误；`tenantID` 为空 → `400 / 40002`（`sites.tenant_id` NOT NULL，提前显式拒绝）。

2. **IAM 客户端支持组织类型**（`backend/services/iam_client.go`）
   - `CreateOrganizationRequest` 增 `Type string \`json:"type,omitempty"\``；`Organization` 增 `Type string \`json:"type"\``（响应侧断言用）。
   - 既有调用方（`merchant.go` / `site.go`）不传该字段 → `omitempty` 不改变请求体。

3. **成员绑定目标确认（不改绑定代码，补契约与断言）**（`backend/handlers/transit_site_admin.go`）
   - `AddTransitSiteMember` 走 `addMembersCore(c, db, site, site.TenantID, siteID, input)`：**IAM 绑定目标一直是 `site.OrgID`**（`site_member_common.go:330`），第 4 个参数只写入本地 `site_members.tenant_id` / `users.tenant_id`。
   - 因此本单不改该调用，只补注释说明「本地租户作用域 = site.TenantID；IAM 目标 = site.OrgID（中转子组织）」。
   - `UpdateTransitSiteMemberRole` / `RemoveTransitSiteMember` 已用 `site.OrgID`，补断言锁定（bind / role / unbind 三条路径的 org 段必须等于 `sites.org_id`）。
   - 角色模板：`assignTransitRoleTemplate` 与 `addMembersCore` 均按 `ListRoleTemplates(namespaceID)` 取**命名空间级**模板；补断言「不创建任何 org 级模板」。

4. **中转成员识别口径**（`backend/handlers/personnel.go`）
   - `personnelSystemView`（:312）与 `hasTransitSiteMembership`（:639）去掉 `s.org_id = ?` 判据，保留 `s.type = 'transit'` + 平台租户判据，成员关系仍经 `site_members.site_id → sites`。
   - 平台租户判据由 `oid` 改为 **`tid`**（`middleware.GetTenantID`，空时已回落 `oid`）：`sites.tenant_id` 建站时写入的就是创建者的 `tid`，用同一来源比较才不会因 `tid ≠ oid`（平台租户 vs 命名空间主组织）而漏检；`personnelSystemView` 增加 `tid` 形参，`hasTransitSiteMembership` 的 org 参数替换为平台租户参数。
   - 理由：子组织建立后 `sites.org_id ≠ 平台组织`，沿用 `org_id = oid` 会让既有中转成员从系统视图消失；`tenant_id` 保留作平台域隔离（防止商户侧 `type` 脏值把自有成员带进平台视图）。

5. **存量回填工具（只读清单 + 显式 apply，不自动执行）**
   - 新增 `backend/services/transit_site_org_backfill.go`：
     - `PlanTransitSiteOrgBackfill(db, iam) ([]Row, error)`：列出 `type='transit'` 且 `org_id = tenant_id`（或 `org_id` 空）的存量站 → 只读清单（`site_id/name/tenant_id/org_id/status/member_count/needs_backfill`）。
     - `ApplyTransitSiteOrgBackfill(db, iam, rows, opts)`：逐站建分支子组织（`parent_id` = 命名空间主组织，`Type="branch"`，无 owner）→ 更新 `sites.org_id`；已回填站跳过（幂等）。
   - 新增 `backend/tools/backfill_transit_site_orgs/main.go` 薄 CLI：默认 `--dry-run`（只打印清单，0 写入）；`--apply` 才写；`--phase=orgs|members`，`--phase=members` 才按 `site_members.role` 重新绑定成员到新组织（`BindUserToOrganization`，client credentials，operator=`backfill`），默认不动成员关系。
   - **不在任何启动路径调用**（不挂 `iam_bootstrap`、不挂 migration）。

6. **文档同步**
   - `docs/spec/api/api.md`：§10.18.0（:6261）与 §10.18.x 可见性口径（:6498）的中转判据改为 `sites.type='transit'` + `sites.tenant_id = oid`（成员经 `site_members.site_id`），并注明中转网点持有自己的分支子组织（`sites.org_id`）。
   - `docs/cases/transit.md`（:59）：归属改为「命名空间组织下的直营分支子组织（`type=branch`，无 owner）」；成员 IAM 绑定目标 = 该子组织。
   - `docs/ops/tenant-id-audit.md`（:433）：`transit_site_admin.go` 行改为「tenant=平台租户；org=中转子组织（#2547）」。
   - `docs/spec/database/database.md`（:362）：`sites.org_id` 说明补一句「中转网点同规则，组织为命名空间组织下的 branch 子组织」。

### Files

- `backend/handlers/transit_site_admin.go`
- `backend/handlers/personnel.go`
- `backend/services/iam_client.go`
- `backend/services/transit_site_org_backfill.go`（新增）
- `backend/tools/backfill_transit_site_orgs/main.go`（新增）
- `backend/handlers/transit_site_admin_test.go`（mock 扩展 + 既有创建用例适配）
- `backend/handlers/transit_site_admin_2547_test.go`（新增）
- `backend/handlers/personnel_transit_org_2547_test.go`（新增）
- `backend/services/transit_site_org_backfill_2547_test.go`（新增）
- `scripts/verify-2547.sh`（新增）
- `docs/spec/api/api.md`
- `docs/cases/transit.md`
- `docs/ops/tenant-id-audit.md`
- `docs/spec/database/database.md`
- `docs/evidence/2547/plan.md`
- `docs/evidence/2547/evidence.json`
- `docs/evidence/2547/pre-ready-2547.json`
- `docs/evidence/2547/R1-create-org.txt`
- `docs/evidence/2547/R2-member-bind-org.txt`
- `docs/evidence/2547/R3-personnel-visibility.txt`
- `docs/evidence/2547/R4-backfill-dryrun.txt`
- `docs/evidence/2547/R5-docs-sync.txt`
- `docs/evidence/2547/R6-build-regression.txt`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 建站创建 IAM 分支子组织并写入 `sites.org_id` | `backend/handlers/transit_site_admin.go`、`backend/services/iam_client.go` | `POST /api/admin/transit-sites` | mock IAM：`is_primary=rootOrg`、create 返回 `newOrg` | backend（Postgres 测试库） | 响应 `data.org_id=newOrg`；IAM 收到 `parent_id=rootOrg`、`type=branch`、无 `admin_info`；IAM 失败 / 无主组织 / `tenant_id` 空 → 均无 `sites` 行；同名 → 409 | R1 |
| R2 成员 bind/role/unbind 的 org 段 = `sites.org_id` | `backend/handlers/transit_site_admin.go` | `POST/PUT/DELETE /api/admin/transit-sites/:id/members` | `site.OrgID=newOrg` 且 `site.TenantID=tid`（两者不等） | backend（Postgres 测试库） | 三条路径的 org 段均为 `newOrg`；`site_members.tenant_id=tid`；模板请求走 namespace 级且无 org 级模板创建 | R2 |
| R3 中转成员识别改为站点维度（列表 + by-id） | `backend/handlers/personnel.go` | `GET /api/admin/personnel`、`GET /api/admin/staff/:id` | 中转站 `org_id=newOrg`、`tenant_id=平台租户`；另有 `type=store` 站与 `type=transit` 他租户站 | backend（Postgres 测试库） | 中转成员在系统视图与 by-id 均可见；`type=store` 同租户成员、他租户 `transit` 站成员均不可见 | R3 |
| R4 存量回填 dry-run 0 写入 + apply 幂等 | `backend/services/transit_site_org_backfill.go`、`backend/tools/backfill_transit_site_orgs/main.go` | `PlanTransitSiteOrgBackfill` / `ApplyTransitSiteOrgBackfill` | 2 个存量站（`org_id=tenant_id`）+ 1 个已回填站 | backend（Postgres 测试库） | dry-run 清单 2 条且 0 写入；apply 改 2 站、跳过 1 站、建 2 个 IAM 组织；二次 apply 0 变更 | R4 |
| R5 文档口径同步 | `docs/spec/api/api.md`、`docs/cases/transit.md`、`docs/ops/tenant-id-audit.md`、`docs/spec/database/database.md` | 文档 grep | 修后文档 | 文档 | 四份文档均含新口径断言（见 G0） | R5 |
| R6 构建 + 定向回归 | `backend/`、`scripts/verify-2547.sh` | `bash scripts/verify-2547.sh r6` | 修后工作树 | backend | `BUILD_RC=0`、`GOFMT_DIFF=0`、中转/人员定向套件 0 失败 | R6 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1–R4 | runtime | Go 测试运行真实 handler + 真实 Postgres 测试库 + mock IAM（捕获请求体/路径），非静态探针 |
| R5 | static | 文档 grep 断言（无运行时语义） |
| R6 | runtime | `go build ./...` + 定向测试 + `gofmt -l` |

### Input Truth & Tool Preflight

- **Source of truth（口径唯一）**
  - `sites.org_id` = 「网点自身的 IAM 组织 ID，创建网点时由 IAM Create Organization 返回」（`docs/spec/database/database.md:362`）——中转网点现状违反该定义，本单收敛。
  - 组织类型只有 `organizations.type`（beaconiam `internal/handler/org_handler.go` `CreateOrgRequest.Type`；`Organizations` 表无 `kind` 列），#509 的 `kind` / `org_links` 尚未实现。
  - 分支允许 0 owner：beaconiam `CreateOrg` 仅在 `admin_info.username && admin_info.email` 均非空时才建 owner（`org_handler.go:199`），不传即无 owner。
  - 成员 IAM 绑定目标 = `site.OrgID`（`backend/handlers/site_member_common.go:330`），本地 `tenant_id` 参数与绑定目标无关。
- **差异核对（只读，/analyze 已执行）**
  - `grep -n "s.org_id = ?\|s.tenant_id = ?" backend/handlers/personnel.go` → 命中 :312 与 :639 两处，均需改。
  - `grep -rn "transitSiteType\|type = 'transit'" backend/handlers` → 创建/列表/成员/路由各点；除 `personnel.go` 两处外无第三处依赖 `org_id = 平台组织`。
  - `grep -n "Type" backend/services/iam_client.go` → `CreateOrganizationRequest` 当前无 `Type` 字段（需补）。
  - `grep -n "OrgID" backend/handlers/transit_site_admin.go` → 成员三条路径已用 `site.OrgID`，无需改绑定目标。
- **损坏输入拒绝**
  - IAM `CreateOrganization` 返回缺 `org_id` → 客户端已显式报错（`iam_client.go:335`）；handler 不落库。
  - 主组织解析结果 ≠ 1 → `50000` 显式失败（不回落 `tenantID`，避免把 branch 建到错误父节点）。
  - `tenantID` 为空 → `40002`（不写 NOT NULL 空值）。
- **工具盲区**
  - mock IAM 只证明「请求形状正确」，不证明预生产 IAM 的真实接受度；真实建组织/绑定的端到端确认属部署侧（Decision Lock D6）。
  - `grep` 文档断言只证明文案存在，不证明语义被下游代码消费；语义由 R1–R4 的运行时断言承担。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | Issue 中的 `kind=branch` 如何落地 | 写入 `organizations.type="branch"`（beaconiam 仅此列可用，`kind`/`org_links` 属 #509 未实现）。不新增 `kind` 列，不建 `org_links`。authority: `target_semantics` |
| D2 | `addMembersCore` 的 `tenantID` 参数 | 仅本地租户列作用域；**不**改为 `site.OrgID`（会破坏 `site_members→sites.tenant_id` 的本地隔离）。IAM 目标固定 `site.OrgID`。 |
| D3 | 中转成员识别判据 | `sites.type='transit' AND sites.tenant_id = 平台租户(tid，空则回落 oid)` + `site_members.site_id → sites`；移除 `s.org_id = oid`。tenant 判据保留平台域隔离（商户侧 `type` 脏值不得把自有成员带进平台视图）。 |
| D4 | 建站路由权限 | 维持 `SysPermTenantView`（bit5）；**不**加 `SysPermOrganizationCreate`（bit12）——`sys_admin`/`namespace_admin` 模板位为 `{5,6,7,8,9,15..19}`（`services/role_templates.go:18`），加会 403，需另立 IAM 角色模板同步单。 |
| D5 | 建站顺序与失败补偿 | 先 IAM 建组织 → 再落 `sites` 行；落库失败补偿 `DeleteOrganization`（best-effort + 日志），保证「无站点的孤儿组织」可见于日志、「无组织的站点」不存在。 |
| D6 | 存量回填执行 | 属部署侧（Owner 授权 + 预生产先行）；本单只交付脚本 + dry-run 证据，**不执行任何数据写入**。 |
| D7 | 站点改名/停用是否联动 IAM 组织 | 本单不做（Out of Scope），仅记录为已知边界，避免静默扩张。 |
| D8 | 证据形态 | R1–R4 为运行时断言（handler + Postgres 测试库 + mock IAM），R5 静态文档断言，R6 构建/回归；无 UI 改动故无渲染证据需求。 |

### Out of Scope

- 存量中转网点回填的**实际执行**（数据修复需 Owner 授权，属部署侧，收口时另立 Ops 跟踪单）。
- 站点改名 / 停用与 IAM 组织名 / 组织停用的联动（D7）。
- 直营店 / 加盟店创建表单与类型语义（另属 #2545）。
- 用例库全量审核与其余文件修订（文档类 #2544；本单只改 `docs/cases/transit.md` 的中转归属行以解锁 C1）。
- beaconiam 侧 `kind` 列、`org_links`、角色模板作用域（#509）。
- 前端改动（`TransitCenter.jsx` 已按 `r.code !== 20000` 透出后端 message，无需改）。
- 中转路由 / 物流费 / 转发会话语义。

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 触发条件 | 平台管理员在「中转中心」创建中转网点 → 只落本地 `sites` 行（`OrgID=TenantID`）→ 随后加成员时 `site.OrgID` 仍是平台租户组织 | `transit_site_admin.go:86-100`、`:271` |
| 2 | 根因 | 「站点自身组织」与「平台租户组织」两个概念被同一列值冒充；建站路径缺少 IAM 组织创建步骤（商户网点有，中转网点没有） | `site.go:339`（有）vs `transit_site_admin.go`（无） |
| 3 | 为何长期未发现 | 成员绑定/角色更新只校验 HTTP 200，不校验 org 段；本地 `site_members` 一切正常；`sites.org_id = tenant_id` 在 dev 小数据下「看起来对」 | 既有 `transit_site_admin_test.go` 无 org 断言 |
| 4 | 同类扫描 | `grep -rn "transit" backend/handlers backend/services` 后仅 `personnel.go:312/639` 第二处依赖 `org_id = 平台组织`；其余（`transit_route.go`、`site.go:460`）只按 `type='transit'` 判定，不受影响 | /analyze grep 实测 |
| 5 | 防复发（代码） | 建站强制先建 IAM 组织再落库；主组织解析唯一入口（`ListOrganizations` + `IsPrimary` 恰好 1）；成员路径绑定目标固定 `site.OrgID` | 变更点 1–3 |
| 6 | 防复发（测试） | 新增「org 段 = `sites.org_id`」断言（bind/role/unbind 三条）；新增「IAM 失败 / 无主组织 / tenant 空 → 无站点行」负例 | R1/R2 |
| 7 | 防复发（文档） | `sites.org_id` 定义、中转归属、人员可见性三处文档同批更新，避免下一轮按旧文档实现 | R5、`## Core Update` |
| 8 | 假阴性反例 | 若 handler 仍写 `OrgID: tenantID` → R1 `CREATE_SITE_ORG` 不等于 `newOrg` → FAIL；若成员绑定仍取 tenant → R2 `BIND_ORG_NOT_TENANT=0` → FAIL；若人员视图仍用 `org_id=oid` → R3 `SYSTEM_TRANSIT_VISIBLE=0` → FAIL；若 dry-run 写库 → R4 `DRYRUN_WRITES=0` 失败 | R1–R4 负例 |
| 9 | 回归面 | 既有 `TestCreateAdminTransitSiteValidation`、`TestTransitSiteMemberIAMSync`、`TestTransitSiteMemberRoleTemplateErrorReported`、`TestPersonnel2065_*`、`TestPersonnel2526_*` 全绿（夹具 `transitSite.TenantID=rootOrg` 仍命中新判据） | R6 |
| 10 | 未修边界 | 存量站点回填执行（D6）、改名/停用联动（D7）、beaconiam `kind`（#509） | Out of Scope |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | mock IAM：`GET /api/v1/organizations` → `[{id:rootOrg,is_primary:true}]`；`POST /namespaces/:ns/organizations` → `{org_id:newOrg}`；失败档返回 500 | `POST /api/admin/transit-sites`（带 `Authorization: Bearer`） | `docs/evidence/2547/R1-create-org.txt` | `CREATE_SITE_ORG=1`、`CREATE_PARENT_IS_NAMESPACE=1`、`CREATE_TYPE_BRANCH=1`、`CREATE_ADMIN_INFO_ABSENT=1`、`NO_SITE_ON_IAM_FAILURE=1`、`NO_SITE_ON_NO_PRIMARY=1`、`NO_SITE_ON_EMPTY_TENANT=1`、`DUPLICATE_NAME_409=1` | 站点 `org_id` 仍等于 `tenant_id`，或失败分支残留 `sites` 行 → FAIL |
| R2 | site：`org_id=newOrg`、`tenant_id=tid`（`newOrg≠tid`）；user 行已存在 | `POST`/`PUT`/`DELETE /api/admin/transit-sites/:id/members` | `docs/evidence/2547/R2-member-bind-org.txt` | `BIND_ORG_IS_SITE_ORG=1`、`BIND_ORG_NOT_TENANT=1`、`ROLE_ORG_IS_SITE_ORG=1`、`UNBIND_ORG_IS_SITE_ORG=1`、`MEMBER_TENANT_IS_LOCAL_TENANT=1`、`ROLE_TEMPLATE_NS_SCOPED=1`、`NO_ORG_TEMPLATE_CREATE=1` | 任一 IAM 路径 org 段等于 `tenant_id` → FAIL |
| R3 | 中转站 `type=transit`、`org_id=newOrg`、`tenant_id=平台租户`；对照站 `type=store` 同租户；对照站 `type=transit` 他租户；各站 1 名 active 成员 | `GET /api/admin/personnel?actor=system`、`GET /api/admin/staff/:id` | `docs/evidence/2547/R3-personnel-visibility.txt` | `SYSTEM_TRANSIT_VISIBLE=1`、`SYSTEM_NON_TRANSIT_PLATFORM_SITE_HIDDEN=1`、`SYSTEM_FOREIGN_TENANT_TRANSIT_HIDDEN=1`、`STAFF_DETAIL_TRANSIT_VISIBLE=1` | 中转成员缺席系统视图，或 `type=store`/他租户成员被误纳入 → FAIL |
| R4 | 2 个存量站（`org_id=tenant_id`）+ 1 个已回填站；mock IAM 记录 create 次数 | `PlanTransitSiteOrgBackfill` / `ApplyTransitSiteOrgBackfill` | `docs/evidence/2547/R4-backfill-dryrun.txt` | `DRYRUN_ROWS=2`、`DRYRUN_WRITES=0`、`DRYRUN_IAM_CREATES=0`、`APPLY_UPDATED=2`、`APPLY_SKIPPED_MODERN=1`、`APPLY_IAM_CREATES=2`、`SECOND_RUN_UPDATED=0` | dry-run 触发 IAM/DB 写入，或二次 apply 重复建组织 → FAIL |
| R5 | 修后四份文档 | `bash scripts/verify-2547.sh r5` | `docs/evidence/2547/R5-docs-sync.txt` | `API_TRANSIT_RULE=1`、`CASES_TRANSIT_ORG=1`、`AUDIT_DOC_UPDATED=1`、`DB_DOC_TRANSIT=1`、`OLD_ORG_EQUALS_TENANT_RULE=0` | 任一份文档仍写 `org_id = 平台组织` 判据 → FAIL |
| R6 | 修后工作树 | `bash scripts/verify-2547.sh r6` | `docs/evidence/2547/R6-build-regression.txt` | `BUILD_RC=0`、`GOFMT_DIFF=0`、`TRANSIT_SUITE_OK=1`、`PERSONNEL_SUITE_OK=1`、`SERVICES_SUITE_OK=1` | 任一构建/套件/格式失败 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2547.sh r1 > docs/evidence/2547/R1-create-org.txt` | `docs/evidence/2547/R1-create-org.txt` | exit 0；含 `CREATE_SITE_ORG=1`、`CREATE_PARENT_IS_NAMESPACE=1`、`CREATE_TYPE_BRANCH=1`、`NO_SITE_ON_IAM_FAILURE=1`、`DUPLICATE_NAME_409=1` | PASS | 站点 org 仍等于 tenant，或失败后残留站点行 → FAIL | dev |
| R2 | `bash scripts/verify-2547.sh r2 > docs/evidence/2547/R2-member-bind-org.txt` | `docs/evidence/2547/R2-member-bind-org.txt` | exit 0；含 `BIND_ORG_IS_SITE_ORG=1`、`BIND_ORG_NOT_TENANT=1`、`MEMBER_TENANT_IS_LOCAL_TENANT=1`、`NO_ORG_TEMPLATE_CREATE=1` | PASS | 任一 IAM org 段等于 tenant → FAIL | dev |
| R3 | `bash scripts/verify-2547.sh r3 > docs/evidence/2547/R3-personnel-visibility.txt` | `docs/evidence/2547/R3-personnel-visibility.txt` | exit 0；含 `SYSTEM_TRANSIT_VISIBLE=1`、`SYSTEM_NON_TRANSIT_PLATFORM_SITE_HIDDEN=1`、`SYSTEM_FOREIGN_TENANT_TRANSIT_HIDDEN=1`、`STAFF_DETAIL_TRANSIT_VISIBLE=1` | PASS | 中转成员缺席或非中转站/他租户成员被纳入 → FAIL | dev |
| R4 | `bash scripts/verify-2547.sh r4 > docs/evidence/2547/R4-backfill-dryrun.txt` | `docs/evidence/2547/R4-backfill-dryrun.txt` | exit 0；含 `DRYRUN_WRITES=0`、`DRYRUN_IAM_CREATES=0`、`APPLY_UPDATED=2`、`SECOND_RUN_UPDATED=0` | PASS | dry-run 写入或二次 apply 重复建组织 → FAIL | dev |
| R5 | `bash scripts/verify-2547.sh r5 > docs/evidence/2547/R5-docs-sync.txt` | `docs/evidence/2547/R5-docs-sync.txt` | exit 0；含 `API_TRANSIT_RULE=1`、`CASES_TRANSIT_ORG=1`、`AUDIT_DOC_UPDATED=1`、`OLD_ORG_EQUALS_TENANT_RULE=0` | PASS | 任一文档保留旧判据 → FAIL | dev |
| R6 | `bash scripts/verify-2547.sh r6 > docs/evidence/2547/R6-build-regression.txt` | `docs/evidence/2547/R6-build-regression.txt` | exit 0；含 `BUILD_RC=0`、`GOFMT_DIFF=0`、`TRANSIT_SUITE_OK=1`、`PERSONNEL_SUITE_OK=1`、`SERVICES_SUITE_OK=1` | PASS | 构建/套件/格式任一失败 → FAIL | dev |

### Artifact Provenance

- 生成器：`scripts/verify-2547.sh`（子命令 `r1`–`r6`）。每个文本产物首行固定为 `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`，次行 `GENERATED_AT_HEAD=<40 位 sha>`；`dirty=false` 由脚本校验工作树干净后写出。
- 逐条清单（全部位于 `docs/evidence/2547/`）：
  - `docs/evidence/2547/R1-create-org.txt`
  - `docs/evidence/2547/R2-member-bind-org.txt`
  - `docs/evidence/2547/R3-personnel-visibility.txt`
  - `docs/evidence/2547/R4-backfill-dryrun.txt`
  - `docs/evidence/2547/R5-docs-sync.txt`
  - `docs/evidence/2547/R6-build-regression.txt`
  - `docs/evidence/2547/evidence.json`
  - `docs/evidence/2547/pre-ready-2547.json`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2547 --summary-file tmp/pre-ready-2547-summary.md --changed-files docs/evidence/2547/changed-files.txt --evidence-file docs/evidence/2547/evidence.json --report docs/evidence/2547/pre-ready-2547.json`
- `evidence.json` 的 build 字段须匹配 evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Dependencies

- 无

### Workload

- Size: M
- Basis: 后端 3 文件改动 + 1 服务 + 1 CLI + 3 个测试文件（含 mock 扩展）+ 1 个验证脚本 + 4 份文档 + 6 条证据行；无 migration、无前端改动、无外部依赖阻塞
- Confidence: high

### Core Update

| 文档路径 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §10.18.0（:6261）与 §10.18.x（:6498）把中转成员判据写成 `sites.tenant_id/org_id = oid` | 改为 `sites.type='transit' AND sites.tenant_id = oid`，成员经 `site_members.site_id → sites`；补一句中转网点持有自己的 `branch` 子组织（`sites.org_id`） | 子组织建立后 `sites.org_id ≠ 平台组织`，旧判据会让中转成员从系统视图消失 |
| `docs/cases/transit.md` | :59 写「归属：平台顶层组织（租户=平台根）」 | 改为「归属：命名空间组织下的直营分支子组织（`type=branch`，无 owner）；成员 IAM 绑定目标 = 该子组织」 | Owner 2026-11-26 组织独立性模型；同时回答 #2544 C1 |
| `docs/ops/tenant-id-audit.md` | :433 写「tenant=org=平台租户」 | 改为「tenant=平台租户；org=中转子组织（#2547）」 | 审计表口径与实现对齐 |
| `docs/spec/database/database.md` | :362 `sites.org_id` 仅述一般网点 | 补「中转网点同规则：组织为命名空间组织下的 branch 子组织，由建站时 IAM 创建」 | 消除「中转例外」的隐含假设 |

