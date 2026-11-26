## Accepted Plan — #2546 商户所有人（唯一 OWNER）+ 转让所有权（原 OWNER 降 ADMIN）｜Rev.3

> Tags: enhancement, code:go, api-contract, scope:pc, code:js
> 关联依据：beaconiam#508（`4a0cb5c`，预生产包 `beaconiam_20261126-063053_61f8a8f.zip`）已上线转让端点与单一 OWNER 不变量；用例/契约由 #2544 已修订（A1/A2）
> 上游输入：Round 1 Audit（评论 `2026-11-26T10:16:54Z`，REJECT：Bug 1 门控真值 / Bug 2 PUT 组合字段 / Bug 3 半同步静默 + Low-1 / Low-2）与 Investigation Packet（评论 `2026-11-26T10:18:54Z`）的 `plan_delta` 六项

### 修订记录（Rev.2 → Rev.3：修 Input Truth 的门控真值 + 补 actor 入口 + 补 R12–R16 验收）

| # | 修订项 | Rev.2 | Rev.3 | 依据 |
|---|---|---|---|---|
| 1 | 转让门控信号 | `localStorage.user_info.isOwner`（= JWT `is_owner` = beaconiam `users.is_owner`，平台/命名空间级遗留标记） | 后端 `viewer_is_owner`（`merchants.admin_uid == 当前登录 user_id`，与 D1 本地权威同源）+ 前端纯函数 `canTransferOwnership` | beaconiam `internal/auth/handler.go:632-646`（`is_owner ← user.IsOwner`；`role ← orRole(ctx.Role, ...)` 取自 `user_org_relations`）、`internal/handler/org_handler.go` 转让只改关系表；dev IAM `Tuneloop测试服1` 7 个 active OWNER 中 6 个 `users.is_owner=false` |
| 2 | actor 与入口路径（审计 D13 缺口） | 未覆盖：`/merchants` 菜单与路由需 `sys_perm` bit 6/5，商户管理员模板只有 bit 10-29 → 商户 OWNER 进不来 | **D13 = 方案 (a)**：本商户 `merchant_admin` 可读 4 条路由（列表/详情/成员列表/转让），全部收窄到本商户；平台专属写路由保持 `sys_perm` 门控；前端菜单与路由加 `businessRoles`，平台专属操作按 `businessRole` 隐藏 | `ListMerchants`（`merchant.go:28-45`）与 `GetMerchant`（`merchant.go:133-160`）**已有** `BusinessRoleMerchantAdmin` 收窄分支（#2274/#2276）、`loadMerchantForAccess`（`merchant_member.go:620-646`）同口径；`docs/spec/permissions.md:275` 与 `docs/spec/ui/ui.md:3172` 已声明「商户管理 — 商户管理员可见」；Issue 正文「现状」表指向该 Tab；`docs/cases/organization.md` O-03 `role: [tenant_admin]` |
| 3 | `PUT /merchants/:id` 组合字段 | 转让成功后 `First(&merchant)` 重载覆盖内存字段 + `Save` 写回旧值 → 同请求的 `name/phone/...` 静默丢失 | 字段更新与所有权转让**分离且有序**：先 `Updates` 普通字段，再转让；禁止「重载后 Save」；新增 R13 组合 fixture | `backend/handlers/merchant.go:551-573`（内存赋值）与 `:601-612`（重载 + Save）；`docs/spec/api/api.md` §12.3.3 示例即组合形态 |
| 4 | 角色提升半同步 | `promoteErr != nil` 仅 `log.Printf` 后仍 `return nil` → 200，`admin_uid` 已迁移而目标行非 `merchant_admin` | 同一 `db.Transaction`：`admin_uid/admin_pending` 与目标 `merchant_members.role` 同时成功；任一 `RowsAffected != 1` 或报错 → 回滚 + `500/50000` 显式文案（标注 IAM 已转让需人工核对）；新增 R14 失败分支 | `backend/handlers/merchant_member.go` 两条独立语句 + log-only；AGENTS 红线禁止静默吞错 |
| 5 | R8 口径升级 | `TRANSFER_UI_WIRED=1`（源码出现字符串） | 删除该弱断言，改为 `TRANSFER_BUTTON_USES_VIEWER_IS_OWNER=1`，门控真值由 **R12** 承载；新增 R15（后端 actor 授权矩阵）/ R16（商户管理员入口接线）承载 D13 | 审计「R8 由『源码出现字符串』升级为『门控真值 + 纯函数断言』」 |
| 6 | Low-1 / Low-2 | `api.md` 写「sys_perm bit 5（tenant:update）」；候选 `page_size: 200` 超后端上限 100 被截断 | `api.md` 改 bit 8（`tenant:update`）；候选改 `page_size=100` 分页补拉至 `total` | `backend/middleware/permissions.go:29-32`（`SysPermTenantView=5`、`SysPermTenantUpdate=8`）；`backend/handlers/pagination_helpers.go` `maxPageSize=100` |

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 「负责人管理」Tab：列表只取 `role='merchant_admin'` 的成员 | `backend/handlers/merchant_member.go:51`、`:65`；`frontend-pc/src/pages/MerchantManagement.jsx:280` |
| 「最后负责人保护」允许多个负责人，仅拦截删到 0 个（`40009`） | `backend/handlers/merchant_member.go:536-546` |
| 添加成员把业务角色 `merchant_admin` 用 `toIAMRole` 映射为 IAM `OWNER` | `backend/handlers/merchant_member.go:316`、`:438`；`backend/handlers/permission_manage.go:240-255` |
| 改成员角色时 `merchant_admin` 分支同样 bind IAM `OWNER` | `backend/handlers/merchant_member.go:445-456` |
| 编辑商户（`PUT /merchants/:id`）改 `admin_uid` = bind 新 `OWNER` → demote 旧为 `USER`，失败有补偿解绑 | `backend/handlers/merchant.go:576-660` |
| 本地所有权指针已存在：`merchants.admin_uid`（创建时写创建者，列表/详情已返回） | `backend/models/models.go:1053`；`backend/handlers/merchant.go:107`、`:384` |
| 业务角色判定来自 JWT claims（`tid == oid` → merchant_admin），不读本地 `users.role` | `backend/middleware/iam.go:645-648` |
| **商户管理员已具备数据面收窄**：列表按 `org_id`（`#2274/#2276`），详情按 `org_id`，成员/转让经 `loadMerchantForAccess` | `backend/handlers/merchant.go:33-43`、`:140-150`；`backend/handlers/merchant_member.go:620-646` |
| **但路由门控挡住宿主**：`GET /merchants`、`GET /merchants/:id`、`GET|POST /admin/merchants/:id/members`、转让端点均 `RequireSysPerm(...)`；商户管理员模板 `sys_perm=1073740800`（bit 10-29），不含 bit 5-9 | `backend/main.go:493-501`；`backend/middleware/permissions.go:41-56`；dev IAM `functional_role_templates`（merchant_admin = 1073740800） |
| 文档已声明商户管理员可见「商户管理」，与路由门控矛盾 | `docs/spec/permissions.md:275`；`docs/spec/ui/ui.md:3172` |
| 转让按钮当前门控 `localStorage.user_info.isOwner` | `frontend-pc/src/components/MerchantMemberManagement.jsx:61-68`、`:423` |
| IAM 侧契约（beaconiam#508）：`POST /api/v1/organizations/:id/transfer-ownership {user_id}`；仅当前 OWNER（或平台超管恢复）可调；403/409/400/404 语义见下表；成功后**同一事务**旧 OWNER→ADMIN、目标→OWNER、写 `TRANSFER_OWNERSHIP` 审计、bump perm_version | beaconiam `internal/handler/org_handler.go:1568-1671`（`4a0cb5c`） |

**根因（一句话）**：本地把「负责人」建模成**可多个的业务角色 `merchant_admin`** 并直接当 IAM `OWNER` 写入；IAM 已改为**单 OWNER + 只能转让**，于是本地存在三条互不相让的所有权写入路径（添加成员 / 改角色 / 编辑商户），且**没有任何一条是转让语义**；同时前端把「本商户 OWNER」错当平台级 `users.is_owner`，导致真实 OWNER 无入口。

### 方案（变更点）

1. **本地所有权权威 = `merchants.admin_uid`（已有列，无迁移）**
   - 纯函数 `merchantOwnerUID(m models.Merchant) string`：`admin_uid` 为空或 nil UUID 时返回 `""`。
   - `merchant_members.role` 保持业务角色语义（OWNER 与 ADMIN 均为 `merchant_admin`）；「原 OWNER 降 ADMIN」= `admin_uid` 指针迁移，不改 role。
