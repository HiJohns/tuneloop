## Accepted Plan — #2540 邻居收敛：`ActivateUser` / 批量导入改用 `toIAMRole`，激活权限模板跟随 `users.role`，商户管理员确认绑定 `OWNER`｜Rev.3

> 来源：#2467（已并入 #2466 关闭）的邻居扫描。同类缺陷只修了 `UpdateUser` 网点分支，本单收敛剩余两处。

### 变更点

1. **`backend/handlers/user_staff.go`（`ActivateUser`）**
   - 角色模板解析（新纯函数，可脱 DB 单测）：
     ```go
     // resolveActivationRoleTemplate 返回激活时使用的业务角色模板码：
     // user.Role 命中 services.AllRoleTemplates 才采用；空/非法 → "site_member"（legacy 兜底）。
     func resolveActivationRoleTemplate(userRole string) string
     ```
   - `iamRole := toIAMRole(roleTemplate)`；`iamRole == ""` → `"STAFF"`（`toIAMRole` 对未知值原样返回，必须兜底）。
   - 客户权限位（`ComputeCusPermBitmapExt`）与功能角色模板分配（`AssignRoleTemplateToUserWithToken`）统一使用**同一个** `roleTemplate`，删除硬编码 `AllRoleTemplates["site_member"]`。
   - 失败补偿顺序不变（bind 失败 → purge；set-perm 失败 → unbind + purge）。

2. **`backend/services/bulk_import_accounts.go`**
   - `iamRole := GetBusinessRole(acc.Role)` → `iamRole := toIAMRole(acc.Role)`；无法识别（`""` 或不在 IAM 角色集合）→ `"STAFF"`。
   - 说明：`toIAMRole` 在 `handlers` 包，`services` 不能反向依赖 → 把映射下沉为 `services.ToIAMRole(code string) string`（`backend/services/role_templates.go`），`handlers.toIAMRole` 改为薄封装（保持既有调用点不变）。

3. **`backend/handlers/confirmation_session.go`（商户管理员审批确认）**
   - `BindUserToOrganization(session.UserID, merchant.OrgID, "admin", "")` → `toIAMRole("merchant_admin")`（= **`OWNER`**）。
   - 依据：Owner 2026-11-26 裁定「新建商户的管理员应该是商户的 owner」；字面量 `"admin"` 既不是唯一真源（映射散落）也把商户创建者降为 ADMIN，导致其无法再创建网点/角色模板（见 `## OWNER vs ADMIN` 说明）。

4. **测试（fake IAM 捕获 bind payload）**
   - `backend/handlers/user_activate_role_2540_test.go`：`site_member`→`STAFF`+`site_member` 模板；`site_admin`→`ADMIN`+`site_admin` 模板（权限位与模板一致）；空角色→`STAFF`+`site_member`；未知角色（如 `customer`）→`STAFF`+`site_member`。
   - `backend/services/bulk_import_accounts_2540_test.go`：导入 `site_member`/`site_admin` 两行 → bind `role` 分别为 `STAFF`/`ADMIN`；`acc.Role` 空 → `STAFF`。

5. **文档**
   - `docs/spec/api/api.md`：`POST /api/users/:id/activate` 补「IAM 关系角色 = `toIAMRole(users.role)`，空/非法回落 `STAFF`；权限模板与角色模板同源」；商户创建/审批确认路径补「管理员绑定 `OWNER`（唯一权威：`toIAMRole('merchant_admin')`）」。

### Files

