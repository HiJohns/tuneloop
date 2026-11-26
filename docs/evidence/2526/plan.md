## Accepted Plan — #2526 编辑人员详情 404（零租户存量行）：可见性口径与人员列表对齐｜Rev.2

> Tags: bug, code:go, api-contract, scope:pc
> 关联：#2065/#2067（三视图 + membership 权威）、#2429（详情按 id 加载）、#2508（同页 ID 域缺陷）、#2375（UpdateUser 角色白名单）、#2249 S1（`users.tenant_id` 是本地缓存，禁止用于过滤）、#2495（被本单阻塞的验收对象）、#2527（本单显式排除的同根因后续项）

### 修订记录（Rev.1 → Rev.2：D1 口径自相矛盾 → 采纳角色感知并集）

来源：Investigation Packet（D1「超集」与 R4「纯顾客 404」互斥）+ 预生产只读核对。

| # | 修订项 | Rev.1 | Rev.2 |
|---|---|---|---|
| 1 | D1 可见性口径 | `旧 tenant/org 口径 ∪ membership`（无角色条件）→ 纯顾客因 `users.tenant_id = tid` 命中，R4 不可达 | **`membership ∪（tenant/org 命中 ∧ 非顾客角色 `lower(users.role) ∉ {'','user'}`）`** |
| 2 | 语义变更清单 | 「**无翻转**（超集）」 | 「**无既有断言翻转**；登记 1 处生产行为收窄：纯顾客 by-id 详情/保存/激活 `200 → 404`」 |
| 3 | 判定表 | merchant_admin / site_admin 的旧口径无角色条件 | 两处补 `lower(users.role) ∉ {'','user'}`；system_admin 与「其他」行不变 |
| 4 | R1 夹具保真 | `zeroTenantMember`（`role=member`） | 追加 `zeroTenantRoleUser`（nil 租户/组织 + `role=USER` + `site_members(siteA)`）变体，R1 增加 `DETAIL_ZERO_TENANT_USER_ROLE_STATUS=200`，对齐主诉生产样本 |
| 5 | B10 / B4 修复层表述 | B4 写「超集」，B10 写「区分 membership」 | B4 改为「角色感知并集（不加宽到纯顾客）」；B10 改为「tenant 分支排除顾客角色，membership 优先」 |

**预生产只读核对（回答 Packet open_question 1，2026-11-26）**：`users` 中 `lower(role) ∈ {'','user'}`、`deleted_at IS NULL`、`tenant_id` 非 nil UUID、且无任何 active `site_members`/`merchant_members`/`technician_profiles` 关系的行 = **11 行，全部属于商户 `tuneloop`（tenant `3cfa99da-…`），且 11 行全部 `iam_sub` 非空**。这 11 行因列表以 membership 锚定而**不在人员列表里**，仅能通过直接 URL 打开 → Option B 把它们从「可直接打开」变为 404，**没有任何列表可见行由此失去访问**（B10 正是要消除这条 PII 旁路）。

不在本次修订范围：产品代码实现方式（`loadVisibleStaffUser` + `personnelUserVisible` 单一判定）、R5 不变量、`attribution_missing` 字段与 PC 渲染。

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 人员列表三视图**以 membership 为权威**：商户视图按 `sites.tenant_id` / `merchant_members.tenant_id` / `technician_profiles.tenant_id` 取行，并在取 `users` 时显式关闭自动租户范围（`ExplicitTenantScopeOnly`），注释明确「`users.tenant_id` 不可靠（零租户用户）」 | `backend/handlers/personnel.go:137-205`、`:225-235` |
| 详情 `GetStaffUser` 却按 `users.id = ? AND users.tenant_id = ?` 过滤，并附加 `ApplyOrgScope`（`org_id IN (...)`) | `backend/handlers/user_staff.go:961-965` |
| 保存 `UpdateUser` 的存在性判断同一条 `users.tenant_id = ?`；真正写库的 UPDATE 也带 `tenant_id = ?`，且**不检查 `RowsAffected`** | `backend/handlers/user_staff.go:1066`、`:1219` |
| 激活 `ActivateUser` 的存在性判断同样是 `users.tenant_id = ?` | `backend/handlers/user_staff.go:619` |
| 生产样本（Owner，预生产 `1.0.98`）：`users.id=3332ac1f-…`（`linwx1978@163.com`）`role=USER`、`tenant_id`/`org_id` 均为 `00000000-0000-0000-0000-000000000000`，`iam_sub` 非空；列表可见，`GET /api/admin/staff/3332ac1f-…` 连续 3 次 404 | #2526 正文 |
| 既有夹具已复刻该形态：`zeroTenantMember` 的 `users.tenant_id/org_id` 全零，但 `site_members` 归属本商户 `siteA` | `backend/handlers/personnel_2065_test.go:38`、`:87`、`:102-104` |
| 同一坑已有先例：`UpdateIAMUser` 因带 `tenant_id` 过滤导致「零租户行静默 0 行更新」，改为按 `iam_sub` 更新 | `backend/handlers/iam_proxy.go:617-625` |
| 契约文档只写了列表口径，**没有** `GET /admin/staff/:id` 的可见性定义 → 列表与详情的口径分裂无人校验 | `docs/spec/api/api.md:6242-6265` |

