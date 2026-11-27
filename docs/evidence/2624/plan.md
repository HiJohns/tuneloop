## Accepted Plan — #2624 [Sub-task of #2622 L2] 用户缓存对账「可选修复」：`--apply`（TENANT/ORG/NIL_UUID）+ Owner 审批闸门 + 回滚 SQL + 审计留痕

### 背景与现状核对（代码为据）

| # | 事实 | 位置 / 证据 |
|---|---|---|
| 1 | 本单是 **L2**，父 #2622 已锁定「判定与 token 契约唯一 owner = #2623（L1）」，本单**只消费**、禁止复制判定逻辑 | #2622 计划 `P2`；#2623 正文「输出契约」 |
| 2 | L1 契约（本单的输入，逐字来自 #2623 正文）：汇总 token `RECONCILE_DATE / USERS_LOCAL / USERS_IAM_ACTIVE / TENANT_MISMATCH / ORG_MISMATCH / NIL_UUID / MISSING_LOCAL / ORPHAN_LOCAL / ROLE_MISMATCH / ROLE_UNVERIFIED / DB_WRITES / RESULT`；`--json` 明细字段 `iam_sub / email / 类别 / 本地值 / IAM 值`，按 `iam_sub` 排序 | #2623 正文 |
| 3 | 既有写库闸门先例：`OWNER_APPROVED_*` env + `--confirm-rows` 与实际行数**严格相等**才允许写（#2484 伪阳性防线），未授权即 `exit 2` + `DB_WRITES=0` | `scripts/report-merchant-direct-role-drift-2462.sh:37-46,115-135` |
| 4 | 审计可复用既有表，**无需 migration**：`audit_logs` 有 `action / resource_type / resource_id / user_id / tenant_id / details(jsonb) / actor_name / actor_role / status`，足够承载「谁改了哪行、旧值→新值、批次号」 | `backend/models/audit_log.go:5-27` |
| 5 | `ORPHAN_LOCAL`（IAM 无活跃关系 = 离职/解绑）**永不自动修**；`MISSING_LOCAL` 需产品确认是否建行 → 两者本单均跳过并计数 | 父 #2622 `P4`；#2622 正文 L2 段 |
| 6 | `ROLE_MISMATCH` 修复必须以 #2623 锁定的「IAM relation role + org 类型 → 本地 role」映射表为前提；未锁定则 `ROLE_APPLY_SKIPPED=<n>` 跳过 | 父 #2622 `P3` |
| 7 | 目标行同时有 `tenant_id` 与 `org_id`，错值可能是二者之一或 `00000000-…`；逐行修复必须是**比较并设置**（compare-and-set），否则并发写会被覆盖 | 本地表结构：`backend/models/models.go` `User.TenantID/OrgID`；#2622 差异分类 |
| 8 | 一次性测试库口径已存在：`scripts/lib-2341-env.sh`（`127.0.0.1:15433`，**不用** `jobmaster-postgres`），重型命令经 `heavy-run.sh` 取锁 | `scripts/lib-2341-env.sh`；`scripts/reconcile-user-cache.sh`（#2623 产出） |
| 9 | 本单**不新增表、不改模型、不改 API**：`audit_logs` 直接 SQL INSERT 即可（审计是脚本行为，不经 handler） | 第 4 条 |

**结论**：L2 = 在 #2623 的只读对账脚本上加一个**受闸门约束的单事务修复通道**：先生成「修复 SQL + 回滚 SQL」，再在**一个事务**内执行 compare-and-set UPDATE 与审计 INSERT，最后用同一套判定逻辑复查归零。判定逻辑与 token 契约零改动。

### 变更点

#### 1. CLI 契约（在 `scripts/reconcile-user-cache.sh` 上扩展，判定逻辑不动）

```
bash scripts/reconcile-user-cache.sh --apply \
  --confirm-rows <n> \
  --sql-out <path> --rollback-file <path> \
  [--iam-fixture <file>] [--sql-only]
```