- `backend/handlers/user_staff.go`
- `backend/handlers/confirmation_session.go`
- `backend/handlers/permission_manage.go`
- `backend/services/role_templates.go`
- `backend/services/bulk_import_accounts.go`
- `backend/handlers/user_activate_role_2540_test.go`（新增）
- `backend/handlers/merchant_admin_confirm_role_2540_test.go`（新增）
- `backend/services/bulk_import_accounts_2540_test.go`（新增）
- `docs/spec/api/api.md`
- `scripts/verify-2540.sh`（新增）
- `docs/evidence/2540/plan.md`
- `docs/evidence/2540/evidence.json`
- `docs/evidence/2540/pre-ready-2540.json`
- `docs/evidence/2540/R1-activate-role.txt`
- `docs/evidence/2540/R2-bulk-import-role.txt`
- `docs/evidence/2540/R3-no-business-role-static.txt`
- `docs/evidence/2540/R4-docs.txt`
- `docs/evidence/2540/R6-merchant-owner.txt`
- `docs/evidence/2540/R5-build.txt`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 激活 site_member | `backend/handlers/user_staff.go` | `POST /api/users/:id/activate` | `users.role=site_member`，无 `iam_sub` | backend（测试库 + fake IAM） | `BIND_ROLE=STAFF`、`PERM_TEMPLATE=site_member`、`CUS_PERM_MATCH=1` | R1 |
| 激活 site_admin（不降权） | 同上 | 同上 | `users.role=site_admin` | backend | `BIND_ROLE=ADMIN`、`PERM_TEMPLATE=site_admin`、`CUS_PERM_MATCH=1` | R1 |
| 激活 legacy 空角色 | 同上 | 同上 | `users.role=""` | backend | `BIND_ROLE=STAFF`、`PERM_TEMPLATE=site_member` | R1 |
| 批量导入角色 | `backend/services/bulk_import_accounts.go` | 批量导入账号服务 | `acc.Role=site_member` / `site_admin` / 空 | backend | `BIND_ROLE ∈ {STAFF, ADMIN}`；`LEGACY_BUSINESS_ROLE=0` | R2 |
| 非法业务角色清除（静态） | `backend/handlers/user_staff.go`、`backend/services/bulk_import_accounts.go` | grep | 修后源码 | backend | `GetBusinessRole_as_iam_role=0`（`BindUserToOrganization` 第三参数不再来自 `GetBusinessRole`） | R3 |
| 商户管理员确认绑定 OWNER | `backend/handlers/confirmation_session.go` | 商户管理员审批确认流程（fake IAM） | `session.ActionType='merchant_admin'` | backend | `BIND_ROLE=OWNER`、`PERM_TEMPLATE=merchant_admin`（对端后果「建网点需 `PermissionOwner`」见 `### OWNER vs ADMIN`，不属本行可执行期望） | R6 |
| 文档同步 | `docs/spec/api/api.md` | grep | — | 文档 | 含 `toIAMRole` / `STAFF` 回落表述 | R4 |
| 构建/测试 | backend | 定向测试 + `go build` + 全量基线差异 | 修后工作树 | backend | `GO_TEST_OK=1`、`BUILD_OK=1`、`FULL_GATE_EXIT=1`（仓库根无 `go.mod` → pre-ready P5 判 N/A）、`FULL_GATE_NEW_FAILURES=0`（与 `origin/develop` 基线同集失败视为等价） | R5 |

### Input Truth & Tool Preflight

- **Source of truth**：
  - IAM 合法关系角色以 `toIAMRole`（`backend/handlers/permission_manage.go:240-255`）与 #2467 期望为准：`{OWNER, ADMIN, STAFF, repair_technician}`；
  - `BusinessRoleMapping`（`backend/services/role_templates.go:111-118`）是**业务展示角色**，不得作为 IAM 关系角色（#2467 根因）；
  - 对端识别：`beaconiam/internal/middleware/permission.go:52-66`（`ToUpper` 后仅认 `OWNER/ADMIN/NAMESPACE_ADMIN/CLIENT`）。
- **差异核对（实施前先跑）**：
  - `grep -rn "GetBusinessRole" backend/ --include=*.go | grep -v _test` → 修后仅剩「展示/上下文」用途，不得出现在 `BindUserToOrganization` 参数中；
  - `grep -rn "AllRoleTemplates\[\"site_member\"\]" backend/handlers/user_staff.go` → 修后 0 处（改由 `roleTemplate` 变量）。
