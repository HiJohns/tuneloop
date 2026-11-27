# 用户缓存只读对账（`scripts/reconcile-user-cache.sh`）

> 来源：#2623（父 #2622 L1 foundation）。本工具**只读**，修复见 #2624，例行巡检/告警见 #2625，
> 角色权威来源的设计缺口见 #2626。

## 1. 它解决什么

`tuneloop.users` 是 IAM 的**本地缓存**：`iam_sub` 是 IAM 用户 id，`tenant_id`/`org_id` 是登录 JWT
`tid`/`oid` 的缓存，`role` 是业务域角色。缓存可能与 IAM 权威关系漂移（写路径漏写、历史迁移、
手工操作、多上下文用户只能存一个上下文……），表现为具体功能 404 或数据范围错乱（#2620 实例），
而此前**没有任何例行发现手段**。

本工具把「缓存 ↔ 权威」落成**唯一可执行判定**，输出分类差异 + 机器可读 token，供后续
修复（#2624）与例行（#2625）消费。

## 2. 数据源

| 侧 | 真实库模式来源 | 说明 |
|---|---|---|
| IAM（权威） | `user_org_relations`（`is_active=true`）+ `organizations`（求根）+ `users`（存在性） | env：`BEACONIAM_DB_HOST/PORT/USER/PASSWORD/NAME`（或 `BEACONIAM_DB`） |
| tuneloop（缓存） | `users WHERE deleted_at IS NULL` | env：`TUNELOOP_DB_HOST/PORT/USER/PASSWORD/NAME`（或 `TUNELOOP_DB`） |

- 两库可同名不同 host：**分别配置**，脚本不假设同一连接，脚本内零凭据。
- 只读硬闸门：连接后立即 `SET default_transaction_read_only = on`（并同时设置
  `PGOPTIONS=-c default_transaction_read_only=on`），脚本源码不含任何写入语句；
  输出恒为 `DB_WRITES=0`。
- `--fixture <dir>`（`iam.tsv` + `local.tsv`）或 `--iam-fixture/--local-fixture` 可离线复跑，
  与真实库模式**共用同一个判定函数**（同一条代码路径，不存在「测试绿、线上错」的第二套逻辑）。
  混合模式（IAM fixture + 本地真库）用于一次性 PG 验证。

fixture 格式（TSV，`#` 注释行与空行忽略）：

```
iam.tsv    6 列：user_id  org_id  role  functional_roles(逗号分隔或空)  is_active(0|1)  org_parent_id(空=根)
local.tsv  6 列：iam_sub  user_id  tenant_id  org_id  role  status
```

## 3. 判定口径（唯一谓词）

与 beaconiam 登录时 `resolveUserOrg` 的语义对齐（`internal/auth/handler.go`）：

```
loginable = IAM 活跃关系中 role != 'member' 的记录    # member 是零权限影子绑定
```

### 3.1 上下文（租户 / 组织）

| 情形 | 判定 |
|---|---|
| `loginable == 1` | `expect_org = relation.org_id`；`expect_tenant = root(expect_org)`（`organizations.parent_id` 递归至根） |
| 根无法求出（父链成环/冲突/超过 64 跳） | `ORG_UNRESOLVED`（只计数，不猜） |
| `loginable == 0` 且**有** `member` 关系 | `MEMBER_ONLY`（只计数） |
| `loginable == 0` 且**无任何**活跃关系 | 本地非零 tenant/org → `ORPHAN_LOCAL`；本地零 UUID → 正常（顾客） |
| `loginable >= 2` | `CONTEXT_AMBIGUOUS`（只计数） |
| IAM 有可登录关系、本地无 `users` 行 | `MISSING_LOCAL` |
| 本地有行、IAM 侧无该 user_id | `IAM_MISSING`（只计数，仅在真实 IAM 库模式可判定） |

`loginable == 1` 且本地有行时按**互斥优先级**归类（一行只归一类）：

