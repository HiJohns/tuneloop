## Work Summary — #2623 [Sub-task of #2622 L1] 用户缓存只读对账工具

### 修改内容

1. **新增只读对账工具** `scripts/reconcile-user-cache.sh`：把本地 `users` 缓存与 IAM 权威关系比对，输出 6 类差异分类 + 机器可读 token。
   - 判定口径（与 beaconiam `resolveUserOrg` 对齐）：`loginable` = 活跃关系中 `role != 'member'`（`member` 是零权限影子绑定，不参与上下文推导）。
   - `loginable == 1` → `expect_org = relation.org_id`、`expect_tenant = parent_id 递归根`；本地按互斥优先级 `NIL_UUID` > `TENANT_MISMATCH` > `ORG_MISMATCH` 归类。
   - `loginable == 0`：有 `member` 关系 → `MEMBER_ONLY`；无任何活跃关系时本地非零 → `ORPHAN_LOCAL`，本地零 UUID → 正常（顾客）。
   - `loginable >= 2` → `CONTEXT_AMBIGUOUS`（多上下文用户本地只能存一个，判差异必然误报）。
   - 角色只做**集合判定**（`ToIAMRole` 多对一不可逆）：`OWNER→{merchant_admin}`、`ADMIN→{site_admin, merchant_admin}`、`STAFF→{site_member, merchant_staff}`、`repair_technician→{repair_technician}`；白名单外/本地 role 空 → `ROLE_UNVERIFIED`；`ROLE_REPAIR_SUPPORTED=0`（不产出修复目标值）。
   - 父链求根带 64 跳上限与环检测 → `ORG_UNRESOLVED`（不崩、不死循环）。
   - **只读硬闸门**：连接后 `SET default_transaction_read_only = on`（并设 `PGOPTIONS=-c default_transaction_read_only=on`），源码零写语句，输出恒为 `DB_WRITES=0`。
   - 双输入路径共用**同一判定函数**（D9）：`--fixture <dir>` / `--iam-fixture` / `--local-fixture` 离线复跑；两侧可独立取源（混合模式 = IAM fixture + 本地真库）。
   - 退出码：`0` 比对完成（有差异也算 0）；`2` 参数/连接/env 错误 + `RESULT: FAIL`；`3` 输入损坏（字段数不符 / `is_active` 非 0/1 / `role` 非标识符）。
2. **新增回归/证据测试** `scripts/tests/test_reconcile_user_cache_2623.sh`（`r1` / `r2` / `r3`）。
3. **新增 fixture** `scripts/fixtures/reconcile-2623/iam.tsv` + local.tsv（覆盖 1 关系 / 0 关系 / member-only / 多关系 / 缺失本地 / 角色集合 4 例）。
4. **新增文档** `docs/ops/user-cache-reconcile.md`：判定口径 A/B、为什么多上下文与 `member` 只计数、`ROLE_REPAIR_SUPPORTED=0` 的理由（多对一不可逆）、#2626 设计缺口、`--fixture` 复跑、只读声明与「修复见 #2624」。

### 涉及文件

- `scripts/reconcile-user-cache.sh`（新增）
- `scripts/tests/test_reconcile_user_cache_2623.sh`（新增）
- `scripts/fixtures/reconcile-2623/iam.tsv`（新增）
- `scripts/fixtures/reconcile-2623/local.tsv`（新增）
- `docs/ops/user-cache-reconcile.md`（新增）
- `docs/evidence/2623/plan.md`、`docs/evidence/2623/R1-classification.txt`、`docs/evidence/2623/R2-readonly-negative.txt`、`docs/evidence/2623/R3-live-db-readonly.txt`、`docs/evidence/2623/evidence.json`、`docs/evidence/2623/pre-ready-2623.json`、`docs/evidence/2623/summary.md`、`docs/evidence/2623/changed-files.txt`

### Commit

