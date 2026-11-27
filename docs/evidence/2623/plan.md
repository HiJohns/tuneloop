## Accepted Plan — #2623 [Sub-task of #2622 L1] 用户缓存只读对账工具：6 类差异分类 + 汇总 token + fixture 离线可测 + 只读硬闸门

### 背景与现状核对（代码为据）

| # | 事实 | 位置 / 证据 |
|---|---|---|
| 1 | 本单是 **L1 foundation**：差异判定口径与 token 契约的**唯一 owner**；#2624（修复）与 #2625（例行）只消费 | #2622 计划 `P2`、拓扑图 |
| 2 | IAM 侧权威数据：`user_org_relations`（`user_id/org_id/role/functional_roles(jsonb)/is_active`）+ `organizations`（`id/parent_id`，递归求根组织） | `~/beaconiam/internal/models/user_org_relation.go:10-20`；`~/beaconiam/internal/auth/handler.go:288-341` |
| 3 | **登录上下文的权威推导**（= 本地缓存应对齐的口径）：`resolveUserOrg` 先剔除 `role='member'`（零权限影子关系）→ 剩 **1 条**才签发 `tid=ResolveRootOrg(org)`、`oid=org`；**≥2 条**则返回空 ctx 让用户选择上下文；**0 条**回退 `users.org_id` | `~/beaconiam/internal/auth/handler.go:288-316`；`service.ResolveRootOrg`（`parent_id` 递归） |
| 4 | tuneloop 的 `users.tenant_id/org_id` 就是该 JWT 的 `tid`/`oid` 缓存；商户管理员 JWT `tid` = 商户根组织 | `backend/middleware/iam.go:272-276`（claims → ctx）；`backend/handlers/merchant_member.go:170`（`merchant.OrgID` 即根组织） |
| 5 | **角色映射是「多对一」**，正向表在代码里唯一：`merchant_admin→OWNER`、`site_admin→ADMIN`、`site_member→STAFF`、`merchant_staff→STAFF`、`repair_technician→repair_technician`；白名单 `IAMRelationRoles` | `backend/services/role_templates.go:129-166`（`ToIAMRole` / `IAMRelationRoles` / `NormalizeIAMRelationRole`） |
| 6 | 因第 5 条多对一，**反向不可唯一恢复**：`STAFF` 可能是 `site_member` 或 `merchant_staff`；`ADMIN` 可能是 `site_admin`，也可能是以 `ADMIN` 绑定的 `merchant_admin`（`merchantMemberIAMRole`） | `backend/handlers/merchant_member.go:752-760` |
| 7 | 关系上另有 `functional_roles`（tuneloop 侧赋的**角色模板码**，如 `site_admin`），是唯一可能恢复「具体域角色」的来源；但可能多条（#2505 记录过「只增不减」） | `backend/handlers/user_staff.go:461,707`（`toIAMRole(...)` + 模板赋值）；#2505 正文 |
| 8 | **客户（无员工关系）的 `00000000-…` 是正确值**，不是差异：新建用户即以零 UUID 占位 | `backend/handlers/iam_callback.go:240-247`；`backend/middleware/iam.go:583-592` |
| 9 | 只读运维工具的既有范式（fixture 可测、畸形行 fail-fast、token 输出）：`scripts/report-merchant-direct-role-drift-2462.sh` | 该脚本 `:50-110` |
| 10 | 一次性测试库口径：`~/.config/opencode/scripts/heavy-run.sh` 取锁 + `127.0.0.1:15433`（**不用** `jobmaster-postgres`） | `scripts/lib-2341-env.sh` |

**结论**：L1 = 一个新只读脚本 + 两个 fixture + 一份口径文档。核心是把第 3 条（登录上下文推导）与第 5–7 条（角色多对一 → 只能做集合判定）落成**唯一判定规则**，并保证「歧义一律不判差异」。

### 判定口径（本单锁定的唯一谓词）

#### A. 租户 / 组织（`TENANT_MISMATCH` / `ORG_MISMATCH` / `NIL_UUID` / `ORPHAN_LOCAL` / `MISSING_LOCAL`）

对每个 `iam_sub`（本地 ∪ IAM 的并集）：

```
loginable = IAM 活跃关系中 role != 'member' 的记录        # 与 resolveUserOrg 一致
```