| 序 | 条件 | 类别 |
|---|---|---|
| 1 | 本地 `tenant_id` 与 `org_id` 均为 `00000000-0000-0000-0000-000000000000` | `NIL_UUID` |
| 2 | 本地 `tenant_id` ≠ `expect_tenant` | `TENANT_MISMATCH` |
| 3 | 本地 `org_id` ≠ `expect_org` | `ORG_MISMATCH` |
| 4 | 否则 | 一致（不计差异） |

### 3.2 角色（只做集合判定）

`ToIAMRole`（`backend/services/role_templates.go`）是**多对一**：

| 正向映射 | IAM 关系角色 |
|---|---|
| `merchant_admin` | `OWNER` |
| `site_admin` | `ADMIN` |
| `site_member` / `merchant_staff` | `STAFF` |
| `repair_technician` | `repair_technician` |

逆推**不唯一**（`STAFF` 可能是 `site_member` 或 `merchant_staff`；`ADMIN` 可能是 `site_admin`，
也可能是以 `ADMIN` 绑定的 `merchant_admin`）。因此本工具只判**集合归属**：

| IAM 角色 | 允许的本地 `users.role` |
|---|---|
| `OWNER` | `{merchant_admin}` |
| `repair_technician` | `{repair_technician}` |
| `ADMIN` | `{site_admin, merchant_admin}` |
| `STAFF` | `{site_member, merchant_staff}` |
| 其它 / 空 | 不可判定 |

- 关系上的 `functional_roles` 若含唯一可正向映射到该 IAM 角色的角色模板码，会并入允许集合（提高精度）；
- 本地 `role` ∉ 允许集合 → `ROLE_MISMATCH`；集合内 → 一致（即使无法确定具体是哪一个）；
- IAM 角色在白名单外、或本地 `role` 为空 → `ROLE_UNVERIFIED`（只计数）；
- **本工具不产出修复目标值** → `ROLE_REPAIR_SUPPORTED=0`（多对一不可逆，猜具体值就是伪造权威）。

## 4. 为什么多上下文与 `member` 只计数、不判差异

- `CONTEXT_AMBIGUOUS`（`loginable >= 2`）：beaconiam 在这种情况下**不签发** org 上下文（让用户选择），
  本地只能存一个 `tenant_id`/`org_id`。此时判「不一致」必然误报 —— 只计数，由人工看。
  这类用户也**不再**追加 `ROLE_UNVERIFIED`（同一用户双重计数会让巡检阈值失真；歧义已由
  `CONTEXT_AMBIGUOUS` 表达）。
- `MEMBER_ONLY`：`member` 是零权限影子关系，不参与上下文推导（与 `resolveUserOrg` 一致）；
  本地值可能是历史回退结果，判差异同样会误报。
- `IAM_MISSING`：本地 `iam_sub` 在 IAM 已不存在（网关/迁移侧问题），**永不**自动修复。

## 5. 输出

文本（逐字锁定，供 #2624/#2625 解析）：

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
DETAIL_COUNT=<n>
DETAIL iam_sub=<..> category=<..> local[..] iam[..]     # 按 iam_sub 升序
RESULT: PASS
```

`--json` 输出 `{"tokens":{…},"rows":[{"iam_sub","email","category","local","iam"}]}`，
`rows` 按 `iam_sub` 升序，`--limit N` 截断（默认 20；`--limit 0` 全量）。

`RECONCILE_SOURCE` 语义：本地缓存侧取真实库时为 `live`（即使 IAM 侧来自 fixture），否则 `fixture`。

计数恒等式（测试断言）：六类可报告差异之和 = 明细中六类行数；
六类 + `ROLE_UNVERIFIED` + `CONTEXT_AMBIGUOUS` + `MEMBER_ONLY` = 明细行数（一用户一行）。

退出码：

| 码 | 含义 |
|---|---|
| 0 | 比对完成（**有差异也算 0**，差异靠 token 表达） |
| 2 | 参数/连接/env 错误（输出 `RESULT: FAIL`，**不得**冒充「0 差异」） |
| 3 | 输入损坏（fixture 字段数不符、`is_active` 非 0/1、`role` 非标识符） |

## 6. 怎么用

```bash
# 真实库（只读）
export BEACONIAM_DB_HOST=… BEACONIAM_DB_NAME=… BEACONIAM_DB_USER=… BEACONIAM_DB_PASSWORD=…
export TUNELOOP_DB_HOST=… TUNELOOP_DB_NAME=… TUNELOOP_DB_USER=… TUNELOOP_DB_PASSWORD=…
bash scripts/reconcile-user-cache.sh
bash scripts/reconcile-user-cache.sh --json --limit 50

