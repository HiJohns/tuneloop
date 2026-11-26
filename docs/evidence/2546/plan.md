## Accepted Plan — #2546 商户所有人（唯一 OWNER）+ 转让所有权（原 OWNER 降 ADMIN）｜Rev.2

> Tags: enhancement, code:go, api-contract, scope:pc, code:js
> 关联依据：beaconiam#508（`4a0cb5c`，预生产包 `beaconiam_20261126-063053_61f8a8f.zip`）已上线转让端点与单一 OWNER 不变量；用例/契约由 #2544 已修订（A1/A2）
> 上游拦截来源：#2546 评论「needMore — 缺少 Accepted Plan」列出的 5 项锁定要求

### 修订记录（Rev.1 → Rev.2：补齐「添加管理员仍绑 OWNER」与「第三条所有权写入路径」）

Rev.1（2026-11-26 08:19 评论）已锁定端点与 UI 方向，Rev.2 只修**会被对端 IAM 直接拒绝**的部分与本地权威口径：

| # | 修订项 | Rev.1 | Rev.2 | 依据 |
|---|---|---|---|---|
| 1 | 本地所有人标记 | `merchant_members` 新增 `is_owner BOOL` + migration + 部分唯一索引；存量回填推给 Ops（服务端无法从 IAM 读关系角色） | **不新增列、不迁移**：权威用已有 `merchants.admin_uid`；`is_owner` 仅作为**响应计算字段**（不落库） | `merchants.admin_uid` 已存在（`backend/models/models.go:1053`），创建商户即写创建者；新增布尔列会产生第二个可能与 IAM 漂移的权威源，且 Rev.1 的回填在 tuneloop 侧**没有数据来源**（IAM 无「列组织关系角色」的可用端点） |
| 2 | 添加成员 | 只拦「目标已是 owner」，未改 IAM bind 角色 | `merchant_admin` 经 `merchantMemberIAMRole` bind **`ADMIN`**；显式 `role=OWNER/owner` → `40002` | `toIAMRole("merchant_admin")="OWNER"`（`permission_manage.go:240-243`）+ beaconiam#508 规则「组织已有 OWNER 时 bind OWNER → 400」⇒ Rev.1 下「添加管理员」**必然失败** |
| 3 | `PUT /merchants/:id` 改 `admin_uid` | 未覆盖（仍是 bind 新 OWNER + 旧降 `USER`） | 改走**同一**转让 helper；置空 → `40002` | 该路径在 #508 下同样必然失败（bind 400 + 降级 OWNER 400），是第三条所有权写入路径 |
| 4 | 审计与 perm_version | 手写 `audit_logs` 1 条 + `IncrementPermVersion()` | 用 `auditRouteMap` 表项（中间件自动落审计）；**不**再调 `IncrementPermVersion()` | beaconiam 转让在同一事务内已 bump perm_version（`org_handler.go:1659-1670`），双写会重复；手写审计与既有审计中间件重复记录 |
| 5 | 用例路径对齐 | 端点 `/api/merchants/:id/transfer-ownership`（与 Issue 正文一致），但 `docs/cases/organization.md` 写的是 `/admin/merchants/:id/transfer-ownership` | 代码采用 Issue 正文路径 `/api/merchants/:id/transfer-ownership`，并**同批修正用例文档路径** | Issue 正文（Owner）是交付权威；文档与实现不得各写一套 |
| 6 | 计划完整性 | 7 条证据行，缺「加成员绑 ADMIN」「编辑商户走转让」「列表 owner 标记」「文档落字」四类验收 | 11 条证据行（R1–R11），含语义翻转登记与四份文档落字 | 期望 1–3 的每条约束都要有机器可判定证据 |

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 「负责人管理」Tab：列表只取 `role='merchant_admin'` 的成员，标题写「负责人管理」 | `backend/handlers/merchant_member.go:51`、`:65`；`frontend-pc/src/pages/MerchantManagement.jsx:280` |
| 「最后负责人保护」允许多个负责人，仅拦截删到 0 个（`40009 不能删除最后一个负责人`） | `backend/handlers/merchant_member.go:536-546` |
| 添加成员时把业务角色 `merchant_admin` 用 `toIAMRole` 映射为 IAM `OWNER` | `backend/handlers/merchant_member.go:316`、`:438`；`backend/handlers/permission_manage.go:240-255` |
| 改成员角色时 `merchant_admin` 分支同样 bind IAM `OWNER`（其余走 `/role` 降级） | `backend/handlers/merchant_member.go:445-456` |
| 编辑商户（`PUT /merchants/:id`）改 `admin_uid` = bind 新 IAM `OWNER` → demote 旧为 `USER`，失败有补偿解绑 | `backend/handlers/merchant.go:576-660` |
| 本地「负责人」指针已存在：`merchants.admin_uid`（创建时写创建者；列表/详情接口已返回） | `backend/models/models.go:1053`；`backend/handlers/merchant.go:384`、`:107` |
| 业务角色判定来自 JWT claims（`tid == oid` → merchant_admin），不读本地 `users.role` | `backend/middleware/iam.go:645-648` |
| 用例已按新模型改写（「每商户恰好 1 个 OWNER」「OWNER 不能删除/降级，只能转让」），但该文件的 `admin_uid` 本地同步语义仍是**未决问号** | `docs/cases/organization.md:180-236`（`:208` 注释「merchant_admin 之外的 ADMIN 语义？见下」） |
| 前端角色标签只有 `merchant_admin`（无 OWNER/ADMIN 区分）、移除按钮按「仅剩 1 个」禁用；转让入口不存在 | `frontend-pc/src/components/MerchantMemberManagement.jsx:310-355`、`:368-374` |
| IAM 侧契约（beaconiam#508）：`POST /api/v1/organizations/:id/transfer-ownership {user_id}`；仅当前 OWNER（或平台超管恢复）可调；403 `only the current owner can transfer ownership`；409 `organization has no active owner`；400 `user is already the owner`；404 `target user is not an active member of this organization`；成功后**同一事务**旧 OWNER→ADMIN、目标→OWNER、写 `TRANSFER_OWNERSHIP` 审计、bump perm_version | beaconiam `internal/handler/org_handler.go:1568-1671`（`4a0cb5c`） |
| IAM 侧 bind 语义：`role=OWNER` 仅在组织无 OWNER 时允许，否则 400 `organization already has an owner; use POST /organizations/:id/transfer-ownership`；直接降级/移除 OWNER → 400/403 | beaconiam `internal/handler/org_handler.go:1163-1180`、`:930-953`、`:846` |

