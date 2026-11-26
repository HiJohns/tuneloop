## Accepted Plan — #2527 归属缺失行的批量重置密码与 IAM 归属推导（#2526 后续）｜Rev.2

> Tags: bug, code:go, api-contract, scope:pc
> 前置（硬依赖）：#2526（可见性判定 `personnelUserVisible` / `loadVisibleStaffUser`、`attribution_missing`）
> 关联：#2389（重置密码空邮箱显式 400）、#2375（`UpdateUser` 角色白名单 + `role_errors`）、#2466（网点调动 IAM 重绑与补偿）、#2495（被阻塞的验收对象）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 批量重置密码按 `id IN ? AND tenant_id = ?` 取行 | `backend/handlers/user_staff.go:1905-1912` |
| 取不到行时不报错：`iamSubs` 为空 → `20000 sent=0 skipped=N`，用户侧看不到任何提示（静默无操作） | `backend/handlers/user_staff.go:1926-1946` |
| 角色变更用 `existingUser.OrgID`（为零时回落 `tenantID`）作 IAM org | `backend/handlers/user_staff.go:1201-1216` |
| 网点调动用 `existingUser.OrgID` 作 `oldOrg`，nil UUID 会直接发给 IAM unbind | `backend/handlers/user_staff.go:1180-1200`、`:770-935`（`performSiteTransfer`） |
| 归属缺失（`users.tenant_id`/`org_id` 为 nil UUID）的存量行已由 #2526 修好详情/保存/激活，但上述两项仍是坏的 | #2526、#2527 正文 |
| `users.tenant_id` 是本地缓存（不可用于过滤）、membership 才是归属权威 | #2249 S1（`backend/handlers/iam_proxy.go:617-625`）、#2067（`backend/handlers/personnel.go:142-145`） |
| 测试可用 fake IAM（`services.SetIAMInternalURLForTesting`）断言 IAM 收到的 org/id | `backend/handlers/user_site_transfer_2466_test.go:108-138` |

**根因**：与 #2526 同源，但落在两条**写路径**上——(a) 批量重置密码用 `users.tenant_id` 过滤，归属缺失行被静默丢弃且响应仍是 `20000`；(b) IAM 调用直接消费 `users.org_id`，归属缺失时把 nil UUID 当组织发给 IAM，失败信息被折叠成 502 或 `role_errors` 警告，用户看不出原因。

### 修复设计（最小、可测、语义显式）

1. **批量重置密码（`ResetPassword`，`user_staff.go:1900`）**
   - 取行改 `database.ExplicitTenantScopeOnly(ctx)` 下的 `id IN ? AND deleted_at IS NULL`；
   - 逐行过 #2526 的 `personnelUserVisible`（复用，不复制谓词），分成 `visible` / `skipped_invisible` / `skipped_missing`；
   - 可见集合内沿用 #2389 口径：全部无邮箱 → `40002`；部分无邮箱 → 记入 `skipped_no_email`；
   - 响应新增 `skipped_invisible` / `skipped_missing`（`sent` / `skipped` 保持既有 IAM 语义不变）；**任一分类非空时不得静默**。
2. **IAM 归属推导（`UpdateUser`，`user_staff.go:1052`）**
   - 新增纯辅助 `effectiveOrgID(db, ctx, user) (string, error)`：`users.org_id` 非空且非 nil UUID 时原样返回；否则按 `site_members → sites.org_id`（优先与 `users.org_id` 匹配的主行，其次最早 `created_at`）推导，取不到返回空串；
   - 角色变更：以 `effectiveOrgID` 作 IAM org；推导为空 → **不调 IAM**，把「account attribution missing」写进既有 `role_errors`（沿用 #2375 D3 非阻断语义，但必须可见）；
   - 网点调动：`performSiteTransfer` 的 `oldOrg` 同样取 `effectiveOrgID`（只用于 IAM 参数，**不回写** `users.org_id`）；推导为空 → `40002`「账号归属缺失，请先补全成员关系」并中止，禁止把 nil UUID 发给 IAM。