- fix: `2ad70e7ba`（`fix(issue-2623): read-only user cache reconcile tool with 6-class diff + fixture re-run`）
- evidence: `docs(evidence): issue-2623 …`（fix commit 之后从该 commit 重新生成，`dirty=false`）

### 实现前差异核对（计划「Input Truth & Tool Preflight」逐条）

| 检查 | 期望 | 实测 |
|---|---|---|
| `origin/develop` 中 reconcile-user-cache 命中 | 修前 0 | 0 ✅ |
| `grep -c IAMRole backend/services/role_templates.go` | 记录基线 | 16（**未修改该文件**，`git diff --stat` 为空） ✅ |
| 工具源码写语句 | 修后 0 | `FORBIDDEN_SQL_STMTS=0`（R2 断言） ✅ |
| 一次性 PG 口径 | 127.0.0.1:15433（非 jobmaster-postgres） | `scripts/lib-2341-env.sh` + `require_pg` ✅ |

### 验证层级（Issue #70）

- 本单为子任务（`Parent: #2622`）：只跑**定向**回归 `scripts/tests/test_reconcile_user_cache_2623.sh r1|r2|r3`，不跑全量 `go test ./...` / 前端 build（本单不动 Go / 前端 / API）。
- R3 使用一次性 PostgreSQL（127.0.0.1:15433），**未触碰** dev `jobmaster-postgres`、预生产或生产库。

### Remote

remote: origin/issue-2623; fix=2ad70e7ba pushed=yes; evidence 提交 pushed=yes

### 红绿实证

红（实现前）：`scripts/reconcile-user-cache.sh` 不存在，`bash scripts/tests/test_reconcile_user_cache_2623.sh r1|r2|r3` 三行全部无法执行（`No such file`，退出码非 0），分类 token 一个都不存在。

绿（fix commit `2ad70e7ba`）：三行全部 `TEST_EXIT=0` / `RESULT: PASS`：

```
R1: TOOL_EXIT=0; TENANT_MISMATCH=2 ORG_MISMATCH=2 NIL_UUID=1 MISSING_LOCAL=1 ORPHAN_LOCAL=1
    ROLE_MISMATCH=2 ROLE_UNVERIFIED=1 CONTEXT_AMBIGUOUS=1 MEMBER_ONLY=1
    ROWS_SORTED=1 JSON_TOKENS_MATCH=1 DB_WRITES=0
R2: FORBIDDEN_SQL_STMTS=0 READONLY_DSN_GUARD=1 BAD_FIXTURE_EXIT=3 UNREACHABLE_EXIT=2
    UNREACHABLE_RESULT_FAIL=1 MISSING_ENV_EXIT=2 FALSE_ZERO_DIFF=0
R3: LIVE_EXIT=0 RECONCILE_SOURCE=live TENANT_MISMATCH=1 CASE_2620_DETECTED=1
    EXIT_0_WITH_DIFF=1 DB_WRITES=0 READONLY_TXN_OK=1 FIXTURE_PARITY=1
```

关键负例实测（R2/R3 内部）：5 列 fixture → `exit 3` + `ROW_FIELDS=5`；`is_active=2` → `exit 3`；库不可达（127.0.0.1:1）→ `exit 2` + `RESULT: FAIL`；env 缺失 → `exit 2` + `RESULT: FAIL`；只读会话内 `INSERT` 被 PG 拒绝（`cannot execute INSERT in a read-only transaction`）。

### Accepted Plan 对照（逐条完成）