**根因（一句话）**：本地把「负责人」建模成**可多个的业务角色 `merchant_admin`**，并把该业务角色直接当成 IAM 关系角色 `OWNER` 写入；IAM 已改为**单 OWNER + 只能转让**，于是本地存在三条互不相让的所有权写入路径（添加成员 / 改角色 / 编辑商户），且没有任何一条是转让语义。

### 方案（变更点）

1. **本地所有权权威 = `merchants.admin_uid`（已有列，无迁移）**
   - 新增纯函数 `merchantOwnerUID(m models.Merchant) string`：`admin_uid` 为空或 nil UUID（`00000000-0000-0000-0000-000000000000`）时返回 `""`，否则原样返回。
   - `merchant_members.role` **保持业务角色语义**：OWNER 与 ADMIN 在 tuneloop 内都是 `merchant_admin`（依据 `docs/cases/organization.md:219-223` 「业务角色不变」与 `middleware/iam.go:645`）。因此「原 OWNER → ADMIN」在本地**不是改 role**，而是 `admin_uid` 指针迁移；`is_owner` 只在响应里按 `user_id == merchantOwnerUID(merchant)` 计算，不落库。
2. **新增 IAM 客户端契约（唯一转让出口）**
   - `backend/services/iam_client.go` 新增 `TransferOwnershipWithToken(token, orgID, targetUserID string) error`：`POST /api/v1/organizations/{orgID}/transfer-ownership`，body `{"user_id": targetUserID}`（IAM user id，不是本地 id）。
   - 新增类型化错误 `IAMTransferOwnershipError{StatusCode, Message}`（解析 beaconiam `{"error": "..."}`），供 handler 精确映射；**不得**把 4xx 当成功，也**不得**只 `log` 后返回 200。
   - 不调用 `IncrementPermVersion()`：beaconiam 在转让事务内已 bump（`perm_version` 由 IAM 负责，双写会重复）。
3. **新增 handler `TransferOwnership`（`POST /api/merchants/:id/transfer-ownership`）**
   - 路由：`authRequired` + `ExplicitTenantScope()` + `RequireSysPerm(SysPermTenantUpdate)`（与 `PUT /merchants/:id` 同组，`backend/main.go:496`）。
   - 流程：`loadMerchantForAccess` → 校验 `user_id`（缺失/非 UUID → `40001`）→ 目标必须是本商户 active `merchant_members` 行（否则 `40400`，且**不调 IAM**）→ 目标不得等于当前 owner（否则 `40002`）→ 解析目标 `iam_sub`（空则 `40002`「目标用户缺少 IAM 账号」）→ 调 IAM 转让 → 成功后本地事务：`merchants.admin_uid = <目标本地 id>`、`admin_pending = false`（`RowsAffected` 必须为 1，否则 `50000` + ERROR 日志「IAM 已转让，本地同步失败」）、把目标 `merchant_members.role` 提升为 `merchant_admin`（原本已是则 1 行不变）。
   - 错误映射（D5）：IAM 403 → `403/40303`（「仅当前所有人可转让所有权」）；404 → `404/40400`；409 → `409/40900`（「该商户当前没有所有人，请走修复流程」）；400 → `400/40002`（透传 IAM `error` 文案，例如 `user is already the owner`）；其他/5xx/网络 → `502/50200`「转让所有权失败（IAM）：…」。
   - 响应：`{code:20000, data:{merchant_id, previous_owner_id, new_owner_id}}`。
4. **收敛其余所有权写入路径（禁止第二个 OWNER）**
   - 新增 `merchantMemberIAMRole(businessRole string) string`：`merchant_admin → "ADMIN"`，其余委托 `toIAMRole`（`toIAMRole` 本身**不动**，与 #2540 兼容）。`AddMember`（`:316`）与 `UpdateMemberRole`（`:438`）改用它。
   - `AddMember`/`UpdateMemberRole` 收到 `owner`/`OWNER`（大小写不敏感）→ `400/40002`「所有人不能通过添加/改角色设置，请使用转让所有权」，**不调 IAM**。
   - `UpdateMemberRole` 目标是当前 owner → `400/40002`「所有人不能改角色，请使用转让所有权」（禁止经 `/role` 或 bind 降级 OWNER）。
   - `RemoveMember`：目标是当前 owner → `403/40303`「所有人不能移除，只能转让所有权」（排在既有「最后负责人」兜底之前，且**不调 IAM**）；非 owner 管理员沿用既有 `40009` 兜底与 IAM 解绑路径（回归保持，不改既有错误码）。
   - `PUT /merchants/:id`（`UpdateMerchant`）：`admin_uid` 变化改走**同一个**转让 helper（`transferMerchantOwnership`，新增于 `merchant_member.go`，两处调用），删除原 bind-OWNER + demote-`USER` + 补偿解绑代码块；`admin_uid` 置空/nil（清空所有人）→ `400/40002`「所有人不能清空，只能转让」。
5. **列表契约：标记所有人 + 候选成员**
   - `ListMembers` 响应每行新增 `is_owner`（`user_id == merchantOwnerUID(merchant)`）与 `status` 字段；顶层新增 `owner_uid`（空串表示本地未锚定）。
   - 新增可选查询参数 `include=all`：返回本商户全部 active 成员（默认仍只返回 `role='merchant_admin'`，保持既有调用方行为不变）；转让候选 = 该结果排除当前 owner。
6. **审计接线**
   - `backend/middleware/audit_logger.go` 的 `auditRouteMap` 新增 `POST /api/merchants/:id/transfer-ownership → {TRANSFER_OWNERSHIP, merchant, CRITICAL}`（中间件自动落 `audit_logs`，`resource_id` 取 `:id`，CRITICAL 会快照 request body；不手写 audit 插入）。
