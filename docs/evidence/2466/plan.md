## Accepted Plan — #2466（员工切换网点：site_members 迁移 + IAM 正确重绑 + functional_roles 重建）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 编辑员工「归属网点」只在 **org 变化**时改 `users.org_id`，**从不更新 `site_members`** | `backend/handlers/user_staff.go:926-995`（`updates["org_id"] = newOrgID`，无 site_members 写入） |
| 列表/编辑页读的是 `site_members`（`Order("sites.created_at ASC")` 取首行作主网点）→ 改完看不到变化 | `user_staff.go:88-163`（ListStaff）、`:743-769`（GetStaffUser） |
| IAM 重绑角色用 `services.GetBusinessRole(user.Role)`（业务角色）而非 IAM 角色 | `user_staff.go:967`、`services/role_templates.go:111-116`（`site_member → "user"`） |
| 切换网点分支**不重建 functional_roles**（只有「改角色」分支调 `AssignRoleTemplateToUserWithToken`） | `user_staff.go:1003-1015` |
| `site_members` 唯一键 `(tenant_id, site_id, user_id)` → 迁移到「已隶属网点」会冲突，必须按语义删除旧主行 | `backend/database/migrations/026_add_site_members.up.sql:25` |
| 站点角色的 IAM 映射已存在：`site_admin→ADMIN`、`site_member/merchant_staff→STAFF` | `backend/handlers/permission_manage.go:240-254` |
| 同 org 多网点编辑当前**完全短路**（连 `users.org_id` 都不写）→ 主网点切换失效 | `user_staff.go:954-956`（`newOrgID == oldOrgID` 时跳过整段） |

**结论**：三处根因叠加 —— D1 本地主网点未迁移、D2 IAM 角色取错映射、D3 functional_roles 未重建；且缺失败原子性语义。

### 变更点

1. `backend/handlers/user_staff.go` → `UpdateUser` 的 `req.SiteID` 分支重构：
   - 解析**目标网点**与**当前主网点**（主网点定义：`site_members` 中按 `sites.created_at ASC, site_members.created_at ASC` 的第一行，与 ListStaff/GetStaffUser 取法一致）；
   - **不管 org 是否变化**，只要「目标网点 ≠ 当前主网点」就迁移 `site_members`：
     a. 用户已隶属目标网点（存在 (user_id, target_site) 行）→ **删除旧主行**（主网点切换，不产生重复键）；
     b. 否则 → `UPDATE site_members SET site_id=<target> WHERE id=<旧主行 id>`（保留 role/roles）；
     c. 用户无 `site_members` 行（异常/直属员工误设）→ 按 `users.role`（缺省 `site_member`）**插入**目标网点行；
   - **org 变化才触发 IAM 重绑**：`Unbind(oldOrg)` → `Bind(newOrg, toIAMRole(<主行角色>))`（主行角色缺失时回落 `users.role`，再回落 `STAFF`）→ `SetUserCustomerPermissions(newOrg, …)`（按该角色模板）→ **按主行 `roles` 集合**逐个 `AssignRoleTemplateToUserWithToken(userToken, sub, newOrg, code)` 重建 functional_roles；
   - **失败语义（禁止半态）**：IAM 任一步失败 → 不写本地，尝试回滚已完成的 IAM 步骤（unbind 新 org + rebind 旧 org 用原角色/权限/模板），返回 `502/50000`（含「切换网点失败」+ 原因）；回滚失败 → `500/50000` + `CRITICAL` 日志 + 「请联系管理员人工核对 IAM 组织关系」。
   - 本地写入（`users.org_id` + `site_members` 迁移）放**同一 DB 事务**，在 IAM 全部成功后提交；本地提交失败 → 同款 IAM 回滚语义。
   - 审计日志：新增 `user.site_transfer`（`from_site` / `to_site` / `from_org` / `to_org` / `iam_role`）。