| 计划项 | 状态 |
|---|---|
| 变更点 1 `scripts/reconcile-user-cache.sh`（默认双库只读 / `--fixture` / `--json` / 只读硬闸门 / 退出码 0·2·3） | ✅ |
| 变更点 2 token 逐字锁定（17 个 token + `RESULT`） | ✅ |
| 变更点 3 `docs/ops/user-cache-reconcile.md` | ✅ |
| 变更点 4 测试 r1/r2/r3（6 类 + 角色集合 + 只计数三类 + 负例 + 一次性 PG） | ✅ |
| 判定口径 A（含 `ORG_UNRESOLVED` / `MEMBER_ONLY` / `CONTEXT_AMBIGUOUS` / `IAM_MISSING`） | ✅ |
| 判定口径 B（集合判定、`ROLE_REPAIR_SUPPORTED=0`、`functional_roles` 并入允许集合） | ✅ |
| Decision Lock D1–D10 | ✅（D1 唯一实现；D2 多上下文不判差异；D3 `member` 不参与推导；D4 顾客零 UUID 正确；D5 集合判定；D6 缺口登记 #2626；D7 只读闸门；D8 失败必 FAIL；D9 双路径同一函数；D10 真实库只读跑留部署侧） |
| Out of Scope 未越界 | ✅ 未做 `--apply`、timer、回填、backend/services/role_templates.go 修改 |

### 口径说明（计划内两处表述的最小对齐，需 Auditor 知悉）

计划 §B 提到「`|loginable| >= 2` … → `ROLE_UNVERIFIED`」，而 R1 的锁定期望是 `CONTEXT_AMBIGUOUS=1` 且 `ROLE_UNVERIFIED=1`。若多上下文用户同时计入两类，`ROLE_UNVERIFIED` 会变成 2，与锁定的 G0 期望冲突。实现取**同一用户只归一类**（与 D2「歧义只计数、不判差异」一致）：多上下文用户只计 `CONTEXT_AMBIGUOUS`，`ROLE_UNVERIFIED` 只来自「IAM 角色在白名单外/本地 role 为空」（fixture 中为 `u-role-unverified`，IAM role=`VIEWER`）。已在 `docs/ops/user-cache-reconcile.md` §4 明文记录。

### Evidence Contract（3/3 行，全部 PASS）

| 行 | 命令 | 产物 | 关键 token | 状态 |
|---|---|---|---|---|
| R1 | `bash scripts/tests/test_reconcile_user_cache_2623.sh r1` | `docs/evidence/2623/R1-classification.txt` | TEST_EXIT=0 / TENANT_MISMATCH=2 / ORG_MISMATCH=2 / NIL_UUID=1 / MISSING_LOCAL=1 / ORPHAN_LOCAL=1 / ROLE_MISMATCH=2 / ROLE_UNVERIFIED=1 / CONTEXT_AMBIGUOUS=1 / MEMBER_ONLY=1 / ROWS_SORTED=1 / JSON_TOKENS_MATCH=1 / DB_WRITES=0 | PASS |
| R2 | `bash scripts/tests/test_reconcile_user_cache_2623.sh r2` | `docs/evidence/2623/R2-readonly-negative.txt` | TEST_EXIT=0 / FORBIDDEN_SQL_STMTS=0 / READONLY_DSN_GUARD=1 / BAD_FIXTURE_EXIT=3 / UNREACHABLE_EXIT=2 / UNREACHABLE_RESULT_FAIL=1 / MISSING_ENV_EXIT=2 / FALSE_ZERO_DIFF=0 | PASS |
| R3 | `bash scripts/tests/test_reconcile_user_cache_2623.sh r3` | `docs/evidence/2623/R3-live-db-readonly.txt` | TEST_EXIT=0 / DB_WRITES=0 / TENANT_MISMATCH=1 / CASE_2620_DETECTED=1 / EXIT_0_WITH_DIFF=1 / READONLY_TXN_OK=1 | PASS |

依据 `docs/evidence/2623/evidence.json`（schema_version=1、issue=2623、build=1.0.2ad70e7b、commit=2ad70e7ba…、dirty=false、plan_sha256=3a74a9bc…）。

### G0 Golden Acceptance