7. **PC 前端（`MerchantMemberManagement.jsx`）**
   - Tab 改名：`MerchantManagement.jsx:280` 「负责人管理」→「所有人」。
   - 顶部「所有人（唯一）」卡片：读 `is_owner` 行展示姓名/邮箱/加入时间 + `OWNER` 标签；无 owner（`owner_uid` 空）→ `Alert` 提示「未锚定所有人（请联系平台核对）」并禁用转让。
   - 转让按钮（仅当 `localStorage.user_info.isOwner === true` 且存在 owner 且有候选时显示）：`Modal` 内 `Select` 候选 + 固定文案「转让后你将成为管理员」+ `Modal.confirm` 二次确认 → `POST /merchants/:id/transfer-ownership` → 成功 `message.success` + 刷新；失败展示后端 `message`（403/400 文案原样）。
   - 列表：owner 行 `Tag color="red">所有人（OWNER）`、无「移除」；其余行 `管理员`；「添加管理员」按钮与 Modal 文案从「负责人」统一为「管理员」（不再有「设为负责人」选项）。
   - 纯函数抽到新文件 `frontend-pc/src/utils/merchantOwnership.js`：`pickOwnerRow(members)`、`transferCandidates(members, ownerUid)`、`canRemoveMember(record)`、`transferConfirmText(ownerName)`，同目录 `merchantOwnership.test.js` 用 `node --test` 覆盖。

### 语义变更（必须翻转的既有断言，强制清单）

| 文件:行 | 旧断言 | 新断言 | 理由 |
|---|---|---|---|
| `backend/handlers/member_role_iam_fail_2461_test.go:304-331`（`TestUpdateMerchantAdminDemoteFailureCompensates_2461`） | demote `/role` 500 → `502` + 补偿解绑 `IAM_UNBIND_CALLS=1` + 本地 `admin_uid` 不变 | **200** + IAM 只调 `transfer-ownership`（`IAM_TRANSFER_CALLS=1`、`IAM_UNBIND_CALLS=0`）+ 本地 `admin_uid` 已切换 | 所有权变更不再走 bind+demote，改为 IAM 单事务转让；补偿解绑代码已删除（无第二个 OWNER 需要补偿） |
| `backend/handlers/member_role_iam_fail_2461_test.go:332-354`（`TestUpdateMerchantAdminBindFailureVisible_2461`） | bind 500 → `200` + `admin_pending=true` + `admin_sync_warning` | transfer 500 → `502/50200`，`admin_uid` 不变，`admin_pending` 不变 | 转让是**全成败**（IAM 已是单事务），不允许「本地先改、IAM 待确认」的半态；`admin_pending` 只保留给创建商户路径 |

**生产行为变更登记（非测试断言，但必须随单说明）**