**根因**：**可见性判定源不唯一**。列表按 membership 判定（因此零租户行可见），详情/保存/激活按 `users.tenant_id` 判定（因此同一行查不到）→「列表看得见、点开 404」。这是 #2067 只把 `users.tenant_id 不可靠` 修在列表侧、未对称到 by-id 动作造成的口径分裂；「静默」还叠了第二层：保存路径即使放宽查询，UPDATE 仍带 `tenant_id` 且不校验 `RowsAffected` → 会变成「保存成功但没写库」（比 404 更难发现）。

### 修复设计（最小、可测、语义显式）

1. **新增唯一可见性判定（放 `backend/handlers/personnel.go`，与三视图同文件）**：
   - `type staffUserLookup int`（`staffUserFound` / `staffUserMissing` / `staffUserInvisible`）
   - `func loadVisibleStaffUser(db *gorm.DB, ctx context.Context, userID string) (*models.User, staffUserLookup, error)`：用 `database.ExplicitTenantScopeOnly(ctx)` 按 `id + deleted_at IS NULL` 取行（不按租户过滤），再交给可见性判定；`staffUserInvisible` 与 `staffUserMissing` 在 handler 侧都映射为 `404 40400`（不泄漏存在性），但前者额外打 `WARN` 日志（可观测，沿用 #2427 D3 纪律）。
   - `func personnelUserVisible(db *gorm.DB, ctx context.Context, u *models.User) (bool, error)`——**可见 = 列表 membership 口径 ∪（旧 tenant/org 口径 ∧ 非顾客角色 `lower(u.Role) ∉ {"", "user"}`）**（Rev.2）：
     | businessRole | 判定 |
     |---|---|
     | merchant_admin | 存在 `site_members→sites.tenant_id = tid` / `merchant_members.tenant_id = tid` / `technician_profiles.tenant_id = tid` 的 active 关系（membership 权威，优先判定）**或** `users.tenant_id = tid` **且** `lower(users.role) ∉ {'', 'user'}`（旧口径，仅限员工角色） |
     | site_admin | 存在 `site_members→sites.org_id = oid` 的 active 关系（与 #2508 列表口径一致，不递归子网点）**或** `users.tenant_id = tid` + `ApplyOrgScope` **且** `lower(users.role) ∉ {'', 'user'}`（旧口径仅限员工角色） |
     | system_admin | `users.org_id = oid` 且 `lower(role) ∈ {staff, namespace_admin, sys_admin}`（与 `personnelSystemView` 同源）**或** 中转网点成员（`sites.type='transit'` 且 `sites.tenant_id/org_id = oid`）**或** `tid` 非空时的旧口径（`users.tenant_id = tid`，覆盖 NAMESPACE_ADMIN 带租户 claim 的情形，避免收窄） |
     | 其他（customer / site_member / 未知上下文） | 仅旧口径（`users.tenant_id = tid` + `ApplyOrgScope`）——保持 #2429/#2466/#2375 夹具的既有行为不变 |
   - 该并集相对 Rev.1「无角色条件超集」的变化（必须显式登记）：**加宽 2 类**——`siteMemberA` 这类 `users.org_id ≠ tid` 但属本商户网点的行、`zeroTenantMember`/`zeroTenantRoleUser` 这类 nil 租户但 membership 命中的行；**收窄 1 类**——纯顾客（`role=USER` 且无任何 membership）不再因 `users.tenant_id = tid` 被当员工打开（B10）。跨商户/跨网点仍不可见，R4/R5 负例把关。
2. **三个 by-id 动作改走同一判定**：
   - `GetStaffUser`（`user_staff.go:953`）：替换 961-965 的查询为 `loadVisibleStaffUser`；响应体其余字段与 `site_memberships` 聚合口径不变。
   - `UpdateUser`（`user_staff.go:1052`）：存在性判断改走 `loadVisibleStaffUser`；写库语句改为 `database.ExplicitTenantScopeOnly(ctx)` 下的 `WHERE id = ? AND deleted_at IS NULL`，并**校验 `RowsAffected`**（0 行 → `50000` + `ERROR` 日志，禁止「保存成功但没写」）。
   - `ActivateUser`（`user_staff.go:607`）：存在性判断改走 `loadVisibleStaffUser`；`iam_sub` 已存在 → 仍返回 `40001 user already activated`。
3. **列表归属标记**：
   - `personnelRow` 新增 `AttributionMissing bool \`json:"attribution_missing"\``（`personnel.go:30`）；
   - 由 `nilUUID = 00000000-0000-0000-0000-000000000000` 判定 `users.tenant_id` 或 `users.org_id` 为空/nil（`assemblePersonnel` 与 `personnelSystemView` 两条装配路径都填），保证「商户/网点/系统」三视图一致。
4. **PC 消费标记**（`frontend-pc/src/pages/StaffManagement.jsx:502-508`）：「所属网点」列在网点名后追加 `归属缺失` 小标签（`Tag color="gold"`，附说明性 `title`）；判定抽成纯函数 `attributionMissingBadge(record)` 放 `frontend-pc/src/utils/personnelQuery.js`（PC 无组件测试运行器，纯函数用 `node --test` 覆盖，D6）。
5. **不做的重构**：不把三视图查询改写成同一个 SQL 构造器（会把 #2065/#2427/#2508 的既有绿灯置于风险中）；口径一致性改由 R9「列表 ↔ 详情逐行一致性」用不变量断言把关（列表每一行都必须能被详情加载，且 `site_id`/`site_name` 一致）。