2. 不做的事：不改 `site_members` 唯一约束、不改角色模板定义、不改 `GET /staff` 数据面。
3. 文档：`docs/cases/user-management.md` 补「编辑员工切换归属网点」流程与失败语义；`docs/spec/api/api.md` 的 `PUT /users/:id` 补 `site_id` 语义（主网点迁移 + 仅 org 变化时重绑 IAM + 错误码）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 同 org 切网点：本地迁移、IAM 不动 | `backend/handlers/user_staff.go` | `PUT /users/:id` | 用户主网点 A、目标 B（同 org）；fake IAM 计数 | 后端 | `site_members.site_id=B`；`users.org_id` 不变；IAM 调用 **0** 次 | R1 |
| R2 跨 org 切网点：IAM 重绑 STAFF | `backend/handlers/user_staff.go` | 同上 | 主网点 A(org1)、目标 B(org2)；主行 role=`site_member` | 后端 | `site_members.site_id=B`、`users.org_id=org2`；IAM `unbind(org1)` → `bind(org2, role=STAFF)` 顺序；`functional_roles` 含 `site_member` | R2 |
| R3 跨 org 切网点：site_admin 不降权 | `backend/handlers/user_staff.go` | 同上 | 主行 role=`site_admin` | 后端 | `bind(org2, role=ADMIN)`；`functional_roles` 含 `site_admin`（负例：`role=user` 或空 functional_roles → FAIL） | R3 |
| R4 多网点切到已隶属网点 | `backend/handlers/user_staff.go` | 同上 | 用户隶属 A（主）、B；目标 B | 后端 | 删除旧主行 A → 行数 2→1；B 行保留；无唯一键冲突错误 | R4 |
| R5 多网点切到新网点 | `backend/handlers/user_staff.go` | 同上 | 用户隶属 A（主）、B；目标 C（新） | 后端 | A 行 `site_id=C`；B 行不变；行数仍 2 | R5 |
| R6 IAM bind 失败：无半态 | `backend/handlers/user_staff.go` | 同上 | bind 返回 500 | 后端 | HTTP 502；`site_members.site_id` 与 `users.org_id` **均不变**；发生补偿 rebind(org1)（`IAM_COMPENSATED=1`） | R6 |
| R7 functional_roles 失败：显式失败 + 补偿 | `backend/handlers/user_staff.go` | 同上 | bind 200、模板分配 500 | 后端 | HTTP 502 + message 含「切换网点失败」；`IAM_UNBIND_NEW=1`；本地不变 | R7 |
| R8 详情/列表即时反映新网点 | `backend/handlers/user_staff.go` | `GET /admin/staff/:id`、`GET /staff` | R2 完成后读取 | 后端 | 详情 `site_id=B`、`site_name=新网点`；列表该行 `site_id=B`（`LIST_SITE_MATCH=1`） | R8 |
| R9 gate 四源一致 + 全量套件（读校验） | `docs/evidence/2466/pre-ready-2466.json`、`docs/evidence/2466/plan.md` | `/work` 以 `--full-gate-cmd` 跑一次 pre-ready（含 `cd backend && go test ./handlers -count=1 -timeout 15m` 与 `./services`），随后 `plan-source.sh --issue 2466` | 修后树（干净） | 后端 | gate `result=PASS` 且含 `FULL_SUITE_EXIT=0 FAIL_COUNT=0 PANIC_COUNT=0`（P5 不得为 N/A）；`PLAN_SOURCE: PASS` | R9 |
| R10 构建回归 | `backend` | `go build ./...` + 定向测试 | 修后树 | 后端 | `BUILD_EXIT=0`、`TESTS_FAIL=0` | R10 |

## Input Truth & Tool Preflight

