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

## 8. 例行任务与告警（#2625）

判定与 token 契约的唯一 owner 仍是 #2623（本仓库工具）；本节只描述**调度 + 告警**。

### 8.1 安装 / 卸载

```bash
# 预览（默认，不写任何文件）
bash scripts/install-user-cache-reconcile-timer.sh

# 安装（systemd user timer 优先，失败回退 cron；只安装调度，不执行写库动作）
bash scripts/install-user-cache-reconcile-timer.sh --apply

# 指定时间/后端
bash scripts/install-user-cache-reconcile-timer.sh --apply --backend systemd --at 03:30

# 卸载（精确移除 unit 与带 marker 的 cron 行）
bash scripts/install-user-cache-reconcile-timer.sh --uninstall
```

- 默认 `03:30`（宿主本地时区；`OnCalendar=*-*-* 03:30:00` + `Persistent=true`）。
  宿主为 UTC 时，请在 env 文件里显式声明 `TZ`（D10）。
- unit 名称 `opencode-user-cache-reconcile.{service,timer}`，位于 `~/.config/systemd/user/`；
  cron 兜底使用 marker `# opencode-user-cache-reconcile`（一行，幂等，不重复追加）。
- `ExecStart` 指向 `scripts/reconcile-user-cache-cron.sh`，**不含** `--apply`（自动化只读）。

### 8.2 env 文件字段（凭据不进 unit）

`~/.config/tuneloop/reconcile.env`（由运维填写，权限建议 `600`）：

| 字段 | 说明 |
|---|---|
| `BEACONIAM_DB_HOST` / `BEACONIAM_DB_PORT` / `BEACONIAM_DB_USER` / `BEACONIAM_DB_PASSWORD` / `BEACONIAM_DB_NAME` | IAM 权威库（只读连接） |
| `TUNELOOP_DB_HOST` / `TUNELOOP_DB_PORT` / `TUNELOOP_DB_USER` / `TUNELOOP_DB_PASSWORD` / `TUNELOOP_DB_NAME` | 本地缓存库 |
| `TZ` | 宿主时区非本地时显式声明 |

unit 内只出现 `EnvironmentFile=%h/.config/tuneloop/reconcile.env`，**不得**出现
`PASSWORD`/`SECRET`/`TOKEN` 字面量。

### 8.3 运行器分支（`scripts/reconcile-user-cache-cron.sh`）

| 分支 | 行为 |
|---|---|
| 无差异（6 类计数全 0） | 只归档，**不建 Issue、不通知**（`GH_CALLS=0`） |
| 有差异 | 归档 + 建 `[Ops] 用户缓存对账差异 <date>`（标签**仅** `waitForConfirm`，正文含汇总 token + 前 20 条明细 + 人工处置指引）；同日期已存在则复用，不重复建单 |
| 连续 2 天不降 | 同类差异今日 >0 且不低于昨日 → 在既有单追加**恰好 1 条**升级评论，**不新建第二单** |
| 对账失败（`exit != 0` / 输出非法 / 缺必需 token） | 建 `[Ops] 用户缓存对账失败 <date>`（`waitForConfirm`）并 `exit 2`；**禁止**静默或冒充「0 差异」 |
| 缺 `gh` / 缺 env / 归档目录不可写 | `exit 2` + 打印 `MISSING_ENV=<name>` |

归档位置：`${XDG_STATE_HOME:-$HOME/.local/state}/tuneloop/reconcile/<date>.json` 与 `.txt`
（**不写仓库**，定时任务不产生 Git 变更）；`LAST_SUMMARY` token 指向当日 `.txt`。

`--dry-run` 只打印将执行的动作（`DRY_RUN=1`、`GH_CALLS=0`、`FILES_WRITTEN=0`）。

### 8.4 标签与升级规则

- 告警单**只有** `waitForConfirm`，**不带**任何 `status:*` → 永不被 `find_issue.sh` 认领（不进流水线）。
- 升级 = 在既有差异单追加 1 条评论（含 `ESCALATE_REASON=<类>:<昨日>-><今日>`），不新建 Issue。
- 人工处置（修复）走 #2624，需 Owner 授权；自动化永远只读，`--apply` 永不进入定时任务。