| 行为 | 修改前 | 修改后 | 依据 |
|---|---|---|---|
| 添加成员 `role=merchant_admin` | IAM bind `OWNER`（已有 OWNER 时 IAM 400 ⇒ 添加失败） | IAM bind `ADMIN`（成功，成为第二管理员而非第二所有人） | 期望 2「添加管理员 = ADMIN，不得写成第二个 OWNER」 |
| `PUT /merchants/:id` 改 `admin_uid` | bind 新 `OWNER` + 旧降 `USER`（平台管理员即可操作） | 走 IAM 转让（**仅当前 OWNER** 或 IAM 超管可成功；否则 IAM 403 → 前端可见 `40303`） | IAM#508 转让端点授权规则；关闭「平台代改所有人」旁路 |
| `PUT /merchants/:id` 把 `admin_uid` 置空 | 允许（本地清空 + IAM 降级） | `400/40002` 拒绝 | OWNER 不能删除/清空，只能转让 |
| 移除当前 owner | 最后一个负责人时 `40009`；多个负责人时**可移除** | 一律 `403/40303`「所有人不能移除，只能转让所有权」 | 期望 3 语义改写 |
| `GET /admin/merchants/:id/members` | 每行无 owner 标记 | 新增 `is_owner` / `status` / 顶层 `owner_uid`；新增 `include=all` | 前端所有人卡片与转让候选的数据来源 |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志） |
|---|---|---|---|---|---|---|
| R1 转让成功（IAM 契约 + 本地同步） | `backend/handlers/merchant_member.go`、`backend/services/iam_client.go` | `POST /merchants/:id/transfer-ownership` | fake IAM 200；商户有 owner A + 管理员 B | backend（测试库 + fake IAM） | `TRANSFER_STATUS=200`、`IAM_METHOD=POST`、`IAM_PATH_SUFFIX=/transfer-ownership`、`IAM_BODY_USER_ID=1`、`LOCAL_OWNER_UPDATED=1`、`OLD_OWNER_LOCAL_ROLE=merchant_admin`、`TARGET_LOCAL_ROLE=merchant_admin`、`ADMIN_PENDING_CLEARED=1`、`OWNER_FIELD_PRESENT=1` | R1 |
| R2 IAM 失败映射（403/404/409/400/5xx）+ 本地不变 | `backend/handlers/merchant_member.go`、`backend/services/iam_client.go` | 同上 | fake IAM 依次返回 403 / 404 / 409 / 400 / 500 | backend | `IAM403_STATUS=403`、`IAM403_CODE=40303`、`IAM404_STATUS=404`、`IAM404_CODE=40400`、`IAM409_STATUS=409`、`IAM409_CODE=40900`、`IAM400_CODE=40002`、`IAM500_STATUS=502`、`IAM500_CODE=50200`、`LOCAL_OWNER_UNCHANGED=1` | R2 |
| R3 前置拒绝（不调 IAM） | `backend/handlers/merchant_member.go` | 同上 | 目标非本商户成员 / 目标已是 owner / `user_id` 空 | backend | `NONMEMBER_STATUS=404`、`NONMEMBER_CODE=40400`、`NONMEMBER_IAM_CALLS=0`、`SAME_OWNER_STATUS=400`、`SAME_OWNER_CODE=40002`、`SAME_OWNER_IAM_CALLS=0`、`EMPTY_UID_STATUS=400`、`EMPTY_UID_CODE=40001` | R3 |
| R4 添加/改角色只 bind ADMIN，OWNER 请求被拒 | `backend/handlers/merchant_member.go` | `POST /admin/merchants/:id/members`、`PUT /admin/merchants/:id/members/:uid` | `role=merchant_admin`；另测 `role=OWNER` | backend | `ADD_BIND_ROLE=ADMIN`、`UPDATE_BIND_ROLE=ADMIN`、`OWNER_BINDS=0`、`OWNER_REQUEST_STATUS=400`、`OWNER_REQUEST_CODE=40002` | R4 |
| R5 OWNER 移除/降级保护 + 非 owner 管理员回归 | `backend/handlers/merchant_member.go` | `DELETE /admin/merchants/:id/members/:uid`、`PUT /admin/merchants/:id/members/:uid` | 目标是 owner / 目标是普通管理员 | backend | `REMOVE_OWNER_STATUS=403`、`REMOVE_OWNER_CODE=40303`、`REMOVE_OWNER_IAM_CALLS=0`、`OWNER_ROW_KEPT=1`、`DEMOTE_OWNER_STATUS=400`、`DEMOTE_OWNER_CODE=40002`、`REMOVE_ADMIN_STATUS=200` | R5 |
| R6 编辑商户改所有人走转让（翻转 #2461 既有断言） | `backend/handlers/merchant.go` | `PUT /merchants/:id` | 请求含 `admin_uid` 变化；另一请求置空 | backend | `UPD_ADMIN_STATUS=200`、`UPD_ADMIN_IAM_PATH_SUFFIX=/transfer-ownership`、`UPD_ADMIN_LOCAL=1`、`UPD_ADMIN_IS_OWNER_FLIP=1`、`CLEAR_OWNER_STATUS=400`、`CLEAR_OWNER_CODE=40002` | R6 |
| R7 列表 owner 标记 + 候选（`include=all`） | `backend/handlers/merchant_member.go` | `GET /admin/merchants/:id/members` | 默认 / `include=all` 两种；成员含 owner、管理员、直属员工 | backend | `OWNER_FLAG_HITS=1`、`OWNER_UID_MATCH=1`、`DEFAULT_ROWS=2`、`ALL_ROWS=3`、`OWNER_FIELD_PRESENT=1` | R7 |
| R8 PC 所有人 Tab 接线（纯函数 + 源码结构断言） | `frontend-pc/src/components/MerchantMemberManagement.jsx`、`frontend-pc/src/pages/MerchantManagement.jsx`、`frontend-pc/src/utils/merchantOwnership.js` | `node --test frontend-pc/src/utils/merchantOwnership.test.js` + 结构断言 + eslint + build | 所有人卡片 / 转让入口 / owner 行禁用移除 / 候选排除 owner | PC | `NODE_TEST_RC=0`、`TAB_RENAMED=1`、`TRANSFER_UI_WIRED=1`、`OWNER_REMOVE_DISABLED=1`、`FRONTEND_LINT_RC=0`、`FRONTEND_BUILD_RC=0` | R8 |
| R9 审计路由接线 | `backend/middleware/audit_logger.go` | `auditRouteMap` 单元断言 | 路由键存在且字段完整 | backend | `AUDIT_ROUTE_FOUND=1`、`AUDIT_ACTION=TRANSFER_OWNERSHIP`、`AUDIT_RESOURCE=merchant`、`AUDIT_PRIORITY=CRITICAL` | R9 |
| R10 构建 + 既有成员/商户用例回归 | `backend/handlers/merchant_member.go`、`backend/handlers/merchant.go` | `go build ./...` + 定向回归 | 修后工作树 | backend | `BUILD_RC=0`、`REGRESSION_RC=0`、`REGRESSION_PASS=1` | R10 |
| R11 文档同步落字 | `docs/spec/api/api.md`、`docs/spec/ui/ui.md`、`docs/spec/database/database.md`、`docs/cases/organization.md` | grep 结构断言 | — | 文档 | `DOC_API_TRANSFER=1`、`DOC_API_MEMBERS_OWNER=1`、`DOC_UI_OWNER_TAB=1`、`DOC_DB_ADMIN_UID=1`、`DOC_CASE_PATH_ALIGNED=1` | R11 |

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 商户 M（org=O，`admin_uid=A`）+ `merchant_members`(A=merchant_admin, B=merchant_admin) + `users`(A,B 均有 `iam_sub`) | `bash scripts/verify-2546.sh r1` | `docs/evidence/2546/R1-transfer-success.txt` | `TRANSFER_STATUS=200`、`IAM_METHOD=POST`、`IAM_PATH_SUFFIX=/transfer-ownership`、`IAM_BODY_USER_ID=1`、`LOCAL_OWNER_UPDATED=1`、`OLD_OWNER_LOCAL_ROLE=merchant_admin`、`TARGET_LOCAL_ROLE=merchant_admin`、`ADMIN_PENDING_CLEARED=1`、`OWNER_FIELD_PRESENT=1` | `IAM_PATH_SUFFIX` 非 `/transfer-ownership`（走了 bind）或 `LOCAL_OWNER_UPDATED=0` → FAIL |
| R2 | 同 R1，fake IAM 状态码可控 | `bash scripts/verify-2546.sh r2` | `docs/evidence/2546/R2-iam-errors.txt` | `IAM403_STATUS=403`、`IAM403_CODE=40303`、`IAM404_STATUS=404`、`IAM404_CODE=40400`、`IAM409_STATUS=409`、`IAM409_CODE=40900`、`IAM400_CODE=40002`、`IAM500_STATUS=502`、`IAM500_CODE=50200`、`LOCAL_OWNER_UNCHANGED=1` | 任一错误被吞成 `200`，或 `LOCAL_OWNER_UNCHANGED=0`（IAM 失败却改了本地）→ FAIL |
| R3 | 同 R1；额外用户 C 无 `merchant_members` 行 | `bash scripts/verify-2546.sh r3` | `docs/evidence/2546/R3-preflight-negatives.txt` | `NONMEMBER_STATUS=404`、`NONMEMBER_CODE=40400`、`NONMEMBER_IAM_CALLS=0`、`SAME_OWNER_STATUS=400`、`SAME_OWNER_CODE=40002`、`SAME_OWNER_IAM_CALLS=0`、`EMPTY_UID_STATUS=400`、`EMPTY_UID_CODE=40001` | `NONMEMBER_IAM_CALLS=1`（无效目标仍打 IAM）或 `SAME_OWNER_STATUS=200` → FAIL |
| R4 | 同 R1，目标用户 D 未入商户 | `bash scripts/verify-2546.sh r4` | `docs/evidence/2546/R4-admin-bind.txt` | `ADD_BIND_ROLE=ADMIN`、`UPDATE_BIND_ROLE=ADMIN`、`OWNER_BINDS=0`、`OWNER_REQUEST_STATUS=400`、`OWNER_REQUEST_CODE=40002` | `ADD_BIND_ROLE=OWNER` 或 `OWNER_BINDS=1` → FAIL（产生第二个 OWNER，IAM 必然 400） |
| R5 | 同 R1（A=owner，B=普通管理员） | `bash scripts/verify-2546.sh r5` | `docs/evidence/2546/R5-owner-protection.txt` | `REMOVE_OWNER_STATUS=403`、`REMOVE_OWNER_CODE=40303`、`REMOVE_OWNER_IAM_CALLS=0`、`OWNER_ROW_KEPT=1`、`DEMOTE_OWNER_STATUS=400`、`DEMOTE_OWNER_CODE=40002`、`REMOVE_ADMIN_STATUS=200` | `REMOVE_OWNER_STATUS=200` 或 `OWNER_ROW_KEPT=0` → FAIL（所有人被移除） |
| R6 | 同 R1，经 `PUT /merchants/:id` 传 `admin_uid=B`；另测 `admin_uid=<nil UUID>` | `bash scripts/verify-2546.sh r6` | `docs/evidence/2546/R6-update-merchant-transfer.txt` | `UPD_ADMIN_STATUS=200`、`UPD_ADMIN_IAM_PATH_SUFFIX=/transfer-ownership`、`UPD_ADMIN_LOCAL=1`、`UPD_ADMIN_IS_OWNER_FLIP=1`、`CLEAR_OWNER_STATUS=400`、`CLEAR_OWNER_CODE=40002` | `UPD_ADMIN_IAM_PATH_SUFFIX` 非转让端，或 `CLEAR_OWNER_STATUS=200` → FAIL |
| R7 | 同 R1 + 直属员工 E（`role=merchant_staff`） | `bash scripts/verify-2546.sh r7` | `docs/evidence/2546/R7-list-owner-flag.txt` | `OWNER_FLAG_HITS=1`、`OWNER_UID_MATCH=1`、`DEFAULT_ROWS=2`、`ALL_ROWS=3`、`OWNER_FIELD_PRESENT=1` | `OWNER_FLAG_HITS` ≠ 1（无标记或多人被标 OWNER），或 `DEFAULT_ROWS` ≠ 2（默认口径被改）→ FAIL |
| R8 | `merchantOwnership.test.js` 四类 fixture（owner 存在/缺失、候选为空、owner 行/普通行）；源码结构断言 | `bash scripts/verify-2546.sh r8` | `docs/evidence/2546/R8-pc-owner-tab.txt` | `NODE_TEST_RC=0`、`TAB_RENAMED=1`、`TRANSFER_UI_WIRED=1`、`OWNER_REMOVE_DISABLED=1`、`FRONTEND_LINT_RC=0`、`FRONTEND_BUILD_RC=0` | `TAB_RENAMED=0`（Tab 仍叫负责人管理）或 `TRANSFER_UI_WIRED=0`（后端有端点、前端无入口 → 死代码）→ FAIL |
| R9 | `auditRouteMap` 静态表 | `bash scripts/verify-2546.sh r9` | `docs/evidence/2546/R9-audit-route.txt` | `AUDIT_ROUTE_FOUND=1`、`AUDIT_ACTION=TRANSFER_OWNERSHIP`、`AUDIT_RESOURCE=merchant`、`AUDIT_PRIORITY=CRITICAL` | 路由键缺失或 action 不是 `TRANSFER_OWNERSHIP` → FAIL（转让无审计） |
| R10 | 修后工作树 + 既有用例（`TestAddMember`/`TestRemoveMember`/`TestUpdateMemberRole`/`TestUpdateMerchant`/`TestMerchantMember`） | `bash scripts/verify-2546.sh r10` | `docs/evidence/2546/R10-regression.txt` | `BUILD_RC=0`、`REGRESSION_RC=0`、`REGRESSION_PASS=1` | `BUILD_RC` ≠ 0 或回归用例 FAIL → FAIL |
| R11 | 四份契约文档修后内容 | `bash scripts/verify-2546.sh r11` | `docs/evidence/2546/R11-docs.txt` | `DOC_API_TRANSFER=1`、`DOC_API_MEMBERS_OWNER=1`、`DOC_UI_OWNER_TAB=1`、`DOC_DB_ADMIN_UID=1`、`DOC_CASE_PATH_ALIGNED=1` | 任一为 `0`（文档未落字，下一轮仍会各写一套）→ FAIL |

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
| R8 | static | PC 无组件测试运行器（`frontend-pc/package.json` 仅 dev/build/lint/preview）→ 纯函数 `node --test` + 源码结构断言；真实渲染截图归关闭时的 `[Ops-Followup of #2546]` |
| R9 | static | `auditRouteMap` 表项单元断言 |
| R10 | runtime | `go build` + 定向回归 |
| R11 | static | 文档 grep 结构断言 |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2546.sh r1` | `docs/evidence/2546/R1-transfer-success.txt` | exit 0；grep `TRANSFER_STATUS=200`、`IAM_METHOD=POST`、`IAM_PATH_SUFFIX=/transfer-ownership`、`IAM_BODY_USER_ID=1`、`LOCAL_OWNER_UPDATED=1`、`OLD_OWNER_LOCAL_ROLE=merchant_admin`、`TARGET_LOCAL_ROLE=merchant_admin`、`ADMIN_PENDING_CLEARED=1`、`OWNER_FIELD_PRESENT=1` | PASS | `IAM_PATH_SUFFIX` ≠ `/transfer-ownership` 或 `LOCAL_OWNER_UPDATED=0` → FAIL | dev |
| R2 | `bash scripts/verify-2546.sh r2` | `docs/evidence/2546/R2-iam-errors.txt` | exit 0；grep `IAM403_STATUS=403`、`IAM403_CODE=40303`、`IAM404_STATUS=404`、`IAM404_CODE=40400`、`IAM409_STATUS=409`、`IAM409_CODE=40900`、`IAM400_CODE=40002`、`IAM500_STATUS=502`、`IAM500_CODE=50200`、`LOCAL_OWNER_UNCHANGED=1` | PASS | 任一 IAM 错误返回 200，或 `LOCAL_OWNER_UNCHANGED=0` → FAIL | dev |
| R3 | `bash scripts/verify-2546.sh r3` | `docs/evidence/2546/R3-preflight-negatives.txt` | exit 0；grep `NONMEMBER_STATUS=404`、`NONMEMBER_CODE=40400`、`NONMEMBER_IAM_CALLS=0`、`SAME_OWNER_STATUS=400`、`SAME_OWNER_CODE=40002`、`SAME_OWNER_IAM_CALLS=0`、`EMPTY_UID_STATUS=400`、`EMPTY_UID_CODE=40001` | PASS | `NONMEMBER_IAM_CALLS=1` 或 `SAME_OWNER_STATUS=200` → FAIL | dev |
| R4 | `bash scripts/verify-2546.sh r4` | `docs/evidence/2546/R4-admin-bind.txt` | exit 0；grep `ADD_BIND_ROLE=ADMIN`、`UPDATE_BIND_ROLE=ADMIN`、`OWNER_BINDS=0`、`OWNER_REQUEST_STATUS=400`、`OWNER_REQUEST_CODE=40002` | PASS | `ADD_BIND_ROLE=OWNER` 或 `OWNER_BINDS=1` → FAIL | dev |
| R5 | `bash scripts/verify-2546.sh r5` | `docs/evidence/2546/R5-owner-protection.txt` | exit 0；grep `REMOVE_OWNER_STATUS=403`、`REMOVE_OWNER_CODE=40303`、`REMOVE_OWNER_IAM_CALLS=0`、`OWNER_ROW_KEPT=1`、`DEMOTE_OWNER_STATUS=400`、`DEMOTE_OWNER_CODE=40002`、`REMOVE_ADMIN_STATUS=200` | PASS | `REMOVE_OWNER_STATUS=200` 或 `OWNER_ROW_KEPT=0` → FAIL | dev |
| R6 | `bash scripts/verify-2546.sh r6` | `docs/evidence/2546/R6-update-merchant-transfer.txt` | exit 0；grep `UPD_ADMIN_STATUS=200`、`UPD_ADMIN_IAM_PATH_SUFFIX=/transfer-ownership`、`UPD_ADMIN_LOCAL=1`、`UPD_ADMIN_IS_OWNER_FLIP=1`、`CLEAR_OWNER_STATUS=400`、`CLEAR_OWNER_CODE=40002` | PASS | `UPD_ADMIN_IAM_PATH_SUFFIX` 非转让端，或 `CLEAR_OWNER_STATUS=200` → FAIL | dev |
| R7 | `bash scripts/verify-2546.sh r7` | `docs/evidence/2546/R7-list-owner-flag.txt` | exit 0；grep `OWNER_FLAG_HITS=1`、`OWNER_UID_MATCH=1`、`DEFAULT_ROWS=2`、`ALL_ROWS=3`、`OWNER_FIELD_PRESENT=1` | PASS | `OWNER_FLAG_HITS` ≠ 1，或 `DEFAULT_ROWS` ≠ 2 → FAIL | dev |
| R8 | `bash scripts/verify-2546.sh r8` | `docs/evidence/2546/R8-pc-owner-tab.txt` | exit 0；grep `NODE_TEST_RC=0`、`TAB_RENAMED=1`、`TRANSFER_UI_WIRED=1`、`OWNER_REMOVE_DISABLED=1`、`FRONTEND_LINT_RC=0`、`FRONTEND_BUILD_RC=0` | PASS | `TAB_RENAMED=0` 或 `TRANSFER_UI_WIRED=0` → FAIL | dev |
| R9 | `bash scripts/verify-2546.sh r9` | `docs/evidence/2546/R9-audit-route.txt` | exit 0；grep `AUDIT_ROUTE_FOUND=1`、`AUDIT_ACTION=TRANSFER_OWNERSHIP`、`AUDIT_RESOURCE=merchant`、`AUDIT_PRIORITY=CRITICAL` | PASS | 路由键缺失或 action 不符 → FAIL | dev |
| R10 | `bash scripts/verify-2546.sh r10` | `docs/evidence/2546/R10-regression.txt` | exit 0；grep `BUILD_RC=0`、`REGRESSION_RC=0`、`REGRESSION_PASS=1` | PASS | `BUILD_RC` ≠ 0 或回归用例 FAIL → FAIL | dev |
| R11 | `bash scripts/verify-2546.sh r11` | `docs/evidence/2546/R11-docs.txt` | exit 0；grep `DOC_API_TRANSFER=1`、`DOC_API_MEMBERS_OWNER=1`、`DOC_UI_OWNER_TAB=1`、`DOC_DB_ADMIN_UID=1`、`DOC_CASE_PATH_ALIGNED=1` | PASS | 任一 `=0` → FAIL | dev |

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

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Workload

- Size: L
- Basis: 5 个后端实现文件（IAM 客户端、members handler、merchant handler、路由、审计表）+ 4 个后端测试文件（新增 3 + 翻转 1）+ 3 个 PC 前端文件（组件/页面/纯函数 + 1 测试）+ 1 个验证脚本 + 4 份契约文档 + 11 条证据行；新增对外 API 端点（api-contract）且与 #2459/#2540 同区协同；无 migration、无 DB 结构变更
- Confidence: high

### Files（变更文件清单）

- `backend/services/iam_client.go`
- `backend/handlers/merchant_member.go`
- `backend/handlers/merchant.go`
- `backend/main.go`
- `backend/middleware/audit_logger.go`
- `backend/handlers/merchant_ownership_2546_test.go`
- `backend/handlers/member_role_iam_fail_2461_test.go`
- `backend/services/iam_transfer_2546_test.go`
- `backend/middleware/audit_route_2546_test.go`
- `frontend-pc/src/components/MerchantMemberManagement.jsx`
- `frontend-pc/src/pages/MerchantManagement.jsx`
- `frontend-pc/src/utils/merchantOwnership.js`
- `frontend-pc/src/utils/merchantOwnership.test.js`
- `scripts/verify-2546.sh`
- `docs/spec/api/api.md`
- `docs/spec/ui/ui.md`
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

## Input Truth & Tool Preflight

- **Source of truth（三方，禁止互相替代）**：
  1. **IAM 关系角色与授权**：beaconiam#508（`4a0cb5c`）。转让端点为 `POST /api/v1/organizations/:id/transfer-ownership {user_id}`；仅当前 OWNER（或 IAM 超管恢复）可调；错误码语义见「背景与现状核对」表最后两行。
  2. **本地所有权权威** = `merchants.admin_uid`（单指针，`backend/models/models.go:1053`），不新增列、不做迁移。
  3. **本地业务角色** = `merchant_members.role`（`merchant_admin` 同时覆盖 OWNER 与 ADMIN）+ JWT claims 判定（`backend/middleware/iam.go:645`）。
- **差异核对（实施前先跑，结果入 R1/R10 artifact）**：
  - `grep -rn 'BindUserToOrganizationWithToken' backend/handlers/merchant.go backend/handlers/merchant_member.go` → 修后**只剩** `CreateMerchant` 的创建者 bind（`merchant.go:410`），成员/编辑路径为 0 处。
  - `grep -rn 'toIAMRole(normalizedRole)' backend/handlers/merchant_member.go` → 修后为 0 处（改 `merchantMemberIAMRole`）。
  - `grep -rn 'merchant_admin' backend/handlers/permission_manage.go` → `toIAMRole` 映射**保持不变**（`merchant_admin→OWNER`），避免与 #2540 accepted 计划冲突。
  - `grep -n 'adminCount <= 1' backend/handlers/merchant_member.go` → owner 守卫必须排在它之前，且 `grep -n 'admin_uid' backend/handlers/merchant.go` 不再出现 `BindUserToOrganizationWithToken`。
- **损坏输入拒绝**：`user_id` 空/非 UUID → `40001`；目标无 `merchant_members` 行 → `40400`（不泄漏是否为平台用户）；目标 `iam_sub` 空 → `40002`；目标 = 当前 owner → `40002`；`admin_uid` 置空 → `40002`；`role=OWNER/owner` → `40002`。以上分支均**不调 IAM**（用 `IAM_CALLS=0` 断言）。
- **工具盲区（显式声明）**：
  - fake IAM 只能证明 method/path/body 与状态码映射，**不能**证明 beaconiam 侧授权；IAM 授权以 #508 单测与其 403/409 分支为准。
  - PC 无组件测试运行器 → R8 用纯函数 `node --test` + 源码结构断言（D8，Owner 豁免）；真实渲染/预生产回评归关闭时 Ops-Followup。
  - light 节点不查预生产库；存量 `admin_uid` 与 IAM OWNER 是否一致由 #2553（Ops，只读清单）核对。
  - GORM 自动租户范围对 `Table(...)` 无效 → 新查询一律显式 `tenant_id = 商户 org` 条件（沿用 `#2250/#2274` 锚定口径）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | **本地所有权权威 = `merchants.admin_uid`**；`merchant_members.role` 只表达业务角色（OWNER 与 ADMIN 均为 `merchant_admin`）；`is_owner` 仅为响应级计算字段，**不落库、不建索引**；转让时把目标提升为 `merchant_admin`，旧 owner 行保持 `merchant_admin`；**无 DB 结构变更、无迁移** | Architect | 本计划锁定（依据 `docs/cases/organization.md:219-223` 业务角色不变 + `middleware/iam.go:645`） | 若要求本地落 `is_owner` 列 → 需要 migration + 从 IAM 回填（tuneloop 无该读端点）→ 回 Architect 重立并先解决回填数据源 |
