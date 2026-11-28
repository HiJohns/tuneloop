## Accepted Plan — #2626 用户角色权威与术语收敛（Owner 2026-11-28 裁定：IAM 权威 + 一人多角色 + 存储层用 IAM 术语）｜Rev.2（采纳 Packet A：R3 降级为静态+ops 回执；R6 收敛；db.go 登记）｜Rev.3（Files +3、R2 扩至 5 写侧+allowlist、R4 数据源行、既有断言、D1 过渡兜底）
> **Rev.3 说明（按 Round 1 audit 逐条回填）**：
> ① `### Files` 追加 `backend/handlers/merchant.go`、`backend/handlers/iam_proxy.go`、`frontend-pc/tests/staffForm.test.js`；
> ② **R2 扫描面 3 → 5 个写侧**（`iam_callback.go` / `merchant_member.go` / `user_staff.go` / `merchant.go` / `iam_proxy.go`），`WRITE_SITES=5`，并新增 **allowlist**（`role_derive.go` 内的优先级表与测试字面量豁免）与负例「allowlist 外仍出现内联 `site_member` 字面量 → FAIL」；
> ③ **R4 增数据源行**：明确 `roles[]` 来源 = IAM `functional_roles` 镜像（不是页面自造），断言 `ROLES_SOURCE=iam`；
> ④ 新增 **`### 既有断言影响`** 段（列出被翻转的断言）；
> ⑤ **D1 追加过渡兜底**：镜像列尚未回填完时，读侧回退链 = `users.roles` → IAM 关系（实时）→ `users.role`，且**只允许该回退存在于读侧**，写侧不得据此回写。
> **Rev.2 说明（采纳 Investigation Packet option A）**：
> ① `### Files` 追加 `backend/database/db.go`（`defaultMigrationArtifacts` 登记）、`backend/database/db_artifacts_2626_test.go`；
> ② **R3 改为静态可判定**：断言 up.sql 含 `ADD COLUMN IF NOT EXISTS roles jsonb NOT NULL DEFAULT '[]'` 与列注释、down.sql 含 `DROP COLUMN IF EXISTS roles`、`defaultMigrationArtifacts` 登记 `{20261128001,"users","roles"}`、`models.User.Roles` 存在；**真实 up/down 执行**转 `waitForConfirm`（Owner 指定库与清理方式）；
> ③ **回填不写猜测值**：`BACKFILL_MATCHES_IAM` → **`BACKFILL_NO_GUESS=1`**（迁移只加列，不在 SQL 内跨库推导角色）；
> ④ **R6 收敛**：本单只断言「**不引入单值判据**」（`RECON_SINGLE_ONLY=0`），对账多值口径由 #2623 合入后复核（`RECON_ROLE_MULTI` 移出本单）。

### 背景与 Owner 裁定

`users.role` 目前由三处写侧各自赋值，口径互斥；且它把「结构角色 × 组织级别」压平成一个复合标签（`site_admin` / `merchant_staff`），**单值字段装不下多角色**——预生产实测 16 行 `ROLE_MISMATCH`、6 行多重功能角色（详见本单 2026-11-28 的修复前清单评论）。

| ID | Owner 裁定 | 本计划的落地原则 |
|---|---|---|
| **Q1** | 权威来源 = **IAM 侧用户（关系）表的角色字段**；**一个用户可以有多个角色** | IAM `user_org_relations`（`role` + `functional_roles[]`）为唯一权威；本地 `users.role` 降为**派生镜像**，禁止独立赋值 |
| **Q2** | 「域角色」术语多余（角色本就是「用户在某组织上的标签」） | **废弃「域角色」**；统一为「组织角色 `role` / 功能角色 `functional_roles[]` / 组织级别 `site|merchant|franchise`」三词 |
| **Q3** | `functional_roles` 清理 = 整体覆盖 | 复用 IAM 集合语义端点，并入 #2505 |
| **Q4** | 修复前先列表确认 | 已执行 F1–F4（清单与回滚见本单 2026-11-28 评论），本计划只承接**代码侧**收口 |

### 术语收敛（唯一口径）

| 概念 | 定义 | 存哪 |
|---|---|---|
| **组织角色** | `OWNER` / `ADMIN` / `STAFF` / `WORKER` / `member`，决定管理权层级 | IAM `user_org_relations.role` |
| **功能角色** | `merchant_admin` / `site_admin` / `merchant_staff` / `site_member` / `repair_technician` / `customer` / `namespace_admin`，决定业务能力（模板码，可多个） | IAM `user_org_relations.functional_roles[]` |
| **组织级别** | `namespace` / `merchant`(tenant) / `franchise` / `branch`(site) —— **是组织的属性，不是角色的属性** | IAM `organizations.type` / 派生 |

