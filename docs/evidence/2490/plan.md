## Accepted Plan — #2490 v2（`scripts/migrate.sh`：非空白分隔符 + 字段校验 + 写库二次确认，消除空 email 左移）

> **v2 说明（回应 Investigation Packet：位掩码算术口径错误 + 前置假设不成立）**：
> ① v1 的 G0/矩阵把 `196608 | mask` 误写为 `458752`。实测 `NEW_BITS_MASK=(1<<18)|(1<<19)=786432`（`scripts/migrate.sh:46`；位定义见 `backend/services/registry` 18/19），故 `196608|786432=**983040**`；
> ② v1 的 R4 幂等 fixture `524288` 实际**不幂等**（`524288 & 786432 = 524288`）→ 改为 `983040`；
> ③ v1 的 R6 旧版期望 `458752` 应为 **786432**（空 `cus_perm`→0，`0|mask`），新版 `196608|786432=983040`，并删掉原文的「？」；
> ④ Input Truth 的「`grep` 修复后 0 处」基于「#2484 已合入 develop」的错误前提：`#2484` 的 fix `827f9d1dc` **只在 `origin/issue-2484`**，本单 base 为 `origin/develop` → 修完 `migrate.sh` 的 2 处后，`scripts/report-merchant-direct-role-drift-2462.sh:75` 仍有 1 处（由 #2484 合入后清零）。
> **代码与 Decision Lock 不变**（D5 位定义 18/19 不动）。

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| Step 3 用 Python 输出 `uid\temail\tcus_perm`；`email = u.get('email','')`，**手机号注册的管理员 email 为空串**（合法形态） | `scripts/migrate.sh:111-122` |
| Step 4/展示循环用 **Tab-IFS** 解析：`while IFS=$'\t' read -r uid email cus_perm`（Tab 属 IFS 空白 → 空字段被折叠、后续列左移） | `scripts/migrate.sh:128`、`:139` |
| 左移后 `cus_perm` 读到空串 → `$((cus_perm))` 求值 **0** → `NEW_PERM = 0 \| mask` → `PUT customer-permissions` **覆写为仅 mask 的位**（既有权限位丢失，且不回滚） | `scripts/migrate.sh:140-141`、`:162-171` |
| 仅 `--yes` 一个开关即写库，无「将变更条数」二次确认；无离线 fixture 入口（无法自测） | `scripts/migrate.sh:20-23` |
| 同类脚本已在 #2484 修复为 `\|` 分隔 + 字段校验 + `--confirm-rows`（本单沿用同一口径） | `scripts/report-merchant-direct-role-drift-2462.sh` |

**结论**：根因与 #2484 完全同源（用含空白的 IFS 解析可能含空列的结构化输出）。修复 = 非空白分隔符 + 严格校验 + 二次确认 + 离线 fixture 化（可自测）。

### 变更点

1. `scripts/migrate.sh` 分隔符与校验：
   - Python 输出改为 `uid|email|cus_perm`；两处解析改 `while IFS='|' read -r uid email cus_perm`；
   - 新增**整体校验**（解析前先扫一遍）：
     - 字段数 ≠ 3 → `BAD_ROW=1` + `ROW_FIELDS=<n>` + `MIGRATE_INVALID=1` + **exit 3**（不写库、不猜字段）；
     - `uid` 空 → 同上；
     - `cus_perm` 非空且**必须为非负整数**（`^[0-9]+$`）→ 否则 `BAD_ROW=1` + `BAD_PERM=<值>` + exit 3（**禁止默认 0**）；
     - `email` 允许空（合法），必须保持列位。