- **Source of truth**：`site_members` 是「用户-网点」事实源（含 `role` 与多角色 `roles` jsonb，唯一键 `(tenant_id, site_id, user_id)`）；`users.org_id` 是 IAM 组织锚；`technician_profiles`/`merchant_members` 不参与本单。
- **差异核对（预生产实况）**：`users.id=99ad41e6…` 的 `users.org_id=4684fed9`（新网点）但 `site_members.site_id=0a0fb375`（旧网点）；IAM 新关系 `role=user`、`functional_roles` 空，旧关系 `ADMIN+["site_admin"]` 已解绑 → 与代码三处根因一致。
- **主网点定义**：与既有读取口径一致（`sites.created_at ASC` 首行）；本单**不引入**新的 `is_primary` 列（避免 migration）。
- **损坏输入拒绝**：`site_id` 非法/跨租户/不存在 → 400/40001；目标网点 `status != active` → 400/40001；用户无 IAMSub → 只做本地迁移（不触 IAM，日志记录）。
- **工具盲区**：IAM 三态（unbind/bind/template）需 fake IAM 分别控制；`functional_roles` 无法从 tuneloop 本地读出，只能以 fake IAM 捕获的调用序列与参数断言（真实 IAM 侧一致性由部署侧 follow-up 实测）。
- **影响面预检**：`user_staff_site_noop_2429_test.go`（同 org / 同网点短路用例）断言仅 `users.org_id` 与 `position`，不断言行数或 IAM 调用参数 → 本单改动**不翻转**其断言；其它调用 `GetBusinessRole(user.Role)` 的位置（`user_staff.go:967,973`）在本单内统一改 `toIAMRole`。
- **运行环境**：`handlers` 包 `TestMain` + 真实测试库（每个用例独立 DB，见既有 `-run 2429` 模式）；fake IAM 用 `httptest` + `services.SetIAMInternalURLForTesting`。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 「归属网点」= `site_members` 主行（`sites.created_at ASC, site_members.created_at ASC` 首行）；不新增 `is_primary` 列 | Architect | 本计划锁定 | 阻塞实现 |
| D2 | 目标网点已隶属 → 删除旧主行（而非更新，避免唯一键冲突）；否则更新旧主行 `site_id`；无行 → 按 `users.role` 缺省 `site_member` 插入 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | IAM 重绑角色**唯一来源**：`toIAMRole(<主行角色>)`，缺省回落 `users.role`，再回落 `STAFF`；禁止再用 `GetBusinessRole` | Architect | 本计划锁定 | 阻塞实现 |
| D4 | functional_roles 按主行 `roles` 集合逐个重建；任一失败 → 整体失败（502）+ IAM 回滚，不做「部分成功」 | Architect | 本计划锁定 | 阻塞实现 |
| D5 | 同 org 仅切网点 → **不触 IAM**（保持 #2429 既有语义），只做本地迁移 | Architect | 本计划锁定 | 阻塞实现 |
| D6 | 失败原子性：IAM 先行 → 本地单事务；本地提交失败或 IAM 失败 → 回滚 IAM 到旧组织（原角色/权限/模板）；回滚失败 → 500 + CRITICAL + 人工核对提示 | Architect | 本计划锁定 | 阻塞实现 |
| D7 | 已有 `users.role` 空值不被本单回填（#2462 负责域落库）；本单仅在主行角色缺失时以 `users.role` 兜底 | Architect | 本计划锁定 | 若要求同批回填 → 回 Architect |
| D8 | 部署侧实测（预生产切换网点 → IAM `role=ADMIN/STAFF` + `functional_roles` 非空）属 Ops Follow-up | Owner / ops | 合入并部署后执行 | 关闭时创建 `[Ops-Followup of #2466]`（`waitForConfirm`） |

## Out of Scope