3. **PC 提示（`frontend-pc/src/pages/StaffManagement.jsx` 的 `handleResetPassword`）**：按响应分类给出 `message.warning`（哪些被跳过、为什么跳过），判定抽成纯函数 `resetPasswordSummary(data)` 放 `frontend-pc/src/utils/resetPasswordSummary.js`（PC 无组件测试运行器，`node --test` 覆盖，D5）。
4. **不做**：不改 `users.org_id` 落库（回填需 Owner 授权，见 #2527 T3）；不改 `personnelUserVisible`（#2526 单一判定源）。

### 语义变更（必须翻转的既有断言，强制清单）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `frontend-pc/src/pages/StaffManagement.jsx:handleResetPassword`（无单测） | 只用 `res.data.sent` / `res.data.skipped` 判断成功 | 追加读 `skipped_invisible` / `skipped_missing` / `skipped_no_email` 并给出提示 | 响应新增分类字段后必须消费，否则仍是静默 |
| `backend/handlers/user_staff.go:1926-1946` | 不可见行被过滤 → `20000 sent=0` 且响应无解释 | `20000 sent=0` + `skipped_invisible>0` 显式分类 | 静默吞错是红线（`AGENTS.md` §红线禁令） |
| `backend/handlers/reset_password_no_email_2389_test.go:49`（`TestResetPasswordNoEmail_2389`） | fixture 用户 `Role=""`、无任何 membership；期望 `400 / 40002`（无邮箱） | **fixture 改为可见成员**（`role=site_member` + `site_members(siteA)` + 空邮箱）→ 仍期望 `40002`；并**新增**用例：同一批里放入纯顾客（`Role=""`、无 membership）→ `20000` 且 `skipped_invisible=1`、不返回 `40002` | #2526 的可见性判定在「无邮箱」之前执行：纯顾客属不可见集合，直接进 `skipped_invisible`，永不到达 #2389 的 40002 分支（Investigation Packet 实证 `EXISTING_2389_PASS=0`） |