2. 离线 fixture 入口（可测、不触网）：新增 `--fixture <file>` —— 跳过 Step 1–3（不取 token、不调 IAM），把文件内容当作 Step 3 的输出继续走「校验 → 计划打印 → 汇总」，**强制 dry-run**（`--fixture` 与 `--yes` 互斥 → 同时给出即 exit 2）。
3. 写库二次确认：`--yes` 时新增 `--confirm-rows <n>`，必须与本次**计划变更条数**（`PLAN_UPDATES`）严格相等，否则 `APPLY_CONFIRM_MISMATCH=1` + `DB_WRITES=0` + **exit 2**；匹配时打印 `APPLY_READY=1` 再逐条 PUT。
4. 输出键（机器可判定）：`PLAN_UPDATES`、`SKIPPED`、`BAD_ROW`、`ROW_FIELDS`、`BAD_PERM`、`MIGRATE_INVALID`、`APPLY_READY`、`APPLY_CONFIRM_MISMATCH`、`DB_WRITES`、`RESULT: PASS|FAIL`。
5. 新增 `scripts/tests/test_migrate_2490.sh`（fixture 驱动，离线）：空 email / 空 cus_perm / 2 列 / 已置位幂等 / `--yes` 三种确认态 / **红绿**（父提交版本对空 email 计算丢位）。
6. 新增 `scripts/verify-2490.sh`：R1–R8 证据生成器（provenance 由 `git rev-parse` 派生）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 空 email 不左移 | `scripts/migrate.sh` | `--fixture <空 email 行>` | `email=''`、`cus_perm=196608`（bits 16/17） | Shell | `ACTION=UPDATE uid=u1 email= old_perm=196608 new_perm=983040`（`196608\|786432`）、`PLAN_UPDATES=1`、`DB_WRITES=0`（dry-run） | R1 |
| R2 非法 cus_perm 拒绝 | `scripts/migrate.sh` | `--fixture <空/非数字 cus_perm>` | `cus_perm=''` 或 `abc` | Shell | `BAD_ROW=1`、`MIGRATE_INVALID=1`、**exit 3**、`DB_WRITES=0` | R2 |
| R3 字段数不足拒绝 | `scripts/migrate.sh` | `--fixture <2 列行>` | 字段数 2 | Shell | `BAD_ROW=1`、`ROW_FIELDS=2`、**exit 3** | R3 |
| R4 幂等（位已置） | `scripts/migrate.sh` | `--fixture u5\|a@b\|983040`（已含 16/17/18/19） | `cus_perm & mask == mask`（`983040 & 786432 = 786432`） | Shell | `ACTION=SKIP`、`SKIPPED=1`、`PLAN_UPDATES=0`、`DB_WRITES=0` | R4 |
| R5 写库二次确认 | `scripts/migrate.sh` | `--yes`（无/错/对的 `--confirm-rows`）+ `--fixture` 互斥 | 3 种组合 | Shell | 无/错 → `APPLY_CONFIRM_MISMATCH=1` + exit 2 + `DB_WRITES=0`；`--fixture` + `--yes` → exit 2；`--confirm-rows` 匹配 → `APPLY_READY=1` | R5 |
| R6 红绿（旧版丢位） | `scripts/tests/test_migrate_2490.sh` | 父提交版本 vs 修复版，同空 email fixture（`u1\|\|196608`） | 1 行 | Shell | 旧版 `OLD_NEW_PERM=786432`（`0\|mask`，bits 16/17 被清）→ `OLD_LOST_BITS=1`；新版 `NEW_PERM=983040` → `FIXED=1`；`RED_GREEN=1` | R6 |
| R7 静态检查 + 同类扫描 | `scripts/migrate.sh` | `bash -n`（+ 有 shellcheck 则跑）+ `grep -rn "IFS=\$'\\t'" scripts/` | 修后树 | Shell | `SYNTAX_OK=1`、`SHELLCHECK_EXIT=0`（不可用记 N/A）；信息行 `SAME_PATH_SCAN=<n>`（本单改完应=1：`report-merchant-direct-role-drift-2462.sh`，由 #2484 合入后归零；**不参与 PASS/FAIL**） | R7 |
| R8 gate 四源 + 批次 gate | `docs/evidence/2490/pre-ready-2490.json`、`docs/evidence/2490/plan.md` | `--full-gate-cmd "bash scripts/tests/test_migrate_2490.sh"` + `plan-source.sh --issue 2490` | 修后树（干净） | Shell | `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | R8 |

## Input Truth & Tool Preflight

- **Source of truth**：IAM `GET /organizations/$NS_ORG_ID/users` 的 `users[].{id,email,cus_perm}`；`cus_perm` 为位掩码（bit18=`category:manage`、bit19=`attribute:manage`）。空 `email` 是合法数据（手机号注册），**不是**脏数据。
- **差异核对**：`grep -rn "IFS=\$'\\\\t'" scripts/` 现状 **3 处**；本单改 `migrate.sh` 的 2 处后剩 **1 处**（`scripts/report-merchant-direct-role-drift-2462.sh:75`，其修复 `827f9d1dc` 属 #2484、尚未合入 develop）。该残留**不阻塞本单**，以 R7 的信息行 `SAME_PATH_SCAN=1` 记录，待 #2484 合入后归零。
- **损坏输入拒绝**：字段数 ≠3、`uid` 空、`cus_perm` 非 `^[0-9]+$` → `exit 3` 且不写库；`email` 空允许。
- **工具盲区**：`--fixture` 只能验证解析/计划/确认链路，真实 `PUT customer-permissions` 由 ops 在预生产执行（`--yes --confirm-rows N`），本单不在审计机触网。
- **影响面预检**：脚本仅被人手动执行（`git grep -n "migrate.sh"` 确认无 CI/部署引用）；`--yes` 语义保持（新增 `--confirm-rows` 为其前置，属收紧）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 分隔符 `\|`（email 不含 `\|`，uuid/数字天然安全） | Architect | 本计划锁定 | 若将来 email 可能含 `\|` → 改 `\x1f`（同批改 Python 输出） |
| D2 | 畸形行 **fail-fast（exit 3）**，不跳过、不默认 0 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | `--fixture` **强制 dry-run**，与 `--yes` 互斥（防止误把测试数据写库） | Architect | 本计划锁定 | 阻塞实现 |
| D4 | `--yes` 必须同时给 `--confirm-rows`，且等于 `PLAN_UPDATES` | Architect | 本计划锁定 | 阻塞实现 |
| D5 | 位定义（18/19）与 `NS_ORG_ID` 选择逻辑**不改** | Architect | 本计划锁定 | 若需改位定义 → 另立 Issue |

## Out of Scope

- 真实执行迁移（`--yes`）→ ops，且需 Owner 授权；本单只保证解析与安全闸门正确。
- IAM 侧端点/权限模型（`PUT .../customer-permissions` 的鉴权）。
- 把该脚本迁到 Go/后台页面。
- 其它脚本的 IFS 审计（#2484 已覆盖漂移脚本；`grep` 复核仅作证据）。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `u1\|\|196608`（空 email，既有 bits 16/17） | `bash scripts/migrate.sh --fixture` | `docs/evidence/2490/R1-empty-email.txt` | `ACTION=UPDATE`、`email=`（空）、`old_perm=196608`、`new_perm=983040`、`PLAN_UPDATES=1`、`DB_WRITES=0` | `email=196608`（左移）→ FAIL |
| R2 | `u2\|a@b\|\n` 与 `u3\|a@b\|abc` | 同上 | `docs/evidence/2490/R2-bad-perm.txt` | `BAD_ROW=1`、`MIGRATE_INVALID=1`、exit 3、`DB_WRITES=0` | exit 0 → FAIL |
| R3 | `u4\|a@b`（2 列） | 同上 | `docs/evidence/2490/R3-short-row.txt` | `ROW_FIELDS=2`、exit 3 | exit 0 → FAIL |
| R4 | `u5\|a@b\|983040`（已含 16/17/18/19） | 同上 | `docs/evidence/2490/R4-idempotent.txt` | `ACTION=SKIP`、`SKIPPED=1`、`PLAN_UPDATES=0` | 重复更新 → FAIL |
| R5 | 1 行需更新 + 三种确认组合 | `--yes [--confirm-rows n]` / `--fixture` 互斥 | `docs/evidence/2490/R5-apply-confirm.txt` | 无/错 → `APPLY_CONFIRM_MISMATCH=1` + exit 2 + `DB_WRITES=0`；互斥 → exit 2；匹配 → `APPLY_READY=1` | 未确认即写库 → FAIL |
| R6 | 同 R1 fixture（旧版 vs 新版） | 两个版本 | `docs/evidence/2490/R6-red-green.txt` | 旧版 `OLD_NEW_PERM=786432` + `OLD_LOST_BITS=1`（bits 16/17 被清）、新版 `NEW_PERM=983040` + `FIXED=1`、`RED_GREEN=1` | 旧版行为相同 → FAIL |
| R7 | 修后脚本 | `bash -n` | `docs/evidence/2490/R7-syntax.txt` | `SYNTAX_OK=1` | 语法错误 → FAIL |
| R8 | 修后干净树 + gate | 读校验 | `docs/evidence/2490/R8-gate.txt` | `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | `P5=NA` → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2490.sh r1` | `docs/evidence/2490/R1-empty-email.txt` | exit 0；grep `ACTION=UPDATE`、`old_perm=196608`、`PLAN_UPDATES=1` | PASS | `email=196608` → FAIL | dev |
| R2 | `bash scripts/verify-2490.sh r2` | `docs/evidence/2490/R2-bad-perm.txt` | exit 0；grep `MIGRATE_INVALID=1`、`EXIT=3` | PASS | exit 0 → FAIL | dev |
| R3 | `bash scripts/verify-2490.sh r3` | `docs/evidence/2490/R3-short-row.txt` | exit 0；grep `ROW_FIELDS=2`、`EXIT=3` | PASS | exit 0 → FAIL | dev |
| R4 | `bash scripts/verify-2490.sh r4` | `docs/evidence/2490/R4-idempotent.txt` | exit 0；grep `ACTION=SKIP`、`PLAN_UPDATES=0` | PASS | 重复更新 → FAIL | dev |
| R5 | `bash scripts/verify-2490.sh r5` | `docs/evidence/2490/R5-apply-confirm.txt` | exit 0；grep `APPLY_CONFIRM_MISMATCH=1`、`APPLY_READY=1`、`DB_WRITES=0` | PASS | 未确认写库 → FAIL | dev |
| R6 | `bash scripts/verify-2490.sh r6` | `docs/evidence/2490/R6-red-green.txt` | exit 0；grep `OLD_LOST_BITS=1`、`FIXED=1`、`RED_GREEN=1` | PASS | 旧版相同 → FAIL | dev |
| R7 | `bash scripts/verify-2490.sh r7` | `docs/evidence/2490/R7-syntax.txt` | exit 0；grep `SYNTAX_OK=1` | PASS | 语法错误 → FAIL | dev |
| R8 | `bash scripts/verify-2490.sh r8` | `docs/evidence/2490/R8-gate.txt` | exit 0；grep `GATE_RESULT=PASS`、`PLAN_SOURCE=PASS` | PASS | `P5=NA` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2490/R1-empty-email.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2490/R2-bad-perm.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2490/R3-short-row.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2490/R4-idempotent.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2490/R5-apply-confirm.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2490/R6-red-green.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2490/R7-syntax.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2490/R8-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 | 触发条件 | 根因 | 修复点 | 覆盖行 | 负例断言 |
|---|---|---|---|---|---|---|
| B1 | 空 email 管理员被算成 `cus_perm=0` → 覆写丢位 | 手机号注册的 namespace_admin | Tab-IFS 折叠空字段 | 变更点 1 | R1/R6 | `email=196608` → R1 失败 |
| B2 | 权限位被静默清掉（无 dry-run 提示） | 同上 | 无校验 + 无二次确认 | 变更点 1/3 | R2/R5 | 未确认写库 → R5 失败 |
| B3 | 非法 `cus_perm` 被当 0 | 上游字段异常 | `$(( ))` 对空/非数字宽容 | 变更点 1 | R2 | exit 0 → R2 失败 |
| B4 | 字段错位静默继续 | 列缺失 | 无字段数校验 | 变更点 1 | R3 | exit 0 → R3 失败 |
| B5 | 重复执行重复写入 | 已置位账号 | 无幂等短路 | 变更点 1 | R4 | 重复更新 → R4 失败 |
| B6 | 测试数据误写生产 | `--fixture` 与 `--yes` 同时使用 | 无互斥 | 变更点 2 | R5 | 写库 → R5 失败 |
| B7 | 同类脚本再次复现 | 其它运维脚本 | 未统一口径 | 变更点 1（对齐 #2484） | R7 | 仅作信息行 `SAME_PATH_SCAN`（不判 FAIL，避免与他单未合入耦合） |
| B8 | 改动误伤 bits 定义/org 选择 | 顺手重构 | 无口径锁定 | D5 | R1/R4 | 位定义变化 → R1 失败 |

### Workload

- Size: S
- Basis: 1 个脚本改造（分隔符/校验/fixture/二次确认）+ 1 个新测试 + 1 个 verify 脚本 + 8 条证据行；无后端编译、无 migration、不触网
- Confidence: high

### Files（变更文件清单）

- `scripts/migrate.sh`
- `scripts/tests/test_migrate_2490.sh`
- `scripts/verify-2490.sh`
- `docs/evidence/2490/plan.md`
- `docs/evidence/2490/pre-ready-2490.json`
- `docs/evidence/2490/R1-empty-email.txt`
- `docs/evidence/2490/R2-bad-perm.txt`
- `docs/evidence/2490/R3-short-row.txt`
- `docs/evidence/2490/R4-idempotent.txt`
- `docs/evidence/2490/R5-apply-confirm.txt`
- `docs/evidence/2490/R6-red-green.txt`
- `docs/evidence/2490/R7-syntax.txt`
- `docs/evidence/2490/R8-gate.txt`
- `docs/evidence/2490/evidence.json`

## Dependencies

- 无

### 依赖与关联说明（非硬依赖）

- **#2484（done）**：同类脚本已改「非空白分隔符 + 字段校验 + `--confirm-rows`」，本单沿用同一口径（D1/D2/D4 与 #2484 一致）。
- **ops（人工）**：真实迁移执行（`--yes --confirm-rows N`）由 Owner 授权后在预生产/生产执行，本单不触网；回执由 `[Ops-Followup of #2490]`（本单 PASS 时创建）承载。
- **#2493（holdon）**：与 `cus_perm`/角色模板无关，不同层。

*Model: DeepSeek V4.1 Flash 1017*