- `site_admin` 的准确读法是「**级别=site 的组织上的 ADMIN 角色**」→ 是**派生标签**，不是独立词汇；
- 沟通/文档里继续用 `site_admin`、`merchant_staff`（效率高），但**写库与判据只用上表三词**；
- 禁止新增任何 `*_admin` / `*_staff` 形态的**存储列**。

### 变更点

1. **单一派生函数**（新增 `backend/services/role_derive.go`）：`DeriveRoles(orgLevel, relationRole, functionalRoles) → { primary string, all []string }`；`namespace_admin` / `sys_admin` 直接保留（不参与业务角色优先级）；多角色按固定优先级取主角色（`merchant_admin > site_admin > repair_technician > merchant_staff > site_member`），`all` 保序去重。
2. **写路径收口**（3 处，禁止内联字面量）：`iam_callback.go`（原样写 IAM payload）、`merchant_member.go:256/308`（硬编码 `site_member`）、`user_staff.go:374`（域角色）统一改为「调派生函数 → 写镜像」。
3. **多值镜像**：`users` 新增 `roles jsonb NOT NULL DEFAULT '[]'`（镜像 IAM `functional_roles` ∪ 派生 primary），列注释写明「派生自 IAM，禁止独立写」；迁移仅加列 + 回填（回填值来自 IAM，不猜）。
4. **读侧派生**：前端判据（PC 人员/角色相关页面 ≥1 处、`staffForm.js` 的 `resolveInitialRole`）改为从 `roles[]` + `org level` 判定；**禁止**再以 `users.role` 单值作为判据来源。
5. **`users.role` 定位**：保留列以兼容旧读路径，语义固定为「**派生主角色**」；文档与代码注释统一标注 deprecated 计划（三步走：镜像多值 → 前端改读 → 删列）。
6. **对账联动**：#2622 的 L1 `ROLE_MISMATCH` 判定改为**集合比较**（`users.roles` vs IAM `functional_roles`），单值差异不再单列。
7. **文档同步**：`docs/spec/permissions.md` 用上述三词重写「角色」定义段；`docs/spec/api/api.md` 的人员相关响应说明补 `roles[]`；删除「域角色」表述。

### Verification Matrix

| 变更点 | 文件/组件 | 入口 | 触发状态 | 平台 | 期望 | 证据 |
|---|---|---|---|---|---|---|
| 派生函数（单值/多值/平台角色） | `backend/services/role_derive.go` | `go test ./services` | `ADMIN+[site_admin]`、`ADMIN+[merchant_staff,site_member]`、`OWNER+[namespace_admin]`、`STAFF+[]` | backend | `DERIVE_PRIMARY_SITE_ADMIN=site_admin`、`DERIVE_PRIMARY_MULTI=merchant_staff`、`DERIVE_ALL_LEN=2`、`DERIVE_NAMESPACE_ADMIN_KEPT=1` | R1 |
| 五处写路径收口（静态 + allowlist） | `iam_callback.go`、`merchant_member.go`、`user_staff.go`、`merchant.go`、`iam_proxy.go` | `bash scripts/verify-2626.sh r2` | 修后树 | backend（静态） | `WRITE_SITES=5`、`WRITE_INLINE_LITERAL=0`、`MIRROR_WRITE_DELEGATED=1`、`ALLOWLIST_HITS=0` | R2 |
| 多值镜像列（静态可判定）+ 真实执行 | 两个 2626 迁移文件、`models.User`、`backend/database/db.go` | `bash scripts/verify-2626.sh r3`（静态）；执行走 waitForConfirm | 修后树 | backend（静态） | `COLUMN_ROLES_EXISTS=1`、`BACKFILL_NO_GUESS=1`、`DOWN_REMOVES_COLUMN=1`、`ARTIFACT_REGISTERED=1`、`MODEL_FIELD_EXISTS=1` | R3 |
| 读侧派生（PC）+ 数据源 | `frontend-pc/src/utils/staffForm.js`、相关页面 | `node --test` | `roles:["site_member","repair_technician"]` 等 | PC | `PC_USES_ROLES_SET=1`、`PC_READS_USERS_ROLE=0`、`ROLES_SOURCE=iam`（镜像来自 IAM `functional_roles`，页面不得自造） | R4 |
| 术语清理（文档/注释） | `docs/spec/permissions.md`、`docs/spec/api/api.md`、相关代码注释 | `bash scripts/verify-2626.sh r5` | 修后树 | docs | `DOC_TERM_DOMAIN_ROLE=0`、`DOC_TERM_SET_OK=1`、`DOC_ROLES_FIELD=1` | R5 |
| 不引入单值判据（收敛） | `scripts/verify-2626.sh` | `bash scripts/verify-2626.sh r6` | 修后树 | static | `RECON_SINGLE_ONLY=0`、`SINGLE_VALUE_JUDGE_RESIDUAL=0` | R6 |