| D2 | **IAM 转让是唯一所有权变更语义**：`AddMember`/`UpdateMemberRole` 通过 `merchantMemberIAMRole` bind `ADMIN`；`toIAMRole` 保持 `merchant_admin→OWNER`（#2540 兼容）；显式 `role=OWNER/owner` 请求 `400/40002` | Architect | 本计划锁定 | 若改成全局 `toIAMRole` 改映射 → 需同步 #2540 accepted 计划与激活路径 |
| D3 | 转让候选 = 本商户**任意 active `merchant_members` 行**（含直属员工），排除当前 owner；成功后本地提升为 `merchant_admin` | Architect | 本计划锁定（`docs/cases/organization.md:196-206`「仅本商户 active 成员」） | 若 Owner 要求只允许管理员之间转让 → 前端候选过滤 + 后端加 `role='merchant_admin'` 守卫，回 Architect |
| D4 | `PUT /merchants/:id` 的 `admin_uid` 变化**复用同一转让 helper**（不保留 bind+demote 第二套语义）；置空/nil → `400/40002` | Architect | 本计划锁定（避免两条所有权写入路径） | 若要求直接拒绝 admin_uid 变更（400 且不转让）→ 需同步翻转 #2459 中「admin_uid 事务」表述并接受平台代改能力消失 |
| D5 | IAM 错误映射：403→`40303`、404→`40400`、409→`40900`、400→`40002`（透传 IAM `error` 文案）、其他/5xx/网络→`50200`；一律显式返回，禁止只 log | Architect | 本计划锁定（AGENTS 红线：外部 API 失败不得静默；错误码沿用 Rev.1 已公布口径） | 若要求统一 500 → 与 #2457/#2540 的 502 口径冲突，回 Architect |
| D6 | 审计由 `auditRouteMap` 表项驱动（`TRANSFER_OWNERSHIP/merchant/CRITICAL`），handler 不手写 `audit_logs`；不额外调 `IncrementPermVersion()` | Architect | 本计划锁定（与既有 `PUT /orders/:id/transfer-ownership` 同模式；IAM 转让已 bump） | 若手写审计/重复 bump → 会与中间件/IAM 重复记录，需另立统一审计 Issue |
| D7 | **存量对齐不在本单**：本地 `admin_uid` ↔ IAM OWNER 的核对与收敛由 #2553（`waitForConfirm`，需 Owner 授权）承载；本单不改库、不做迁移 | Owner / ops | #2553 已建（本计划关联） | 未授权 → 保持现状；`owner_uid` 为空时前端显示「未锚定所有人」并禁用转让（不静默猜测） |
| D8 | R8 UI 证据形态 = 纯函数 `node --test` + 源码结构断言（PC 无组件测试运行器）→ **Owner 豁免**（沿用 #2526 D6）；真实渲染截图与预生产回评归关闭时创建的 `[Ops-Followup of #2546]`（仅 `waitForConfirm`） | Architect / Owner | 本计划锁定（Owner 豁免惯例） | 若审计要求 render → 另立「PC 组件测试基建」Issue，本单不得以「构建通过」冒充 |
| D9 | 本单**不拆子任务**：后端端点 + 三条写入路径收敛 + 列表字段 + 前端入口 + 文档属同一验收链 | Architect | 本计划锁定 | 拆分会造成「端点已上、前端无入口」或「字段无消费方」的中间态，审计应判 FAIL |
| D10 | 转让成功后**不改** `users.role` 缓存（业务角色由 JWT claims 判定，列表读 `merchant_members`） | Architect | 本计划锁定（避免跨表缓存写放大） | 若发现某页面只读 `users.role` 导致显示错 → 另立 Issue 收敛读取口径 |
| D11 | `merchants.admin_uid` 语义从「管理员用户 ID」升级为「所有人（OWNER）指针」；`docs/spec/database/database.md:121` 同批改口径 | Architect | 本计划锁定 | 若 Owner 要求新列（如 `owner_uid`）→ 需 migration + 数据回填授权，回 Architect |
| D12 | `PUT /merchants/:id` 的既有 `40009`（最后负责人兜底）**保持不变**；仅 owner 相关新增分支使用 `40303/40002` | Architect | 本计划锁定（不做无谓既有契约变更） | 若要求统一为 Rev.1 的 `40900` → 需同步 `frontend` 文案与既有测试，回 Architect |