- **损坏输入拒绝**：`users.role` 为未知码（如 `customer`）→ 回落 `site_member` 模板 + `STAFF` 关系角色（不写入未知码）；`toIAMRole` 返回非白名单值 → 一律 `STAFF`。
- **工具盲区**：fake IAM 只能证明 payload 取值，不能证明对端真实权限位；对端权限语义以 `permission_calc`（直授 ∪ 角色，按位或）为准，本单不改。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | `services.ToIAMRole` 为唯一映射实现，`handlers.toIAMRole` 变薄封装（避免跨包复制） | 本计划锁定 |
| D2 | 激活时权限模板跟随 `users.role`（**员工级**白名单：`site_admin`/`site_member`/`merchant_staff`/`repair_technician`；**排除平台级 `namespace_admin`/`sys_admin` 与 `customer`**），空/非法 → `site_member` | 本计划锁定（Rev.3：平台模板 + 员工关系角色属语义混搭，收敛为员工级；存量 `users.role=namespace_admin` 行激活后权限面变窄，属有意取舍） |
| D3 | 关系角色白名单 `{OWNER, ADMIN, STAFF, repair_technician}`，其它一律 `STAFF` | 本计划锁定 |
| D4 | `confirmation_session.go:360` 商户管理员审批确认绑定 **`OWNER`**（`toIAMRole("merchant_admin")`），删除字面量 `"admin"` | **Owner 2026-11-26 裁定**：「新建商户的管理员应该是商户的 owner」 |
| D5 | 存量已绑定为 `ADMIN`/`admin` 的商户管理员**不在本单回填**：只读扫描 + Owner 授权后执行 | 本计划锁定（另立 Ops 单） |

### OWNER vs ADMIN（对端授权面，供实现与审计对照）

| 动作 | OWNER | ADMIN | 依据 |
|---|:---:|:---:|---|
| 创建组织/子组织（建商户、建网点） | ✅ | ❌ | `cmd/api/main.go:285`、`:305`、`:370`（`PermissionOwner`） |
| 删除组织 | ✅ | ❌ | `main.go:287` |
| 定义/修改/删除功能角色模板 | ✅ | ❌ | `main.go:300-302` |
| 修改角色模板的客户权限位 | ✅ | ❌ | `main.go:311` |
| 改动其他 OWNER 的关系角色 | ✅ | ❌ | `org_handler.go:901-903` |
| 命名空间创建/删除/轮换密钥 | ✅ | ❌ | `main.go:275,278-280` |
| 组织成员 bind/unbind、改关系角色 | ✅ | ✅ | `main.go:289-292` |
| 分配成员功能角色集合 | ✅ | ✅ | `main.go:293` |
| 设置成员客户权限位（直授） | ✅ | ✅ | `main.go:312` |
| 用户 CRUD / 重置密码 / 批量导入 | ✅ | ✅ | `main.go:242-266` |
| 组织资料更新 | ✅ | ✅ | `main.go:286` |

不变量：不能移除/解绑组织**最后一个 OWNER**（`org_handler.go:914`、`:1059`）。

⇒ 「商户管理员绑定 ADMIN」会使其**无法创建网点**（建网点走 `POST /organizations`，要求 PermissionOwner）——这正是 D4 改为 OWNER 的直接理由。

### Out of Scope