### Input Truth & Tool Preflight

- **IAM 契约**：`GET /api/v1/users/{id}` / 关系端点返回 `role`（结构）+ `functional_roles[]`；集合写入用 `PUT /api/v1/organizations/:id/users/:user_id/functional-roles`（#2505 已确认）。
- **本地现状**：`users.role` varchar、单值；`site_members.roles jsonb` 已是多值（#2034）；`merchant_members.role` 单值。
- **派生优先级**来源：`docs/spec/permissions.md` 角色表 + `backend/services/role_templates.go` 模板码；`namespace_admin`/`sys_admin` 不参与业务优先级。
- **历史数据**：F1–F3 已执行（30 行，`NIL 25→11`、`ROLE 16→0`、`TENANT 3→0`），F4 已把 11 行 IAM-孤儿标 `obsolete`；回滚 SQL 见本单评论。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 权威来源 | **IAM `user_org_relations`**（`role` + `functional_roles[]`）；本地为镜像。**过渡兜底（只读侧）**：`users.roles` → IAM 关系实时查询 → `users.role`；写侧**禁止**据此回写 |
| D2 | 多角色 | 一人可多角色；镜像存**多值**（`users.roles jsonb`），`users.role` = 派生主角色 |
| D3 | 存储术语 | 只写 `org_id` / `role` / `functional_roles[]`；**禁止**复合列（`*_admin`/`*_staff`） |
| D4 | 派生优先级 | `merchant_admin > site_admin > repair_technician > merchant_staff > site_member`；`namespace_admin`/`sys_admin` 直接保留 |
| D5 | 历史数据 | F1–F4 已执行（含回滚）；不回填猜测值，IAM 无关系者不猜 |
| D6 | 术语 | 废弃「域角色」；统一「组织角色 / 功能角色 / 组织级别」三词 |
| D7 | 删列时机 | 本单**不删** `users.role`；三步走后另立 Issue |
| **D8** | 迁移证据口径 | 迁移**只在代码侧可判定**（文本 + 工件登记 + 模型字段）；真实 up/down 执行由 `waitForConfirm` 回执（Owner 指定库与清理方式）；迁移**不写猜测值**（`BACKFILL_NO_GUESS=1`） |

### 既有断言影响（翻转清单）

| 断言 | 位置 | 旧期望 | 新期望 |
|---|---|---|---|
| `resolveInitialRole` 读单值 | `frontend-pc/src/utils/staffForm.js` | 返回 `user.role` | 返回 `roles[0]`（集合优先），`user.role` 仅在集合为空时兜底 |
| `TestCreateOrg_ExistingAdminGetsOwnerRole` 一类「handler 不校验调用者」 | `backend/handlers/*_test.go` | 依赖宽松调用 | 由 #515 起已按角色断言（本单不重复翻转，仅登记） |
| 写侧硬编码 `"site_member"` | `merchant_member.go:256/308` | 直接写常量 | 调 `DeriveRoles` 得镜像 |

> 口径：本单**只翻转**与「单值判据」直接相关的断言；其余保持。

### Out of Scope

- 删除 `users.role` 列（三步走的第三步，另立）。
- `merchant_members.role` 改多值（其集合语义当前不存在，需先定义业务规则）。
- IAM 侧 `user_task_queues`、模板码重命名（`site_admin` 等模板码保持不变）。
- 前端全部判据一次性迁移（本单只覆盖人员/角色相关 ≥1 处 + `staffForm.js`，其余按批次）。

### G0 Golden Acceptance