2. **IAM 客户端（唯一转让出口）**：`TransferOwnershipWithToken(token, orgID, targetUserID)` → `POST /api/v1/organizations/{orgID}/transfer-ownership {"user_id": …}`；类型化 `IAMTransferOwnershipError{StatusCode, Message}`；4xx 不得当成功，不得只 log 后 200；不调 `IncrementPermVersion()`（IAM 已 bump）。
3. **handler `TransferOwnership`（`POST /api/merchants/:id/transfer-ownership`）**
   - 路由：`authRequired` + `ExplicitTenantScope()` + `RequireSysPermOrRole(SysPermTenantUpdate, BusinessRoleMerchantAdmin)`（见 §8）。
   - 流程：`loadMerchantForAccess` → `user_id` 缺失/非 UUID → `40001` → 目标须为本商户 active `merchant_members` 行（否则 `40400`，不调 IAM）→ 目标 ≠ 当前 owner（否则 `40002`）→ 目标 `iam_sub` 空 → `40002` → IAM 转让 → **单事务**本地同步（见 §6）。
   - 错误映射（D5）：403→`403/40303`；404→`404/40400`；409→`409/40900`；400→`400/40002`（透传 IAM `error`）；其他/5xx/网络→`502/50200`。
   - 响应：`{code:20000, data:{merchant_id, previous_owner_id, new_owner_id}}`。
4. **收敛其余所有权写入路径**：`merchantMemberIAMRole(businessRole)`（`merchant_admin → "ADMIN"`，其余委托 `toIAMRole`；`toIAMRole` 本身不动）；`AddMember`/`UpdateMemberRole` 收到 `owner/OWNER` → `40002`；`UpdateMemberRole` 目标是当前 owner → `40002`；`RemoveMember` 目标是当前 owner → `403/40303`（排在既有 `40009` 兜底之前，不调 IAM）；`PUT /merchants/:id` 的 `admin_uid` 变化复用 `transferMerchantOwnership`，置空 → `40002`。
5. **列表契约**：`ListMembers` 每行 `is_owner`、`status`；顶层 `owner_uid`；新增 `include=all`（默认仍只 `role='merchant_admin'`）；**新增顶层 `viewer_is_owner` / `viewer_uid`**（`merchant.admin_uid == middleware.GetUserID(ctx)`）。
6. **本地转让同步 = 单事务（R14）**
   - `db.Transaction`：① `merchants` 置 `admin_uid=目标`、`admin_pending=false`（`RowsAffected == 1`）；② 目标 `merchant_members.role='merchant_admin'`（`RowsAffected == 1`）；③ 任一失败/行数不符 → **回滚** 并返回 `500/50000`「IAM 已转让，但本地同步失败，请联系平台人工核对（旧/新所有人：…）」；禁止 log-only + 200。
7. **PC 前端（`MerchantMemberManagement.jsx`）**
   - Tab 改名「所有人」；顶部唯一 OWNER 卡片（读 `is_owner` 行）；无 owner（`owner_uid` 空）→ 提示「未锚定所有人（请联系平台核对）」并禁用转让。
   - **转让门控 = 后端 `viewer_is_owner`**（删除 `localStorage.user_info.isOwner`），由纯函数 `canTransferOwnership({viewerIsOwner, ownerUid, viewerUid, candidates})` 判定；`Modal` 内候选 `Select` + 固定文案「转让后你将成为管理员」+ `Modal.confirm` 二次确认 → `POST /merchants/:id/transfer-ownership` → 成功刷新；失败原样展示后端 `message`。
   - 候选来源：`include=all&page_size=100` **分页补拉**到 `total`（Low-2），排除当前 owner；owner 行不渲染移除入口；添加成员不再有「设为负责人」。
   - 纯函数集中于 `frontend-pc/src/utils/merchantOwnership.js`（`pickOwnerRow`/`transferCandidates`/`canRemoveMember`/`canTransferOwnership`/`transferConfirmText`）。
8. **actor 与入口（D13 = 方案 (a)）**
   - 后端新增中间件 `RequireSysPermOrRole(bit int, roles ...string)`：`sysPerm == 0` 保持既有放行语义不变；`sysPerm` 命中 bit → 放行；`GetBusinessRole(ctx)` ∈ roles → 放行；否则 `403/40303`。
   - **仅 4 条路由**改用该中间件并追加 `BusinessRoleMerchantAdmin`：`GET /merchants`、`GET /merchants/:id`、`GET /admin/merchants/:id/members`、`POST /admin/merchants/:id/transfer-ownership`。**其余保持不变**（`POST/PUT/DELETE /merchants`、`POST/PUT/DELETE /admin/merchants/:id/members`、invites 仍 `RequireSysPerm`）→ 平台专属写操作对商户管理员仍是 `40303`。
   - 数据面收窄沿用既有分支（`ListMerchants`/`GetMerchant` 按 `org_id`；成员/转让经 `loadMerchantForAccess`），**不新增第二套 scoping**。
   - 前端：`checkPermission` 增加可选 `perm.businessRoles`（命中 `userInfo.businessRole` → 通过，与 sys/cus 为 OR；调用方 `App.jsx` 与 `ProtectedRoute` 同批传 `userInfo`）；`/merchants` 菜单与 `/merchants/:id` 路由追加 `businessRoles: ['merchant_admin']`；`/merchants/new` **不追加**（平台专属）。
   - `MerchantManagement.jsx`：`businessRole === 'merchant_admin'` 时隐藏「创建商户」「删除」「编辑基本信息」「分账配置」与列表编辑入口，详情默认 Tab = 所有人（只读信息 + 转让）。平台管理员视图不变。

### 语义变更（必须翻转的既有断言，强制清单）

| 文件:行 | 旧断言 | 新断言 | 理由 |
|---|---|---|---|
| `backend/handlers/member_role_iam_fail_2461_test.go:304-331`（`TestUpdateMerchantAdminDemoteFailureCompensates_2461`） | demote `/role` 500 → `502` + 补偿解绑 `IAM_UNBIND_CALLS=1` + 本地 `admin_uid` 不变 | **200** + IAM 只调 `transfer-ownership`（`IAM_TRANSFER_CALLS=1`、`IAM_UNBIND_CALLS=0`）+ 本地 `admin_uid` 已切换 | 所有权变更改走 IAM 单事务转让；补偿解绑代码已删除 |
| `backend/handlers/member_role_iam_fail_2461_test.go:332-354`（`TestUpdateMerchantAdminBindFailureVisible_2461`） | bind 500 → `200` + `admin_pending=true` + `admin_sync_warning` | transfer 500 → `502/50200`，`admin_uid` 与 `admin_pending` 均不变 | 转让是全成败，不允许「本地先改、IAM 待确认」半态 |
| R8 旧断言 `TRANSFER_UI_WIRED=1` | 源码出现 `transfer-ownership` 字符串即通过 | `TRANSFER_BUTTON_USES_VIEWER_IS_OWNER=1` + `CANDIDATE_PAGE_SIZE=100`；门控真值由 R12 断言 | 字符串存在 ≠ 门控正确（审计 Bug 1 根因） |
| R6 旧 fixture 仅 `{admin_uid}` | 组合字段未覆盖 | R6 保留单字段语义；组合请求由 **R13** 独立断言三者同时落库 | 一行一套语义，避免证据无法表达 |
| `frontend-pc/src/components/MerchantMemberManagement.jsx` 旧门控 `user_info.isOwner` | 平台级 `is_owner` 当商户 OWNER | 后端 `viewer_is_owner` | 对端语义核对结论（见修订记录 #1） |

**生产行为变更登记（非测试断言，必须随单说明）**