## Out of Scope

- 存量 `merchants.admin_uid` ↔ IAM active OWNER 的核对与收敛（含 nil UUID / 多 OWNER 遗留）→ **#2553**（Ops，仅 `waitForConfirm`，执行需 Owner 授权）；本单不写库、不做数据修复。
- beaconiam 侧的 OWNER 唯一性/迁移（#508 已完成）。
- `merchant_members.is_owner` 落库列 + 部分唯一索引（Rev.1 原设计，Rev.2 取消；见 D1）。
- `toIAMRole` 全局映射调整、`ActivateUser`/批量导入把业务角色当 IAM 角色写 → **#2540** 范围（已在其 Issue 评论补约束提示：`merchant_admin` 激活应避免 bind `OWNER`）。
- `mapIAMRoleToSiteRole`（IAM 同步把 `ADMIN` 映射为 `site_admin`）的语义重定 —— 与所有权无关，另立。
- 直营店/加盟店类型与「加盟店必须指定所有人」（#2544 A3/A4/A5、#509 商业口径）—— 另单。
- `PUT /merchants/:id` 的 IAM 组织改名（#2459 accepted 计划）—— 本单只动同函数的 `admin_uid` 分支；两者合并顺序建议 #2459 → #2546。
- 平台管理员「代改所有人」能力保留（新模型下由 IAM 授权决定，tuneloop 不加旁路）。
- PC 组件级测试基建（Vitest/RTL）与真实渲染截图。
- 错误文案本地化改造（沿用既有中文文案 + IAM 原文透传）。

