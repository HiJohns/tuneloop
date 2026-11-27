## Work Summary — #2624 [Sub-task of #2622 L2] 用户缓存对账「可选修复」`--apply`

### 修改内容

1. **受闸门约束的写通道**（扩展 `scripts/reconcile-user-cache.sh`；**判定逻辑与 token 契约零改动**，不加 `--apply` 时行为与 L1 只读完全一致）：
   - 闸门：`OWNER_APPROVED_USER_CACHE_RECONCILE=1` → 否则 `UNAUTHORIZED_APPLY=1` / `exit 2` / `DB_WRITES=0`；`--confirm-rows` 必须与本次 `REPAIR_ROWS` **逐字相等**（`APPLY_CONFIRM_MISMATCH=1`）；`REPAIR_ROWS=0` → `NOTHING_TO_REPAIR=1`（无事可做不是成功修复）；`--sql-out` / `--rollback-file` 必填；`--sql-only` 只产 SQL 不连库。
   - 修复形态：**单事务** `BEGIN; … COMMIT;` + 每行 **compare-and-set** `UPDATE users SET tenant_id=…, org_id=…, updated_at=now() WHERE id=… AND iam_sub=… AND tenant_id=<旧> AND org_id=<旧>;` + 每行 `audit_logs` 审计（`action=USER_CACHE_RECONCILE`，`details={batch_id,category,old,new}`）+ 事务末**后置校验** `DO $$ … RAISE EXCEPTION 'reconcile: concurrent change detected for <id>' … $$;`（任一行未达预期即整体回滚，杜绝半修）。
   - 回滚文件镜像生成（`old`/`new` 互换，`action=USER_CACHE_RECONCILE_ROLLBACK`），两份 SQL 均可独立执行。
   - `BATCH_ID=<UTC yyyyMMddTHHMMSSZ>-<6hex>` 贯穿 SQL、审计 `details`、回滚 SQL 与输出 token；语句按 `iam_sub` 升序（确定性 diff）。
   - 修复范围：仅 `TENANT_MISMATCH` / `ORG_MISMATCH` / `NIL_UUID`；`ORPHAN_LOCAL` / `MISSING_LOCAL` / `ROLE_UNVERIFIED` 跳过并计数；`ROLE_MISMATCH` 默认跳过（`ROLE_APPLY_SKIPPED=1`）；未知类别或明细缺 `local_id`/非 uuid → `exit 3` 不生成 SQL。
2. **测试** `scripts/tests/test_reconcile_user_cache_apply_2624.sh`（r1 闸门 + SQL 形状离线；r2 一次性 PG 真修复 + 审计 + 复查归零；r3 回滚还原 + 并发中止）。
3. **fixtures** scripts/fixtures/reconcile-2624/iam.tsv 与 scripts/fixtures/reconcile-2624/local.tsv：6 行可修（3 TENANT + 2 ORG + 1 NIL）+ 2 ORPHAN + 1 MISSING + 1 ROLE_UNVERIFIED。
4. **文档** `docs/ops/user-cache-reconcile.md` 追加 **§9 可选修复通道 `--apply`**：命令、闸门、范围表、两份 SQL 形态、token 与运维执行顺序（含「AI 不得对真实库执行 `--apply`」红线）。

### 涉及文件

- `scripts/reconcile-user-cache.sh`（扩展写通道；只读路径不变）
- `scripts/tests/test_reconcile_user_cache_apply_2624.sh`（新增）
- `scripts/fixtures/reconcile-2624/iam.tsv`、`scripts/fixtures/reconcile-2624/local.tsv`（新增）
- `docs/ops/user-cache-reconcile.md`
- `docs/evidence/2624/plan.md`、`docs/evidence/2624/R1-apply-gates.txt`、`docs/evidence/2624/R2-apply-pg.txt`、`docs/evidence/2624/R3-rollback-concurrency.txt`、`docs/evidence/2624/evidence.json`、`docs/evidence/2624/pre-ready-2624.json`、`docs/evidence/2624/summary.md`、`docs/evidence/2624/changed-files.txt`

### Commit