# 离线复跑（不需要任何库）
bash scripts/reconcile-user-cache.sh --fixture scripts/fixtures/reconcile-2623

# 回归证据（3 行）
bash scripts/tests/test_reconcile_user_cache_2623.sh r1   # 分类 + JSON 一致性 + 计数恒等式 + 成环
bash scripts/tests/test_reconcile_user_cache_2623.sh r2   # 只读硬闸门 + 坏 fixture / 不可达 / 缺 env
bash scripts/tests/test_reconcile_user_cache_2623.sh r3   # 一次性 PG 15433 真实本地库只读跑 + 双路径一致
```

真实库只读跑在**预生产/生产**上由运维执行（#2623 的 `[Ops-Followup]`），本仓库证据只用
一次性 PostgreSQL（`127.0.0.1:15433`，`scripts/lib-2341-env.sh` 口径）与 fixture。

## 7. 已知缺口与边界

- **角色权威来源没有单一 owner**：`users.role` 到底该由 IAM 关系角色还是 `functional_roles`
  决定、两者优先级与清理规则（#2505 记录过 `functional_roles` 只增不减）尚未定义 → #2626。
  本工具因此在角色维度只做集合判定，不产出修复目标值。
- 本工具**只读**：不修数据、不写库、不改 `backend/services/role_templates.go`；
  修复见 #2624（`ORPHAN_LOCAL` / `IAM_MISSING` 永不自动修复）。
- 不通过 IAM HTTP API 枚举关系（只做只读 SQL）；若某环境 SQL 不可达，另立 Issue。
- 存量回填与 `tenant_id` 体系整体口径（#2255 P3）不在本工具范围。

## 9. 可选修复通道 `--apply`（#2624）

> **红线**：AI 不得对 dev / 预生产 / 生产库执行 `--apply`。真实修复必须由 **Owner 授权**
> 后交运维执行；本仓库证据只在一次性 PostgreSQL（`127.0.0.1:15433`）产生。
> 判定逻辑与 token 契约的唯一 owner 仍是 #2623 —— 本节只描述**写通道**。

### 9.1 命令

```bash
OWNER_APPROVED_USER_CACHE_RECONCILE=1 \
bash scripts/reconcile-user-cache.sh --apply \
  --confirm-rows <本次算出的 REPAIR_ROWS> \
  --sql-out   /path/repair-<batch>.sql \
  --rollback-file /path/rollback-<batch>.sql \
  [--iam-fixture <file>] [--sql-only] [--include-role]