| 行为 | 修改前 | 修改后 | 依据 |
|---|---|---|---|
| 添加成员 `role=merchant_admin` | IAM bind `OWNER`（已有 OWNER 时 IAM 400 ⇒ 添加失败） | IAM bind `ADMIN` | 期望 2「添加管理员 = ADMIN」 |
| `PUT /merchants/:id` 改 `admin_uid` | bind 新 `OWNER` + 旧降 `USER`（平台可代改） | 走 IAM 转让（仅当前 OWNER，否则 `40303`） | IAM#508 授权规则 |
| `PUT /merchants/:id` 置空 `admin_uid` | 允许清空 | `400/40002` 拒绝 | OWNER 不能删除，只能转让 |
| 移除当前 owner | 多个负责人时可移除 | `403/40303` | 期望 3 |
| 商户管理员访问 `GET /merchants`、`GET /merchants/:id`、`GET /admin/merchants/:id/members`、`POST /merchants/:id/transfer-ownership` | `40303`（路由 sys_perm 拦截） | **200**，数据面收窄到本商户（`org_id`/`loadMerchantForAccess`） | D13=(a)：Issue 期望 2 + O-03 + 文档已声明 |
| 商户管理员访问平台专属写路由（创建/删除商户、编辑商户、成员增删改） | `40303` | 仍是 `40303`（不改） | 越权面不得扩大 |
| `GET /admin/merchants/:id/members` 响应 | 每行无 owner 标记 | 新增 `is_owner`/`status`/`owner_uid`/`viewer_is_owner`/`viewer_uid` | 前端所有人卡片与门控数据源 |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志） |
|---|---|---|---|---|---|---|
| R1 转让成功（IAM 契约 + 本地同步） | `backend/handlers/merchant_member.go`、`backend/services/iam_client.go` | `POST /merchants/:id/transfer-ownership` | fake IAM 200；商户有 owner A + 管理员 B | backend（测试库 + fake IAM） | `TRANSFER_STATUS=200`、`IAM_METHOD=POST`、`IAM_PATH_SUFFIX=/transfer-ownership`、`IAM_BODY_USER_ID=1`、`LOCAL_OWNER_UPDATED=1`、`OLD_OWNER_LOCAL_ROLE=merchant_admin`、`TARGET_LOCAL_ROLE=merchant_admin`、`ADMIN_PENDING_CLEARED=1`、`OWNER_FIELD_PRESENT=1` | R1 |
| R2 IAM 失败映射（403/404/409/400/5xx）+ 本地不变 | `backend/handlers/merchant_member.go`、`backend/services/iam_client.go` | 同上 | fake IAM 依次返回 403 / 404 / 409 / 400 / 500 | backend | `IAM403_STATUS=403`、`IAM403_CODE=40303`、`IAM404_STATUS=404`、`IAM404_CODE=40400`、`IAM409_STATUS=409`、`IAM409_CODE=40900`、`IAM400_CODE=40002`、`IAM500_STATUS=502`、`IAM500_CODE=50200`、`LOCAL_OWNER_UNCHANGED=1` | R2 |
| R3 前置拒绝（不调 IAM） | `backend/handlers/merchant_member.go` | 同上 | 目标非本商户成员 / 目标已是 owner / `user_id` 空 | backend | `NONMEMBER_STATUS=404`、`NONMEMBER_CODE=40400`、`NONMEMBER_IAM_CALLS=0`、`SAME_OWNER_STATUS=400`、`SAME_OWNER_CODE=40002`、`SAME_OWNER_IAM_CALLS=0`、`EMPTY_UID_STATUS=400`、`EMPTY_UID_CODE=40001` | R3 |
| R4 添加/改角色只 bind ADMIN，OWNER 请求被拒 | `backend/handlers/merchant_member.go` | `POST /admin/merchants/:id/members`、`PUT /admin/merchants/:id/members/:uid` | `role=merchant_admin`；另测 `role=OWNER` | backend | `ADD_BIND_ROLE=ADMIN`、`UPDATE_BIND_ROLE=ADMIN`、`OWNER_BINDS=0`、`OWNER_REQUEST_STATUS=400`、`OWNER_REQUEST_CODE=40002` | R4 |
| R5 OWNER 移除/降级保护 + 非 owner 管理员回归 | `backend/handlers/merchant_member.go` | `DELETE /admin/merchants/:id/members/:uid`、`PUT /admin/merchants/:id/members/:uid` | 目标是 owner / 目标是普通管理员 | backend | `REMOVE_OWNER_STATUS=403`、`REMOVE_OWNER_CODE=40303`、`REMOVE_OWNER_IAM_CALLS=0`、`OWNER_ROW_KEPT=1`、`DEMOTE_OWNER_STATUS=400`、`DEMOTE_OWNER_CODE=40002`、`REMOVE_ADMIN_STATUS=200` | R5 |
| R6 编辑商户改所有人走转让（翻转 #2461 既有断言） | `backend/handlers/merchant.go` | `PUT /merchants/:id` | 请求含 `admin_uid` 单字段变化；另一请求置空 | backend | `UPD_ADMIN_STATUS=200`、`UPD_ADMIN_IAM_PATH_SUFFIX=/transfer-ownership`、`UPD_ADMIN_LOCAL=1`、`UPD_ADMIN_IS_OWNER_FLIP=1`、`CLEAR_OWNER_STATUS=400`、`CLEAR_OWNER_CODE=40002` | R6 |
| R7 列表 owner 标记 + 候选（`include=all`） | `backend/handlers/merchant_member.go` | `GET /admin/merchants/:id/members` | 默认 / `include=all` 两种；成员含 owner、管理员、直属员工 | backend | `OWNER_FLAG_HITS=1`、`OWNER_UID_MATCH=1`、`DEFAULT_ROWS=2`、`ALL_ROWS=3`、`OWNER_FIELD_PRESENT=1` | R7 |
| R8 所有人 Tab 接入（纯函数 + 源码结构断言） | `frontend-pc/src/components/MerchantMemberManagement.jsx`、`frontend-pc/src/pages/MerchantManagement.jsx`、`frontend-pc/src/utils/merchantOwnership.js` | `node --test frontend-pc/src/utils/merchantOwnership.test.js` + 结构断言 + eslint + build | owner 行禁用移除 / 候选排除 owner / 候选分页 | PC | `NODE_TEST_RC=0`、`TAB_RENAMED=1`、`TRANSFER_BUTTON_USES_VIEWER_IS_OWNER=1`、`OWNER_REMOVE_DISABLED=1`、`CANDIDATE_PAGE_SIZE=100`、`FRONTEND_LINT_RC=0`、`FRONTEND_BUILD_RC=0` | R8 |
| R9 审计路由接线 | `backend/middleware/audit_logger.go` | `auditRouteMap` 单元断言 | 路由键存在且字段完整 | backend | `AUDIT_ROUTE_FOUND=1`、`AUDIT_ACTION=TRANSFER_OWNERSHIP`、`AUDIT_RESOURCE=merchant`、`AUDIT_PRIORITY=CRITICAL` | R9 |
| R10 构建 + 既有成员/商户用例回归 | `backend/handlers/merchant_member.go`、`backend/handlers/merchant.go` | `go build ./...` + 定向回归 | 修后工作树 | backend | `BUILD_RC=0`、`REGRESSION_RC=0`、`REGRESSION_PASS=1` | R10 |
| R11 文档同步落字（含位数与权限口径） | `docs/spec/api/api.md`、`docs/spec/ui/ui.md`、`docs/spec/database/database.md`、`docs/cases/organization.md`、`docs/spec/permissions.md` | grep 结构断言 | — | 文档 | `DOC_API_TRANSFER=1`、`DOC_API_MEMBERS_OWNER=1`、`DOC_API_BIT8=1`、`DOC_UI_OWNER_TAB=1`、`DOC_DB_ADMIN_UID=1`、`DOC_CASE_PATH_ALIGNED=1`、`DOC_PERM_MERCHANTS_SCOPED=1` | R11 |
| R12 转让门控真值（后端 `viewer_is_owner` + 纯函数） | `backend/handlers/merchant_member.go`、`frontend-pc/src/utils/merchantOwnership.js`、`frontend-pc/src/components/MerchantMemberManagement.jsx` | `GET /admin/merchants/:id/members` + `node --test` | 当前登录者为 owner / 原 owner 已降 ADMIN / 未锚定 owner / 无候选 | backend + PC | `GATE_FIELD=viewer_is_owner`、`BE_VIEWER_IS_OWNER_TRUE=1`、`BE_VIEWER_IS_OWNER_FALSE=1`、`GATE_OWNER_TRUE=1`、`GATE_DEMOTED_FALSE=1`、`GATE_UNANCHORED_FALSE=1`、`GATE_NO_CANDIDATE_FALSE=1`、`GATE_USERINFO_OWNER_USAGE=0`、`NODE_TEST_RC=0` | R12 |
| R13 `PUT /merchants/:id` 组合字段不丢失 | `backend/handlers/merchant.go` | `PUT /merchants/:id` | 请求 `{"name","phone","admin_uid"}` 三者同时提交 | backend | `COMBINED_STATUS=200`、`NAME_PERSISTED=1`、`PHONE_PERSISTED=1`、`ADMIN_UID_SWITCHED=1` | R13 |
| R14 角色提升失败不半同步 | `backend/handlers/merchant_member.go` | `POST /merchants/:id/transfer-ownership` | 注入 `merchant_members` 更新失败（GORM callback） | backend | `PROMOTE_FAIL_STATUS=500`、`PROMOTE_FAIL_CODE=50000`、`ADMIN_UID_UNCHANGED=1`、`ROLE_UNCHANGED=1`、`MSG_HAS_MANUAL_CHECK=1` | R14 |
| R15 actor 授权矩阵（D13） | `backend/middleware/permissions.go`、`backend/main.go`、`backend/handlers/merchant.go`、`backend/handlers/merchant_member.go` | 4 条放开路由 + 平台专属写路由 | JWT: `businessRole=merchant_admin`、`sys_perm` 仅 bit 10-29 | backend | `MA_LIST_STATUS=200`、`MA_LIST_ROWS=1`、`MA_GET_OWN_STATUS=200`、`MA_GET_OTHER_STATUS=404`、`MA_MEMBERS_STATUS=200`、`MA_TRANSFER_STATUS=200`、`MA_CREATE_MERCHANT_STATUS=403`、`MA_UPDATE_MERCHANT_STATUS=403`、`MA_DELETE_MERCHANT_STATUS=403`、`MA_ADD_MEMBER_STATUS=403` | R15 |
| R16 商户管理员入口接线（菜单/路由/平台操作隐藏） | `frontend-pc/src/config/menuPermissions.js`、`frontend-pc/src/App.jsx`、`frontend-pc/src/pages/MerchantManagement.jsx` | `node --test frontend-pc/src/config/menuPermissions.test.js` + 结构断言 + eslint | `businessRole=merchant_admin` / 平台管理员 | PC | `CHECKPERM_MA=1`、`CHECKPERM_PLATFORM=1`、`MENU_BUSINESS_ROLES=1`、`ROUTE_BUSINESS_ROLES=1`、`NEW_ROUTE_PLATFORM_ONLY=1`、`PLATFORM_ACTIONS_HIDDEN=1`、`FRONTEND_LINT_RC=0` | R16 |

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 商户 M（org=O，`admin_uid=A`）+ `merchant_members`(A, B) + `users`(A,B 有 `iam_sub`) | `bash scripts/verify-2546.sh r1` | `docs/evidence/2546/R1-transfer-success.txt` | `TRANSFER_STATUS=200`、`IAM_METHOD=POST`、`IAM_PATH_SUFFIX=/transfer-ownership`、`IAM_BODY_USER_ID=1`、`LOCAL_OWNER_UPDATED=1`、`OLD_OWNER_LOCAL_ROLE=merchant_admin`、`TARGET_LOCAL_ROLE=merchant_admin`、`ADMIN_PENDING_CLEARED=1`、`OWNER_FIELD_PRESENT=1` | `IAM_PATH_SUFFIX` 非 `/transfer-ownership`（走了 bind）或 `LOCAL_OWNER_UPDATED=0` → FAIL |
| R2 | 同 R1，fake IAM 状态码可控 | `bash scripts/verify-2546.sh r2` | `docs/evidence/2546/R2-iam-errors.txt` | `IAM403_STATUS=403`、`IAM403_CODE=40303`、`IAM404_STATUS=404`、`IAM404_CODE=40400`、`IAM409_STATUS=409`、`IAM409_CODE=40900`、`IAM400_CODE=40002`、`IAM500_STATUS=502`、`IAM500_CODE=50200`、`LOCAL_OWNER_UNCHANGED=1` | 任一错误被吞成 `200`，或 `LOCAL_OWNER_UNCHANGED=0` → FAIL |
| R3 | 同 R1；额外用户 C 无 `merchant_members` 行 | `bash scripts/verify-2546.sh r3` | `docs/evidence/2546/R3-preflight-negatives.txt` | `NONMEMBER_STATUS=404`、`NONMEMBER_CODE=40400`、`NONMEMBER_IAM_CALLS=0`、`SAME_OWNER_STATUS=400`、`SAME_OWNER_CODE=40002`、`SAME_OWNER_IAM_CALLS=0`、`EMPTY_UID_STATUS=400`、`EMPTY_UID_CODE=40001` | `NONMEMBER_IAM_CALLS=1` 或 `SAME_OWNER_STATUS=200` → FAIL |
| R4 | 同 R1，目标用户 D 未入商户 | `bash scripts/verify-2546.sh r4` | `docs/evidence/2546/R4-admin-bind.txt` | `ADD_BIND_ROLE=ADMIN`、`UPDATE_BIND_ROLE=ADMIN`、`OWNER_BINDS=0`、`OWNER_REQUEST_STATUS=400`、`OWNER_REQUEST_CODE=40002` | `ADD_BIND_ROLE=OWNER` 或 `OWNER_BINDS=1` → FAIL |
| R5 | 同 R1（A=owner，B=普通管理员） | `bash scripts/verify-2546.sh r5` | `docs/evidence/2546/R5-owner-protection.txt` | `REMOVE_OWNER_STATUS=403`、`REMOVE_OWNER_CODE=40303`、`REMOVE_OWNER_IAM_CALLS=0`、`OWNER_ROW_KEPT=1`、`DEMOTE_OWNER_STATUS=400`、`DEMOTE_OWNER_CODE=40002`、`REMOVE_ADMIN_STATUS=200` | `REMOVE_OWNER_STATUS=200` 或 `OWNER_ROW_KEPT=0` → FAIL |
| R6 | 同 R1，`PUT` 传 `admin_uid=B`；另测 nil UUID | `bash scripts/verify-2546.sh r6` | `docs/evidence/2546/R6-update-merchant-transfer.txt` | `UPD_ADMIN_STATUS=200`、`UPD_ADMIN_IAM_PATH_SUFFIX=/transfer-ownership`、`UPD_ADMIN_LOCAL=1`、`UPD_ADMIN_IS_OWNER_FLIP=1`、`CLEAR_OWNER_STATUS=400`、`CLEAR_OWNER_CODE=40002` | `UPD_ADMIN_IAM_PATH_SUFFIX` 非转让端，或 `CLEAR_OWNER_STATUS=200` → FAIL |
| R7 | 同 R1 + 直属员工 E（`role=merchant_staff`） | `bash scripts/verify-2546.sh r7` | `docs/evidence/2546/R7-list-owner-flag.txt` | `OWNER_FLAG_HITS=1`、`OWNER_UID_MATCH=1`、`DEFAULT_ROWS=2`、`ALL_ROWS=3`、`OWNER_FIELD_PRESENT=1` | `OWNER_FLAG_HITS` ≠ 1，或 `DEFAULT_ROWS` ≠ 2 → FAIL |
| R8 | `merchantOwnership.test.js` 四类 fixture + 源码结构断言 | `bash scripts/verify-2546.sh r8` | `docs/evidence/2546/R8-pc-owner-tab.txt` | `NODE_TEST_RC=0`、`TAB_RENAMED=1`、`TRANSFER_BUTTON_USES_VIEWER_IS_OWNER=1`、`OWNER_REMOVE_DISABLED=1`、`CANDIDATE_PAGE_SIZE=100`、`FRONTEND_LINT_RC=0`、`FRONTEND_BUILD_RC=0` | `TAB_RENAMED=0`，或 `CANDIDATE_PAGE_SIZE` >100（后端截断仍截断）→ FAIL |
| R9 | `auditRouteMap` 静态表 | `bash scripts/verify-2546.sh r9` | `docs/evidence/2546/R9-audit-route.txt` | `AUDIT_ROUTE_FOUND=1`、`AUDIT_ACTION=TRANSFER_OWNERSHIP`、`AUDIT_RESOURCE=merchant`、`AUDIT_PRIORITY=CRITICAL` | 路由键缺失或 action 不符 → FAIL |
| R10 | 修后工作树 + 既有用例 | `bash scripts/verify-2546.sh r10` | `docs/evidence/2546/R10-regression.txt` | `BUILD_RC=0`、`REGRESSION_RC=0`、`REGRESSION_PASS=1` | `BUILD_RC` ≠ 0 或回归 FAIL → FAIL |
| R11 | 五份契约文档修后内容 | `bash scripts/verify-2546.sh r11` | `docs/evidence/2546/R11-docs.txt` | `DOC_API_TRANSFER=1`、`DOC_API_MEMBERS_OWNER=1`、`DOC_API_BIT8=1`、`DOC_UI_OWNER_TAB=1`、`DOC_DB_ADMIN_UID=1`、`DOC_CASE_PATH_ALIGNED=1`、`DOC_PERM_MERCHANTS_SCOPED=1` | 任一为 `0` → FAIL |
| R12 | 三组：A 是本地 owner / A 已被降级 / `owner_uid` 为空；`canTransferOwnership` 四类入参 | `bash scripts/verify-2546.sh r12` | `docs/evidence/2546/R12-owner-gate.txt` | `GATE_FIELD=viewer_is_owner`、`BE_VIEWER_IS_OWNER_TRUE=1`、`BE_VIEWER_IS_OWNER_FALSE=1`、`GATE_OWNER_TRUE=1`、`GATE_DEMOTED_FALSE=1`、`GATE_UNANCHORED_FALSE=1`、`GATE_NO_CANDIDATE_FALSE=1`、`GATE_USERINFO_OWNER_USAGE=0`、`NODE_TEST_RC=0` | `GATE_FIELD` 为 `user_info.isOwner`，或 `GATE_DEMOTED_FALSE=0`（被降级者仍可转让）→ FAIL |
| R13 | 同 R1；`PUT` body 同时含 `name=N2`、`phone`、`admin_uid=B` | `bash scripts/verify-2546.sh r13` | `docs/evidence/2546/R13-put-combined-fields.txt` | `COMBINED_STATUS=200`、`NAME_PERSISTED=1`、`PHONE_PERSISTED=1`、`ADMIN_UID_SWITCHED=1` | 任一 `PERSISTED=0`（组合请求静默丢字段）→ FAIL |
| R14 | 同 R1；GORM callback 令 `merchant_members` UPDATE 失败 | `bash scripts/verify-2546.sh r14` | `docs/evidence/2546/R14-promote-failure.txt` | `PROMOTE_FAIL_STATUS=500`、`PROMOTE_FAIL_CODE=50000`、`ADMIN_UID_UNCHANGED=1`、`ROLE_UNCHANGED=1`、`MSG_HAS_MANUAL_CHECK=1` | `PROMOTE_FAIL_STATUS=200`，或 `ADMIN_UID` 已变而 role 未变（半同步）→ FAIL |
| R15 | 两个商户 M1（呼叫者为其 OWNER）/ M2；JWT businessRole=merchant_admin、sys_perm=1073740800 | `bash scripts/verify-2546.sh r15` | `docs/evidence/2546/R15-actor-authz.txt` | `MA_LIST_STATUS=200`、`MA_LIST_ROWS=1`、`MA_GET_OWN_STATUS=200`、`MA_GET_OTHER_STATUS=404`、`MA_MEMBERS_STATUS=200`、`MA_TRANSFER_STATUS=200`、`MA_CREATE_MERCHANT_STATUS=403`、`MA_UPDATE_MERCHANT_STATUS=403`、`MA_DELETE_MERCHANT_STATUS=403`、`MA_ADD_MEMBER_STATUS=403` | `MA_GET_OTHER_STATUS=200`（跨商户可见），或平台专属写路由对商户管理员放行 → FAIL |
| R16 | `checkPermission` 三类入参 + 源码结构断言（菜单/路由/平台操作分支） | `bash scripts/verify-2546.sh r16` | `docs/evidence/2546/R16-merchant-admin-entry.txt` | `CHECKPERM_MA=1`、`CHECKPERM_PLATFORM=1`、`MENU_BUSINESS_ROLES=1`、`ROUTE_BUSINESS_ROLES=1`、`NEW_ROUTE_PLATFORM_ONLY=1`、`PLATFORM_ACTIONS_HIDDEN=1`、`FRONTEND_LINT_RC=0` | `MENU_BUSINESS_ROLES=0`（菜单仍不可见 → Issue 期望 2 不可达）或 `NEW_ROUTE_PLATFORM_ONLY=0`（`/merchants/new` 对商户管理员开放）→ FAIL |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | runtime | fake IAM 记录 method/path/body + 本地 DB 读回 |
| R2 | runtime | 同上，状态码矩阵 |
| R3 | runtime | 前置拒绝 + `IAM_CALLS=0` |
| R4 | runtime | fake IAM 捕获 bind role |
| R5 | runtime | 本地行保留 + IAM 调用计数 |
| R6 | runtime | `PUT /merchants/:id` 经转让端点 + 翻转后断言 |
| R7 | runtime | 列表 JSON 字段与行数 |
| R8 | static | PC 无组件测试运行器（`frontend-pc/package.json` 仅 dev/build/lint/preview）→ 纯函数 `node --test` + 源码结构断言；真实渲染截图归关闭时 `[Ops-Followup of #2546]`（D8 Owner 豁免） |
| R9 | static | `auditRouteMap` 表项单元断言 |
| R10 | runtime | `go build` + 定向回归 |
| R11 | static | 文档 grep 结构断言 |
| R12 | runtime | 后端 `viewer_is_owner` JSON 断言 + 纯函数 `node --test` 四类 fixture（无组件渲染） |
| R13 | runtime | 组合请求后逐字段读回 DB |
| R14 | runtime | 事务回滚后读回 `admin_uid` 与 `role` |
| R15 | runtime | 路由门控矩阵（JWT 固定 merchant_admin 位图） |
| R16 | static | `checkPermission` 纯函数 `node --test` + 菜单/路由/分支源码结构断言（D8 Owner 豁免覆盖 PC 呈现形态） |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2546.sh r1` | `docs/evidence/2546/R1-transfer-success.txt` | exit 0；grep `TRANSFER_STATUS=200`、`IAM_METHOD=POST`、`IAM_PATH_SUFFIX=/transfer-ownership`、`IAM_BODY_USER_ID=1`、`LOCAL_OWNER_UPDATED=1`、`OLD_OWNER_LOCAL_ROLE=merchant_admin`、`TARGET_LOCAL_ROLE=merchant_admin`、`ADMIN_PENDING_CLEARED=1`、`OWNER_FIELD_PRESENT=1` | PASS | `IAM_PATH_SUFFIX` 非转让端或 `LOCAL_OWNER_UPDATED=0` → FAIL | dev |
| R2 | `bash scripts/verify-2546.sh r2` | `docs/evidence/2546/R2-iam-errors.txt` | exit 0；grep `IAM403_STATUS=403`、`IAM403_CODE=40303`、`IAM404_STATUS=404`、`IAM404_CODE=40400`、`IAM409_STATUS=409`、`IAM409_CODE=40900`、`IAM400_CODE=40002`、`IAM500_STATUS=502`、`IAM500_CODE=50200`、`LOCAL_OWNER_UNCHANGED=1` | PASS | 任一 IAM 错误返回 200，或 `LOCAL_OWNER_UNCHANGED=0` → FAIL | dev |
| R3 | `bash scripts/verify-2546.sh r3` | `docs/evidence/2546/R3-preflight-negatives.txt` | exit 0；grep `NONMEMBER_STATUS=404`、`NONMEMBER_CODE=40400`、`NONMEMBER_IAM_CALLS=0`、`SAME_OWNER_STATUS=400`、`SAME_OWNER_CODE=40002`、`SAME_OWNER_IAM_CALLS=0`、`EMPTY_UID_STATUS=400`、`EMPTY_UID_CODE=40001` | PASS | `NONMEMBER_IAM_CALLS=1` 或 `SAME_OWNER_STATUS=200` → FAIL | dev |
| R4 | `bash scripts/verify-2546.sh r4` | `docs/evidence/2546/R4-admin-bind.txt` | exit 0；grep `ADD_BIND_ROLE=ADMIN`、`UPDATE_BIND_ROLE=ADMIN`、`OWNER_BINDS=0`、`OWNER_REQUEST_STATUS=400`、`OWNER_REQUEST_CODE=40002` | PASS | `ADD_BIND_ROLE=OWNER` 或 `OWNER_BINDS=1` → FAIL | dev |
| R5 | `bash scripts/verify-2546.sh r5` | `docs/evidence/2546/R5-owner-protection.txt` | exit 0；grep `REMOVE_OWNER_STATUS=403`、`REMOVE_OWNER_CODE=40303`、`REMOVE_OWNER_IAM_CALLS=0`、`OWNER_ROW_KEPT=1`、`DEMOTE_OWNER_STATUS=400`、`DEMOTE_OWNER_CODE=40002`、`REMOVE_ADMIN_STATUS=200` | PASS | `REMOVE_OWNER_STATUS=200` 或 `OWNER_ROW_KEPT=0` → FAIL | dev |
| R6 | `bash scripts/verify-2546.sh r6` | `docs/evidence/2546/R6-update-merchant-transfer.txt` | exit 0；grep `UPD_ADMIN_STATUS=200`、`UPD_ADMIN_IAM_PATH_SUFFIX=/transfer-ownership`、`UPD_ADMIN_LOCAL=1`、`UPD_ADMIN_IS_OWNER_FLIP=1`、`CLEAR_OWNER_STATUS=400`、`CLEAR_OWNER_CODE=40002` | PASS | `UPD_ADMIN_IAM_PATH_SUFFIX` 非转让端，或 `CLEAR_OWNER_STATUS=200` → FAIL | dev |
| R7 | `bash scripts/verify-2546.sh r7` | `docs/evidence/2546/R7-list-owner-flag.txt` | exit 0；grep `OWNER_FLAG_HITS=1`、`OWNER_UID_MATCH=1`、`DEFAULT_ROWS=2`、`ALL_ROWS=3`、`OWNER_FIELD_PRESENT=1` | PASS | `OWNER_FLAG_HITS` ≠ 1，或 `DEFAULT_ROWS` ≠ 2 → FAIL | dev |
| R8 | `bash scripts/verify-2546.sh r8` | `docs/evidence/2546/R8-pc-owner-tab.txt` | exit 0；grep `NODE_TEST_RC=0`、`TAB_RENAMED=1`、`TRANSFER_BUTTON_USES_VIEWER_IS_OWNER=1`、`OWNER_REMOVE_DISABLED=1`、`CANDIDATE_PAGE_SIZE=100`、`FRONTEND_LINT_RC=0`、`FRONTEND_BUILD_RC=0` | PASS | `TAB_RENAMED=0` 或 `CANDIDATE_PAGE_SIZE` 大于 100 → FAIL | dev |
| R9 | `bash scripts/verify-2546.sh r9` | `docs/evidence/2546/R9-audit-route.txt` | exit 0；grep `AUDIT_ROUTE_FOUND=1`、`AUDIT_ACTION=TRANSFER_OWNERSHIP`、`AUDIT_RESOURCE=merchant`、`AUDIT_PRIORITY=CRITICAL` | PASS | 路由键缺失或 action 不符 → FAIL | dev |
| R10 | `bash scripts/verify-2546.sh r10` | `docs/evidence/2546/R10-regression.txt` | exit 0；grep `BUILD_RC=0`、`REGRESSION_RC=0`、`REGRESSION_PASS=1` | PASS | `BUILD_RC` 非 0 或回归 FAIL → FAIL | dev |
| R11 | `bash scripts/verify-2546.sh r11` | `docs/evidence/2546/R11-docs.txt` | exit 0；grep `DOC_API_TRANSFER=1`、`DOC_API_MEMBERS_OWNER=1`、`DOC_API_BIT8=1`、`DOC_UI_OWNER_TAB=1`、`DOC_DB_ADMIN_UID=1`、`DOC_CASE_PATH_ALIGNED=1`、`DOC_PERM_MERCHANTS_SCOPED=1` | PASS | 任一 `=0` → FAIL | dev |
| R12 | `bash scripts/verify-2546.sh r12` | `docs/evidence/2546/R12-owner-gate.txt` | exit 0；grep `GATE_FIELD=viewer_is_owner`、`BE_VIEWER_IS_OWNER_TRUE=1`、`BE_VIEWER_IS_OWNER_FALSE=1`、`GATE_OWNER_TRUE=1`、`GATE_DEMOTED_FALSE=1`、`GATE_UNANCHORED_FALSE=1`、`GATE_NO_CANDIDATE_FALSE=1`、`GATE_USERINFO_OWNER_USAGE=0`、`NODE_TEST_RC=0` | PASS | `GATE_FIELD` 非 `viewer_is_owner`，或 `GATE_DEMOTED_FALSE=0`，或 `GATE_USERINFO_OWNER_USAGE=1` → FAIL | dev |
| R13 | `bash scripts/verify-2546.sh r13` | `docs/evidence/2546/R13-put-combined-fields.txt` | exit 0；grep `COMBINED_STATUS=200`、`NAME_PERSISTED=1`、`PHONE_PERSISTED=1`、`ADMIN_UID_SWITCHED=1` | PASS | 任一 `PERSISTED=0` 或 `ADMIN_UID_SWITCHED=0` → FAIL | dev |
| R14 | `bash scripts/verify-2546.sh r14` | `docs/evidence/2546/R14-promote-failure.txt` | exit 0；grep `PROMOTE_FAIL_STATUS=500`、`PROMOTE_FAIL_CODE=50000`、`ADMIN_UID_UNCHANGED=1`、`ROLE_UNCHANGED=1`、`MSG_HAS_MANUAL_CHECK=1` | PASS | `PROMOTE_FAIL_STATUS=200`，或 `ADMIN_UID_UNCHANGED=0` 且 `ROLE_UNCHANGED=1` → FAIL | dev |
| R15 | `bash scripts/verify-2546.sh r15` | `docs/evidence/2546/R15-actor-authz.txt` | exit 0；grep `MA_LIST_STATUS=200`、`MA_LIST_ROWS=1`、`MA_GET_OWN_STATUS=200`、`MA_GET_OTHER_STATUS=404`、`MA_MEMBERS_STATUS=200`、`MA_TRANSFER_STATUS=200`、`MA_CREATE_MERCHANT_STATUS=403`、`MA_UPDATE_MERCHANT_STATUS=403`、`MA_DELETE_MERCHANT_STATUS=403`、`MA_ADD_MEMBER_STATUS=403` | PASS | `MA_GET_OTHER_STATUS=200` 或任一平台专属写路由非 403 → FAIL | dev |
| R16 | `bash scripts/verify-2546.sh r16` | `docs/evidence/2546/R16-merchant-admin-entry.txt` | exit 0；grep `CHECKPERM_MA=1`、`CHECKPERM_PLATFORM=1`、`MENU_BUSINESS_ROLES=1`、`ROUTE_BUSINESS_ROLES=1`、`NEW_ROUTE_PLATFORM_ONLY=1`、`PLATFORM_ACTIONS_HIDDEN=1`、`FRONTEND_LINT_RC=0` | PASS | `MENU_BUSINESS_ROLES=0` 或 `NEW_ROUTE_PLATFORM_ONLY=0` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2546/R1-transfer-success.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R2-iam-errors.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R3-preflight-negatives.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R4-admin-bind.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R5-owner-protection.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R6-update-merchant-transfer.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R7-list-owner-flag.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R8-pc-owner-tab.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R9-audit-route.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R10-regression.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R11-docs.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R12-owner-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R13-put-combined-fields.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R14-promote-failure.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R15-actor-authz.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2546/R16-merchant-admin-entry.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

逐份另需第二行 `GENERATED_AT_HEAD=<40 位 sha>`；生成器 `scripts/verify-2546.sh`（`r1`–`r16`）在 fix commit 之后运行，`dirty=false` 必须与 evidence.json 的 `build`/`commit` 一致。

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Workload

- Size: L
- Basis: 7 个后端实现/路由文件（IAM 客户端、members handler、merchant handler、路由、权限中间件、审计表）+ 6 个后端测试文件（新增 5 + 翻转 1）+ 5 个 PC 前端文件（组件/页面/菜单配置/路由/纯函数 + 2 测试）+ 1 个验证脚本（r1–r16）+ 5 份契约文档 + 16 条证据行；新增对外端点与路由授权面（api-contract，需跨商户负例）；无 migration、无 DB 结构变更
- Confidence: high

### Files（变更文件清单）

- `backend/services/iam_client.go`
- `backend/handlers/merchant_member.go`
- `backend/handlers/merchant.go`
- `backend/main.go`
- `backend/middleware/permissions.go`
- `backend/middleware/audit_logger.go`
- `backend/handlers/merchant_ownership_2546_test.go`
- `backend/handlers/merchant_owner_entry_2546_test.go`
- `backend/handlers/member_role_iam_fail_2461_test.go`
- `backend/services/iam_transfer_2546_test.go`
- `backend/middleware/audit_route_2546_test.go`
- `backend/middleware/permissions_2546_test.go`
- `frontend-pc/src/components/MerchantMemberManagement.jsx`
- `frontend-pc/src/pages/MerchantManagement.jsx`
- `frontend-pc/src/config/menuPermissions.js`
- `frontend-pc/src/config/menuPermissions.test.js`
- `frontend-pc/src/App.jsx`
- `frontend-pc/src/utils/merchantOwnership.js`
- `frontend-pc/src/utils/merchantOwnership.test.js`
- `scripts/verify-2546.sh`
- `docs/spec/api/api.md`
- `docs/spec/ui/ui.md`
- `docs/spec/permissions.md`
- `docs/spec/database/database.md`
- `docs/cases/organization.md`
- `docs/evidence/2546/plan.md`
- `docs/evidence/2546/evidence.json`
- `docs/evidence/2546/pre-ready-2546.json`
- `docs/evidence/2546/R1-transfer-success.txt`
- `docs/evidence/2546/R2-iam-errors.txt`
- `docs/evidence/2546/R3-preflight-negatives.txt`
- `docs/evidence/2546/R4-admin-bind.txt`
- `docs/evidence/2546/R5-owner-protection.txt`
- `docs/evidence/2546/R6-update-merchant-transfer.txt`
- `docs/evidence/2546/R7-list-owner-flag.txt`
- `docs/evidence/2546/R8-pc-owner-tab.txt`
- `docs/evidence/2546/R9-audit-route.txt`
- `docs/evidence/2546/R10-regression.txt`
- `docs/evidence/2546/R11-docs.txt`
- `docs/evidence/2546/R12-owner-gate.txt`
- `docs/evidence/2546/R13-put-combined-fields.txt`
- `docs/evidence/2546/R14-promote-failure.txt`
- `docs/evidence/2546/R15-actor-authz.txt`
- `docs/evidence/2546/R16-merchant-admin-entry.txt`

## Input Truth & Tool Preflight

- **Source of truth（四方，禁止互相替代）**：
  1. **IAM 关系角色与授权**：beaconiam#508（`4a0cb5c`）。转让端点 `POST /api/v1/organizations/:id/transfer-ownership {user_id}`；仅当前 OWNER（或 IAM 超管恢复）可调；错误码语义见「背景与现状核对」最后两行。
  2. **组织 OWNER 的唯一真值 = `user_org_relations.role='OWNER'`（写入 JWT `role`）与本地镜像 `merchants.admin_uid`**。`users.is_owner`（JWT `is_owner`）是**平台/命名空间级遗留标记**，**禁止**作为商户所有权门控（beaconiam `internal/auth/handler.go:632-646`；`internal/handler/org_handler.go` 转让不动 `users.is_owner`）。
  3. **本地所有权权威 = `merchants.admin_uid`**（单指针，`backend/models/models.go:1053`），不新增列、不做迁移。
  4. **本地业务角色 = `merchant_members.role`**（`merchant_admin` 同时覆盖 OWNER 与 ADMIN）+ JWT claims 判定（`backend/middleware/iam.go:645`）。
- **actor 入口真值（新）**：商户管理员 JWT `sys_perm=1073740800`（bit 10-29，dev IAM `functional_role_templates` 实测），**不含** bit 5/6/8 → 现路由 `RequireSysPerm` 一律 `40303`；`RequireSysPerm` 在 `sysPerm==0` 时放行（既有语义，本单不改）。因此 D13 只放开 4 条路由并对 `businessRole=merchant_admin` 放行，平台专属写路由保持门控。
- **差异核对（实施前先跑，结果入 R10/R15/R16 artifact）**：
  - `grep -rn 'BindUserToOrganizationWithToken' backend/handlers/merchant.go backend/handlers/merchant_member.go` → 修后**只剩** `CreateMerchant` 的创建者 bind（`merchant.go:410`），成员/编辑路径为 0 处。
  - `grep -rn 'toIAMRole(normalizedRole)' backend/handlers/merchant_member.go` → 修后为 0 处（改 `merchantMemberIAMRole`）。
  - `grep -rn 'user_info.isOwner' frontend-pc/src/` → 修后 0 处（门控改 `viewer_is_owner`）。
  - `grep -n 'RequireSysPermOrRole' backend/main.go` → 恰好 4 处，且全部含 `BusinessRoleMerchantAdmin`。
  - `grep -n 'adminCount <= 1' backend/handlers/merchant_member.go` → owner 守卫必须排在它之前。
  - `grep -n 'First(&merchant)' backend/handlers/merchant.go` → `UpdateMerchant` 内转让后不得出现「重载 + Save」组合（R13 覆盖）。
- **损坏输入拒绝**：`user_id` 空/非 UUID → `40001`；目标无 `merchant_members` 行 → `40400`；目标 `iam_sub` 空 → `40002`；目标 = 当前 owner → `40002`；`admin_uid` 置空 → `40002`；`role=OWNER/owner` → `40002`。以上分支均**不调 IAM**（`IAM_CALLS=0` 断言）。
- **工具盲区（显式声明）**：
  - fake IAM 只能证明 method/path/body 与状态码映射，**不能**证明 beaconiam 侧授权；IAM 授权以 #508 单测与其 403/409 分支为准。
  - R15 用固定 JWT 位图验证路由门控矩阵，**不能**替代 IAM 侧 OWNER 校验；两者叠加才是完整授权链。
  - PC 无组件测试运行器 → R8/R16 用纯函数 `node --test` + 源码结构断言（D8，Owner 豁免）；真实渲染/预生产回评归关闭时 Ops-Followup。
  - light 节点不查预生产库；存量 `admin_uid` 与 IAM OWNER 是否一致由 #2553（Ops，只读清单）核对。
  - GORM 自动租户范围对 `Table(...)` 无效 → 新查询一律显式 `tenant_id = 商户 org` 条件（沿用 `#2250/#2274` 锚定口径）。
  - R14 的失败注入用 GORM `Callback().Update().Before("gorm:update")` 精确命中 `merchant_members` 表；不得用「删行后再转让」等非确定性手段。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | **本地所有权权威 = `merchants.admin_uid`**（target_semantics；historical_baseline = #1718「负责人可多个」，已废止）；`merchant_members.role` 只表达业务角色；`is_owner` 为响应级计算字段，**不落库、不建索引**；**无 DB 结构变更、无迁移** | Architect | 本计划锁定（`docs/cases/organization.md:219-223` + `middleware/iam.go:645`） | 若要求本地落 `is_owner` 列 → 需 migration + IAM 回填数据源，回 Architect |