- fix: `bc543314a`（`fix(issue-2624): gated --apply repair channel with compare-and-set, audit and rollback SQL`）
- evidence: `docs(evidence): issue-2624 …`（fix commit 之后从该 commit 重新生成，`dirty=false`）

### 分支基线说明（需 Auditor 知悉）

- 本单依赖 #2623（L1，`status:ready` 未合入 `develop`），且需要**修改 #2623 产出的 `scripts/reconcile-user-cache.sh`**。因此本分支**叠在 `origin/issue-2623` 之上**（L1 源码 + 本单 hunk），避免同文件 add/add 冲突。
- 合并顺序必须是 **#2623 → #2624**（本分支已含 #2623 全部提交，按序合并为 fast-forward/无冲突）。

### 实现前差异核对（计划「Input Truth & Tool Preflight」逐条）

| 检查 | 期望 | 实测 |
|---|---|---|
| `scripts/reconcile-user-cache.sh` 是否已含 `--apply` | 修前无 | 0 命中 ✅（#2623 为纯只读；未触发 HOLD/Investigation Packet） |
| `grep -rn 'USER_CACHE_RECONCILE' backend/ scripts/` | 修前 0 | 0 命中 ✅（新审计 action 由本单引入） |
| fixture 明细行数守恒 | `REPAIR_ROWS + SKIPPED_*` = 明细行数 | 6 + 2 + 1 + 0 + 1 = 10 = `DETAIL_COUNT` ✅ |
| #2623 只读回归（不加 `--apply`） | 三行仍全绿 | `test_reconcile_user_cache_2623.sh r1|r2|r3` 全部 exit 0 ✅ |

### 验证层级（Issue #70）

- 本单为子任务（`Parent: #2622`）：只跑**定向**测试 r1/r2/r3（一次性 PG 15433），不跑全量 `go test ./...`（本单不动 Go 代码）。
- 同时复跑 #2623 的 r1/r2/r3 作为只读回归（结果全绿，见上表）。

### Remote

remote: origin/issue-2624; fix=bc543314a pushed=yes; evidence 提交 pushed=yes

### 红绿实证

红（`--apply` 尚未实现时的同一入口）：`bash scripts/reconcile-user-cache.sh --apply …` → `RESULT: FAIL (unknown argument: --apply)` / `exit 2`，`REPAIR_ROWS`/`UNAUTHORIZED_APPLY`/`APPLY_CONFIRM_MISMATCH`/`SQL_HAS_CAS` 等 token 全部不存在 → r1/r2/r3 断言失败。

绿（fix commit `bc543314a`）：三行 `TEST_EXIT=0` / `RESULT: PASS`：

```
R1: UNAUTHORIZED_APPLY=1 APPLY_CONFIRM_MISMATCH=1 REPAIR_ROWS=6 SKIPPED_ORPHAN=2
    SKIPPED_MISSING_LOCAL=1 SKIPPED_ROLE_UNVERIFIED=1 DB_WRITES=0 SQL_HAS_CAS=1
    SQL_HAS_AUDIT=6 SQL_HAS_POSTCHECK=1 SQL_ORDERED_BY_IAM_SUB=1
R2: DB_WRITES=6 AUDIT_ROWS=6 AUDIT_ACTION_USER_CACHE_RECONCILE=6 BATCH_ID_MATCH=1
    RECHECK_TENANT_MISMATCH=0 RECHECK_ORG_MISMATCH=0 RECHECK_NIL_UUID=0 RECHECK_ORPHAN_LOCAL=2
R3: RESTORED_ROWS=6 ROLLBACK_AUDIT_ROWS=6 POST_ROLLBACK_TENANT_MATCH=6 CONCURRENT_ABORT=1
    DB_WRITES=0 ROWS_CHANGED_AFTER_ABORT=0
```

并发中止实证原文（R3 artifact 内）：

```
psql:.../r3-concurrent.sql:65: ERROR:  reconcile: concurrent change detected for aaaa0000-0000-4000-8000-000000000001
```

### Accepted Plan 对照（逐条完成）

