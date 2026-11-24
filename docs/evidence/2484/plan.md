## Accepted Plan — #2484（漂移清单脚本：非空白分隔符 + 字段数校验 + `--apply` 二次确认）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 解析用 `while IFS=$'\t' read -r user_id mm_role users_role merchant_id has_site_member` | `scripts/report-merchant-direct-role-drift-2462.sh:74` |
| Tab 属 IFS **空白**字符 → 连续 Tab 折叠、**空字段被丢弃**，后续字段整体左移 | POSIX `read` 语义（`王五` 用例：`users_role=''` 时 `merchant_id` 被读成 `users_role`，#2484 正文有实测） |
| 查询已 `COALESCE(u.role,'')`（空串仍在输出中），但被 `read` 折叠 | 同脚本 `:62-72` |
| `--apply` 已有 `OWNER_APPROVED_MERCHANT_DIRECT_ROLE=1` 闸门，但**无「将变更行数」二次确认** | 同脚本 `:34-44`、`:80-92` |
| 既有测试仅覆盖「2 漂移 + 1 正常」（**无空字段**用例）→ 缺陷未被捕获 | `scripts/tests/test_merchant_direct_role_drift_2462.sh` |

**结论**：伪阳性来自 IFS 空白折叠（左移），不是 SQL 错。修复 = **改用非空白分隔符**（`|`）+ **严格字段数校验**（畸形行显式失败，不静默）+ `--apply` 二次确认；并新增空字段红绿用例。

### 变更点

1. `scripts/report-merchant-direct-role-drift-2462.sh`：
   - 查询输出分隔符 `psql -At -F $'\t'` → **`-F '|'`**；解析 `while IFS=$'\t' read` → **`while IFS='|' read -r user_id mm_role users_role merchant_id has_site_member`**（UUID/角色名不含 `|`，安全）；
   - **字段数校验**：逐行 `awk -F'|' 'NF!=5'`（或读入后计数字段）→ 不符则 `BAD_ROW=1` + `ROW_FIELDS=<n>` + `FIXTURE_INVALID=1`，**exit 3**（不写库、不静默丢弃）；
   - `--fixture` 的 fixture 文件同样按 `|` 分隔（头部注释与 `-h` 输出同步更新为 5 列 `|` 格式）；
   - `--apply` 二次确认：新增 `--confirm-rows <n>`；未提供或 `n != align_rows` → `APPLY_CONFIRM_MISMATCH=1` + `DB_WRITES=0` + **exit 2**；提供且匹配才写库（仍要求 `OWNER_APPROVED_MERCHANT_DIRECT_ROLE=1`）。
2. `scripts/tests/test_merchant_direct_role_drift_2462.sh`：fixture 改为 `|` 分隔（否则既有用例会因格式变更失败）。
3. 新增 `scripts/tests/test_merchant_direct_role_drift_2484.sh`：
   - 空 `users_role` + `has_site_member=1` → `ACTION=none`、`ALIGN_ROWS=0`、`merchant_id` 正确（不左移）；
   - 空 `merchant_id` → 字段仍对齐；
   - 字段数 4 → `FIXTURE_INVALID=1` + exit 3 + `DB_WRITES=0`；
   - `--apply --confirm-rows 0` 但存在 1 行漂移 → exit 2、`DB_WRITES=0`；
   - **红绿**：对旧脚本（`git show HEAD~:scripts/report-merchant-direct-role-drift-2462.sh`，Tab 版）跑同一空字段 fixture → `ALIGN_ROWS=1`（红）。