| 情形 | 判定 |
|---|---|
| `|loginable| == 1` | `expect_org = loginable[0].org_id`；`expect_tenant = root_org(expect_org)`（`organizations.parent_id` 递归至根；找不到 → `ORG_UNRESOLVED`） |
| `|loginable| == 0` 且 IAM **无任何**活跃关系 | 本地非零 tenant/org → `ORPHAN_LOCAL`；本地零 UUID → 正常（顾客） |
| `|loginable| == 0` 但存在 `member` 关系 | `MEMBER_ONLY=<n>`（只计数，不判差异——与 `resolveUserOrg` 回退语义一致，避免误判） |
| `|loginable| >= 2` | `CONTEXT_AMBIGUOUS=<n>`（只计数，**不判** TENANT/ORG——多上下文用户本地只能存一个，判差异必然误报） |
| IAM 有 `|loginable| >= 1`、本地**无** `users` 行 | `MISSING_LOCAL` |
| IAM 无 `users` 行（`iam_sub` 在 IAM 侧不存在） | `IAM_MISSING=<n>`（只计数，不修） |

命中 `|loginable| == 1` 时，按**互斥优先级**归类（一行只归一类）：

| 序 | 条件 | 类别 |
|---|---|---|
| 1 | 本地 `tenant_id` 与 `org_id` 均为 `00000000-0000-0000-0000-000000000000` | `NIL_UUID` |
| 2 | 本地 `tenant_id` != `expect_tenant` | `TENANT_MISMATCH` |
| 3 | 本地 `org_id` != `expect_org` | `ORG_MISMATCH` |
| 4 | 否则 | 一致（不计差异） |

#### B. 角色（`ROLE_MISMATCH` / `ROLE_UNVERIFIED`）—— 只做**集合判定**，不猜具体值

`staff_code_set = AllRoleTemplates 键中能正向映射到该 IAM 角色的集合`（`backend/services/role_templates.go:129-140`）：

| IAM 角色 | 允许的本地 `users.role` 集合 | 可判定？ |
|---|---|---|
| `OWNER` | `{merchant_admin}` | 是（唯一原像） |
| `repair_technician` | `{repair_technician}` | 是 |
| `ADMIN` | `{site_admin, merchant_admin}` | 仅「不在集合内」可判定 |
| `STAFF` | `{site_member, merchant_staff}` | 仅「不在集合内」可判定 |
| `member` / 其他 / 空 | — | 否 → `ROLE_UNVERIFIED` |

- `functional_roles` 中若含**唯一**一个 `AllRoleTemplates` 键（如 `repair_technician`/`site_member`），把它并入允许集合后再判定（提高精度，不改变「多对一不可逆」结论）；
- 本地 `role` ∉ 允许集合 → `ROLE_MISMATCH`；集合内 → 一致（**不**报 mismatch，即使无法确定具体是哪一个）；
- `|loginable| >= 2`、`role` 不在白名单、`functional_roles` 为空且角色为 `ADMIN`/`STAFF` 以外的未知值 → `ROLE_UNVERIFIED`（只计数）；
- **本单不产出修复目标值**（多对一不可逆）→ `ROLE_REPAIR_SUPPORTED=0`（#2624 据此保持 `ROLE_APPLY_SKIPPED=1`）。

#### C. 角色权威来源的设计缺口（显式登记）

`users.role` 的唯一权威来源（IAM 关系角色 + `functional_roles` 的优先级/清理规则）目前**不存在单一 owner**（#2505 的「functional_roles 只增不减」会污染推导）。本单只做集合判定并把该缺口登记到 `docs/ops/user-cache-reconcile.md`，由 **#2626** 跟踪（本次 analyze 已创建）；本单不实现优先级推导。

### 变更点

#### 1. `scripts/reconcile-user-cache.sh`（新增，只读）

```
bash scripts/reconcile-user-cache.sh [--json] [--limit 20]
  [--iam-fixture <tsv>] [--local-fixture <tsv>] [--fixture <dir>]
```