- `users.role` / `merchant_members.role` 域落库与编辑预填（#2462）；IAM 角色同步 JSON body（#2461）。
- 新增 `is_primary`/`sort_order` 列或改变 `site_members` 唯一约束（无 migration）。
- 多网点「新增/移除网点」的独立管理界面（只处理「归属网点切换」这一动作）。
- 直属员工（`merchant_direct`）/维修师切换商户（`merchant_members` 迁移）——本单只处理网点。
- 角色模板定义与权限位（`role_templates.go`）调整。
- 顾客端/小程序入口与展示口径（#2455）。
- 全量 `go test ./...` 作为合同行（由 R9 gate 承担）。

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|---|---|---|---|
| R1 | 租户 T；网点 A、B 同 org(O1)；用户 U 主隶属 A（role=site_member），`users.org_id=O1`；body `{site_id:B}` | `PUT /users/:id` | `docs/evidence/2466/R1-same-org.txt` |
| R2 | 网点 A(org=O1)、B(org=O2)；U 主隶属 A、`users.org_id=O1`、`users.role=site_member`；fake IAM 全 200 | 同上 | `docs/evidence/2466/R2-cross-org.txt` |
| R3 | 同 R2，主行 `role=site_admin`、`roles=["site_admin"]` | 同上 | `docs/evidence/2466/R3-admin.txt` |
| R4 | U 隶属 A(主,created 早) 与 B；body `{site_id:B}` | 同上 | `docs/evidence/2466/R4-existing-target.txt` |
| R5 | U 隶属 A(主)、B；目标 C（U 未隶属） | 同上 | `docs/evidence/2466/R5-new-target.txt` |
| R6 | 同 R2，fake IAM：unbind 200、bind 500、rebind(旧) 200 | 同上 | `docs/evidence/2466/R6-iam-fail.txt` |
| R7 | 同 R2，fake IAM：bind 200、模板分配 500、unbind(新) 200 | 同上 | `docs/evidence/2466/R7-template-fail.txt` |
| R8 | R2 执行后读取 | `GET /admin/staff/:id` + `GET /staff` | `docs/evidence/2466/R8-readback.txt` |
| R9 | 修后干净树 + `--full-gate-cmd` 产出的 gate 报告 | 读校验（`plan-source.sh` + gate 解析） | `docs/evidence/2466/R9-gate.txt` |
| R10 | 修后 backend | `go build ./...` + 定向测试 | `docs/evidence/2466/R10-build.txt` |

### Assertions

- R1：`SITE_ID=B`、`ORG_UNCHANGED=1`、`IAM_CALLS=0`；负例：`IAM_CALLS>0` 或 `site_members.site_id` 仍为 A → FAIL。
- R2：`SITE_ID=B`、`USERS_ORG=O2`、`IAM_ORDER=unbind_then_bind`、`BIND_ROLE=STAFF`、`FUNC_ROLES_CONTAINS=site_member`、`CUSPERM_SET=1`；负例：`BIND_ROLE=user`（旧映射）或 `FUNC_ROLES_CONTAINS=-` → FAIL。
- R3：`BIND_ROLE=ADMIN`、`FUNC_ROLES_CONTAINS=site_admin`；负例：`BIND_ROLE=STAFF`（降权）→ FAIL。
- R4：`ROWS_AFTER=1`、`TARGET_ROW_KEPT=1`、`UNIQUE_VIOLATION=0`；负例：`ROWS_AFTER=2` 或出现 23505 → FAIL。
- R5：`ROWS_AFTER=2`、`PRIMARY_SITE=C`、`OTHER_SITE_UNCHANGED=B`；负例：其它行被改 → FAIL。
- R6：`STATUS=502`、`SITE_UNCHANGED=1`、`ORG_UNCHANGED=1`、`IAM_COMPENSATED=1`；负例：本地被改或未补偿 → FAIL。
- R7：`STATUS=502`、`MSG_CONTAINS=切换网点失败`、`IAM_UNBIND_NEW=1`、`SITE_UNCHANGED=1`；负例：返回 200 → FAIL。
- R8：`DETAIL_SITE_ID=B`、`DETAIL_SITE_NAME=新网点`、`LIST_SITE_MATCH=1`；负例：仍返回旧网点 → FAIL。
- R9：`FULL_SUITE_EXIT=0`、`FAIL_COUNT=0`、`PANIC_COUNT=0`、`PLAN_SOURCE=PASS`、`GATE_PLAN_SHA_MATCH=1`、`P5!=NA`；负例：P5=N/A（未跑全量）或 sha 不一致 → FAIL。
- R10：`BUILD_EXIT=0`、`TESTS_FAIL=0`；负例：任一非 0 → FAIL。
- 通用 provenance：每个 artifact 首行 `# build=1.0.<fix> # commit=<sha> # dirty=false`。
- 负例（跨行）：任何 artifact 出现 `BIND_ROLE=user`、`GetBusinessRole` 作为 IAM role、或 `functional_roles` 为空 → FAIL。

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2466/R1-same-org.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2466/R2-cross-org.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2466/R3-admin.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2466/R4-existing-target.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2466/R5-new-target.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2466/R6-iam-fail.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2466/R7-template-fail.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2466/R8-readback.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2466/R9-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2466/R10-build.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## 被翻转的既有断言（AGENTS.md §语义变更，强制）

**本单不翻转任何既有断言**（逐点核对）：