### 语义变更（必须翻转的既有断言，强制清单）

**无既有断言翻转**；但登记 **1 处生产行为收窄**（Rev.2 新增，必须随单说明）：

| 行为 | 修改前 | 修改后 | 依据 |
|---|---|---|---|
| 纯顾客（`lower(users.role) ∈ {'','user'}`、无任何 membership）by-id 详情/保存/激活 | `200 20000`（`users.tenant_id = tid` 命中旧口径） | `404 40400` | B10「顾客 PII by-id 暴露」；预生产现存 11 行（商户 `tuneloop`），均不在人员列表内，无列表可见行受损 |

下列既有断言逐条核对后**保持原期望**，由 R10 回归证明：

| 文件:行 | 断言 | 为何不受影响 |
|---|---|---|
| `backend/handlers/user_staff_test.go:105-116` | 随机 user id → `404 40400` | 行不存在 → `staffUserMissing` |
| `backend/handlers/user_role_assign_2375_test.go:232` | 跨租户用户改角色 → `404` | 跨租户 user 无本租户 membership，且 `users.tenant_id ≠ tid` → 不可见 |
| `backend/handlers/user_role_assign_2375_test.go:231` | 同租户（无成员关系）用户提权 → `403`（白名单先报错） | 旧口径仍命中（`users.tenant_id = tid`），与修复前一致 |
| `backend/handlers/user_staff_site_noop_2429_test.go:71,168` | 详情返回 `site_id`/`site_name`/`is_direct_staff` | 仅换了取行方式，响应字段不变；夹具上下文（仅 tenant claim）走「其他角色→旧口径」分支 |
| `backend/handlers/user_site_transfer_2466_test.go:348-395` | 调动后详情/列表读回新网点 | 同上 |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志） |
|---|---|---|---|---|---|---|
| R1 主诉：详情可加载零租户成员（红绿主线） | `backend/handlers/user_staff.go`、`backend/handlers/personnel.go` | `GET /admin/staff/:id`（测试路由，actor merchant_admin） | `setupPersonnel2065` 夹具：`zeroTenantMember` 的 `users.tenant_id/org_id` 全零，`site_members` 在 `siteA` | backend（Postgres 测试库） | `DETAIL_ZERO_TENANT_STATUS=200`、`DETAIL_ZERO_TENANT_USER_ROLE_STATUS=200`（`role=USER` 的零租户成员同样可编辑）、`OLD_PREDICATE_ROWS=0`、`MEMBERSHIP_ROWS=1`、`DETAIL_SITE_NAME=网点A`、`RED_CONFIRMED=1` | R1 |
| R2 site_admin 详情 + 跨组织负例 | `backend/handlers/personnel.go` | `GET /admin/staff/:id`，actor `oid = sites.org_id` | 同夹具，`zeroTenantMember` 属本组织站点、`siteMemberB` 属他组织 | backend | `SITE_DETAIL_ZERO=200`、`SITE_DETAIL_CROSS_ORG=404` | R2 |
| R3 system_admin 详情口径 | `backend/handlers/personnel.go` | `GET /admin/staff/:id`，actor `oid = rootOrg` | 同夹具：平台员工 / 中转网点成员 / 零租户网点成员 | backend | `SYSTEM_PLATFORM=200`、`SYSTEM_TRANSIT=200`、`SYSTEM_ZERO_TENANT=404` | R3 |
| R4 越权负例（不泄漏存在性） | `backend/handlers/personnel.go` | `GET /admin/staff/:id`，actor merchant_admin | 纯顾客（无任何 membership）与他商户成员 | backend | `MERCHANT_CUSTOMER=404`、`MERCHANT_OTHER_TENANT=404`、`MERCHANT_OTHER_TENANT_BODY_CODE=40400` | R4 |
| R5 列表 ↔ 详情逐行一致性（不变量） | `backend/handlers/personnel.go`、`backend/handlers/user_staff.go` | `GET /admin/personnel` 与 `GET /admin/staff/:id` 同 actor 顺序调用 | 商户视图 5 行（含零租户成员/直属员工/师傅） | backend | `PARITY_LIST_ROWS=5`、`PARITY_DETAIL_OK=5`、`PARITY_MISMATCH=0`（判定：详情非 200，或列表 `site_id` 非空而详情 `site_id` 不一致） | R5 |
| R6 保存：零租户行真正落库 + 跨租户不可写 | `backend/handlers/user_staff.go` | `PUT /users/:id` | 商家管理员改零租户行 `name`/`position`；另对他商户成员提交同一请求 | backend | `UPDATE_ZERO_STATUS=200`、`UPDATE_ZERO_NAME_PERSISTED=1`、`UPDATE_CROSS_TENANT_STATUS=404`、`UPDATE_CROSS_TENANT_UNCHANGED=1` | R6 |
| R7 激活行操作：零租户行不再 404 | `backend/handlers/user_staff.go` | `POST /users/:id/activate` | 零租户行已有 `iam_sub`（不再走 IAM 创建）；他商户成员同请求 | backend | `ACTIVATE_ZERO_STATUS=40001`、`ACTIVATE_CROSS_TENANT_STATUS=404` | R7 |
| R8 列表归属缺失标记（三视图一致） | `backend/handlers/personnel.go` | `GET /admin/personnel?actor=merchant`、`?actor=site` | 同夹具：零租户成员 vs 正常成员 | backend | `FLAG_ZERO_TENANT=1`、`FLAG_NORMAL_MEMBER=0`、`FLAG_SITE_VIEW_ZERO=1` | R8 |
| R9 PC 列渲染接线（纯函数 + 静态接线） | `frontend-pc/src/pages/StaffManagement.jsx`、`frontend-pc/src/utils/personnelQuery.js` | `node --test frontend-pc/src/utils/personnelQuery.test.js` + 构建 | `attribution_missing` 为 true/false/缺失/字符串 `true` 四类 fixture | PC | `NODE_TEST_RC=0`、`RENDER_WIRED=1`（源码结构断言 `evidence_type: static`，Owner 豁免见 D6）、`FRONTEND_LINT_RC=0`、`FRONTEND_BUILD_RC=0` | R9 |
| R10 回归 + 构建 | `backend/handlers/personnel.go`、`backend/handlers/user_staff.go` | `go build ./...` + 既有人员/员工用例 | 修后工作树 | backend | `BUILD_RC=0`、`REGRESSION_PASS=1`、`FAIL_COUNT=0`，含 `--- PASS: TestGetStaffUser_ReturnsSiteAndDirectFlag_2429` 与 `--- PASS: TestUpdateUserRoleForbidden_2375` | R10 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R9 | static | PC 无组件测试运行器（`frontend-pc/package.json` 仅 dev/build/lint/preview）→ 纯函数 `node --test` + 源码结构断言；Owner 豁免见 D6，真实渲染截图归 #[Ops-Followup] |

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `setupPersonnel2065`（`zeroTenantMember` 租户/组织全零 + `site_members(siteA)`） | `bash scripts/verify-2526.sh r1` | `docs/evidence/2526/R1-detail-zero-tenant.txt` | `DETAIL_ZERO_TENANT_STATUS=200`、`DETAIL_ZERO_TENANT_USER_ROLE_STATUS=200`、`OLD_PREDICATE_ROWS=0`、`MEMBERSHIP_ROWS=1`、`DETAIL_SITE_NAME=网点A`、`RED_CONFIRMED=1` | `DETAIL_ZERO_TENANT_STATUS=404`（主诉未修复）、`DETAIL_ZERO_TENANT_USER_ROLE_STATUS=404`（`role=USER` 的生产形态仍不可编辑）或 `OLD_PREDICATE_ROWS=1`（夹具未复刻缺陷，红例不成立）→ FAIL |
| R2 | 同夹具，actor `oid = sites.org_id`（≠ `sites.id`） | `bash scripts/verify-2526.sh r2` | `docs/evidence/2526/R2-site-admin-detail.txt` | `SITE_DETAIL_ZERO=200`、`SITE_DETAIL_CROSS_ORG=404` | `SITE_DETAIL_CROSS_ORG=200` → 跨组织泄漏 → FAIL |
| R3 | 同夹具，actor `oid = rootOrg`、`tid` 空 | `bash scripts/verify-2526.sh r3` | `docs/evidence/2526/R3-system-admin-detail.txt` | `SYSTEM_PLATFORM=200`、`SYSTEM_TRANSIT=200`、`SYSTEM_ZERO_TENANT=404` | `SYSTEM_ZERO_TENANT=200` → system 口径被误加宽 → FAIL |
| R4 | 同夹具：`customerOnly`（无 membership）、`otherTenantUser` | `bash scripts/verify-2526.sh r4` | `docs/evidence/2526/R4-isolation-negative.txt` | `MERCHANT_CUSTOMER=404`、`MERCHANT_CUSTOMER_BODY_CODE=40400`、`MERCHANT_OTHER_TENANT=404`、`MERCHANT_OTHER_TENANT_BODY_CODE=40400` | 任一 `=200` → 越权读 → FAIL |
| R5 | 商户视图 5 行（`siteMemberA`/`siteMemberB`/`zeroTenantMember`/`directMember`/`technician`） | `bash scripts/verify-2526.sh r5` | `docs/evidence/2526/R5-list-detail-parity.txt` | `PARITY_LIST_ROWS=5`、`PARITY_DETAIL_OK=5`、`PARITY_MISMATCH=0` | 任一列表行详情 ≠ 200，或列表 `site_id` 非空且与详情 `site_id` 不一致 → FAIL（口径再次分裂） |
| R6 | 同夹具，`zeroTenantMember` 与 `otherTenantUser` | `bash scripts/verify-2526.sh r6` | `docs/evidence/2526/R6-update-persist.txt` | `UPDATE_ZERO_STATUS=200`、`UPDATE_ZERO_NAME_PERSISTED=1`、`UPDATE_CROSS_TENANT_STATUS=404`、`UPDATE_CROSS_TENANT_UNCHANGED=1` | `UPDATE_ZERO_STATUS=200` 但 `UPDATE_ZERO_NAME_PERSISTED=0` → 静默空更新（`RowsAffected=0` 未报错）→ FAIL |
| R7 | 同夹具，`zeroTenantMember`（已激活）与他商户成员 | `bash scripts/verify-2526.sh r7` | `docs/evidence/2526/R7-activate-row-action.txt` | `ACTIVATE_ZERO_STATUS=40001`、`ACTIVATE_CROSS_TENANT_STATUS=404` | `ACTIVATE_ZERO_STATUS=404` → 行操作仍不可达 → FAIL |
| R8 | 同夹具，`actor=merchant` 与 `actor=site` | `bash scripts/verify-2526.sh r8` | `docs/evidence/2526/R8-attribution-flag.txt` | `FLAG_ZERO_TENANT=1`、`FLAG_NORMAL_MEMBER=0`、`FLAG_SITE_VIEW_ZERO=1` | `FLAG_NORMAL_MEMBER=1`（把正常行标脏）或 `FLAG_ZERO_TENANT=0`（标记未生效）→ FAIL |
| R9 | `attribution_missing` = `true` / `false` / 缺失 / 字符串 `'true'`；`StaffManagement.jsx` 源码接线 | `bash scripts/verify-2526.sh r9` | `docs/evidence/2526/R9-pc-wiring.txt` | `NODE_TEST_RC=0`、`RENDER_WIRED=1`、`FRONTEND_LINT_RC=0`、`FRONTEND_BUILD_RC=0` | `NODE_TEST_RC≠0` 或 `RENDER_WIRED=0`（列未消费字段）→ FAIL |
| R10 | 修后工作树 + 既有人员/员工用例 | `bash scripts/verify-2526.sh r10` | `docs/evidence/2526/R10-regression.txt` | `BUILD_RC=0`、`REGRESSION_PASS=1`、`FAIL_COUNT=0`、含两条既有 PASS 行 | 任一既有用例 FAIL，或 `BUILD_RC≠0` → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2526.sh r1` | `docs/evidence/2526/R1-detail-zero-tenant.txt` | exit 0；grep `DETAIL_ZERO_TENANT_STATUS=200`、`DETAIL_ZERO_TENANT_USER_ROLE_STATUS=200`、`OLD_PREDICATE_ROWS=0`、`MEMBERSHIP_ROWS=1`、`DETAIL_SITE_NAME=网点A`、`RED_CONFIRMED=1` | PASS | 出现 `DETAIL_ZERO_TENANT_STATUS=404` 或 `DETAIL_ZERO_TENANT_USER_ROLE_STATUS=404` → 主诉未修复 → FAIL | dev |
| R2 | `bash scripts/verify-2526.sh r2` | `docs/evidence/2526/R2-site-admin-detail.txt` | exit 0；grep `SITE_DETAIL_ZERO=200`、`SITE_DETAIL_CROSS_ORG=404` | PASS | `SITE_DETAIL_CROSS_ORG=200` → 跨组织泄漏 → FAIL | dev |
| R3 | `bash scripts/verify-2526.sh r3` | `docs/evidence/2526/R3-system-admin-detail.txt` | exit 0；grep `SYSTEM_PLATFORM=200`、`SYSTEM_TRANSIT=200`、`SYSTEM_ZERO_TENANT=404` | PASS | `SYSTEM_ZERO_TENANT=200` → 口径误加宽 → FAIL | dev |
| R4 | `bash scripts/verify-2526.sh r4` | `docs/evidence/2526/R4-isolation-negative.txt` | exit 0；grep `MERCHANT_CUSTOMER=404`、`MERCHANT_CUSTOMER_BODY_CODE=40400`、`MERCHANT_OTHER_TENANT=404`、`MERCHANT_OTHER_TENANT_BODY_CODE=40400` | PASS | 任一 `=200` → 越权读 → FAIL | dev |
| R5 | `bash scripts/verify-2526.sh r5` | `docs/evidence/2526/R5-list-detail-parity.txt` | exit 0；grep `PARITY_LIST_ROWS=5`、`PARITY_DETAIL_OK=5`、`PARITY_MISMATCH=0` | PASS | `PARITY_MISMATCH=1` → 列表与详情再次分裂 → FAIL | dev |
| R6 | `bash scripts/verify-2526.sh r6` | `docs/evidence/2526/R6-update-persist.txt` | exit 0；grep `UPDATE_ZERO_STATUS=200`、`UPDATE_ZERO_NAME_PERSISTED=1`、`UPDATE_CROSS_TENANT_STATUS=404`、`UPDATE_CROSS_TENANT_UNCHANGED=1` | PASS | `UPDATE_ZERO_NAME_PERSISTED=0` → 静默空更新 → FAIL | dev |
| R7 | `bash scripts/verify-2526.sh r7` | `docs/evidence/2526/R7-activate-row-action.txt` | exit 0；grep `ACTIVATE_ZERO_STATUS=40001`、`ACTIVATE_CROSS_TENANT_STATUS=404` | PASS | `ACTIVATE_ZERO_STATUS=404` → 行操作不可达 → FAIL | dev |
| R8 | `bash scripts/verify-2526.sh r8` | `docs/evidence/2526/R8-attribution-flag.txt` | exit 0；grep `FLAG_ZERO_TENANT=1`、`FLAG_NORMAL_MEMBER=0`、`FLAG_SITE_VIEW_ZERO=1` | PASS | `FLAG_NORMAL_MEMBER=1` → 误标正常行 → FAIL | dev |
| R9 | `bash scripts/verify-2526.sh r9` | `docs/evidence/2526/R9-pc-wiring.txt` | exit 0；grep `NODE_TEST_RC=0`、`RENDER_WIRED=1`、`FRONTEND_LINT_RC=0`、`FRONTEND_BUILD_RC=0` | PASS | `RENDER_WIRED=0` → 字段无消费方（死代码）→ FAIL | dev |
| R10 | `bash scripts/verify-2526.sh r10` | `docs/evidence/2526/R10-regression.txt` | exit 0；grep `BUILD_RC=0`、`REGRESSION_PASS=1`、`FAIL_COUNT=0`、`--- PASS: TestGetStaffUser_ReturnsSiteAndDirectFlag_2429`、`--- PASS: TestUpdateUserRoleForbidden_2375` | PASS | 任一既有用例 FAIL 或 `BUILD_RC=1` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2526/R1-detail-zero-tenant.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2526/R2-site-admin-detail.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2526/R3-system-admin-detail.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2526/R4-isolation-negative.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2526/R5-list-detail-parity.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2526/R6-update-persist.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2526/R7-activate-row-action.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2526/R8-attribution-flag.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2526/R9-pc-wiring.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2526/R10-regression.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 symptom | 触发条件 trigger | 根因 root cause | 测试盲区 test blind spot | 同类路径 same-class | 修复层 fix layer | 红绿 red-green | 预防 prevention |
|---|---|---|---|---|---|---|---|---|
| B1 | 列表可见、点「编辑」404「未找到用户」 | 存量行 `users.tenant_id` 为 nil UUID 且属于本商户 | 详情按 `users.tenant_id` 过滤，列表按 membership 过滤（口径分裂） | 零租户成员只测了列表（#2067），没有详情用例 | `GetStaffUser`；同页 `ActivateUser` | handler 可见性判定统一 | R1（旧谓词 0 行 → 详情 200） | R5 列表 ↔ 详情逐行一致性不变量 |
| B2 | 详情修好但「保存成功却没改」 | UPDATE 语句仍带 `tenant_id = ?` 且不校验 `RowsAffected` | 读路径放宽、写路径没跟上；GORM 0 行不报错 | 无零租户行更新用例 | `UpdateUser` 的 `Updates` 语句 | 写库改显式作用域 + `RowsAffected` 校验 | R6 `UPDATE_ZERO_NAME_PERSISTED=1` | 写路径必须断言落库（不只 status 200） |
| B3 | 修复过宽：按 `iam_sub` 直查不校验可见范围 | 详情兜底「先按 iam_sub 查一次」的写法 | 把「定位行」和「能否访问」混成一个查询 | 无跨租户详情负例 | 任何 by-id 详情接口 | 定位（显式作用域）与判定（可见性函数）分离 | R4 跨租户/纯顾客 404 | 负例矩阵固定进合同（R4） |
| B4 | 修复变窄：只按 membership，同租户员工反而打不开 | 只实现列表口径 | 忽略旧口径仍承载的合法访问（#2375 白名单用例） | 无「同租户员工但无 membership」用例 | `UpdateUser` 角色白名单路径 | 可见 = membership ∪（旧口径 ∧ 非顾客角色）；对员工是加宽，对纯顾客是收窄 | R10 `TestUpdateUserRoleForbidden_2375` PASS + R4 顾客 404 | 语义变更清单逐条核对（含 1 处收窄登记） |
| B5 | 新增字段无人消费（死代码） | 后端加 `attribution_missing` 但前端不渲染 | 后端先行、前端遗漏 | 无接线断言 | 其他「新 API 字段」改动 | 前端列渲染 + 纯函数接线断言 | R9 `RENDER_WIRED=1` | 新字段必须同单落地消费方 |
| B6 | 看不到为什么查不到（静默 404） | 行存在但越权/不可见 | 404 分支无痕迹 | 无日志断言 | #2427 D3「空结果 vs 失败」 | `staffUserInvisible` 打 WARN（含 role/tid/oid） | R4 覆盖行为；WARN 由实现评审确认 | 不可见分支必须可观测 |
| B7 | 列表与详情再次分裂（新增成员后详情仍 404） | 未来有人只改某一侧 | 判定源不唯一 | 无跨口径不变量用例 | 三视图 vs by-id 动作 | 单一 `personnelUserVisible` | R5 `PARITY_MISMATCH=0` | 不变量测试取代「两份实现各自正确」 |
| B8 | AI 直接改库回填存量数据 | 想「顺便清理」nil UUID | 把数据修复当代码修复 | 无授权闸门 | 任何 DB 写入 | 只出只读清单，回填需 Owner 授权 | 见 #2527 T3 | Decision Lock D8 落字 |
| B9 | 修了「详情」，同页「重置密码/改网点」仍是坏的一次性看不出来 | 只修主诉单点 | 同根因多入口，未做邻居检查 | 无行操作矩阵 | `ResetPassword` 批量、`performSiteTransfer` IAM 重绑 | 已在 #2527 显式登记（本单不含） | 本单 R7 覆盖激活；其余见 #2527 | 邻居扫描结论必须落 Issue（#2527） |
| B10 | 纯顾客被当员工打开（越权读个人信息） | 顾客有 `users.tenant_id = tid` 且无 membership | 旧口径把「同租户」当「员工」 | 无纯顾客负例 | 人员列表本身已排除顾客 | tenant 分支排除顾客角色（`lower(role) ∉ {'','user'}`），membership 优先 | R4 `MERCHANT_CUSTOMER=404` | 负例固定进合同；收窄登记进语义变更清单 |

### Workload

- Size: M
- Basis: 2 个后端实现文件（`personnel.go` 新增判定 + 字段、`user_staff.go` 三处动作 + 写库校验）+ 1 个后端测试文件 + 1 个验证脚本 + 2 个 PC 前端文件（列渲染 + 纯函数，1 个既有测试文件扩充）+ 2 份契约文档（api.md / ui.md）；无 migration、无新依赖
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/personnel.go`
- `backend/handlers/user_staff.go`
- `backend/handlers/personnel_detail_scope_2526_test.go`
- `frontend-pc/src/pages/StaffManagement.jsx`
- `frontend-pc/src/utils/personnelQuery.js`
- `frontend-pc/src/utils/personnelQuery.test.js`
- `scripts/verify-2526.sh`
- `docs/spec/api/api.md`
- `docs/spec/ui/ui.md`
- `docs/evidence/2526/plan.md`
- `docs/evidence/2526/evidence.json`
- `docs/evidence/2526/pre-ready-2526.json`
- `docs/evidence/2526/R1-detail-zero-tenant.txt`
- `docs/evidence/2526/R2-site-admin-detail.txt`
- `docs/evidence/2526/R3-system-admin-detail.txt`
- `docs/evidence/2526/R4-isolation-negative.txt`
- `docs/evidence/2526/R5-list-detail-parity.txt`
- `docs/evidence/2526/R6-update-persist.txt`
- `docs/evidence/2526/R7-activate-row-action.txt`
- `docs/evidence/2526/R8-attribution-flag.txt`
- `docs/evidence/2526/R9-pc-wiring.txt`
- `docs/evidence/2526/R10-regression.txt`