| 行 | artifact | assertions | negative |
|---|---|---|---|
| R1 | `docs/evidence/2623/R1-classification.txt` | 13 项 token 全为期望值 + `COUNT_SUM_MATCHES_ROWS=12`、`SIX_CLASS_ROWS=9`、`ORG_CYCLE_UNRESOLVED=1`；首两行为 provenance（build/commit/dirty 与 GENERATED_AT_HEAD） | 顾客零 UUID 判差异、多上下文判差异、`ADMIN+site_admin` 误判、`STAFF+merchant_staff` 误判、计数与明细不符、rows 非 `iam_sub` 序 → 任一即 FAIL（实测均为 0 命中） |
| R2 | `docs/evidence/2623/R2-readonly-negative.txt` | 8 项 token 全为期望值 + `BAD_FIXTURE_FIELDS_REPORTED=1`、`BAD_ENUM_EXIT=3`、`BAD_FIXTURE_NO_PASS=0` | 含写语句、不可达仍 PASS、畸形行跳过、缺 env 静默 → FAIL（实测四种失败路径均 exit 2/3 且无 `RESULT: PASS`） |
| R3 | `docs/evidence/2623/R3-live-db-readonly.txt` | 6 项 token 全为期望值 + `RECONCILE_SOURCE=live`、`FIXTURE_PARITY=1`、`CASE_2620_EMAIL` | 真实库写入、#2620 形态漏检、只读事务未生效 → FAIL（实测只读事务拒绝 INSERT、行数前后一致、#2620 行被识别） |

### Verification Matrix（矩阵 3/3）

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 6 类差异 + 角色集合判定（fixture 离线） | PASS | `docs/evidence/2623/R1-classification.txt` |
| R2 只读硬闸门 + 失败负例（不得把「查不到」当无差异） | PASS | `docs/evidence/2623/R2-readonly-negative.txt` |
| R3 真实本地库只读跑一次（一次性 PG + IAM fixture，含 #2620 故障形态） | PASS | `docs/evidence/2623/R3-live-db-readonly.txt` |

### 证据真实性 / 持久化

- 三份 artifact 均位于 `docs/evidence/2623/`（随 `docs(evidence)` 提交）；首行 provenance 为 `# build=1.0.2ad70e7b # commit=2ad70e7bafc8eb42dd02b5ffd5317d70e49a709c # dirty=false`，与 evidence.json 一致；证据在 fix commit **之后**生成。
- 三条证据对应不同入口/断言面，无跨行复用：R1 = fixture 分类 + JSON 一致性；R2 = 静态扫描 + 四种失败路径；R3 = 一次性 PG 真实库只读跑 + 双路径一致性。
- 临时 PG 数据库在用例结束时 drop（`trap`），seed 行数前后一致（`DB_WRITES=0` 用实际行数差证明，不是自报）。

### 测试保真

- r1 的 JSON 一致性断言逐 token 比对 `--json` 与文本输出（17 项），不是只比对单一计数；排序断言对 rows[].iam_sub 做 `LC_ALL=C sort` 比较。
- r3 的 `READONLY_TXN_OK` 不依赖脚本自报：用同一 PG 会话 `SET default_transaction_read_only = on` 后尝试 `INSERT`，必须被 PG 拒绝（实测输出 `ERROR: cannot execute INSERT in a read-only transaction`）。
- r3 的 `FIXTURE_PARITY` 把库里真实行导出为同格式 TSV 再跑全 fixture，13 项 token 与「本地真库 + IAM fixture」逐项相等 → 证明两条输入路径走同一判定（D9）。

### 依赖顺序 / 验收规则

- 计划 `## Dependencies`：无；未修改任何既有验收工具/规则（新增测试脚本不替换既有验收面）。
- 本单是 L1 foundation，#2624（修复）/ #2625（例行）将消费本单锁定的 token 与判定口径（D1）。
- 未出现连续两轮验收失败；无延期项。

### 人工 / 部署侧（不由 Developer 完成）

| 事项 | 负责方 | 回执要求 |
|---|---|---|
| 预生产/生产真实库只读跑一次，输出与人工核对样本一致（D10、父 P8） | 运维 | 关单时按 `[Ops-Followup of #2623]`（`waitForConfirm`，无 `status:*`）另记，需回评实际 token 与样本核对结果 |

*Model: deepseek/deepseek-flash*