- 存量商户管理员的 IAM 关系角色回填（`ADMIN`/`admin` → `OWNER`）：只读扫描 + Owner 授权后执行（另立 Ops 单）。
- 功能角色**集合语义**（降级回收）→ #2506 / #2505。
- 存量已写成 `user`/`organization_admin`/`tenant_admin` 的 IAM 关系**数据修复** → 只读扫描 + Owner 授权后执行（另立 Ops 单）。
- 其它 `GetBusinessRole` 的展示用途（JWT 上下文、前端文案）。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 三个本地用户（`role=site_member` / `site_admin` / `""`）+ 一个 `role=namespace_admin`（平台级），均无 `iam_sub` | `POST /api/users/:id/activate`（fake IAM） | `docs/evidence/2540/R1-activate-role.txt` | `MEMBER_BIND_ROLE=STAFF`、`ADMIN_BIND_ROLE=ADMIN`、`ADMIN_PERM_TEMPLATE=site_admin`、`MEMBER_ROLE_TEMPLATE=site_member`、`LEGACY_ROLE_TEMPLATE=site_member`、`PLATFORM_ROLE_TEMPLATE=site_member`、`ILLEGAL_TEMPLATE_COUNT=0`、`ROLE_TEMPLATE_MISMATCH=0`、`LEGACY_BIND_ROLE=STAFF`、`CUS_PERM_MATCH=1` | 出现 `user`/`organization_admin`/`tenant_admin`、site_admin 用 site_member 权限、或平台模板进入激活 → FAIL |
| R2 | 批量导入 2 行（`site_member`、`site_admin`）+ 1 行空角色 | 导入服务（fake IAM） | `docs/evidence/2540/R2-bulk-import-role.txt` | `BIND_ROLES=STAFF,ADMIN`、`LEGACY_BIND_ROLE=STAFF`、`BUSINESS_ROLE_AS_IAM=0` | 任一 `role` 非白名单 → FAIL |
| R3 | 修后 `backend` 源码 | grep | `docs/evidence/2540/R3-no-business-role-static.txt` | `BIND_WITH_BUSINESS_ROLE=0`、`HARDCODED_SITE_MEMBER=0` | 仍有 → FAIL |
| R6 | 商户管理员确认会话（`ActionType=merchant_admin`，商户 org 已存在） | `bash scripts/verify-2540.sh r6` | `docs/evidence/2540/R6-merchant-owner.txt` | `BIND_ROLE=OWNER`、`PERM_TEMPLATE=merchant_admin` | 仍写 `admin`/`ADMIN` → FAIL |
| R4 | `docs/spec/api/api.md` | grep | `docs/evidence/2540/R4-docs.txt` | `DOC_TOIAMROLE=1`、`DOC_STAFF_FALLBACK=1`、`DOC_ROLE_TEMPLATE=1` | 缺项 → FAIL |
| R5 | 修后工作树 | `bash scripts/verify-2540.sh r5` | `docs/evidence/2540/R5-build.txt` | `GO_TEST_OK=1`、`BUILD_OK=1` | 任一失败 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2540.sh r1` | `docs/evidence/2540/R1-activate-role.txt` | exit 0；含 `ADMIN_BIND_ROLE=ADMIN`、`LEGACY_BIND_ROLE=STAFF`、`PLATFORM_ROLE_TEMPLATE=site_member`、`ILLEGAL_TEMPLATE_COUNT=0` | PASS | 非法角色/平台模板进入 → FAIL | dev |
| R2 | `bash scripts/verify-2540.sh r2` | `docs/evidence/2540/R2-bulk-import-role.txt` | exit 0；含 `BIND_ROLES=STAFF,ADMIN`、`BUSINESS_ROLE_AS_IAM=0` | PASS | 业务角色入 payload → FAIL | dev |
| R3 | `bash scripts/verify-2540.sh r3` | `docs/evidence/2540/R3-no-business-role-static.txt` | exit 0；含 `BIND_WITH_BUSINESS_ROLE=0`、`HARDCODED_SITE_MEMBER=0` | PASS | 残留 → FAIL | dev |
| R6 | `bash scripts/verify-2540.sh r6` | `docs/evidence/2540/R6-merchant-owner.txt` | exit 0；含 `BIND_ROLE=OWNER` | PASS | 仍为 ADMIN/admin → FAIL | dev |
| R4 | `bash scripts/verify-2540.sh r4` | `docs/evidence/2540/R4-docs.txt` | exit 0；含 `DOC_TOIAMROLE=1`、`DOC_ROLE_TEMPLATE=1` | PASS | 缺文档 → FAIL | dev |
| R5 | `bash scripts/verify-2540.sh r5` | `docs/evidence/2540/R5-build.txt` | exit 0；含 `GO_TEST_OK=1`、`BUILD_OK=1`、`FULL_GATE_EXIT=1`、`FULL_GATE_NEW_FAILURES=0` | PASS | 新增失败 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2540.sh`（`r1`–`r5`），每个文本 artifact 首行：`# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`；另起一行 `GENERATED_AT_HEAD=<40 位 sha>`。
- 逐条清单（全部位于 `docs/evidence/2540/`）：
  - `docs/evidence/2540/R1-activate-role.txt`
  - `docs/evidence/2540/R2-bulk-import-role.txt`
  - `docs/evidence/2540/R3-no-business-role-static.txt`
  - `docs/evidence/2540/R6-merchant-owner.txt`
  - `docs/evidence/2540/R4-docs.txt`
  - `docs/evidence/2540/R5-build.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2540 --summary-file … --changed-files … --evidence-file docs/evidence/2540/evidence.json --report docs/evidence/2540/pre-ready-2540.json`；`evidence.json` 的 build 字段须匹配 evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Dependencies

- 无

### Workload

- Size: S（2 处调用点 + 1 个映射下沉 + 2 个测试文件 + 1 份文档 + 1 个 harness）
- Confidence: high