| D2 | **IAM 转让是唯一所有权变更语义**：`AddMember`/`UpdateMemberRole` 经 `merchantMemberIAMRole` bind `ADMIN`；`toIAMRole` 不动（#2540 兼容）；显式 `role=OWNER/owner` → `40002` | Architect | 本计划锁定 | 改全局 `toIAMRole` → 需同步 #2540 计划 |
| D3 | 转让候选 = 本商户任意 active `merchant_members` 行（含直属员工），排除当前 owner；成功后提升为 `merchant_admin` | Architect | 本计划锁定（`docs/cases/organization.md:196-206`） | 只允许管理员之间转让 → 前端过滤 + 后端 role 守卫，回 Architect |
| D4 | `PUT /merchants/:id` 的 `admin_uid` 变化**复用同一转让 helper**；置空 → `400/40002`；**字段更新与转让分离且有序**（先普通字段，再转让；禁止重载后 Save） | Architect | 本计划锁定（避免两条所有权写入路径 + 组合字段丢失） | 若要求保留 bind+demote → 与 #508 冲突，回 Architect |
| D5 | IAM 错误映射：403→`40303`、404→`40400`、409→`40900`、400→`40002`（透传 IAM 文案）、其他/5xx/网络→`50200`；一律显式返回 | Architect | 本计划锁定（AGENTS 红线：外部 API 失败不得静默） | 统一 500 → 与 #2457/#2540 的 502 口径冲突 |
| D6 | 审计由 `auditRouteMap` 表项驱动（`TRANSFER_OWNERSHIP/merchant/CRITICAL`），不手写 `audit_logs`；不额外 `IncrementPermVersion()` | Architect | 本计划锁定（IAM 转让已 bump） | 手写审计/重复 bump → 另立统一审计 Issue |
| D7 | **存量对齐不在本单**：`admin_uid` ↔ IAM OWNER 核对与收敛由 #2553（`waitForConfirm`，需 Owner 授权）承载 | Owner / ops | #2553 已建 | 未授权 → 保持现状；`owner_uid` 为空时显示「未锚定所有人」并禁用转让 |
| D8 | R8/R16 证据形态 = 纯函数 `node --test` + 源码结构断言（PC 无组件测试运行器）→ **Owner 豁免**（沿用 #2526 D6；`evidence_type: static`）；真实渲染截图与预生产回评归关闭时 `[Ops-Followup of #2546]`（仅 `waitForConfirm`） | Architect / Owner | 本计划锁定（Owner 豁免惯例） | 若审计要求 render → 另立「PC 组件测试基建」Issue，本单不得以「构建通过」冒充 |
| D9 | 本单**不拆子任务**：端点 + 三条写入路径收敛 + 门控真值 + actor 入口 + 文档属同一验收链 | Architect | 本计划锁定 | 拆分会造成「端点已上、前端无入口」或「门控无真值」中间态 |
| D10 | 转让成功后**不改** `users.role` 缓存（业务角色由 claims 判定） | Architect | 本计划锁定 | 若某页只读 `users.role` 显示错 → 另立 Issue 收敛 |
| D11 | `merchants.admin_uid` 语义升级为「所有人（OWNER）指针」；`docs/spec/database/database.md:121` 同批改口径 | Architect | 本计划锁定 | 若要求新列 `owner_uid` → 需 migration + 回填授权 |
| D12 | `PUT /merchants/:id` 既有 `40009`（最后负责人兜底）**保持不变**；owner 相关新增分支用 `40303/40002` | Architect | 本计划锁定 | 统一为 `40900` → 需同步前端文案与既有测试 |
| D13 | **actor 与入口路径 = 方案 (a)**：`businessRole=merchant_admin` 可读 4 条路由（`GET /merchants`、`GET /merchants/:id`、`GET /admin/merchants/:id/members`、`POST /merchants/:id/transfer-ownership`），数据面收窄沿用既有 `org_id`/`loadMerchantForAccess`；平台专属写路由（创建/删除/编辑商户、成员增删改、`/merchants/new`）保持 `sys_perm` 门控；入口路径 = 平台管理 → 商户管理 → 唯一商户 → 所有人 Tab | Architect | 本计划锁定（Issue 期望 2 + O-03 `role: [tenant_admin]` + `docs/spec/permissions.md:275`/`ui.md:3172` 已声明 + 后端收窄分支已存在 #2274/#2276） | 若 Owner 改走「只允许平台超管代转」(b) → 必须同批改写 Issue 期望 2 与 O-03 `gate`/`role`，且需 Owner 书面确认；本单不得擅自回退产品语义 |
| D14 | **转让门控真值 = 后端 `viewer_is_owner`**（`merchants.admin_uid == 当前 user_id`），禁止用 `users.is_owner`/JWT `is_owner`；漂移时表现为 IAM `40303` 显式错误（不静默） | Architect | 本计划锁定（对端语义核对结论；D7 承载漂移收敛） | 若改用 JWT `role==='OWNER'` → 多组织/超管上下文误判，回 Architect |
| D15 | **组合字段与所有权变更解耦**：`PUT /merchants/:id` 先写普通字段，再转让；转让后如需响应快照只能重载**不 Save**；R13 断言三者同时落库 | Architect | 本计划锁定（避免 `First` 重载覆盖内存字段） | 保留「重载 + Save」→ 组合请求静默丢字段，审计应判 FAIL |
| D16 | **本地转让同步 = 单事务**（`admin_uid` 与目标 `role` 同时成功），任一失败回滚并 `500/50000` 显式文案；禁止 log-only 半同步 | Architect | 本计划锁定（AGENTS 红线 + 审计 Bug 3） | 若要求「允许半同步 + warning 字段」→ 需定义 warning 契约与消费方，回 Architect |