| 参数 | 语义 |
|---|---|
| `--apply` | 进入修复通道（默认只读，不加即完全等同 L1 行为） |
| `--confirm-rows <n>` | 必须与本次运行算出的 `REPAIR_ROWS` **逐字相等**，否则 `exit 2`、`DB_WRITES=0` |
| `--sql-out` / `--rollback-file` | **必填**：分别写出「修复 SQL」「回滚 SQL」，两者都必须可单独执行 |
| `--sql-only` | 只生成两份 SQL，不连库执行（离线可断言，`DB_WRITES=0`） |
| `--iam-fixture <file>` | 复用 #2623 的离线 IAM 侧输入；本地侧仍连真实库（修复必须落在真库） |

#### 2. 修复范围与跳过（逐类计数，禁止静默）

| 类别 | 处理 | 输出 token |
|---|---|---|
| `TENANT_MISMATCH` / `ORG_MISMATCH` / `NIL_UUID` | 修复（compare-and-set） | 计入 `REPAIR_ROWS` |
| `ORPHAN_LOCAL` | **跳过**（离职/解绑语义，人工处理） | `SKIPPED_ORPHAN=<n>` |
| `MISSING_LOCAL` | **跳过**（是否建行需产品确认） | `SKIPPED_MISSING_LOCAL=<n>` |
| `ROLE_MISMATCH` | 仅当 #2623 映射表已锁定且本次运行 `--include-role` 显式开启才修；否则跳过 | `SKIPPED_ROLE=<n>` / `ROLE_APPLY_SKIPPED=1` |
| `ROLE_UNVERIFIED` | **永不修**（映射无法唯一确定） | `SKIPPED_ROLE_UNVERIFIED=<n>` |
| 其他未知类别 | **fail-fast** `exit 3`（不猜） | `UNKNOWN_CATEGORY=<n>` |

#### 3. 生成的两份 SQL（确定性、可逐行断言）

**修复 SQL（单事务）**：

```sql
BEGIN;
-- 每行一条 compare-and-set：旧值来自本次读快照，改的动才改
UPDATE users SET tenant_id = '<iam_tenant>', org_id = '<iam_org>', updated_at = now()
 WHERE id = '<local_id>' AND iam_sub = '<iam_sub>'
   AND tenant_id = '<old_tenant>' AND org_id = '<old_org>';
-- 每行一条审计（details 含类别/旧值/新值/批次）
INSERT INTO audit_logs (id, tenant_id, org_id, user_id, actor_name, actor_role,
                        action, resource_type, resource_id, status_code, status,
                        details, ip_address, user_agent, created_at)
VALUES (gen_random_uuid(), '<new_tenant>', '<new_org>', '<local_id>', 'reconcile-user-cache', 'ops',
        'USER_CACHE_RECONCILE', 'users', '<iam_sub>', 0, 'success',
        '{"batch_id":"<BATCH_ID>","category":"<类>","old":{"tenant_id":"<old>","org_id":"<old>"},"new":{"tenant_id":"<new>","org_id":"<new>"}}'::jsonb,
        '', '', now());
-- 事务末后置校验：任一目标行未达预期值即整体中止（并发防线）
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM users WHERE id = '<local_id>' AND (tenant_id <> '<new_tenant>' OR org_id <> '<new_org>')) THEN
    RAISE EXCEPTION 'reconcile: concurrent change detected for <local_id>';
  END IF;
END $$;
COMMIT;
```

**回滚 SQL（单事务、镜像）**：`UPDATE ... SET tenant_id='<old>' , org_id='<old>' WHERE id=... AND tenant_id='<new>' AND org_id='<new>'` + 一条 `action='USER_CACHE_RECONCILE_ROLLBACK'` 审计 + 同样的后置校验。