| 行为 | 契约 |
|---|---|
| 默认 | 两库真读：IAM 侧 `BEACONIAM_DB_*`、本地侧 `TUNELOOP_DB_*`（env 全覆盖，脚本内零凭据） |
| `--fixture <dir>` | `iam.tsv` + `local.tsv` 离线输入，**同一套判定**（证据可离线复跑） |
| `--json` | 输出 `{"tokens":{…},"rows":[{"iam_sub","email","category","local","iam"}…]}`，rows 按 `iam_sub` 升序，最多 `--limit` 条（默认 20） |
| 只读硬闸门 | 连接后执行 `SET default_transaction_read_only = on`；脚本源码**不得**含 `INSERT/UPDATE/DELETE/ALTER/DROP/TRUNCATE` 语句（测试静态断言）；输出恒为 `DB_WRITES=0` |
| 退出码 | `0` 比对完成（有差异也算 0）；`2` 连接/参数/env 缺失；`3` 输入损坏（字段数不符、非法枚举、缺必需 token） |

fixture 格式（TSV，`#` 注释行忽略，空行忽略）：
- `iam.tsv`：`user_id<TAB>org_id<TAB>role<TAB>functional_roles(逗号分隔或空)<TAB>is_active(0|1)<TAB>org_parent_id(空=根)`
- `local.tsv`：`iam_sub<TAB>user_id<TAB>tenant_id<TAB>org_id<TAB>role<TAB>status`

既有 SQL（真实库模式）必须与第 2 条事实对齐：
- IAM：`SELECT user_id, org_id, role, functional_roles, is_active FROM user_org_relations WHERE is_active = true`；`WITH RECURSIVE org_root(id, root_id) AS (...)` 求根；
- 本地：`SELECT iam_sub, id, tenant_id, org_id, role, status FROM users WHERE deleted_at IS NULL`。

#### 2. 输出 token（逐字锁定；#2624/#2625 消费）

```
RECONCILE_DATE=<YYYY-MM-DD>
RECONCILE_SOURCE=<live|fixture>
USERS_LOCAL=<n>
USERS_IAM_ACTIVE=<n>
TENANT_MISMATCH=<n>
ORG_MISMATCH=<n>
NIL_UUID=<n>
MISSING_LOCAL=<n>
ORPHAN_LOCAL=<n>
ROLE_MISMATCH=<n>
ROLE_UNVERIFIED=<n>
CONTEXT_AMBIGUOUS=<n>
MEMBER_ONLY=<n>
IAM_MISSING=<n>
ORG_UNRESOLVED=<n>
ROLE_REPAIR_SUPPORTED=0
DB_WRITES=0
RESULT: PASS
```

- 计数恒等式（测试断言）：`USERS_LOCAL + MISSING_LOCAL` = 本地行数 + IAM 侧独有行数；六类差异之和 = 明细行数（`--json` 的 `rows` 在 `--limit 0` 时全量）。

#### 3. 文档 `docs/ops/user-cache-reconcile.md`（新增）

写清：判定口径 A/B、为什么多上下文与 `member` 关系只计数、`ROLE_REPAIR_SUPPORTED=0` 的原因（`ToIAMRole` 多对一）、#2626 设计缺口、如何使用 `--fixture` 复跑、以及「本工具只读，修复见 #2624」。

#### 4. 测试 `scripts/tests/test_reconcile_user_cache_2623.sh`

- fixture 覆盖：`|loginable|=1` 的 6 类（NIL/TENANT/ORG/正常/ROLE_MISMATCH/ORPHAN）+ `|loginable|=0`（顾客零 UUID 正常 / ORPHAN）+ `member` only（MEMBER_ONLY）+ `|loginable|>=2`（CONTEXT_AMBIGUOUS）+ `MISSING_LOCAL` + 角色集合判定（OWNER 错误值 → MISMATCH；ADMIN 的 `site_member` → MISMATCH；ADMIN 的 `site_admin` → 一致；未知角色 → ROLE_UNVERIFIED）。
- 负例：畸形 fixture（字段数不符 / `is_active=2`）→ `exit 3`；env 缺失 → `exit 2` 且 `RESULT: FAIL`（**不得**输出 0 差异）；脚本源码含写语句 → 静态断言失败。

### 既有断言影响（0 翻转）