| 计划项 | 状态 |
|---|---|
| 变更点 1 CLI 契约（`--apply` / `--confirm-rows` / `--sql-out` / `--rollback-file` / `--sql-only` / `--iam-fixture`） | ✅ |
| 变更点 2 修复范围与跳过（3 类修 + 3 类跳过 + 未知类别 fail-fast） | ✅ |
| 变更点 3 两份 SQL（单事务 / CAS / 审计 / 后置校验 / BATCH_ID / 按 iam_sub 排序） | ✅ |
| 变更点 4 新增 token（L1 token 不变） | ✅ |
| 变更点 5 测试两档（离线 SQL 形状 + 一次性 PG 真修复）+ 红绿 | ✅ |
| Decision Lock D1–D10 | ✅（D1 只读不变；D2 三类修/三类跳过；D3 role 默认跳过；D4 复用 `audit_logs` 无 migration；D5 单事务 + CAS + 后置校验；D6 两份 SQL 必填可独立执行；D7 未对任何真实库执行 `--apply`；D8 MISSING_LOCAL 跳过；D9 未做回填；D10 未装任何 timer） |
| Out of Scope 未越界 | ✅ 未改判定逻辑/token、未建新表、未装 timer、未碰生产 |

### 计划口径的两处实现说明（需 Auditor 知悉，均为安全侧收敛）

1. **执行命令**：计划写「`psql -1 -f <sql>`（`-1` = 单事务）」，但同一计划要求 SQL 内含 `BEGIN; … COMMIT;`。两者叠加会导致「事务内再开事务」。实现选择：**SQL 文件自带 `BEGIN/COMMIT`（单事务、可独立交付给 DBA），执行时不加 `-1`** —— 语义仍是单事务，且避免 psql 告警噪声。
2. **`--include-role`**：计划要求「映射表锁定 + 显式开关」才修 `ROLE_MISMATCH`。当前 `ROLE_REPAIR_SUPPORTED=0`（#2623 结论：多对一不可逆），故实现为**保留开关但恒 `ROLE_APPLY_SKIPPED=1`**、`SKIPPED_ROLE=<ROLE_MISMATCH 计数>`，不产出角色修复语句。

### Evidence Contract（3/3 行，全部 PASS）

| 行 | 命令 | 产物 | 关键 token | 状态 |
|---|---|---|---|---|
| R1 | `bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r1` | `docs/evidence/2624/R1-apply-gates.txt` | TEST_EXIT=0 / UNAUTHORIZED_APPLY=1 / APPLY_CONFIRM_MISMATCH=1 / REPAIR_ROWS=6 / SKIPPED_ORPHAN=2 / SKIPPED_MISSING_LOCAL=1 / SKIPPED_ROLE_UNVERIFIED=1 / DB_WRITES=0 / SQL_HAS_CAS=1 / SQL_HAS_AUDIT=6 / SQL_HAS_POSTCHECK=1 / SQL_ORDERED_BY_IAM_SUB=1 | PASS |
| R2 | `bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r2` | `docs/evidence/2624/R2-apply-pg.txt` | TEST_EXIT=0 / DB_WRITES=6 / AUDIT_ROWS=6 / AUDIT_ACTION_USER_CACHE_RECONCILE=6 / BATCH_ID_MATCH=1 / RECHECK_TENANT_MISMATCH=0 / RECHECK_ORG_MISMATCH=0 / RECHECK_NIL_UUID=0 / RECHECK_ORPHAN_LOCAL=2 | PASS |
| R3 | `bash scripts/tests/test_reconcile_user_cache_apply_2624.sh r3` | `docs/evidence/2624/R3-rollback-concurrency.txt` | TEST_EXIT=0 / RESTORED_ROWS=6 / ROLLBACK_AUDIT_ROWS=6 / POST_ROLLBACK_TENANT_MATCH=6 / CONCURRENT_ABORT=1 / DB_WRITES=0 / ROWS_CHANGED_AFTER_ABORT=0 | PASS |

依据 `docs/evidence/2624/evidence.json`（schema_version=1、issue=2624、build=1.0.bc543314a、commit=bc543314a…、dirty=false、plan_sha256=06a876b6…）。