> Rev.2 更正：v1 声称「`ResetPassword` 既有用例期望值不变」不成立——`TestResetPasswordNoEmail_2389` 的夹具是纯顾客，在新可见性判定下被跳过。本单必须同批修改该用例（如上行），否则 R8 永远红。

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志） |
|---|---|---|---|---|---|---|
| R1 主诉：归属缺失行可重置密码 | `backend/handlers/user_staff.go` | `POST /users/reset-password`（actor merchant_admin，fake IAM） | 零租户成员（有邮箱）+ `site_members` 在 `siteA` | backend | `STATUS=200`、`SENT=1`、`IAM_IDS=1`、`SKIPPED_INVISIBLE=0` | R1 |
| R2 混合批次显式分类（不再静默丢弃） | `backend/handlers/user_staff.go` | 同上，`user_ids=[零租户成员, 他商户成员]` | 1 可见 + 1 跨租户 | backend | `STATUS=200`、`IAM_IDS=1`、`SKIPPED_INVISIBLE=1`、`IAM_INCLUDES_CROSS_TENANT=0` | R2 |
| R3 全不可见：空操作必须显式 | `backend/handlers/user_staff.go` | 同上，`user_ids=[他商户成员]` | 0 可见 | backend | `STATUS=200`、`SENT=0`、`SKIPPED_INVISIBLE=1`、`IAM_CALLS=0` | R3 |
| R4 PC 提示接线（纯函数 + 静态接线） | `frontend-pc/src/pages/StaffManagement.jsx`、`frontend-pc/src/utils/resetPasswordSummary.js` | `node --test frontend-pc/src/utils/resetPasswordSummary.test.js` + 源码结构断言 | `{sent,skipped_invisible,skipped_missing,skipped_no_email}` 四类组合 fixture | PC | `NODE_TEST_RC=0`、`RENDER_WIRED=1`、`FRONTEND_LINT_RC=0`、`FRONTEND_BUILD_RC=0`（`evidence_type: static`，Owner 豁免见 D5） | R4 |
| R5 角色变更使用派生 org | `backend/handlers/user_staff.go` | `PUT /users/:id`（`role=site_member`，fake IAM） | 零租户成员，`site_members→sites.org_id=orgA` | backend | `STATUS=200`、`IAM_ORG=orgA`、`NIL_UUID_CALLS=0`、`ROLE_ERRORS=0` | R5 |
| R6 网点调动使用派生 org | `backend/handlers/user_staff.go` | `PUT /users/:id`（`site_id=B`，fake IAM） | 零租户成员在 A（orgA），目标 B（orgB） | backend | `STATUS=200`、`UNBIND_ORG=orgA`、`BIND_ORG=orgB`、`NIL_UUID_CALLS=0` | R6 |
| R7 无法推导归属：显式拒绝，不发 nil UUID | `backend/handlers/user_staff.go` | 同上两条路径 | 归属缺失且**无任何 membership** 的用户 | backend | 调动 `STATUS=40002`；角色变更 `STATUS=200` 且 `ROLE_ERRORS_ATTRIBUTION=1`；两者 `NIL_UUID_CALLS=0` | R7 |
| R8 回归 + 构建 | `backend/handlers/user_staff.go`、`backend/handlers/reset_password_no_email_2389_test.go` | `go build ./...` + `ResetPassword`/`UpdateUser`/调动 既有用例（含改写后的 2389 用例） | 修后工作树 | backend | `BUILD_RC=0`、`REGRESSION_PASS=1`、`FAIL_COUNT=0`、`EXISTING_2389_PASS=1`、`INVISIBLE_CUSTOMER_SKIPPED=1`，含 `--- PASS: TestSiteTransfer_Readback_2466` | R8 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R4 | static | PC 无组件测试运行器（`frontend-pc/package.json` 仅 dev/build/lint/preview）→ 纯函数 `node --test` + 源码结构断言；Owner 豁免见 D5 |

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 零租户成员（`tenant_id`/`org_id` = nil UUID，有 email，`site_members(siteA)`） | `bash scripts/verify-2527.sh r1` | `docs/evidence/2527/R1-reset-visible.txt` | `STATUS=200`、`SENT=1`、`IAM_IDS=1`、`SKIPPED_INVISIBLE=0` | `IAM_IDS=0`（行仍被过滤）→ FAIL |
| R2 | 同夹具 + 他商户成员 | `bash scripts/verify-2527.sh r2` | `docs/evidence/2527/R2-reset-mixed.txt` | `STATUS=200`、`IAM_IDS=1`、`SKIPPED_INVISIBLE=1`、`IAM_INCLUDES_CROSS_TENANT=0` | `IAM_INCLUDES_CROSS_TENANT=1` → 越权重置 → FAIL |
| R3 | 仅他商户成员 | `bash scripts/verify-2527.sh r3` | `docs/evidence/2527/R3-reset-all-invisible.txt` | `STATUS=200`、`SENT=0`、`SKIPPED_INVISIBLE=1`、`IAM_CALLS=0` | 响应缺 `skipped_invisible`（仍静默）→ FAIL |
| R4 | `resetPasswordSummary` 四类组合 + `StaffManagement.jsx` 接线 | `bash scripts/verify-2527.sh r4` | `docs/evidence/2527/R4-pc-wiring.txt` | `NODE_TEST_RC=0`、`RENDER_WIRED=1`、`FRONTEND_LINT_RC=0`、`FRONTEND_BUILD_RC=0` | `RENDER_WIRED=0`（新字段无消费方）→ FAIL |
| R5 | 零租户成员（`site_members→sites.org_id=orgA`） | `bash scripts/verify-2527.sh r5` | `docs/evidence/2527/R5-role-derived-org.txt` | `STATUS=200`、`IAM_ORG=orgA`、`NIL_UUID_CALLS=0`、`ROLE_ERRORS=0` | `NIL_UUID_CALLS>0` → 仍把 nil UUID 发给 IAM → FAIL |
| R6 | 零租户成员在 A、目标网点 B（orgB） | `bash scripts/verify-2527.sh r6` | `docs/evidence/2527/R6-transfer-derived-org.txt` | `STATUS=200`、`UNBIND_ORG=orgA`、`BIND_ORG=orgB`、`NIL_UUID_CALLS=0` | `UNBIND_ORG` 为 nil UUID 或 `STATUS=502` → FAIL |
| R7 | 归属缺失且无 membership | `bash scripts/verify-2527.sh r7` | `docs/evidence/2527/R7-no-attribution.txt` | `TRANSFER_STATUS=40002`、`ROLE_STATUS=200`、`ROLE_ERRORS_ATTRIBUTION=1`、`NIL_UUID_CALLS=0` | `TRANSFER_STATUS=502`（未显式拒绝）或 `NIL_UUID_CALLS>0` → FAIL |
| R8 | 修后工作树 + 既有重置密码/调动/角色用例（含改写后的 `TestResetPasswordNoEmail_2389`） | `bash scripts/verify-2527.sh r8` | `docs/evidence/2527/R8-regression.txt` | `BUILD_RC=0`、`REGRESSION_PASS=1`、`FAIL_COUNT=0`、`EXISTING_2389_PASS=1`、`INVISIBLE_CUSTOMER_SKIPPED=1`、含 `--- PASS: TestSiteTransfer_Readback_2466` | 任一既有用例 FAIL 或 2389 用例未改写（仍按纯顾客断言 400）→ FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2527.sh r1` | `docs/evidence/2527/R1-reset-visible.txt` | exit 0；grep `STATUS=200`、`SENT=1`、`IAM_IDS=1`、`SKIPPED_INVISIBLE=0` | PASS | `IAM_IDS=0` → 行仍被过滤 → FAIL | dev |
| R2 | `bash scripts/verify-2527.sh r2` | `docs/evidence/2527/R2-reset-mixed.txt` | exit 0；grep `STATUS=200`、`IAM_IDS=1`、`SKIPPED_INVISIBLE=1`、`IAM_INCLUDES_CROSS_TENANT=0` | PASS | `IAM_INCLUDES_CROSS_TENANT=1` → 越权 → FAIL | dev |
| R3 | `bash scripts/verify-2527.sh r3` | `docs/evidence/2527/R3-reset-all-invisible.txt` | exit 0；grep `STATUS=200`、`SENT=0`、`SKIPPED_INVISIBLE=1`、`IAM_CALLS=0` | PASS | 缺少 `SKIPPED_INVISIBLE=1` → 仍静默 → FAIL | dev |
| R4 | `bash scripts/verify-2527.sh r4` | `docs/evidence/2527/R4-pc-wiring.txt` | exit 0；grep `NODE_TEST_RC=0`、`RENDER_WIRED=1`、`FRONTEND_LINT_RC=0`、`FRONTEND_BUILD_RC=0` | PASS | `RENDER_WIRED=0` → 字段无消费方 → FAIL | dev |
| R5 | `bash scripts/verify-2527.sh r5` | `docs/evidence/2527/R5-role-derived-org.txt` | exit 0；grep `STATUS=200`、`IAM_ORG=orgA`、`NIL_UUID_CALLS=0`、`ROLE_ERRORS=0` | PASS | `NIL_UUID_CALLS=1` → 仍发 nil UUID → FAIL | dev |
| R6 | `bash scripts/verify-2527.sh r6` | `docs/evidence/2527/R6-transfer-derived-org.txt` | exit 0；grep `STATUS=200`、`UNBIND_ORG=orgA`、`BIND_ORG=orgB`、`NIL_UUID_CALLS=0` | PASS | `STATUS=502` → 调动仍失败 → FAIL | dev |
| R7 | `bash scripts/verify-2527.sh r7` | `docs/evidence/2527/R7-no-attribution.txt` | exit 0；grep `TRANSFER_STATUS=40002`、`ROLE_STATUS=200`、`ROLE_ERRORS_ATTRIBUTION=1`、`NIL_UUID_CALLS=0` | PASS | `TRANSFER_STATUS=502` → 未显式拒绝 → FAIL | dev |
| R8 | `bash scripts/verify-2527.sh r8` | `docs/evidence/2527/R8-regression.txt` | exit 0；grep `BUILD_RC=0`、`REGRESSION_PASS=1`、`FAIL_COUNT=0`、`EXISTING_2389_PASS=1`、`INVISIBLE_CUSTOMER_SKIPPED=1`、`--- PASS: TestSiteTransfer_Readback_2466` | PASS | 任一既有用例 FAIL、`EXISTING_2389_PASS=0` 或 `BUILD_RC=1` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2527/R1-reset-visible.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2527/R2-reset-mixed.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2527/R3-reset-all-invisible.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2527/R4-pc-wiring.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2527/R5-role-derived-org.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2527/R6-transfer-derived-org.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2527/R7-no-attribution.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2527/R8-regression.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 symptom | 触发条件 trigger | 根因 root cause | 测试盲区 test blind spot | 同类路径 same-class | 修复层 fix layer | 红绿 red-green | 预防 prevention |
|---|---|---|---|---|---|---|---|---|
| B1 | 勾选归属缺失行点「重置密码」没有任何反应 | 行被 `tenant_id` 过滤掉 | 批量取行用 `users.tenant_id` | 无零租户行重置用例 | `ResetPassword` | handler 取行口径 | R1（IAM 收到该 id） | R1 断言 IAM 实际收到的 id 集合 |
| B2 | 服务端返回 200 但部分行被静默丢弃 | 混合批次含越权行 | 响应只有 `sent`/`skipped`，无分类 | 无混合批次用例 | 任何批量操作 | handler 响应契约 | R2/R3 显式分类 | 「空操作必须显式」进合同 |
| B3 | 改角色时 IAM 报错但界面只显示「已保存」 | `users.org_id` 为 nil UUID | IAM org 直接取本地缓存列 | 无归属缺失 × 角色变更用例 | `UpdateUserRoleInOrgWithToken`、`AssignRoleTemplateToUserWithToken` | handler IAM 参数派生 | R5 `IAM_ORG=orgA` | 断言 IAM 收到的 org 值，而非只看 HTTP 200 |
| B4 | 改网点 502「切换网点失败」 | 同上，unbind 收到 nil UUID | 同上 | 无归属缺失 × 调动用例 | `performSiteTransfer` | handler IAM 参数派生 | R6 `UNBIND_ORG=orgA` | `NIL_UUID_CALLS` 计数器进断言 |
| B5 | 无法推导归属时把 nil UUID 继续发给 IAM | 无 membership 的脏行 | 缺少「无归属」显式分支 | 无该分支用例 | 同上两处 | handler 显式拒绝/显式警告 | R7 `TRANSFER_STATUS=40002` | 负例矩阵固定进合同 |
| B6 | 修复顺手写库「自愈」归属 | 想顺便回填 | 把数据修复当代码修复 | 无授权闸门 | 任何 DB 写入 | 只派生、不回写 | R5/R6 断言 `users.org_id` 未变（D4） | Decision Lock D4 落字 |
| B7 | 前端不消费新字段（静默仍在） | 只改后端响应 | 前后端契约不同步 | 无接线断言 | 其他「新增响应字段」改动 | PC 提示 + 纯函数测试 | R4 `RENDER_WIRED=1` | 新字段必须同单落地消费方 |
| B8 | 可见性谓词被复制第三份，再次分裂 | 依赖 #2526 的判定但手抄一份 | 图省事复制 SQL | 无「谓词唯一」断言 | #2526 的 `personnelUserVisible` | 直接复用 #2526 函数 | R1/R2 通过复用函数自然继承语义 | 计划显式要求复用（D2） |
| B9 | 修好批量重置却漏掉单人重置/其他 by-id 写路径 | 同根因多入口 | 未做邻居检查 | 无入口矩阵 | `/users/:id/activate`（#2526 已修）、`/users/:id` 角色同步 | 逐入口覆盖 | R5/R6/R7 覆盖写路径 | 邻居扫描结论落本单范围表 |
| B10 | 归属缺失行调动失败被当成「网络问题」反复重试 | 502 文案与原因不符 | 错误未带业务原因 | 无文案断言 | #2466 补偿路径 | handler 显式 40002 + 文案 | R7 `TRANSFER_STATUS=40002` | 40002 语义进 api.md |

### Workload

- Size: M
- Basis: 1 个后端实现文件（`user_staff.go` 两处写路径 + 派生辅助）+ 1 个后端测试文件 + 1 个验证脚本 + 2 个 PC 文件（提示接线 + 纯函数及其测试）+ 1 份契约文档；无 migration、无新依赖
- Confidence: medium（依赖 #2526 的 `personnelUserVisible` 与 fake IAM 夹具按预期可用）

### Files（变更文件清单）

- `backend/handlers/user_staff.go`
- `backend/handlers/reset_password_no_email_2389_test.go`
- `backend/handlers/reset_password_scope_2527_test.go`
- `frontend-pc/src/pages/StaffManagement.jsx`
- `frontend-pc/src/utils/resetPasswordSummary.js`
- `frontend-pc/src/utils/resetPasswordSummary.test.js`
- `scripts/verify-2527.sh`
- `docs/spec/api/api.md`
- `docs/evidence/2527/plan.md`
- `docs/evidence/2527/evidence.json`
- `docs/evidence/2527/pre-ready-2527.json`
- `docs/evidence/2527/R1-reset-visible.txt`
- `docs/evidence/2527/R2-reset-mixed.txt`
- `docs/evidence/2527/R3-reset-all-invisible.txt`
- `docs/evidence/2527/R4-pc-wiring.txt`
- `docs/evidence/2527/R5-role-derived-org.txt`
- `docs/evidence/2527/R6-transfer-derived-org.txt`
- `docs/evidence/2527/R7-no-attribution.txt`
- `docs/evidence/2527/R8-regression.txt`

## Input Truth & Tool Preflight

- **Source of truth**：可见性判定 = #2526 的 `personnelUserVisible`（唯一判定源）；归属权威 = membership（`site_members → sites.org_id`，`sites.id ≠ sites.org_id`，见 #2508）；`users.tenant_id/org_id` 仅作缓存。
- **差异核对（实施前先跑，结果入 R8 artifact）**：
  - `grep -n "personnelUserVisible" backend/handlers/user_staff.go` → 修复后必须 ≥ 2（重置密码 + #2526 既有调用）且无复制实现。
  - `grep -n "existingUser.OrgID" backend/handlers/user_staff.go` → 修复后 IAM 参数处不得直接使用（必须经 `effectiveOrgID`）。
  - `grep -c "00000000-0000-0000-0000-000000000000" backend/handlers/user_staff.go` → 修复后不得新增字面量（统一用 #2526 的 `nilUUID` 常量/`isMissingAttribution`）。
- **损坏输入拒绝**：`user_ids` 空/非数组 → `40001`（既有）；全部不可见 → `20000` + `skipped_invisible`（不得 500、不得静默）；无归属且需 IAM → 调动 `40002`、角色变更 `role_errors`；`iam_sub` 缺失的可见行 → 计入 `skipped_missing` 而非静默丢弃。
- **工具盲区（显式声明）**：
  - fake IAM 通过 `services.SetIAMInternalURLForTesting` 生效，必须 `t.Cleanup` 复位（沿用 #2466 夹具），否则污染同包其他用例。
  - 测试依赖真实 Postgres；`TUNELOOP_TEST_ALLOW_SKIP` 不得设置，skip 视为 FAIL。
  - PC 无组件测试运行器 → R4 用纯函数 `node --test` + 源码结构断言（Owner 豁免 D5）。
  - 预生产真机回评不在本机能力内（Ops-Followup）。
- **影响面预检**：`effectiveOrgID` 只被 `UpdateUser` 的两条 IAM 分支调用；`ResetPassword` 响应为**新增字段**（旧字段语义不变，PC 旧版本仍可工作）；`performSiteTransfer` 只在传参层面变化，补偿逻辑（#2466）不动。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 重置密码响应新增 `skipped_invisible` / `skipped_missing`；`sent` / `skipped` 保持 IAM 原义 | Architect | 本计划锁定 | 若要求合并成单一 `skipped` → 回 Architect（会丢失原因分类） |
| D2 | 可见性**复用** #2526 的 `personnelUserVisible`，不复制谓词 | Architect | 本计划锁定（依赖 #2526） | 若 #2526 未落地 → HOLD（不得手抄 SQL） |
| D3 | IAM org 由 membership 派生，仅在参数层使用 | Architect | 本计划锁定 | 若要求「无归属即拒绝所有写操作」→ 回 Architect（影响 #2375 非阻断语义） |
| D4 | **不回写** `users.org_id/tenant_id`（回填需 Owner 授权，见 #2527 T3） | Owner / ops | 本计划锁定 | 未授权 → 保持 `attribution_missing=true` |
| D5 | R4 UI 证据形态：纯函数 `node --test` + 源码结构断言（PC 无组件测试运行器） | Architect | Owner 豁免（沿用 #2480/#2483 惯例） | 若审计要求 render → 另立前端测试基建 Issue |
| D6 | 无归属且需 IAM 时：调动 40002（阻断）、角色变更 `role_errors`（非阻断） | Architect | 本计划锁定 | 若要求角色变更也阻断 → 回 Architect（与 #2375 D3 冲突） |

## Out of Scope

- 存量数据回填（DB 写入）与预生产只读清单执行 → 本单 T3 / Owner 授权；AI 不改库。
- `UpdateUser` 的手机号/邮箱冲突检查跨租户口径（现状 `users.tenant_id = tid`）→ 另立 Issue，本单不改唯一性语义。
- #2526 已修的详情/保存/激活可见性（本单只消费其结果）。
- IAM 侧角色模板与组织关系的存量纠偏（依赖回填）。
- 错误文案本地化（#2504 家族）。
- 预生产真机走查与截图（归 Ops-Followup）。

## Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` §10.18（人员管理） | `POST /users/reset-password` 的响应只写 `sent`/`skipped`；`PUT /users/:id` 未写 IAM org 取值口径 | ① `reset-password` 响应补 `skipped_invisible` / `skipped_missing` / `skipped_no_email` 语义与示例；② 新增「IAM 归属推导」段：`users.org_id` 缺失时按 `site_members → sites.org_id` 派生且不回写；无法派生时调动 `40002`、角色变更写入 `role_errors` | 静默丢弃之所以能长期存在，是因为契约没写「跳过必须分类」；IAM 参数口径也必须落字，否则下次仍会直传 nil UUID |

## Dependencies

- #2526

### 关联（非硬依赖）

- `#2389`：重置密码空邮箱 → 显式 `40002` 口径，本单沿用。
- `#2375` / `#2466`：角色非阻断 `role_errors` 与调动补偿路径，本单只改 IAM 入参。
- `#2508`：`sites.id ≠ sites.org_id` 的 ID 域结论，派生 org 时复用。
- `#2495`：其验收对象依赖本单 + #2526 一起才能完成双角色账号的完整编辑链路。

*Model: DeepSeek V4.1 Flash 1017*