| 检查 | 结果 | 依据 |
|---|---|---|
| 既有脚本/测试是否断言 `users.tenant_id` 与 IAM 一致 | **0 处**（本单新增工具，无既有断言） | `grep -rn 'reconcile-user-cache' scripts/ backend/` → 修前 0 |
| `scripts/report-merchant-direct-role-drift-2462.sh` 及其测试 | 不受影响（独立脚本、独立口径） | — |
| `backend/services/role_templates.go` | **只读引用**，不修改（避免改变写入映射语义） | 变更点 B |
| 需要翻转的语义断言 | **无** | 本单纯新增 |

### Files（变更文件清单）

- `scripts/reconcile-user-cache.sh`
- `scripts/tests/test_reconcile_user_cache_2623.sh`
- `scripts/fixtures/reconcile-2623/iam.tsv`
- `scripts/fixtures/reconcile-2623/local.tsv`
- `docs/ops/user-cache-reconcile.md`
- `docs/evidence/2623/plan.md`
- `docs/evidence/2623/R1-classification.txt`
- `docs/evidence/2623/R2-readonly-negative.txt`
- `docs/evidence/2623/R3-live-db-readonly.txt`
- `docs/evidence/2623/evidence.json`
- `docs/evidence/2623/pre-ready-2623.json`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 6 类差异 + 角色集合判定（fixture 离线） | `scripts/reconcile-user-cache.sh`、`scripts/tests/test_reconcile_user_cache_2623.sh`、`scripts/fixtures/reconcile-2623/iam.tsv` | `bash scripts/tests/test_reconcile_user_cache_2623.sh r1` | `--fixture` 模式，覆盖 1 关系 / 0 关系 / member-only / 多关系 / 缺失本地 / 角色集合 | 本机（纯文件，无库） | `TEST_EXIT=0`、`TENANT_MISMATCH=2`、`ORG_MISMATCH=2`、`NIL_UUID=1`、`MISSING_LOCAL=1`、`ORPHAN_LOCAL=1`、`ROLE_MISMATCH=2`、`ROLE_UNVERIFIED=1`、`CONTEXT_AMBIGUOUS=1`、`MEMBER_ONLY=1`、`ROWS_SORTED=1`、`JSON_TOKENS_MATCH=1`、`DB_WRITES=0` | R1 |
| R2 只读硬闸门 + 失败负例（不得把「查不到」当无差异） | `scripts/reconcile-user-cache.sh`、`scripts/tests/test_reconcile_user_cache_2623.sh` | `bash scripts/tests/test_reconcile_user_cache_2623.sh r2` | 源码静态扫描；IAM 不可达；fixture 畸形；env 缺失 | 本机（无库 / 坏连接） | `TEST_EXIT=0`、`FORBIDDEN_SQL_STMTS=0`、`READONLY_DSN_GUARD=1`、`BAD_FIXTURE_EXIT=3`、`UNREACHABLE_EXIT=2`、`UNREACHABLE_RESULT_FAIL=1`、`MISSING_ENV_EXIT=2`、`FALSE_ZERO_DIFF=0` | R2 |
| R3 真实本地库只读跑一次（一次性 PG + IAM fixture，含 #2620 故障形态） | `scripts/reconcile-user-cache.sh` | `bash scripts/tests/test_reconcile_user_cache_2623.sh r3` | 一次性 PG 15433 seed（含 `tenant_id≠expect` 的 #2620 形态行）+ `--iam-fixture` | 一次性 PG 15433 | `TEST_EXIT=0`、`DB_WRITES=0`、`TENANT_MISMATCH=1`、`CASE_2620_DETECTED=1`、`EXIT_0_WITH_DIFF=1`、`READONLY_TXN_OK=1` | R3 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `scripts/fixtures/reconcile-2623/{iam.tsv,local.tsv}`：1 关系 ×（NIL/TENANT/ORG/正常/ROLE_MISMATCH/ORPHAN）、0 关系 ×（顾客正常/ORPHAN）、member-only、多关系、MISSING_LOCAL、角色集合 4 例 | `bash scripts/tests/test_reconcile_user_cache_2623.sh r1` | `docs/evidence/2623/R1-classification.txt` | `TEST_EXIT=0`、逐类计数与期望相等（TENANT=2/ORG=2/NIL=1/MISSING_LOCAL=1/ORPHAN=1/ROLE_MISMATCH=2/ROLE_UNVERIFIED=1/CONTEXT_AMBIGUOUS=1/MEMBER_ONLY=1）、`ROWS_SORTED=1`、`JSON_TOKENS_MATCH=1`、`DB_WRITES=0`、`RESULT: PASS` | 顾客零 UUID 被判 NIL_UUID、多上下文被判 TENANT_MISMATCH、`ADMIN + site_admin` 被判 ROLE_MISMATCH、`STAFF + merchant_staff` 被判 mismatch、计数与明细不符、rows 非 `iam_sub` 序 → 任一即 FAIL |
| R2 | 源码 + 坏连接 DSN + 畸形 fixture + 缺失 env | `bash scripts/tests/test_reconcile_user_cache_2623.sh r2` | `docs/evidence/2623/R2-readonly-negative.txt` | `TEST_EXIT=0`、`FORBIDDEN_SQL_STMTS=0`、`READONLY_DSN_GUARD=1`、`BAD_FIXTURE_EXIT=3`、`BAD_FIXTURE_FIELDS_REPORTED=1`、`UNREACHABLE_EXIT=2`、`UNREACHABLE_RESULT_FAIL=1`、`MISSING_ENV_EXIT=2`、`FALSE_ZERO_DIFF=0` | 脚本含 INSERT/UPDATE/DELETE/ALTER/DROP/TRUNCATE、库不可达却 `RESULT: PASS`、畸形行被跳过、缺 env 静默成功 → 任一即 FAIL |
| R3 | 一次性 PG `127.0.0.1:15433`：seed 5 本地行（含 1 行 `tenant_id=平台组织`、IAM=商户 ADMIN 的 #2620 形态）+ `--iam-fixture` | `bash scripts/tests/test_reconcile_user_cache_2623.sh r3` | `docs/evidence/2623/R3-live-db-readonly.txt` | `TEST_EXIT=0`、`DB_WRITES=0`、`TENANT_MISMATCH=1`、`CASE_2620_DETECTED=1`、`EXIT_0_WITH_DIFF=1`、`READONLY_TXN_OK=1` | 真实库模式写入任何行（`DB_WRITES>0`）、#2620 形态行未被识别、只读事务设置失败仍继续 → 任一即 FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/tests/test_reconcile_user_cache_2623.sh r1 > docs/evidence/2623/R1-classification.txt` | `docs/evidence/2623/R1-classification.txt` | exit 0 且含 `TEST_EXIT=0` `TENANT_MISMATCH=2` `ORG_MISMATCH=2` `NIL_UUID=1` `MISSING_LOCAL=1` `ORPHAN_LOCAL=1` `ROLE_MISMATCH=2` `ROLE_UNVERIFIED=1` `CONTEXT_AMBIGUOUS=1` `MEMBER_ONLY=1` `ROWS_SORTED=1` `JSON_TOKENS_MATCH=1` `DB_WRITES=0` | PASS（13 项 token 全为期望值） | 顾客零 UUID 判为差异、多上下文判差异、角色集合误判、计数与明细不符、未排序 → FAIL | dev |
| R2 | `bash scripts/tests/test_reconcile_user_cache_2623.sh r2 > docs/evidence/2623/R2-readonly-negative.txt` | `docs/evidence/2623/R2-readonly-negative.txt` | exit 0 且含 `TEST_EXIT=0` `FORBIDDEN_SQL_STMTS=0` `READONLY_DSN_GUARD=1` `BAD_FIXTURE_EXIT=3` `UNREACHABLE_EXIT=2` `UNREACHABLE_RESULT_FAIL=1` `MISSING_ENV_EXIT=2` `FALSE_ZERO_DIFF=0` | PASS（8 项 token 全为期望值） | 含写语句、不可达仍 PASS、畸形行跳过、缺 env 静默 → FAIL | dev |
| R3 | `bash scripts/tests/test_reconcile_user_cache_2623.sh r3 > docs/evidence/2623/R3-live-db-readonly.txt` | `docs/evidence/2623/R3-live-db-readonly.txt` | exit 0 且含 `TEST_EXIT=0` `DB_WRITES=0` `TENANT_MISMATCH=1` `CASE_2620_DETECTED=1` `EXIT_0_WITH_DIFF=1` `READONLY_TXN_OK=1` | PASS（6 项 token 全为期望值） | 真实库写入、#2620 形态漏检、只读事务未生效 → FAIL | dev |