```

| 参数 | 语义 |
|---|---|
| `--apply` | 进入修复通道；**不加时行为与只读版完全一致**（不加即 L1 只读） |
| `--confirm-rows <n>` | 必须与本次算出的 `REPAIR_ROWS` **逐字相等**，否则 `exit 2` + `DB_WRITES=0` |
| `--sql-out` / `--rollback-file` | **必填**：分别写出「修复 SQL」「回滚 SQL」，两者都可独立执行（DBA 可复核后手动跑） |
| `--sql-only` | 只生成两份 SQL，不连库执行（`DB_WRITES=0`，离线可断言） |
| `--include-role` | 预留：只有角色映射表被锁定后才允许修 `ROLE_MISMATCH`；当前恒为 `ROLE_APPLY_SKIPPED=1` |

### 9.2 修复范围（逐类计数，禁止静默）

| 类别 | 处理 | token |
|---|---|---|
| `TENANT_MISMATCH` / `ORG_MISMATCH` / `NIL_UUID` | compare-and-set 修复 | 计入 `REPAIR_ROWS` |
| `ORPHAN_LOCAL` | 跳过（离职/解绑语义，人工处理） | `SKIPPED_ORPHAN` |
| `MISSING_LOCAL` | 跳过（是否建行属产品决策） | `SKIPPED_MISSING_LOCAL` |
| `ROLE_MISMATCH` | 默认跳过（映射多对一，需先锁表） | `SKIPPED_ROLE` + `ROLE_APPLY_SKIPPED=1` |
| `ROLE_UNVERIFIED` | 永不修 | `SKIPPED_ROLE_UNVERIFIED` |
| 未知类别 / 明细缺 `local_id` / 非 uuid 值 | **fail-fast `exit 3`**，不生成 SQL | `UNKNOWN_CATEGORY=1` |

### 9.3 两份 SQL 的形态（可逐行断言）

- **单事务**：`BEGIN; … COMMIT;`，任一语句失败即整体回滚（`psql -v ON_ERROR_STOP=1 -f`）。
- **compare-and-set**：`UPDATE users SET tenant_id=…, org_id=…, updated_at=now() WHERE id=… AND iam_sub=… AND tenant_id=<旧值> AND org_id=<旧值>;`
  —— 目标行在生成与执行之间被改过 → 命中 0 行，不会覆盖并发写入。
- **审计留痕**：每行一条 `audit_logs`（`action=USER_CACHE_RECONCILE`，`details={batch_id,category,old,new}`）；
  回滚文件写 `action=USER_CACHE_RECONCILE_ROLLBACK`，`old`/`new` 互换。
- **事务末后置校验**：每行一个 `DO $$ ... RAISE EXCEPTION 'reconcile: concurrent change detected for <id>' ... $$;`
  —— 任一行未达预期值即中止整个事务（不可能「修一半」）。
- 语句按 `iam_sub` 升序，`BATCH_ID=<UTC yyyyMMddTHHMMSSZ>-<6 hex>` 贯穿修复 SQL、审计 `details`、回滚 SQL 与输出 token。

### 9.4 输出 token（新增，L1 token 不变）

```
REPAIR_ROWS SKIPPED_ORPHAN SKIPPED_MISSING_LOCAL SKIPPED_ROLE SKIPPED_ROLE_UNVERIFIED
ROLE_APPLY_SKIPPED BATCH_ID AUDIT_ROWS DB_WRITES ROLLBACK_ROWS SQL_OUT ROLLBACK_FILE
```

失败语义：未授权 `UNAUTHORIZED_APPLY=1`；行数不符 `APPLY_CONFIRM_MISMATCH=1`；
`REPAIR_ROWS=0` → `NOTHING_TO_REPAIR=1`（无事可做不是成功修复）；以上一律 `DB_WRITES=0` + `exit 2`。

### 9.5 运维执行顺序（真实库）

1. 只读跑一次拿到 `REPAIR_ROWS` 与明细；
2. Owner 授权（`OWNER_APPROVED_USER_CACHE_RECONCILE=1`）；
3. `--sql-only` 生成 SQL → DBA 复核 `--sql-out`；
4. 执行修复 SQL（单事务）；
5. 再跑只读对账 → 三类计数应归零；`ORPHAN_LOCAL` / `MISSING_LOCAL` / 角色类仍按原样跳过；
6. 如需回退：执行 `--rollback-file`（同样单事务 + 反向审计）。