| 既有断言 | 结论 | 依据 |
|---|---|---|
| `user_staff_site_noop_2429_test.go:27`（同网点保存 → 跳过 IAM） | **保持**（D5：同网点不进任何分支） | 断言只看 `users.org_id`/`position` 与 IAM 未调用 |
| `user_staff_site_noop_2429_test.go:124`（同 org 多网点编辑 → 不改 org、跳 IAM） | **保持**；本单只新增「主网点行迁移」，该用例不断言行数与 IAM 调用参数 | 已核对断言范围 |
| `user_staff_site_noop_2429_test.go:71,168`（详情返回 site/direct 标识） | **保持**（R8 只新增读回校验，不改响应字段语义） | — |
| `create_user_2068_test.go` / `staff_invite_2405_test.go` | **保持**（不涉及 UpdateUser 网点分支） | — |

## Bug Reflection Matrix

| symptom | trigger | root cause | test blind spot | same-class paths | fix layer | red-green | prevention |
|---|---|---|---|---|---|---|---|
| 切网点后列表仍显示旧网点 | 编辑员工改归属网点 | 分支只写 `users.org_id`，不迁移 `site_members` | 无跨网点用例（只有同网点/同 org 短路） | 直属员工改商户（`merchant_members`） | handler | R1/R4/R5 红绿 | D1/D2 主网点语义 |
| IAM 新组织角色变 `user`、functional_roles 空 | 跨 org 切网点 | `GetBusinessRole` 当 IAM 角色；模板只在改角色分支重建 | 无 IAM 重绑参数断言 | 角色变更分支（已用 toIAMRole） | handler | R2/R3 红绿 | D3/D4 |
| 网点管理员迁移后失去管理权 | site_admin 切网点 | 同 org 短路 + 模板未重建 | 无 admin 迁移用例 | 多角色（#2035） | handler | R3 红绿 | 模板按 `roles` 重建 |
| 迁移到已隶属网点报唯一键错误 | 多网点员工切换 | 未处理 `(tenant_id,site_id,user_id)` 冲突 | 无多网点迁移用例 | 邀请/添加成员路径 | handler | R4 红绿 | D2 删除旧主行 |
| IAM 成功但本地失败 → 半态 | 任一切网点 | 无事务/补偿 | 无失败注入用例 | 换商户管理员（#2461 已补偿） | handler | R6/R7 红绿 | D6 回滚语义 |
| 失败被静默 | IAM 500 | 仅 log 后继续 | 无失败可见性断言 | site_member/merchant 角色同步（#2461） | handler | R6/R7 红绿 | 显式 502 + CRITICAL |
| 列表/详情口径不一致 | 读回 | 数据未迁移导致读端错位 | 无读回断言 | #2429 详情口径 | handler | R8 红绿 | 迁移后读回校验 |
| 全量回归未跑 | ready 前只跑定向 | 计划未登记 gate 行 | — | #2461/#2462 同类 | 计划/流程 | R9 红绿 | R9 强制全量 gate |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2466.sh r1` | `docs/evidence/2466/R1-same-org.txt` | exit 0；grep `SITE_ID=B`、`IAM_CALLS=0`、`ORG_UNCHANGED=1` | PASS | `IAM_CALLS>0` 或站点未变 → FAIL | dev |
| R2 | `bash scripts/verify-2466.sh r2` | `docs/evidence/2466/R2-cross-org.txt` | exit 0；grep `BIND_ROLE=STAFF`、`IAM_ORDER=unbind_then_bind`、`FUNC_ROLES_CONTAINS=site_member` | PASS | `BIND_ROLE=user` → FAIL | dev |
| R3 | `bash scripts/verify-2466.sh r3` | `docs/evidence/2466/R3-admin.txt` | exit 0；grep `BIND_ROLE=ADMIN`、`FUNC_ROLES_CONTAINS=site_admin` | PASS | `BIND_ROLE=STAFF` → FAIL | dev |
| R4 | `bash scripts/verify-2466.sh r4` | `docs/evidence/2466/R4-existing-target.txt` | exit 0；grep `ROWS_AFTER=1`、`UNIQUE_VIOLATION=0` | PASS | `ROWS_AFTER=2` → FAIL | dev |
| R5 | `bash scripts/verify-2466.sh r5` | `docs/evidence/2466/R5-new-target.txt` | exit 0；grep `ROWS_AFTER=2`、`PRIMARY_SITE=C` | PASS | 其它行被改 → FAIL | dev |
| R6 | `bash scripts/verify-2466.sh r6` | `docs/evidence/2466/R6-iam-fail.txt` | exit 0；grep `STATUS=502`、`SITE_UNCHANGED=1`、`IAM_COMPENSATED=1` | PASS | 本地被改 → FAIL | dev |
| R7 | `bash scripts/verify-2466.sh r7` | `docs/evidence/2466/R7-template-fail.txt` | exit 0；grep `STATUS=502`、`MSG_CONTAINS=切换网点失败`、`IAM_UNBIND_NEW=1` | PASS | 返回 200 → FAIL | dev |
| R8 | `bash scripts/verify-2466.sh r8` | `docs/evidence/2466/R8-readback.txt` | exit 0；grep `DETAIL_SITE_ID=B`、`LIST_SITE_MATCH=1` | PASS | 仍返回旧网点 → FAIL | dev |
| R9 | `bash scripts/verify-2466.sh r9` | `docs/evidence/2466/R9-gate.txt` | exit 0；grep `FULL_SUITE_EXIT=0`、`FAIL_COUNT=0`、`PLAN_SOURCE=PASS` | PASS | `P5=NA`（未跑全量）→ FAIL | dev |
| R10 | `bash scripts/verify-2466.sh r10` | `docs/evidence/2466/R10-build.txt` | exit 0；grep `BUILD_EXIT=0`、`TESTS_FAIL=0` | PASS | 任一非 0 → FAIL | dev |