## Core Update

[Core Update Required]

| 文档路径 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §12.3.3 只有 `PUT /api/merchants/:id` 的一般字段；§10 无商户成员端点；转让端点不存在 | ① §12.3.3 补「`admin_uid` 变更 = 转让所有权（IAM `POST /organizations/:id/transfer-ownership`）；置空拒绝 `40002`；IAM 失败码 40303/40400/40900/40002/50200」；② 新增小节「商户成员与所有人」：`GET /admin/merchants/:id/members`（默认 `role='merchant_admin'`；`include=all`；每行 `is_owner`；顶层 `owner_uid`）、`POST`/`PUT`/`DELETE members`（IAM 角色 `ADMIN`；owner 移除 `40303`、owner 降级 `40002`）；③ 新增 `POST /api/merchants/:id/transfer-ownership`（body/成功响应/错误码） | 端点新增与语义变更必须有契约落字，否则下一次仍会各写一套 |
| `docs/spec/ui/ui.md` | §3.27 菜单清单只写「商户管理 (`/merchants`) — sys_perm bit 5」及创建/编辑表单要点，无「所有人」Tab 设计 | 在该条目下补：商户详情 →「所有人」Tab = 所有人（唯一）卡片（`OWNER` 标签、不可移除）+ 转让所有权（候选 Select + 「转让后你将成为管理员」二次确认，仅当前 OWNER 可见）+ 管理员列表（「添加管理员」，无「设为负责人」选项）；未锚定所有人时显示提示并禁用转让 | UI 变更必须有设计落字，避免被当作误显示移除 |
| `docs/spec/database/database.md` | `merchants.admin_uid` 描述为「管理员用户 ID」 | 改为「**所有人（OWNER）用户 ID**（本地指针；权威关系在 IAM `user_org_relations.role='OWNER'`）」，并注明业务角色 `merchant_admin` 由 claims 判定、`merchant_members` 无 `is_owner` 列 | 语义升级必须写明，否则后续读者仍按「管理员」理解 |
| `docs/cases/organization.md` | O-03（`id: O-03` 商户所有人）已按新模型改写，但 `:208` 留有未决注释「merchant_admin 之外的 ADMIN 语义？见下」，且转让端点写作 `/admin/merchants/:id/transfer-ownership` | ① 把该注释替换为锁定口径：本地 `merchant_members.role='merchant_admin'` 对 OWNER/ADMIN 通用；所有权指针为 `merchants.admin_uid`；转让只搬指针并提升目标业务角色；`PUT /merchants/:id` 的 `admin_uid` 走同一转让；② 端点路径改为 `/api/merchants/:id/transfer-ownership`（与 Issue 正文/实现一致） | 用例是验收权威，未决问号与路径不一致会让实现者各自解释 |