## Out of Scope

- 存量 `merchants.admin_uid` ↔ IAM active OWNER 的核对与收敛（含 nil UUID / 多 OWNER 遗留）→ **#2553**（Ops，仅 `waitForConfirm`，执行需 Owner 授权）；本单不写库、不做数据修复。
- beaconiam 侧的 OWNER 唯一性/迁移（#508 已完成）。
- `merchant_members.is_owner` 落库列 + 部分唯一索引（Rev.1 原设计，Rev.2 起取消；见 D1）。
- **方案 (b)「只允许平台 `is_owner` 超管代转」**及其对 Issue 期望 2 / O-03 `gate` 的改写 —— 需 Owner 书面确认后另立；本单按 D13 (a) 执行。
- 方案 (c)「转让入口另挂一个新的商户设置页」—— 与 (a) 重复承载面，不在本单。
- 把 `sys_perm` bit 5/6 授予商户管理员角色模板（会造成 `/system/user-management`、`/platform-staff`、`/system/warnings` 等平台页越权）—— 明确排除，本单用路由级 businessRole 放行。
- `toIAMRole` 全局映射调整、`ActivateUser`/批量导入把业务角色当 IAM 角色写 → **#2540** 范围。
- `mapIAMRoleToSiteRole`（IAM 同步 `ADMIN` → `site_admin`）语义重定 —— 另立。
- 直营店/加盟店类型与「加盟店必须指定所有人」（#2544 A3/A4/A5、#509）—— 另单。
- `PUT /merchants/:id` 的 IAM 组织改名（#2459 accepted 计划）—— 本单只动 `admin_uid` 分支；建议合并顺序 #2459 → #2546。
- PC 组件级测试基建（Vitest/RTL）与真实渲染截图。
- 错误文案本地化改造（沿用既有中文文案 + IAM 原文透传）。