- 执行方式：`psql -v ON_ERROR_STOP=1 -1 -f <sql>`（`-1` = 单事务）；任一语句失败 → 全量回滚，`exit 2`、`DB_WRITES=0`。
- `BATCH_ID=<UTC yyyyMMddTHHMMSSZ>-<6 位随机>`，同时写入修复 SQL、审计 `details`、回滚 SQL 与输出 token（可反查批次）。
- 排序：SQL 语句按 `iam_sub` 排序（确定性 diff，证据可逐行比对）。

#### 4. 输出 token（本单新增，L1 token 不变）

```
REPAIR_ROWS=<n>
SKIPPED_ORPHAN=<n>
SKIPPED_MISSING_LOCAL=<n>
SKIPPED_ROLE=<n>
SKIPPED_ROLE_UNVERIFIED=<n>
ROLE_APPLY_SKIPPED=0|1
BATCH_ID=<id>
AUDIT_ROWS=<n>
DB_WRITES=<n>
ROLLBACK_ROWS=<n>
SQL_OUT=<path>
ROLLBACK_FILE=<path>
RESULT: PASS|FAIL
```

#### 5. 测试与证据

- 新增 `scripts/tests/test_reconcile_user_cache_apply_2624.sh`（照 `scripts/tests/test_merchant_direct_role_drift_2484.sh` 风格：fixture + fake/一次性 PG，逐 token 断言）。
- 测试分两档：
  1. **离线**（`--sql-only` + fixture）：闸门与 SQL 形状（compare-and-set 谓词、审计 INSERT、后置校验 DO 块、行数、排序）；
  2. **一次性 PG**（`127.0.0.1:15433`，经 `heavy-run.sh`）：真修复 + 审计行 + 复查归零 + 回滚还原 + 并发中止。
- **红绿实证**：先落「跳过类不修 + 未授权不写」的测试，跑在**未加 `--apply`** 的 #2623 脚本上（`--apply` 未知参数 → 非预期输出/FAIL）；再落实现 → 全绿。红样贴 Work Summary + `evidence.json` 的 `negative_result`。

### 既有断言影响（0 翻转）

| 检查 | 结果 | 依据 |
|---|---|---|
| #2623 的只读断言（`DB_WRITES=0` / 无 `--apply`） | **需保持**：不加 `--apply` 时本单行为与 L1 完全一致 | `scripts/tests/test_reconcile_user_cache_2623.sh`（#2623 产出） |
| 既有 `audit_logs` 查询/展示断言 | 不受影响：新 `action` 值只是新增枚举值，`ListAuditLogs` 按 `action` 过滤，未收录则仅不在筛选项中出现 | `backend/handlers/audit_log.go:20-38` |
| `scripts/tests/*` 其余用例 | 不受影响（新脚本 + 新测试文件） | — |
| 需要翻转的语义断言 | **无** | 本单只新增写通道，不改只读语义 |

### Files（变更文件清单）