### G0 Golden Acceptance

| 行 | artifact | assertions | negative |
|---|---|---|---|
| R1 | `docs/evidence/2624/R1-apply-gates.txt` | 12 项合同 token + `UNAUTHORIZED_EXIT=2`、`UNAUTHORIZED_NO_SQL=0`、`SQL_SINGLE_TRANSACTION=1`、`ROLLBACK_ACTION_ROWS=6`、`ROLLBACK_MIRRORS_OLD=1` | 未授权仍写库、confirm 不符仍执行、ORPHAN/MISSING 被修、SQL 缺 CAS/审计/后置校验、未排序 → 任一即 FAIL（实测 0 命中） |
| R2 | `docs/evidence/2624/R2-apply-pg.txt` | 9 项合同 token + `AUDIT_DETAILS_FIELDS=6`、`RECHECK_DB_WRITES=0` | 修了 ORPHAN、审计缺行/缺字段、复查残留差异、写入行数不符 → 任一即 FAIL |
| R3 | `docs/evidence/2624/R3-rollback-concurrency.txt` | 7 项合同 token + `CONCURRENT_SQL_EXIT_NONZERO=1`、`FULL_TABLE_UNCHANGED_BY_ABORT=1`、`NO_PARTIAL_WRITE=0` | 回滚后任一行未还原、半修残留、并发非整体中止 → 任一即 FAIL（实测整表 hash 不变、审计行数不变） |

### Verification Matrix（矩阵 3/3）

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 闸门与 SQL 生成（未授权 / confirm 不等 / 跳过类 / SQL 形状） | PASS | `docs/evidence/2624/R1-apply-gates.txt` |
| R2 单事务真修复 + 审计 + 复查归零 | PASS | `docs/evidence/2624/R2-apply-pg.txt` |
| R3 回滚还原 + 并发中止（全量回滚） | PASS | `docs/evidence/2624/R3-rollback-concurrency.txt` |

### 证据真实性 / 持久化

- 三份 artifact 位于 `docs/evidence/2624/`（随 `docs(evidence)` 提交），首行 provenance `# build=1.0.bc543314a # commit=bc543314a35c2df02969644b8bbf3b0213649395 # dirty=false`，与 evidence.json 一致；证据在 fix commit **之后**生成。
- 三条证据对应不同断言面（闸门/SQL 形状、真实写+审计+复查、回滚+并发），无复用。
- 库全部是一次性 PG（`apply_2624_*`，用完 drop）；**未对 dev/预生产/生产执行过 `--apply`**。

### 测试保真

- 并发中止不是脚本自报：先 `--sql-only` 取出 SQL 快照 → 用 psql 改一行成「第三值」→ 再直接执行该 SQL 文件 → 断言 psql 非 0 退出、日志含 `reconcile: concurrent change detected`、**整表 md5 不变**、`audit_logs` 行数不变（无半修）。
- `DB_WRITES=6` 与真实改动一致：`users` 6 行被改 + `audit_logs` 6 行新增均按 SQL 查询核对（不是只看 token）。
- 回滚后用 `VALUES` 连接逐行核对 6 行的 `(tenant, org)` 回到旧值（`POST_ROLLBACK_TENANT_MATCH=6`），不是只看条数。

### 依赖顺序 / 验收规则

- 计划 `## Dependencies`：`#2623`（ready）——已满足；本单**未修改判定逻辑与 token 契约**（D1），L1 只读回归三行全绿。
- 未修改任何既有验收工具/规则；未出现连续两轮验收失败；无延期项。

### 人工 / 部署侧（不由 Developer 完成）

| 事项 | 负责方 | 回执要求 |
|---|---|---|
| 预生产/生产真实库的 `--apply`（Owner 授权 + DBA 复核 SQL 后执行）与修复后对账归零确认 | 运维 / Owner | 关单时按 `[Ops-Followup of #2624]`（`waitForConfirm`，无 `status:*`）另记；需回评 BATCH_ID、`DB_WRITES`、复查 token |

*Model: deepseek/deepseek-flash*