## Dependencies

无

### 关联（非硬依赖）

- `beaconiam#508`（已上线预生产 `61f8a8f`）：本单的对端契约与授权规则来源。
- `#2544`（已 done）：用例/契约已按 OWNER 模型修订（A1/A2），本单消费其结论并清掉唯一未决注释 + 对齐端点路径。
- `#2459`（accepted）：同函数 `UpdateMerchant` 的改名分支（IAM 组织名 + D14 半态顺序）；本单只动 `admin_uid` 分支。建议执行顺序 **#2459 → #2546**，避免同一函数两次大改冲突。
- `#2540`（accepted）：`toIAMRole` 单一映射与激活/批量导入收敛；本单**不改** `toIAMRole`，已在其 Issue 评论补「`merchant_admin` 激活不要 bind `OWNER`」约束提示。
- `#2461`（done）：其 R6/R7 两条换管理员失败语义断言由本单翻转（见「语义变更」表）。
- `#1718`：被本单取代的「负责人可多个」旧规则来源，历史保留不改。
- `#2553`（本计划创建，`waitForConfirm`）：存量 `admin_uid` ↔ IAM OWNER 只读核对 + 授权后收敛。
- 关闭本单时需创建 `[Ops-Followup of #2546]`（仅 `waitForConfirm`）承载预生产真机回评与「所有人」Tab 截图（D8）。

*Model: DeepSeek V4.1 Flash 1017*