## Core Update

[Core Update Required]

| 文档路径 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §12.3.3 只有 `PUT /api/merchants/:id` 一般字段；§10 无商户成员端点；转让端点不存在 | ① §12.3.3 补「`admin_uid` 变更 = 转让所有权（IAM `POST /organizations/:id/transfer-ownership`）；置空拒绝 `40002`；IAM 失败码 40303/40400/40900/40002/50200；组合字段与转让分两步不互相覆盖」；② 新增小节「商户成员与所有人」：`GET /admin/merchants/:id/members`（默认 `role='merchant_admin'`；`include=all`；每行 `is_owner`/`status`；顶层 `owner_uid`/`viewer_is_owner`/`viewer_uid`）、`POST`/`PUT`/`DELETE members`（IAM 角色 `ADMIN`；owner 移除 `40303`、owner 降级 `40002`）；③ 新增 `POST /api/merchants/:id/transfer-ownership`（body/成功响应/错误码/授权：当前 OWNER 或平台超管）；④ 路由授权口径补「商户管理员可读 4 条路由并收窄到本商户」；⑤ **修正 §12.3.5.5 的 `sys_perm bit 5（tenant:update）` → `bit 8`** | 端点新增与语义变更必须有契约落字；位数笔误（Low-1）会误导实现者用错权限位 |
| `docs/spec/ui/ui.md` | §3.27 菜单清单只写「商户管理 (`/merchants`) — sys_perm bit 5」及创建/编辑表单要点，无「所有人」Tab 设计 | ① 该条目下补：商户详情 →「所有人」Tab = 所有人（唯一）卡片（`OWNER` 标签、不可移除）+ 转让所有权（候选 Select + 「转让后你将成为管理员」二次确认，仅 `viewer_is_owner=true` 可见）+ 管理员列表（「添加管理员」，无「设为负责人」）；未锚定所有人时提示并禁用；② 补商户管理员视角：菜单「商户管理」对其可见但只列本商户，平台专属操作（创建/删除/编辑基本信息/分账配置）隐藏 | UI 变更必须有设计落字，且 actor 可见性需与 D13 一致 |
| `docs/spec/permissions.md` | L275 菜单映射「系统管理 → 商户管理 → /merchants → sysPerm: [5]」；L39 商户管理员为 sys_perm bit 10-29 | 改为「商户管理：`sysPerm [5]` 或 `businessRole=merchant_admin`（后者只读且列表收窄到本商户；创建/删除/编辑仍仅平台管理员）」；并在 §五 说明 4 条放宽路由与 3 条仍受 sys_perm 门控的写路由 | 文档当前自相矛盾（声明可见但位图不含），是 Bug 1 附带入口问题的根源 |
| `docs/spec/database/database.md` | `merchants.admin_uid` 描述为「管理员用户 ID」 | 改为「**所有人（OWNER）用户 ID**（本地指针；权威关系在 IAM `user_org_relations.role='OWNER'`）」，注明业务角色由 claims 判定、`merchant_members` 无 `is_owner` 列 | 语义升级必须写明 |
| `docs/cases/organization.md` | O-03 已按新模型改写，但 `:208` 留有未决注释「merchant_admin 之外的 ADMIN 语义？见下」，转让端点写作 `/admin/merchants/:id/transfer-ownership` | ① 替换未决注释为锁定口径（本地 `role='merchant_admin'` 通用；所有权指针 `admin_uid`；转让只搬指针并提升目标业务角色；`PUT` 的 `admin_uid` 走同一转让）；② 端点路径改 `/api/merchants/:id/transfer-ownership`；③ seq2 的 `role` 明确为 `merchant_admin`，`gate` 明确为「`viewer_is_owner=true`（本商户 `admin_uid` = 当前登录账号）」，并补 `reach` 含菜单可见性 | 用例是验收权威；`gate` 措辞必须与 D14 门控真值一致，否则实现者各自解释 |

## Dependencies

无

### 关联（非硬依赖）

- `beaconiam#508`（已上线预生产 `61f8a8f`）：对端契约与授权规则来源。
- `#2544`（已 done）：用例/契约已按 OWNER 模型修订（A1/A2）。
- `#2459`（accepted）：同函数 `UpdateMerchant` 的改名分支；本单只动 `admin_uid` 分支。建议执行顺序 **#2459 → #2546**。
- `#2540`（accepted）：`toIAMRole` 单一映射；本单不改 `toIAMRole`。
- `#2461`（done）：其两条换管理员失败语义断言由本单翻转（见「语义变更」表）。
- `#2274` / `#2276`（done）：商户管理员 org 收窄分支来源（本单复用，不新增 scoping）。
- `#1718`：被本单取代的「负责人可多个」旧规则来源，历史保留不改。
- `#2553`（`waitForConfirm`）：存量 `admin_uid` ↔ IAM OWNER 只读核对 + 授权后收敛。
- 关闭本单时需创建 `[Ops-Followup of #2546]`（仅 `waitForConfirm`）承载预生产真机回评与「所有人」Tab 截图（D8）。

*Model: DeepSeek V4.1 Flash 1017*