### Workload

- Size: M
- Basis: 1 个后端实现文件（`user_staff.go` 网点分支重构）+ 1 个后端测试文件（6 类场景 + 失败注入）+ 1 个 verify 脚本 + 2 篇文档 + 10 条证据行；无 migration、无前端改动、无权限模板改动
- Confidence: medium（IAM 三态需 fake 精确编排；functional_roles 只能在 fake 侧断言）

### Files（变更文件清单）

- `backend/handlers/user_staff.go`
- `backend/handlers/user_site_transfer_2466_test.go`
- `scripts/verify-2466.sh`
- `docs/cases/user-management.md`
- `docs/spec/api/api.md`
- `docs/evidence/2466/plan.md`
- `docs/evidence/2466/R1-same-org.txt`
- `docs/evidence/2466/R2-cross-org.txt`
- `docs/evidence/2466/R3-admin.txt`
- `docs/evidence/2466/R4-existing-target.txt`
- `docs/evidence/2466/R5-new-target.txt`
- `docs/evidence/2466/R6-iam-fail.txt`
- `docs/evidence/2466/R7-template-fail.txt`
- `docs/evidence/2466/R8-readback.txt`
- `docs/evidence/2466/R9-gate.txt`
- `docs/evidence/2466/pre-ready-2466.json`
- `docs/evidence/2466/R10-build.txt`
- `docs/evidence/2466/evidence.json`

## Dependencies

- 无

### 依赖与关联说明（非硬依赖）

- **#2462（关联）**：`users.role` 域落库；本单以 **`site_members.role`（主行）** 为 IAM 角色来源，因此不依赖其是否已合入；若已合入，额外断言 `users.role` 与主行角色一致（R2/R3 的可选子断言）。
- **#2461（关联）**：角色同步 JSON body；本单的 IAM 调用走 `BindUserToOrganization` / `AssignRoleTemplateToUserWithToken`，不受其影响。
- **#2467（已关闭）**：本单 D2 即其全部内容（已并入本单）。
- **#2429 / #2438**：R4 第 4 项「切换网点 → 归属变更」实测 FAIL 的上游 Issue；本单为其收口。
- **部署侧**：预生产实测（切网点后 IAM `role=ADMIN/STAFF` + `functional_roles` 非空 + 列表即时显示新网点）需部署后执行 → 关闭时创建 `[Ops-Followup of #2466]`（`waitForConfirm`，无 `status:*`）。

*Model: DeepSeek V4.1 Flash 1017*