- `scripts/reconcile-user-cache.sh`
- `scripts/tests/test_reconcile_user_cache_apply_2624.sh`
- `scripts/fixtures/reconcile-2624/iam.tsv`
- `scripts/fixtures/reconcile-2624/local.tsv`
- `docs/ops/user-cache-reconcile.md`
- `docs/evidence/2624/plan.md`
- `docs/evidence/2624/R1-apply-gates.txt`
- `docs/evidence/2624/R2-apply-pg.txt`
- `docs/evidence/2624/R3-rollback-concurrency.txt`
- `docs/evidence/2624/evidence.json`
- `docs/evidence/2624/pre-ready-2624.json`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 闸门与 SQL 生成（未授权 / confirm 不等 / 跳过类 / SQL 形状） | `scripts/reconcile-user-cache.sh`、`scripts/tests/test_reconcile_user_cache_apply_2624.sh`、`scripts/fixtures/reconcile-2624/iam.tsv` | `bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r1` | 未设 `OWNER_APPROVED_USER_CACHE_RECONCILE`；`--confirm-rows` 与实际不符；fixture 含 ORPHAN/MISSING/ROLE 行 | 一次性 PG 15433（仅 SQL 生成，不执行） | `UNAUTHORIZED_APPLY=1`、`APPLY_CONFIRM_MISMATCH=1`、`REPAIR_ROWS=6`、`SKIPPED_ORPHAN=2`、`SKIPPED_MISSING_LOCAL=1`、`DB_WRITES=0`、`SQL_HAS_CAS=1`、`SQL_HAS_AUDIT=6`、`SQL_HAS_POSTCHECK=1` | R1 |
| R2 单事务真修复 + 审计 + 复查归零 | `scripts/reconcile-user-cache.sh` | `bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r2` | 授权 env + `--confirm-rows 6`；库内 a7 行漂移 | 一次性 PG 15433 | `DB_WRITES=6`、`AUDIT_ROWS=6`、`BATCH_ID_MATCH=1`、复查 `TENANT_MISMATCH=0`、`ORG_MISMATCH=0`、`NIL_UUID=0`、`ORPHAN_LOCAL=2`（不变） | R2 |
| R3 回滚还原 + 并发中止（全量回滚） | `scripts/reconcile-user-cache.sh` | `bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r3` | 应用 `ROLLBACK_FILE`；另起一例在 apply 期间篡改 1 行 | 一次性 PG 15433 | `RESTORED_ROWS=6`、`ROLLBACK_AUDIT_ROWS=6`；并发例 `CONCURRENT_ABORT=1`、`DB_WRITES=0`、`ROWS_CHANGED_AFTER_ABORT=0` | R3 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `scripts/fixtures/reconcile-2624/{iam.tsv,local.tsv}`：3 行 TENANT + 2 行 ORG + 1 行 NIL + 2 行 ORPHAN + 1 行 MISSING + 1 行 ROLE_UNVERIFIED | `bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r1` | `docs/evidence/2624/R1-apply-gates.txt` | `TEST_EXIT=0`、`UNAUTHORIZED_APPLY=1`、`APPLY_CONFIRM_MISMATCH=1`、`REPAIR_ROWS=6`、`SKIPPED_ORPHAN=2`、`SKIPPED_MISSING_LOCAL=1`、`SKIPPED_ROLE_UNVERIFIED=1`、`DB_WRITES=0`、`SQL_HAS_CAS=1`、`SQL_HAS_AUDIT=6`、`SQL_HAS_POSTCHECK=1`、`SQL_ORDERED_BY_IAM_SUB=1` | 未授权仍写库、`--confirm-rows` 不符仍执行、ORPHAN/MISSING 被修、SQL 缺 compare-and-set 谓词或审计或后置校验、语句未按 `iam_sub` 排序 → 任一即 FAIL |
| R2 | 一次性 PG：`users` 6 行漂移（3 TENANT + 2 ORG + 1 NIL）+ 2 行 ORPHAN + fixture IAM 侧 | `bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r2` | `docs/evidence/2624/R2-apply-pg.txt` | `TEST_EXIT=0`、`DB_WRITES=6`、`AUDIT_ROWS=6`、`AUDIT_ACTION_USER_CACHE_RECONCILE=6`、`BATCH_ID_MATCH=1`、`RECHECK_TENANT_MISMATCH=0`、`RECHECK_ORG_MISMATCH=0`、`RECHECK_NIL_UUID=0`、`RECHECK_ORPHAN_LOCAL=2` | 修了 ORPHAN、审计缺行或 `details` 缺旧值/新值/批次、复查仍有同类差异、`DB_WRITES` 与实际改动行数不符 → 任一即 FAIL |
| R3 | R2 的库快照 + 生成的 `--rollback-file`；并发例：apply 前把 1 行改回旧值之外的第三值 | `bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r3` | `docs/evidence/2624/R3-rollback-concurrency.txt` | `TEST_EXIT=0`、`RESTORED_ROWS=6`、`ROLLBACK_AUDIT_ROWS=6`、`POST_ROLLBACK_TENANT_MATCH=6`、`CONCURRENT_ABORT=1`、`DB_WRITES=0`、`ROWS_CHANGED_AFTER_ABORT=0` | 回滚后任一行未还原、部分写入残留（半修）、并发场景非整体中止 → 任一即 FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r1 > docs/evidence/2624/R1-apply-gates.txt` | `docs/evidence/2624/R1-apply-gates.txt` | exit 0 且含 `TEST_EXIT=0` `UNAUTHORIZED_APPLY=1` `APPLY_CONFIRM_MISMATCH=1` `REPAIR_ROWS=6` `SKIPPED_ORPHAN=2` `SKIPPED_MISSING_LOCAL=1` `SKIPPED_ROLE_UNVERIFIED=1` `DB_WRITES=0` `SQL_HAS_CAS=1` `SQL_HAS_AUDIT=6` `SQL_HAS_POSTCHECK=1` `SQL_ORDERED_BY_IAM_SUB=1` | PASS（12 项 token 全为期望值） | 未授权写库、confirm 不符仍执行、跳过类被修、SQL 缺 CAS/审计/后置校验、未排序 → FAIL | dev |
| R2 | `bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r2 > docs/evidence/2624/R2-apply-pg.txt` | `docs/evidence/2624/R2-apply-pg.txt` | exit 0 且含 `TEST_EXIT=0` `DB_WRITES=6` `AUDIT_ROWS=6` `AUDIT_ACTION_USER_CACHE_RECONCILE=6` `BATCH_ID_MATCH=1` `RECHECK_TENANT_MISMATCH=0` `RECHECK_ORG_MISMATCH=0` `RECHECK_NIL_UUID=0` `RECHECK_ORPHAN_LOCAL=2` | PASS（9 项 token 全为期望值） | 修 ORPHAN、审计缺行/缺字段、复查残留差异、写入行数不符 → FAIL | dev |
| R3 | `bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r3 > docs/evidence/2624/R3-rollback-concurrency.txt` | `docs/evidence/2624/R3-rollback-concurrency.txt` | exit 0 且含 `TEST_EXIT=0` `RESTORED_ROWS=6` `ROLLBACK_AUDIT_ROWS=6` `POST_ROLLBACK_TENANT_MATCH=6` `CONCURRENT_ABORT=1` `DB_WRITES=0` `ROWS_CHANGED_AFTER_ABORT=0` | PASS（7 项 token 全为期望值） | 回滚未还原、半修残留、并发非整体中止 → FAIL | dev |

### Artifact Provenance

- 每个文本 artifact 首行 `# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `GENERATED_AT_HEAD=<40 位 sha>`；证据必须在 fix commit **之后**生成（`FIX_REF` 可覆盖锚点，默认 `git log --grep="^fix(issue-2624)"`）。
- 重命令（`psql` 建库/写库、`go build` 如涉及）一律经 `~/.config/opencode/scripts/heavy-run.sh` 取锁；测试库用一次性 PostgreSQL（`127.0.0.1:15433`，`scripts/lib-2341-env.sh` 口径），**禁止**使用 `jobmaster-postgres` 的 dev 库或任何预生产/生产库。
- 逐条清单（全部位于 `docs/evidence/2624/`）：
  - `docs/evidence/2624/R1-apply-gates.txt`
  - `docs/evidence/2624/R2-apply-pg.txt`
  - `docs/evidence/2624/R3-rollback-concurrency.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2624 --summary-file tmp/pre-ready-2624-summary.md --changed-files tmp/pre-ready-2624-changed.txt --evidence-file docs/evidence/2624/evidence.json --report docs/evidence/2624/pre-ready-2624.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - 差异与明细 = `scripts/reconcile-user-cache.sh`（#2623 产出；唯一 owner），本单**只读它的结构化输出**，不重算判定；
  - 闸门口径 = `scripts/report-merchant-direct-role-drift-2462.sh`（`OWNER_APPROVED_*` + `--confirm-rows`）；
  - 审计表结构 = `backend/models/audit_log.go`（无 migration 需求）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `grep -n 'apply\|--confirm-rows' scripts/reconcile-user-cache.sh` → 修前应**无** `--apply`（#2623 只读）；若 #2623 已实现 `--apply`，**HOLD** 并发 `## Investigation Packet`（父 P2 冲突）；
  - `grep -rn 'USER_CACHE_RECONCILE' backend/ scripts/` → 修前 0 命中（新审计 action 未存在）；
  - `bash scripts/reconcile-user-cache.sh --fixture scripts/fixtures/reconcile-2624 --json | jq '.rows|length'` → 与 `REPAIR_ROWS + SKIPPED_*` 一致（明细行数守恒）。