### Artifact Provenance

- 每个文本 artifact 首行 `# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `GENERATED_AT_HEAD=<40 位 sha>`；证据必须在 fix commit **之后**生成（`FIX_REF` 可覆盖锚点，默认 `git log --grep="^fix(issue-2623)"`）。
- R3 使用一次性 PostgreSQL（`127.0.0.1:15433`，`scripts/lib-2341-env.sh` 口径），建库/seed 经 `~/.config/opencode/scripts/heavy-run.sh` 取锁；**禁止**使用 `jobmaster-postgres` 的 dev 库或任何预生产/生产库。
- 逐条清单（全部位于 `docs/evidence/2623/`）：
  - `docs/evidence/2623/R1-classification.txt`
  - `docs/evidence/2623/R2-readonly-negative.txt`
  - `docs/evidence/2623/R3-live-db-readonly.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2623 --summary-file tmp/pre-ready-2623-summary.md --changed-files tmp/pre-ready-2623-changed.txt --evidence-file docs/evidence/2623/evidence.json --report docs/evidence/2623/pre-ready-2623.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - 登录上下文推导 = `~/beaconiam/internal/auth/handler.go:288-341`（`resolveUserOrg` / `fillOrgContext` / `ResolveRootOrg`）；
  - 角色映射 = `backend/services/role_templates.go:129-166`（`ToIAMRole` / `IAMRelationRoles`）——**只读引用，不修改**；
  - 客户零 UUID 语义 = `backend/handlers/iam_callback.go:240-247`；
  - 只读工具范式 = `scripts/report-merchant-direct-role-drift-2462.sh`。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `ls scripts/reconcile-user-cache.sh 2>/dev/null | wc -l` → 修前 0；
  - `grep -c 'IAMRole' backend/services/role_templates.go` → 记录基线（本单不得改动该文件：`git diff --stat backend/services/role_templates.go` 必须为空）；
  - `grep -rn 'INSERT\|UPDATE\|DELETE\|ALTER\|DROP\|TRUNCATE' scripts/reconcile-user-cache.sh | wc -l` → 修后必须 0（R2 断言）。