## Input Truth & Tool Preflight

- **Source of truth（口径唯一）**：
  - 可见性 = 人员列表三视图（`docs/spec/api/api.md` §10.18.0 + `backend/handlers/personnel.go:137-360`）：merchant 按 `sites.tenant_id`/`merchant_members.tenant_id`/`technician_profiles.tenant_id`；site 按 `sites.org_id`；system 按根组织 users ∪ 中转网点。
  - `users.tenant_id` 是本地缓存，不可用于过滤（#2249 S1：`backend/handlers/iam_proxy.go:617-625`；#2067：`personnel.go:142-145` 注释）。
  - ID 域：`sites.org_id`（组织 ID）≠ `sites.id`（站点主键），见 #2508 与 `docs/cases/cases.md:289`。
- **差异核对（实施前先跑，结果入 R1/R10 artifact）**：
  - `grep -n "tenant_id = ? AND deleted_at IS NULL\", userID, tenantID" backend/handlers/user_staff.go` → 修复后应为 0 处（详情/保存/激活）。
  - `grep -n "func personnelUserVisible" backend/handlers/personnel.go` → 恰好 1 个定义；`grep -rn "personnelUserVisible" backend/handlers` → 仅 `personnel.go` 内部 + 三个 by-id 动作经 `loadVisibleStaffUser` 间接调用。
  - `grep -n "RowsAffected" backend/handlers/user_staff.go` → 保存路径必须新增 1 处校验。