- **损坏输入拒绝规则**：
  - 明细里出现未知类别 / 缺 `local_id` / 旧值为非 uuid → `exit 3`，不生成 SQL；
  - `REPAIR_ROWS=0` 时 `--apply` 必须 `exit 2`（无事可做不是成功修复），`DB_WRITES=0`；
  - 目标行在 `BEGIN` 前被改（旧值不匹配）→ 后置校验 `RAISE` → 整体回滚。
- **工具盲区**：
  - 本单测试在一次性 PG 上跑；真实预生产/prod 的 `--apply` 属运维执行（父 `P5`/`P8`），由 closing 时的 `[Ops-Followup]` 跟踪；
  - `audit_logs` 的 `ip_address/user_agent` 为空字符串（脚本行为无 HTTP 上下文），前端展示不依赖这两列；
  - `WITH RECURSIVE` 级的 org 层级推导属于 #2623，本单不实现。
- **契约影响**：无 API 契约变更；新增一个内部审计 `action` 枚举值 `USER_CACHE_RECONCILE` / `USER_CACHE_RECONCILE_ROLLBACK`（不写进 `docs/spec/api/api.md`，因不对前端暴露筛选项）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 判定逻辑与 token 契约的**唯一 owner 仍是 #2623**；本单只新增写通道，`scripts/reconcile-user-cache.sh` 不加 `--apply` 时行为与 L1 完全一致 | 本计划锁定 |
| D2 | 修复范围 = `TENANT_MISMATCH` / `ORG_MISMATCH` / `NIL_UUID`；`ORPHAN_LOCAL` / `MISSING_LOCAL` / `ROLE_UNVERIFIED` **一律跳过并计数**（禁止静默丢弃） | 本计划锁定 |
| D3 | `ROLE_MISMATCH` 修复默认**关闭**：只有 #2623 计划锁定映射表且本次运行显式 `--include-role` 才修；否则 `ROLE_APPLY_SKIPPED=1` | 父 `P3` |
| D4 | 审计复用 `audit_logs`（`action=USER_CACHE_RECONCILE`，`details={batch_id,category,old,new}`），**不新增表/不写 migration**（避免 #1913 类快照库缺件风险） | 本计划锁定 |
| D5 | 单事务 + compare-and-set + 事务末后置校验：任一目标行旧值不匹配或未达新值 → 整体回滚（`DB_WRITES=0`），禁止「能修几行算几行」 | 本计划锁定 |
| D6 | `--sql-out` / `--rollback-file` **必填**且可独立执行（运维可在 DBA 复核后手动执行）；`--sql-only` 用于离线断言 | 本计划锁定 |
| D7 | AI **不得**对 dev/预生产/生产库执行 `--apply`；证据只在一次性 PG（15433）产生，真实修复由 Owner 授权后运维执行（红线） | 本计划锁定 |
| D8 | `MISSING_LOCAL` 是否建行属产品决策（新建 `users` 行会影响登录/权限链路）→ 本单跳过，若 Owner 需要则另立 Issue | 本计划锁定 |
| D9 | 本单不做数据回填批量作业（#2527/#2553–#2557），不做 `ORPHAN_LOCAL` 离职流程 | 本计划锁定 |
| D10 | 生产 timer 与告警在 #2625（L3）；本单不安装任何定时器 | 本计划锁定 |