| R | fixture | 执行 | 产物 | 断言 | 负例 |
|---|---|---|---|---|---|
| R1 | 表驱动 4 组（单值/多值/平台角色/空） | `go test ./services -run TestDeriveRoles2626 -v` | `docs/evidence/2626/R1-derive.txt` | `DERIVE_PRIMARY_SITE_ADMIN=site_admin`、`DERIVE_PRIMARY_MULTI=merchant_staff`、`DERIVE_ALL_LEN=2`、`DERIVE_NAMESPACE_ADMIN_KEPT=1` | 多值只取第一个、平台角色被改写成 merchant_admin、空角色报错 → FAIL |
| R2 | 修后树静态扫描 5 文件（含 allowlist） | `bash scripts/verify-2626.sh r2` | `docs/evidence/2626/R2-write-sites.txt` | `WRITE_SITES=5`、`WRITE_INLINE_LITERAL=0`、`MIRROR_WRITE_DELEGATED=1`、`ALLOWLIST_HITS=0` | 任一写侧仍内联 `"site_member"` 字面量；allowlist 外出现字面量 → FAIL |
| R3 | 修后树静态（up/down 文本、db.go 登记、模型字段） | `bash scripts/verify-2626.sh r3` | `docs/evidence/2626/R3-mirror-column.txt` | `COLUMN_ROLES_EXISTS=1`、`BACKFILL_NO_GUESS=1`、`DOWN_REMOVES_COLUMN=1`、`ARTIFACT_REGISTERED=1`、`MODEL_FIELD_EXISTS=1` | up 无 `ADD COLUMN IF NOT EXISTS`、down 无 `DROP COLUMN IF EXISTS`、db.go 未登记工件、模型缺 `Roles`、SQL 内写猜测值 → 任一即 FAIL；**真实执行结果不由本行承担**（见 `waitForConfirm` 回执） |
| R4 | `staffForm` 集合用例 | `node --test frontend-pc/tests/staffForm*.test.mjs` | `docs/evidence/2626/R4-pc-derive.txt` | `PC_USES_ROLES_SET=1`、`PC_READS_USERS_ROLE=0` | 仍读 `user.role` 单值 → FAIL |
| R5 | 文档/注释静态扫描 | `bash scripts/verify-2626.sh r5` | `docs/evidence/2626/R5-terms.txt` | `DOC_TERM_DOMAIN_ROLE=0`、`DOC_TERM_SET_OK=1`、`DOC_ROLES_FIELD=1` | 仍出现「域角色」、权限文档未写 `roles[]` → FAIL |
| R6 | 修后树静态（判据口径） | `bash scripts/verify-2626.sh r6` | `docs/evidence/2626/R6-recon.txt` | `RECON_SINGLE_ONLY=0`、`SINGLE_VALUE_JUDGE_RESIDUAL=0` | 新增/保留以 `users.role` 单值为判据的读路径 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2626.sh r1 > docs/evidence/2626/R1-derive.txt` | `docs/evidence/2626/R1-derive.txt` | exit 0 且含 `DERIVE_PRIMARY_SITE_ADMIN=site_admin` `DERIVE_PRIMARY_MULTI=merchant_staff` `DERIVE_ALL_LEN=2` `DERIVE_NAMESPACE_ADMIN_KEPT=1` | PASS（4 项 token） | 优先级错、平台角色被改写 → FAIL | dev |
| R2 | `bash scripts/verify-2626.sh r2 > docs/evidence/2626/R2-write-sites.txt` | `docs/evidence/2626/R2-write-sites.txt` | exit 0 且含 `WRITE_SITES=5` `WRITE_INLINE_LITERAL=0` `MIRROR_WRITE_DELEGATED=1` `ALLOWLIST_HITS=0` | PASS（4 项 token） | 内联字面量 / allowlist 外命中 → FAIL | dev |
| R3 | `bash scripts/verify-2626.sh r3 > docs/evidence/2626/R3-mirror-column.txt` | `docs/evidence/2626/R3-mirror-column.txt` | exit 0 且含 `COLUMN_ROLES_EXISTS=1` `BACKFILL_NO_GUESS=1` `DOWN_REMOVES_COLUMN=1` `ARTIFACT_REGISTERED=1` `MODEL_FIELD_EXISTS=1` | PASS（5 项 token） | 迁移文本/工件登记/模型字段任一不符 → FAIL | dev |
| R4 | `bash scripts/verify-2626.sh r4 > docs/evidence/2626/R4-pc-derive.txt` | `docs/evidence/2626/R4-pc-derive.txt` | exit 0 且含 `PC_USES_ROLES_SET=1` `PC_READS_USERS_ROLE=0` `ROLES_SOURCE=iam` | PASS（3 项 token） | 仍读单值 / 数据源自造 → FAIL | dev |
| R5 | `bash scripts/verify-2626.sh r5 > docs/evidence/2626/R5-terms.txt` | `docs/evidence/2626/R5-terms.txt` | exit 0 且含 `DOC_TERM_DOMAIN_ROLE=0` `DOC_TERM_SET_OK=1` `DOC_ROLES_FIELD=1` | PASS（3 项 token） | 仍有「域角色」→ FAIL | dev |
| R6 | `bash scripts/verify-2626.sh r6 > docs/evidence/2626/R6-recon.txt` | `docs/evidence/2626/R6-recon.txt` | exit 0 且含 `RECON_SINGLE_ONLY=0` `SINGLE_VALUE_JUDGE_RESIDUAL=0` | PASS（2 项 token） | 仍以单值为判据 → FAIL | dev |

### Evidence Types

| R | 类型 | 说明 |
|---|---|---|
| R1 | unit | 纯函数表驱动，无 DB |
| R2 | static | 源码扫描 3 个写侧，禁内联字面量 |
| R3 | static | 迁移文本 + 工件登记 + 模型字段（真实执行转 waitForConfirm） |
| R4 | unit | 前端纯函数（`node --test`） |
| R5 | static | 文档/注释术语扫描 |
| R6 | static | 判据口径（禁单值） |

### Artifact Provenance

- R1–R6 首行：`# build=1.0.<短码> commit=<40位> dirty=false`（三 token：`# build=1.0.` / `# commit=` / `# dirty=false`）
- `evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$`
- 产物清单：
  - `docs/evidence/2626/R1-derive.txt`
  - `docs/evidence/2626/R2-write-sites.txt`
  - `docs/evidence/2626/R3-mirror-column.txt`
  - `docs/evidence/2626/R4-pc-derive.txt`
  - `docs/evidence/2626/R5-terms.txt`
  - `docs/evidence/2626/R6-recon.txt`