- **损坏输入拒绝**：`userID` 为空 → `40001`（保存/激活既有守卫）；非 UUID → 查询返回 not found → `404`；`userID` 存在但越权 → `404 40400`（不泄漏存在性）+ WARN；`site_id` 非法 → 沿用 `UpdateUser` 既有 `40001`。
- **工具盲区（显式声明）**：
  - 测试夹具依赖真实 Postgres（`testfixtures.SetupTestDB`）；`TUNELOOP_TEST_ALLOW_SKIP` **不得**设置，skip 视为 FAIL。
  - GORM 自动租户范围只对带 Schema 的模型生效；`site_members`/`sites` 走 `Table(...)`，靠显式 WHERE 隔离（本单不改隔离来源）。
  - PC 无组件测试运行器 → R9 用「纯函数 `node --test` + 源码结构断言」，真实渲染/截图不在本机能力内（D6，Owner 豁免）。
  - 本机 analyze 阶段不查预生产库：报告中的生产样本来自 Issue 正文（Owner 提供），代码侧断言基于夹具的确定行数。
- **影响面预检**：`loadVisibleStaffUser` 只被 `GetStaffUser`/`UpdateUser`/`ActivateUser` 调用；`ListStaff`（旧 `/staff` 列表）与三视图逻辑零改动；`personnelRow` 只新增字段（旧字段/顺序不变，PC 与导出不受影响）；`ResetPassword` 批量与 `performSiteTransfer`/IAM 角色同步**不改**（#2527）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 可见性 = **membership 口径 ∪（旧 tenant/org 口径 ∧ 非顾客角色 `lower(users.role) ∉ {'','user'}`）**；相对 Rev.1 加宽 2 类（org 不一致的本商户网点成员、零租户 membership 行）、收窄 1 类（纯顾客） | Architect | 本计划锁定（Owner 未反对；预生产核对 11 行纯顾客均不在列表内） | 若改为「仅 membership」→ 打破 #2375 白名单用例；若改为「无角色条件超集」→ R4 不可达且 B10 不修 |
| D2 | 零租户行「可见但未授权归属」不返回 40002「账号归属缺失」，而是正常 `20000`（数据按 membership 展示） | Architect | 本计划锁定（#2526 期望 1 给出两种选项，取第一种的 membership 版本） | 若 Owner 要求必须显式 40002 → 回 Architect（前端 lib/errorMessages 需同步） |
| D3 | 不可见与不存在统一返回 `404 40400`（不泄漏存在性），仅日志区分 | Architect | 本计划锁定 | 若要求 40303 → 需同步 #2504 文案口径 |
| D4 | 保存写库改 `ExplicitTenantScopeOnly` 并校验 `RowsAffected`；0 行 → `50000` | Architect | 本计划锁定 | 不允许「静默 0 行当成功」（红线：不静默吞错） |
| D5 | `attribution_missing` 判定 = `users.tenant_id` 或 `users.org_id` 为空/nil UUID；三视图同源 | Architect | 本计划锁定 | 若 Owner 要求更细分类（如区分 tenant 缺 / org 缺）→ 另立 Issue |
| D6 | R9 UI 证据形态：纯函数 `node --test` + 源码结构断言（PC 无组件测试运行器）；真实渲染截图归 Ops-Followup | Architect | Owner 豁免（沿用本仓前端豁免惯例，详见 #2480/#2483） | 若审计要求 render → 另立前端测试基建 Issue，本单不得以「构建通过」冒充 |
| D7 | 不重构三视图查询为共享 SQL 构造器；口径一致性靠 R5 不变量测试守护 | Architect | 本计划锁定 | 若要重构 → 需单独 Issue + #2065/#2427/#2508 全量回归预算 |
| D8 | 存量回填（DB 写入）不在本单：只出只读清单，回填需 Owner 授权 | Owner / ops | #2527 T3 承载 | 未授权 → 保持 `attribution_missing=true`，功能不受阻塞 |
| D9 | 本单不拆分子任务（后端谓词 + 1 个字段 + 1 处列渲染 + 文档属同一验收链） | Architect | 本计划锁定 | 拆分会造成「字段无消费方」死代码中间态，审计应判 FAIL |