- **损坏输入拒绝规则**（全部 fail-fast，不跳过不猜）：
  - fixture 字段数不符 / `is_active ∉ {0,1}` / `role` 非法 → `exit 3`；
  - `org_parent_id` 成环 → `ORG_UNRESOLVED` 计数 + 不崩（环不得导致死循环：递归深度上限）；
  - 库不可达 / env 缺失 → `exit 2` 且 `RESULT: FAIL`（**禁止**输出 `RESULT: PASS`）；
  - 本地 `users` 行 `iam_sub` 为空 → `IAM_SUB_EMPTY=<n>` 只计数（不参与对齐）。
- **工具盲区**：
  - fixture 模式不验证 SQL 语法；真实库模式的 SQL 由 R3 一次性 PG 覆盖（同一条 SQL 路径）；
  - 本单不接触预生产/生产库（父 `P1`：真相在库，但执行由 R3 一次性 PG + 后续运维只读跑验证）；
  - `functional_roles` 的 jsonb 解析在 fixture 模式用逗号串模拟，真实库用 `jsonb_array_elements_text`；两条路径必须有同一个归一化函数（测试断言同一输入的两种来源结果一致 → `FIXTURE_PARITY=1`）。
- **契约影响**：无 API/DB 契约变更（纯只读工具 + 文档）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 判定口径 A/B 为本单唯一实现（不得在 #2624/#2625 复制）；token 与 `--json` 结构逐字锁定（变更需回 Architect） | 本计划锁定 |
| D2 | 多上下文用户（`\|loginable\|>=2`）**不判** TENANT/ORG（本地只能存一个上下文，判差异必误报），只输出 `CONTEXT_AMBIGUOUS` | 本计划锁定（依据 `resolveUserOrg` 语义） |
| D3 | `role='member'` 关系**不参与**上下文推导（与 beaconiam 一致）；只输出 `MEMBER_ONLY` | 本计划锁定 |
| D4 | 客户（无员工关系）的 `00000000-…` 视为**正确**，不判 `NIL_UUID`；`NIL_UUID` 仅在「IAM 有 1 条可登录关系」时成立 | 本计划锁定（依据 `iam_callback.go:240-247`） |
| D5 | 角色只做**集合判定**（`ROLE_MISMATCH` = 不在允许集合内），**不产出修复目标值** → `ROLE_REPAIR_SUPPORTED=0`；多对一不可逆是本单结论，非实现缺陷 | 本计划锁定（依据 `ToIAMRole`） |
| D6 | 角色权威来源/优先级缺口 → **#2626**（本次 analyze 已创建）；本单只在文档登记，不实现优先级推导 | #2626 |
| D7 | 只读硬闸门：只读事务 + 源码零写语句（静态断言）；`DB_WRITES=0` 恒成立 | 父 `P5` |
| D8 | 失败语义：连接失败/env 缺失 → `exit 2` + `RESULT: FAIL`；**任何情况下不得**输出「0 差异」冒充成功 | 父 `P7` |
| D9 | fixture 与真实库两条输入路径共用同一判定函数（`FIXTURE_PARITY=1` 断言），避免「测试绿、线上错」 | 本计划锁定 |
| D10 | 真实预生产/生产库上的只读跑由运维执行（closing 时 `[Ops-Followup of #2623]`，`waitForConfirm`）；本单证据只用一次性 PG + fixture | 父 `P8` |