### Out of Scope

- 差异判定 / 分类 / token 契约变更（唯一 owner #2623）→ 若需改，回 #2623 计划。
- `ORPHAN_LOCAL`（离职/解绑）处理与资产转接；`MISSING_LOCAL` 建行（→ 产品决策另立 Issue）。
- `ROLE_MISMATCH` 的映射表定义（#2623 锁定；本单只消费）。
- 存量批量回填 / 缓存行纠正（#2527 / #2553–#2557）。
- 定时任务、告警 Issue、timer 安装（#2625）。
- 新表 `cache_reconcile_log`（若将来审计需要专门查询再做，本单复用 `audit_logs`）。
- 生产/预生产库上的实际执行（`[Ops-Followup of #2624]`，Owner 授权 + 运维执行）。
- 全量 `go test ./...` 作为合同行（本单不动 Go 代码；如涉 `audit_logs` 展示另议）。

### Dependencies

- #2623

### Workload

- Size: M
- Basis: 脚本扩展 1（`scripts/reconcile-user-cache.sh`）+ 测试 1 + fixture 2 + 文档 1 小节；3 条证据行（R1/R2/R3）；无 migration、无 Go 代码、无前端、无 API 变更
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 本地 `users` 缓存行长期错误（tenant/org 与 IAM 不一致）→ 具体功能突然 404/数据范围错乱，且无任何例行校验 | #2622 正文；#2620 实例 |
| 2 | 触发条件（trigger） | 写路径漏写/写错 + 迁移/重放/手工操作引入漂移；错误静默存在直到某功能命中 | #2622 正文「为什么只修代码不够」 |
| 3 | 根因（root cause） | 缺少「IAM 权威 ↔ 本地缓存」的可执行校验与可控修复通道；人工发现是唯一途径 | #2622 正文 |
| 4 | 测试盲点（test blind spot） | 既有测试只覆盖单点写路径，无「缓存与权威一致」的断言；跨库一致性从未被测 | 全仓无 reconcile 相关测试（`grep -rn reconcile backend scripts` 仅媒体 GC） |
| 5 | 同类扫描（same-class paths） | 同族差异共 6 类；本单只修 3 类，另 3 类显式跳过并计数（不静默） | 变更点 2 |
| 6 | 修复层（fix layer） | 运维工具层（脚本），不改业务写路径；判定归 #2623，修复归本单，例行化归 #2625 | 父 #2622 拓扑 |
| 7 | 红绿实证（red-green） | 红：仅落测试时 `--apply` 不存在 → 断言失败；绿：实现后 R1/R2/R3 全绿 | 变更点 5 |
| 8 | 防复发（prevention） | ① 审批 + `--confirm-rows` 双闸门（#2484 防线）；② compare-and-set + 后置校验（半修不可能）；③ 审计留痕可反查批次；④ 回滚文件必产 | D4/D5/D6；R1/R3 |
| 9 | 影响面（blast radius） | 只影响 `users.tenant_id/org_id`（3 类漂移行）+ 新增审计行；不改写路径、不改 API、不改只读行为 | R2 的 `RECHECK_*`、R3 的回滚 |
| 10 | 未修边界（deferred） | ORPHAN/MISSING/ROLE 跳过；role 映射表归 #2623；例行与告警归 #2625；真实执行归 `[Ops-Followup]` | Out of Scope、D2/D3/D7/D8/D10 |

*Model: deepseek/deepseek-flash*