### Dependencies

### 依赖与关联说明（非硬依赖）

- #2505（角色集合语义收敛）：`functional_roles` 集合端点已存在，本单复用。
- #2622 / #2623（缓存对账）：R6 与其 L1 口径联动，本单不阻塞对账落地。
- #2580（编辑员工角色多选）：前端集合判据同源，实现时避免双写。

### Workload

- Basis: 新增 1 个服务文件 + 1 个迁移 + 3 处写侧 + 2 个前端纯函数 + 3 份文档 + 1 个证据脚本（6 行）；无跨仓改动；预估 S–M。

### Files（变更文件清单）

- `backend/services/role_derive.go`
- `backend/services/role_derive_2626_test.go`
- `backend/models/models.go`（`User.Roles`）
- `backend/database/db.go`（`defaultMigrationArtifacts` 登记 `{20261128001,"users","roles"}`）
- `backend/database/db_artifacts_2626_test.go`
- `backend/database/migrations/20261128001_users_roles_mirror_2626.up.sql`
- `backend/database/migrations/20261128001_users_roles_mirror_2626.down.sql`
- `backend/handlers/iam_callback.go`
- `backend/handlers/merchant_member.go`
- `backend/handlers/user_staff.go`
- `backend/handlers/merchant.go`（写侧 #4：商户管理员角色派生）
- `backend/handlers/iam_proxy.go`（写侧 #5：IAM 代理层的角色映射）
- `frontend-pc/src/utils/staffForm.js`
- `frontend-pc/tests/staffForm.test.mjs`
- `frontend-pc/tests/staffForm.test.js`（集合用例：`roles[]` 优先 + 回退链）
- `docs/spec/permissions.md`
- `docs/spec/api/api.md`
- `scripts/verify-2626.sh`
- `docs/evidence/2626/plan.md`
- `docs/evidence/2626/R1-derive.txt`
- `docs/evidence/2626/R2-write-sites.txt`
- `docs/evidence/2626/R3-mirror-column.txt`
- `docs/evidence/2626/R4-pc-derive.txt`
- `docs/evidence/2626/R5-terms.txt`
- `docs/evidence/2626/R6-recon.txt`
- `docs/evidence/2626/evidence.json`
- `docs/evidence/2626/pre-ready-2626.json`

### 历史数据修复记录（Q4 已执行）

| 批 | 范围 | 结果 |
|---|---|---|
| F1/F3 | `NIL_UUID` + `TENANT/ORG_MISMATCH` | `NIL 25→11`、`TENANT 3→0`（root/主关系 org 回填） |
| F2 | `ROLE_MISMATCH` | `16 → 0`（按 D4 优先级取主角色） |
| F4 | IAM 孤儿 11 行 | 标 `status='obsolete'`（保留行；代码无 `orphan` 语义，`obsolete` 是既有「不可登录、保留审计」口径） |
- 回滚：`/tmp/opencode/rollback.sql`（F1–F3，逐行原值）+ F4 的 `status` 反向 UPDATE（见本单评论）。