### Out of Scope

- `--apply` / 任何写库（#2624）；timer 与告警（#2625）。
- 角色权威来源与 `functional_roles` 优先级/清理（#2626）；`ROLE_MISMATCH` 的目标值推导。
- 修改 `backend/services/role_templates.go` 或任何写入路径语义（#2580 等）。
- 存量回填与数据修正（#2527 / #2553–#2557）。
- 用 IAM HTTP API 枚举关系（本单只做只读 SQL；如需另立 Issue）。
- `tenant_id` 体系整体废弃（#2255 P3）。
- 预生产/生产库上的真实只读跑与结果解读（`[Ops-Followup of #2623]`）。
- 全量 `go test ./...` 作为合同行（本单不动 Go 代码）。

### Dependencies

- 无

### Workload

- Size: M
- Basis: 脚本 1 + 测试 1 + fixture 2 + 文档 1；3 条证据行（R1/R2/R3）；无 migration、无 Go/前端/API 改动、无外部仓库改动
- Confidence: medium

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 本地 `users` 缓存与 IAM 权威不一致，导致具体功能 404/数据范围错乱，且无例行发现手段 | #2622 正文；#2620 实例 |
| 2 | 触发条件（trigger） | 写路径漏写/写错 + 迁移/重放/手工操作；多上下文用户本地只能存一个上下文 | 背景第 2–7 条 |
| 3 | 根因（root cause） | 缺少「缓存 ↔ 权威」的可执行判定；且判定口径未定义（什么算不一致） | 背景第 3–7 条 |
| 4 | 测试盲点（test blind spot） | 全仓无 reconcile 相关测试；`users.tenant_id` 与 IAM 一致性从未被断言 | `grep -rn 'reconcile' backend scripts` → 仅媒体 GC |
| 5 | 同类扫描（same-class paths） | 6 类差异全覆盖 + 3 类只计数（多上下文/member-only/IAM 缺失），避免误报 | 判定口径 A/B |
| 6 | 修复层（fix layer） | 判定层（工具）单点实现，供 #2624/#2625 消费；不改业务代码 | 父 #2622 拓扑 |
| 7 | 红绿实证（red-green） | 红：脚本不存在时 r1/r2/r3 全 FAIL；绿：实现后 3 行全绿，且每行都含反例断言 | 变更点 4 |
| 8 | 防复发（prevention） | ① 唯一判定函数 + fixture/真实库双路径一致（D9）；② 只读硬闸门（D7）；③ 歧义只计数不判差异（D2/D3/D4）；④ 失败必须 FAIL（D8） | R1/R2/R3 |
| 9 | 影响面（blast radius） | 纯新增只读工具 + 文档；不触碰业务代码/DB 写入/API/前端 | D7、Files |
| 10 | 未修边界（deferred） | 修复 #2624、例行 #2625、角色权威 #2626、回填 #2527、真实库只读跑 `[Ops-Followup]` | Out of Scope、D6/D10 |

*Model: deepseek/deepseek-flash*