4. `scripts/verify-2484.sh`：7 行证据生成器（provenance 从 `git rev-parse` 派生）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 空字段不左移 | `scripts/report-merchant-direct-role-drift-2462.sh` | `bash scripts/report-merchant-direct-role-drift-2462.sh --fixture <空 users_role tsv>` | `users_role=''`、`has_site_member=1` | 后端脚本 | `ACTION=none`、`ALIGN_ROWS=0`、`merchant_id=<正确>`、`READONLY=1` | R1 |
| R2 红绿实证 | `scripts/tests/test_merchant_direct_role_drift_2484.sh` | 同 fixture 分别跑「旧 Tab 版」与「修复版」 | 同上 | 后端脚本 | 旧版 `ALIGN_ROWS=1`（红）；新版 `ALIGN_ROWS=0`（绿）→ `RED_GREEN=1` | R2 |
| R3 畸形行显式失败 | `scripts/report-merchant-direct-role-drift-2462.sh` | `--fixture <4 列 tsv>` | 字段数 ≠ 5 | 后端脚本 | `FIXTURE_INVALID=1`、`BAD_ROW=1`、exit **3**、`DB_WRITES=0` | R3 |
| R4 `--apply` 二次确认 | `scripts/report-merchant-direct-role-drift-2462.sh` | `--apply --fixture <1 漂移行> [--confirm-rows n]` | (a) 无 `--confirm-rows`；(b) `--confirm-rows 0`；(c) `--confirm-rows 1` | 后端脚本 | (a)(b) `APPLY_CONFIRM_MISMATCH=1` + exit 2 + `DB_WRITES=0`；(c) 允许执行（仍受 Owner 授权闸门） | R4 |
| R5 只读清单计数正确 | `scripts/report-merchant-direct-role-drift-2462.sh` | `--fixture <预生产导数快照 5 漂移 + 4 双归属>` | 混合空 `users_role` / 双归属 | 后端脚本 | `ALIGN_ROWS=5`、`NONE_ROWS=4`、`READONLY=1`、`DB_WRITES=0`（与 #2484 正文的 5 条一致） | R5 |
| R6 既有 + 新测试回归 | `scripts/tests/test_merchant_direct_role_drift_2462.sh`、`scripts/tests/test_merchant_direct_role_drift_2484.sh` | `bash` 两个测试 | 修后脚本 | 后端脚本 | 两套均 `RESULT: PASS`、`SKIPPED=0`、无 `FAIL` | R6 |
| R7 gate 四源 + 批次 gate | `docs/evidence/2484/pre-ready-2484.json`、`docs/evidence/2484/plan.md` | `--full-gate-cmd "bash scripts/tests/test_merchant_direct_role_drift_2462.sh && bash scripts/tests/test_merchant_direct_role_drift_2484.sh"` + `plan-source.sh --issue 2484` | 修后树（干净） | 后端脚本 | `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | R7 |

## Input Truth & Tool Preflight

- **Source of truth**：预生产只读 SQL 的 5 条真实 `align` 清单（#2484 正文列出 `南哥(df016bf8)`、`张鸿波(98cad058)`、`测试员工2372(1dac4ae3)`、`直属测试2473(57eec9af)`、`赵九(45ea1c82)`）；双归属（有 `site_members` 行）必须为 `ACTION=none`。
- **差异核对**：`grep -n "IFS=\\$'\\\\t'" scripts/report-merchant-direct-role-drift-2462.sh` 仅 1 处解析（查询侧另有 `-F $'\t'`），两处必须同改；`scripts/tests/test_merchant_direct_role_drift_2462.sh` 的 fixture 也需同改（否则自测失效）。
- **损坏输入拒绝**：字段数 ≠ 5 → exit 3（`FIXTURE_INVALID=1`）；`user_id` 为空行 → 跳过（既有行为，不算畸形）；`has_site_member` 仅接受 `0|1`，其它值 → 视为畸形并 exit 3（不猜）。
- **工具盲区**：脚本用 `--fixture` 绕开真库 → 覆盖解析层全部形态；真实库只读计数由 `[Ops-Followup of #2473]` 的预生产回执覆盖（本单不触库）。
- **影响面预检**：`grep -rn "report-merchant-direct-role-drift-2462" --include=*.md --include=*.sh .` → 引用点仅 #2462/#2473 的 ops 文档与本脚本自测；参数新增（`--confirm-rows`）向后兼容（不传则 `--apply` 一律拒绝，属**收紧**）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 分隔符选 `\|`（可读、UUID/角色名不含该字符），不用 `\x1f` | Architect | 本计划锁定 | 若后续字段可能出现 `\|` → 改 `\x1f`（需同批改 psql `-F`） |
| D2 | 畸形行 **fail-fast**（exit 3），不跳过、不猜字段 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | `--apply` 需 `--confirm-rows` 且与 align 行数**严格相等**（在 Owner 授权闸门之上再加一道） | Architect | 本计划锁定 | 阻塞实现 |
| D4 | 不改 SQL 语义（`merchant_staff` 目标角色、`mm.role='site_member'` 条件、`has_site_member` 判定）；不改任何存量数据 | Architect | 本计划锁定 | 若需改口径 → 回 Architect（涉及 #2462 D 系列） |
| D5 | 本单不触库：真实计数由 #2473 的预生产 ops 回执覆盖 | Architect | 本计划锁定 | 阻塞实现 |

## Out of Scope

- 存量数据对齐执行（`--apply` 的真实运行）→ Owner 授权后由 ops 在预生产执行（`[Ops-Followup of #2473]`）。
- 漂移口径调整（谁算「双归属脏数据」）→ #2483 / #2462 的语义范围。
- 把脚本迁到 Go/迁移到后台页面。
- `sites.name`/`merchants.name` 漂移报告（#2457/#2459 各自范围）。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `u3\|site_member\|\|m3\|1`（`users_role` 空，`has_site_member=1`） | `bash scripts/report-merchant-direct-role-drift-2462.sh --fixture` | `docs/evidence/2484/R1-empty-field.txt` | `ACTION=none`、`ALIGN_ROWS=0`、`merchant_id=m3`、`READONLY=1` | `ACTION=align` → FAIL |
| R2 | 同 R1 fixture × 旧/新脚本 | 两个脚本版本 | `docs/evidence/2484/R2-red-green.txt` | `OLD_ALIGN_ROWS=1`、`NEW_ALIGN_ROWS=0`、`RED_GREEN=1` | 旧版也是 0（用例抓不到缺陷）→ FAIL |
| R3 | `u1\|site_member\|site_member\|m1`（4 列） | 同上 | `docs/evidence/2484/R3-invalid-row.txt` | `FIXTURE_INVALID=1`、exit `3`、`DB_WRITES=0` | exit 0 → FAIL |
| R4 | 1 漂移行；三次调用（无/0/1 confirm） | `--apply` | `docs/evidence/2484/R4-apply-confirm.txt` | 无/0 → `APPLY_CONFIRM_MISMATCH=1` + exit 2 + `DB_WRITES=0`；1 → 通过闸门（`APPLY_READY=1`） | 未确认即写库 → FAIL |
| R5 | 预生产导数快照（5 漂移 + 4 双归属，含空 `users_role`） | `--fixture` | `docs/evidence/2484/R5-real-snapshot.txt` | `ALIGN_ROWS=5`、`NONE_ROWS=4`、`READONLY=1` | `ALIGN_ROWS>5`（伪阳性）→ FAIL |
| R6 | 两套自测 | `bash scripts/tests/...` | `docs/evidence/2484/R6-tests.txt` | 两套 `RESULT: PASS`、`SKIPPED=0` | 任一 FAIL → FAIL |
| R7 | 修后干净树 + gate | 读校验 | `docs/evidence/2484/R7-gate.txt` | `GATE_RESULT=PASS`、`FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | `P5=NA` → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2484.sh r1` | `docs/evidence/2484/R1-empty-field.txt` | exit 0；grep `ACTION=none`、`ALIGN_ROWS=0`、`merchant_id=m3` | PASS | `ACTION=align` → FAIL | dev |
| R2 | `bash scripts/verify-2484.sh r2` | `docs/evidence/2484/R2-red-green.txt` | exit 0；grep `OLD_ALIGN_ROWS=1`、`NEW_ALIGN_ROWS=0` | PASS | 旧版 0 → FAIL | dev |
| R3 | `bash scripts/verify-2484.sh r3` | `docs/evidence/2484/R3-invalid-row.txt` | exit 0；grep `FIXTURE_INVALID=1`、`EXIT=3`、`DB_WRITES=0` | PASS | exit 0 → FAIL | dev |
| R4 | `bash scripts/verify-2484.sh r4` | `docs/evidence/2484/R4-apply-confirm.txt` | exit 0；grep `APPLY_CONFIRM_MISMATCH=1`、`APPLY_READY=1`、`DB_WRITES=0` | PASS | 未确认写库 → FAIL | dev |
| R5 | `bash scripts/verify-2484.sh r5` | `docs/evidence/2484/R5-real-snapshot.txt` | exit 0；grep `ALIGN_ROWS=5`、`NONE_ROWS=4` | PASS | `ALIGN_ROWS>5` → FAIL | dev |
| R6 | `bash scripts/verify-2484.sh r6` | `docs/evidence/2484/R6-tests.txt` | exit 0；grep `TESTS_FAIL=0`、`SKIPPED=0` | PASS | 任一 FAIL → FAIL | dev |
| R7 | `bash scripts/verify-2484.sh r7` | `docs/evidence/2484/R7-gate.txt` | exit 0；grep `GATE_RESULT=PASS`、`PLAN_SOURCE=PASS` | PASS | `P5=NA` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2484/R1-empty-field.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2484/R2-red-green.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2484/R3-invalid-row.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2484/R4-apply-confirm.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2484/R5-real-snapshot.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2484/R6-tests.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2484/R7-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 | 触发条件 | 根因 | 修复点 | 覆盖行 | 负例断言 |
|---|---|---|---|---|---|---|
| B1 | 空字段导致伪阳性 align | `users_role=''`（存量常见） | Tab 属 IFS 空白 → 连续 Tab 折叠、空字段丢弃 | 变更点 1 分隔符 | R1 | `ACTION=align` → R1 失败 |
| B2 | 伪阳性被 `--apply` 写库 → 制造域错乱 | 有网点行用户 | 同上 + 无二次确认 | 变更点 1 分隔符 + 变更点 1 确认 | R1/R4 | 写库 → R4 失败 |
| B3 | 畸形数据静默跳过/错位 | 字段数不符 | 无字段数校验 | 变更点 1 校验 | R3 | exit 0 → R3 失败 |
| B4 | 既有自测抓不到缺陷 | 无空字段 fixture | 用例覆盖不足 | 变更点 3 新用例 | R2/R6 | 旧版也 0 → R2 失败 |
| B5 | 计数与真实 SQL 不符 | 预生产导数 | 同 B1 | 变更点 1 | R5 | `ALIGN_ROWS>5` → R5 失败 |
| B6 | 测试随格式变更失效 | fixture 仍用 Tab | 未同步自测 | 变更点 2 | R6 | 2462 自测 FAIL → R6 失败 |
| B7 | 授权后批量误改不可逆 | `--apply` | 无行数确认 | 变更点 1 `--confirm-rows` | R4 | 未确认执行 → R4 失败 |
| B8 | 目标角色/条件被顺手改动 | 改脚本时 | 无口径锁定 | D4 | R5 | 目标角色非 `merchant_staff` → R5 失败 |

### Workload

- Size: S
- Basis: 1 个脚本修复（解析 + 校验 + 确认参数）+ 1 个既有自测同步 + 1 个新自测 + 1 个 verify 脚本 + 7 条证据行；无后端编译、无 migration、不触库
- Confidence: high

### Files（变更文件清单）

- `scripts/report-merchant-direct-role-drift-2462.sh`
- `scripts/tests/test_merchant_direct_role_drift_2462.sh`
- `scripts/tests/test_merchant_direct_role_drift_2484.sh`
- `scripts/verify-2484.sh`
- `docs/evidence/2484/plan.md`
- `docs/evidence/2484/pre-ready-2484.json`
- `docs/evidence/2484/R1-empty-field.txt`
- `docs/evidence/2484/R2-red-green.txt`
- `docs/evidence/2484/R3-invalid-row.txt`
- `docs/evidence/2484/R4-apply-confirm.txt`
- `docs/evidence/2484/R5-real-snapshot.txt`
- `docs/evidence/2484/R6-tests.txt`
- `docs/evidence/2484/R7-gate.txt`
- `docs/evidence/2484/evidence.json`

## Dependencies

- 无

### 依赖与关联说明（非硬依赖）

- **#2462（done）**：脚本来源与 D4 口径（`merchant_staff` 目标角色）。
- **#2473（waitForConfirm）**：其第 4 项「存量漂移清单回执」被本单阻塞；本单修好后由 ops 在预生产重跑只读清单（`[Ops-Followup of #2473]`）。
- **#2483（todo）**：双归属收敛同样消费该清单，本单保证清单不误报。
- **#2482（todo）**：`site_members.roles` 压缩问题与本单无关（不同表/不同脚本）。

*Model: DeepSeek V4.1 Flash 1017*