## Out of Scope

- 批量重置密码（`ResetPassword`，`user_staff.go:1900-1915` 的 `WHERE id IN ? AND tenant_id = ?`）→ #2527 T1（同根因，需 fake-IAM 夹具，单独成单）。
- 归属缺失行的网点调动 / IAM 角色同步（`performSiteTransfer`、`UpdateUserRoleInOrgWithToken` 使用 nil-UUID `users.org_id`）→ #2527 T2；本单不改 IAM 调用参数。
- 存量数据回填（DB DDL/DML）与预生产只读清单执行 → #2527 T3 + Owner 授权；AI 不改库。
- 三视图查询重构为唯一 SQL 构造器（D7）。
- 详情响应里逐条 membership 的租户过滤（现状按 `sm.user_id` 取全部 membership，属既有行为，本单不放宽也不收紧）。
- 手机号/邮箱冲突检查的跨租户口径（现状 `users.tenant_id = tid`；nil 行参与冲突比较的口径另立 Issue，本单不改唯一性语义）。
- 错误文案本地化 / 404 文案改写（#2504 家族）。
- 预生产真机走查与截图（归 Ops-Followup，关闭本单时创建 `waitForConfirm` 跟踪 Issue）。

## Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` §10.18（人员管理） | 只定义列表三视图；`GET /admin/staff/:id`、`PUT /users/:id`、`POST /users/:id/activate` 的可见性口径**未落字** | 新增 `#### 10.18.6 员工详情/保存/激活的可见性口径`：写明「可见 = 列表 membership 口径 ∪（旧 tenant/org 口径 ∧ 非顾客角色 `lower(role) ∉ {'','user'}`）」、三种 businessRole 的判定表、纯顾客一律 `404`（B10）、不可见与不存在统一 `404 40400`、`attribution_missing` 字段说明、保存 `RowsAffected=0 → 50000` | 口径分裂之所以发生，是因为 by-id 动作没有契约；文档不写，下一次仍会各写一套 |
| `docs/spec/ui/ui.md` §人员管理三视图（`:2594-2602`） | 「所属网点」列仅写 `site_name || '-'` | 补一行：`attribution_missing=true` 的行在网点名后追加「归属缺失」标签（金色），说明为本地归属（tenant/org）缺失、数据按成员关系展示 | 列表侧新标记必须有设计落字，避免下次被当作误显示移除 |

## Dependencies

- 无

### 关联（非硬依赖）

- `#2527`：本单显式排除的同根因项（批量重置密码 / 网点调动与 IAM 角色同步 / 存量回填），已建单。
- `#2495`：其验收对象（员工 + 维修师傅双角色账号 `linwx1978@163.com`）被本缺陷阻塞，本单修复后由该单继续。
- `#2508`：同页 ID 域修复；本单 site_admin 判定复用「`sites.org_id` 是组织域」结论。
- `#2067` / `#2249 S1`：`users.tenant_id` 不可靠的两处先例，本单把口径对称到 by-id 动作。
- 部署侧：预生产 `qqlin` 打开该员工「编辑」应正常加载；含 `attribution_missing` 标签的列表截图由关闭时的 `[Ops-Followup of #2526]`（仅 `waitForConfirm`）回评。

*Model: DeepSeek V4.1 Flash 1017*


