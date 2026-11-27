## Accepted Plan — #2625 [Sub-task of #2622 L3] 用户缓存对账例行化与告警：timer 安装 + 无差异静默 + 连续 2 天升级 + 对账失败必须告警

### 背景与现状核对（代码为据）

| # | 事实 | 位置 / 证据 |
|---|---|---|
| 1 | 本单是 **L3**：把 #2623 的只读对账变成每晚 03:30 的例行任务并告警。判定逻辑与 token 契约**唯一 owner = #2623**，本单只消费 | #2622 计划 `P2`；#2623 正文「输出契约」 |
| 2 | timer 安装脚本已有成熟范式：`--print`（默认）/ `--apply` / `--uninstall`、`--backend auto\|systemd\|cron`、systemd user timer 优先 + cron 兜底（带 marker 便于移除）、日志落 `${XDG_STATE_HOME:-$HOME/.local/state}` | `~/.config/opencode/scripts/install-worktree-cleanup-timer.sh:1-170` |
| 3 | 告警 Issue 必须**不进流水线**：父 `P8` 已锁定用 `waitForConfirm` 标签且**不带** `status:*`（`find_issue.sh` 只认 `status:*`，无状态标签的 Issue 永不被认领） | `~/.config/opencode/scripts/find_issue.sh`；`AGENTS.md`「waitForConfirm 跟踪单」口径 |
| 4 | 自动化**绝不能**调用 `--apply`（写库需 Owner 授权，属运维动作） | #2622 计划 `P5`；#2624 计划 `D7` |
| 5 | 「查不到 ≠ 无差异」：数据库不可达/查询失败必须显式告警，禁止输出 `RESULT: PASS` | #2622 计划 `P7`；#2623 正文「负例」 |
| 6 | 本机 `systemctl --user` 可能不可用（容器/CI）→ 必须有静态 `--print` 路径供断言，不依赖运行中的 systemd | 同第 2 条脚本 `--print` 行为；`scripts/tests/` 已有 fake-env 测试范式 |
| 7 | 定时任务需要**两库凭据 + `gh` 权限**，凭据不得写进 unit 文件：用 `EnvironmentFile=` 引用运维填写的 env 文件 | `install-worktree-cleanup-timer.sh` 用 `Environment=` 注入 PATH/GH_BIN 的同类做法 |

**结论**：L3 = 两个新脚本（安装器 + 运行器）+ 一条 `docs/ops` 章节 + 离线可断言的测试（fake `gh`、沙箱 `HOME`/`XDG_STATE_HOME`）。真实宿主安装由运维执行（`[Ops-Followup]`）。

### 变更点

#### 1. `scripts/install-user-cache-reconcile-timer.sh`

```
bash scripts/install-user-cache-reconcile-timer.sh
  [--print|--apply|--uninstall] [--backend auto|systemd|cron]
  [--at 03:30] [--repo DIR]
```

| 行为 | 契约 |
|---|---|
| `--print`（默认） | 只打印将安装的 unit / cron 行 + 目标路径；`FILES_WRITTEN=0` |
| `--apply` | systemd user timer 优先（`install-worktree-cleanup-timer.sh` 风格：`$HOME/.config/systemd/user/opencode-user-cache-reconcile.{service,timer}` + `systemctl --user daemon-reload && enable --now`），否则写带 marker 的 crontab 行；打印 `BACKEND=<systemd\|cron>`、`FILES_WRITTEN=<n>`、`NEXT_ELAPSE=<iso\|unknown>` |
| `--uninstall` | 移除 unit + `systemctl --user disable --now`；或按 marker 精确删 cron 行；`REMOVED=<n>` |
| `--at HH:MM` | 默认 `03:30`；非法值 `exit 2` |
| 幂等 | 重复 `--apply` 不产生重复 cron 行 / unit 内容一致（`IDEMPOTENT=1`） |

unit 内容硬约束（测试逐条断言）：
- `OnCalendar=*-*-* 03:30:00` + `Persistent=true`；
- `ExecStart` 指向 `scripts/reconcile-user-cache-cron.sh`，**不得**含 `--apply`（`UNIT_EXEC_READONLY=1`）；
- 凭据走 `EnvironmentFile=%h/.config/tuneloop/reconcile.env`（安装器只写路径，不写内容）；unit 内**不得**出现 `PASSWORD`/`SECRET`/`TOKEN` 字面量（`UNIT_NO_SECRET=1`）；
- 日志 `StandardOutput=append:` 指向 `${XDG_STATE_HOME:-$HOME/.local/state}/tuneloop/user-cache-reconcile.log`。

#### 2. `scripts/reconcile-user-cache-cron.sh`（薄运行器，不复制判定逻辑）

```
bash scripts/reconcile-user-cache-cron.sh [--date YYYY-MM-DD] [--dry-run]
```

流程与**逐分支**契约：

| 分支 | 条件 | 行为 | 输出 token |
|---|---|---|---|
| 正常无差异 | `RESULT: PASS` 且 6 类计数全 0 | 归档报告，**不建 Issue、不发通知** | `DIFFS=0`、`GH_CALLS=0`、`RESULT: PASS` |
| 有差异 | 任一类别 >0 | 归档报告；若同日期同标题 Issue 不存在 → 建 `[Ops] 用户缓存对账差异 <date>`（`waitForConfirm`，正文含汇总 token + 前 20 条明细 + S2 人工处置命令）；已存在 → **不再建** | `DIFFS=<n>`、`GH_CALLS=<1\|0>`、`ISSUE=<num\|existing>` |
| 连续 2 天不降 | 与上一份归档比较，某类别连续 2 天 >0 且未下降 | 在上述 Issue 追加**恰好 1 条**升级评论（`ESCALATED=1`），不新建第二个 Issue | `ESCALATED=1`、`ESCALATE_COMMENTS=1` |
| 对账失败 | 脚本 `exit != 0` 或 `RESULT: FAIL` | 归档失败原因，建 `[Ops] 用户缓存对账失败 <date>`（`waitForConfirm`），本脚本 `exit 2` | `FAILURE_ISSUE=<num>`、`RESULT: FAIL` |
| 参数/环境缺失 | 缺 `gh`、缺 env 文件 | `exit 2` 并打印缺失项（禁止静默成功） | `MISSING_ENV=<name>` |

- 归档位置：`${XDG_STATE_HOME:-$HOME/.local/state}/tuneloop/reconcile/<date>.json` + `.txt`（**不写仓库**，避免定时任务污染 Git）；`LAST_SUMMARY=<path>` 供次日比较。
- 去重：`gh issue list --search "用户缓存对账差异 <date> in:title" --state all` → 命中即复用。
- `--dry-run`：只打印将要执行的动作（`DRY_RUN=1`、`GH_CALLS=0`、`FILES_WRITTEN=0`），供证据离线断言。

#### 3. 文档

`docs/ops/user-cache-reconcile.md`（#2623 创建）追加「例行任务与告警」章节：安装/卸载命令、env 文件字段清单（两库 DSN + 日志路径）、告警分流（差异 → `waitForConfirm` Issue；失败 → 失败 Issue）、升级规则、以及「自动化只读、`--apply` 永远人工」。

#### 4. 测试（离线，fake `gh` + 沙箱 HOME）

`scripts/tests/test_reconcile_user_cache_cron_2625.sh`：
- fake `gh`（`GH_BIN` 注入，记录调用参数到文件，模拟 `issue list` 命中/未命中、`issue create` 返回号）；
- fake 对账脚本（`RECONCILE_BIN` 注入）按场景输出 token（全 0 / 有差异 / `RESULT: FAIL` / `exit 2`）；
- 沙箱 `HOME` + `XDG_STATE_HOME`（`mktemp -d` 于 `tmp/`），断言 unit/cron 文件与状态目录内容。

### 既有断言影响（0 翻转）

| 检查 | 结果 | 依据 |
|---|---|---|
| #2623/#2624 的只读与 `--apply` 断言 | 不受影响：本单不修改对账脚本 | 变更点 1–2 |
| 既有 timer 安装器（worktree/lock-board/stale-claim 等） | 不受影响：新脚本独立命名/独立 unit 前缀 | `~/.config/opencode/scripts/install-*-timer.sh` |
| 需要翻转的语义断言 | **无**（本单只新增运维脚本与文档） | — |

### Files（变更文件清单）

- `scripts/install-user-cache-reconcile-timer.sh`
- `scripts/reconcile-user-cache-cron.sh`
- `scripts/tests/test_reconcile_user_cache_cron_2625.sh`
- `docs/ops/user-cache-reconcile.md`
- `docs/evidence/2625/plan.md`
- `docs/evidence/2625/R1-timer-install.txt`
- `docs/evidence/2625/R2-cron-alert-branches.txt`
- `docs/evidence/2625/R3-escalation-and-failure.txt`
- `docs/evidence/2625/evidence.json`
- `docs/evidence/2625/pre-ready-2625.json`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 timer 安装器（print/apply/uninstall + unit 硬约束 + 幂等） | `scripts/install-user-cache-reconcile-timer.sh`、`scripts/tests/test_reconcile_user_cache_cron_2625.sh` | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r1` | 沙箱 `HOME`/`XDG_STATE_HOME`；`--backend systemd`（静态 unit 断言）与 `cron` 两种 | 本机（沙箱，不触碰真实 systemd/crontab） | `TEST_EXIT=0`、`PRINT_FILES_WRITTEN=0`、`APPLY_FILES_WRITTEN>=1`、`UNIT_ONCALENDAR_0330=1`、`UNIT_PERSISTENT=1`、`UNIT_EXEC_READONLY=1`、`UNIT_ENVFILE=1`、`UNIT_NO_SECRET=1`、`IDEMPOTENT=1`、`UNINSTALL_REMOVED>=1` | R1 |
| R2 运行器分支（无差异静默 / 有差异建单 / 同日去重） | `scripts/reconcile-user-cache-cron.sh` | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r2` | fake 对账输出全 0；输出 3 类差异；同日二次运行 | 本机（fake `gh`，不发真实 Issue） | `TEST_EXIT=0`、`DIFFS0_GH_CALLS=0`、`DIFFS0_ISSUE_CREATED=0`、`DIFFS_ISSUE_CREATED=1`、`ISSUE_LABEL_WAITFORCONFIRM_ONLY=1`、`ISSUE_BODY_HAS_TOKENS=1`、`SAME_DAY_DUPLICATE=0`、`ARCHIVE_WRITTEN=1` | R2 |
| R3 连续 2 天升级 + 对账失败必须告警 | `scripts/reconcile-user-cache-cron.sh` | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r3` | 次日同类差异未降；对账 `exit 2` / `RESULT: FAIL` | 本机（fake `gh`） | `TEST_EXIT=0`、`ESCALATED=1`、`ESCALATE_COMMENTS=1`、`ESCALATE_NEW_ISSUE=0`、`FAILURE_ISSUE_CREATED=1`、`FAILURE_EXIT=2`、`FAILURE_NOT_SILENT=1`、`MISSING_ENV_EXIT=2` | R3 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 沙箱 `HOME`/`XDG_STATE_HOME`（`tmp/` 下 `mktemp -d`）+ 假 `systemctl`（可选，缺失时走 `--print` 静态断言） | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r1` | `docs/evidence/2625/R1-timer-install.txt` | `TEST_EXIT=0`、`PRINT_FILES_WRITTEN=0`、`APPLY_FILES_WRITTEN>=1`、`CRON_MARKER_PRESENT=1`、`UNIT_ONCALENDAR_0330=1`、`UNIT_PERSISTENT=1`、`UNIT_EXEC_READONLY=1`、`UNIT_ENVFILE=1`、`UNIT_NO_SECRET=1`、`IDEMPOTENT=1`、`UNINSTALL_REMOVED>=1` | `--print` 写文件、unit 含 `--apply`、unit 含密码/密钥字面量、缺 `EnvironmentFile`、重复 apply 产生第二份 cron 行、uninstall 残留 → 任一即 FAIL |
| R2 | fake `gh`（记录调用）+ fake 对账输出（全 0 / 3 类差异）+ 沙箱状态目录 | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r2` | `docs/evidence/2625/R2-cron-alert-branches.txt` | `TEST_EXIT=0`、`DIFFS0_GH_CALLS=0`、`DIFFS0_ISSUE_CREATED=0`、`DIFFS_ISSUE_CREATED=1`、`ISSUE_LABELS_EXACT=waitForConfirm`、`ISSUE_BODY_HAS_TOKENS=1`、`ISSUE_BODY_DETAIL_LIMIT=20`、`SAME_DAY_DUPLICATE=0`、`ARCHIVE_WRITTEN=1` | 无差异仍建 Issue、Issue 带 `status:*`、正文缺汇总 token、同日重复建单、未归档报告 → 任一即 FAIL |
| R3 | 昨日归档（同类差异计数更高或相等）+ fake `gh` 命中既有 Issue；失败场景 fake 对账 `exit 2` | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r3` | `docs/evidence/2625/R3-escalation-and-failure.txt` | `TEST_EXIT=0`、`ESCALATED=1`、`ESCALATE_COMMENTS=1`、`ESCALATE_NEW_ISSUE=0`、`FAILURE_ISSUE_CREATED=1`、`FAILURE_EXIT=2`、`MISSING_ENV_EXIT=2` | 连续 2 天不降未升级、升级时又新建一个 Issue、失败被当成功（`RESULT: PASS` / `exit 0`）、缺 env 静默通过 → 任一即 FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r1 > docs/evidence/2625/R1-timer-install.txt` | `docs/evidence/2625/R1-timer-install.txt` | exit 0 且含 `TEST_EXIT=0` `PRINT_FILES_WRITTEN=0` `UNIT_ONCALENDAR_0330=1` `UNIT_PERSISTENT=1` `UNIT_EXEC_READONLY=1` `UNIT_ENVFILE=1` `UNIT_NO_SECRET=1` `IDEMPOTENT=1` | PASS（8 项 token 全为期望值） | `--print` 写文件、unit 含 `--apply`/密钥、缺 EnvironmentFile、重复安装、卸载残留 → FAIL | dev |
| R2 | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r2 > docs/evidence/2625/R2-cron-alert-branches.txt` | `docs/evidence/2625/R2-cron-alert-branches.txt` | exit 0 且含 `TEST_EXIT=0` `DIFFS0_GH_CALLS=0` `DIFFS0_ISSUE_CREATED=0` `DIFFS_ISSUE_CREATED=1` `ISSUE_LABELS_EXACT=waitForConfirm` `ISSUE_BODY_HAS_TOKENS=1` `SAME_DAY_DUPLICATE=0` `ARCHIVE_WRITTEN=1` | PASS（8 项 token 全为期望值） | 无差异建单、Issue 带 `status:*`、正文缺 token、同日重复、未归档 → FAIL | dev |
| R3 | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r3 > docs/evidence/2625/R3-escalation-and-failure.txt` | `docs/evidence/2625/R3-escalation-and-failure.txt` | exit 0 且含 `TEST_EXIT=0` `ESCALATED=1` `ESCALATE_COMMENTS=1` `ESCALATE_NEW_ISSUE=0` `FAILURE_ISSUE_CREATED=1` `FAILURE_EXIT=2` `MISSING_ENV_EXIT=2` | PASS（7 项 token 全为期望值） | 未升级、升级时重复建单、失败静默、缺 env 静默 → FAIL | dev |

### Artifact Provenance

- 每个文本 artifact 首行 `# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，随后 `GENERATED_AT_HEAD=<40 位 sha>`；证据必须在 fix commit **之后**生成（`FIX_REF` 可覆盖锚点，默认 `git log --grep="^fix(issue-2625)"`）。
- 证据全部在**沙箱**（`HOME`/`XDG_STATE_HOME` 指向 `tmp/` 下临时目录）产生：**不得**写真实 `~/.config/systemd`、真实 crontab、**不得**调用真实 `gh` 建 Issue（`GH_BIN` 注入 fake）。
- 逐条清单（全部位于 `docs/evidence/2625/`）：
  - `docs/evidence/2625/R1-timer-install.txt`
  - `docs/evidence/2625/R2-cron-alert-branches.txt`
  - `docs/evidence/2625/R3-escalation-and-failure.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2625 --summary-file tmp/pre-ready-2625-summary.md --changed-files tmp/pre-ready-2625-changed.txt --evidence-file docs/evidence/2625/evidence.json --report docs/evidence/2625/pre-ready-2625.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - 对账输出契约 = `scripts/reconcile-user-cache.sh`（#2623）；本单**只解析它的 token/JSON**，不重算判定；
  - timer 安装契约 = `~/.config/opencode/scripts/install-worktree-cleanup-timer.sh`（`--print/--apply/--uninstall` + marker + 备份日志）；
  - 告警单标签 = `waitForConfirm` 且**无** `status:*`（父 `P8`）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `ls scripts/install-*-timer.sh 2>/dev/null | wc -l` → 修前 0（仓库内无 timer 安装器，范式在 `~/.config/opencode/scripts/`）；
  - `grep -rn 'reconcile-user-cache-cron' scripts/ | wc -l` → 修前 0；
  - `gh issue list --label waitForConfirm --state open --json number -q 'length'` → 记录基线（本单不得新增真实 Issue）。
- **损坏输入拒绝规则**：
  - 对账输出缺任一必需 token / `RESULT` 行缺失 → 视为**失败**（建失败 Issue + `exit 2`），不得当无差异；
  - `--at` 非法、`GH_BIN` 不可执行、env 文件缺失 → `exit 2` 并打印缺失项；
  - 归档目录不可写 → `exit 2`（不静默丢报告）。
- **工具盲区**：
  - 本机可能无 `systemctl --user`：`--apply` 的 systemd 路径在测试中用静态 unit 断言 + fake `systemctl`；真实 `NEXT_ELAPSE` 归部署侧复验；
  - 真实 Issue 创建不发（fake `gh`），线上行为由 `[Ops-Followup of #2625]` 在安装后首夜回评；
  - 时区：`OnCalendar` 用宿主本地时区（默认 Asia/Shanghai）；若宿主为 UTC 需在 env 文件里显式 `TZ`，文档写清。
- **契约影响**：无 API/前端/DB 契约变更（只新增运维脚本 + 文档章节）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 对账判定与 token 契约唯一 owner = #2623；本单禁止复制判定逻辑，只解析 token/JSON | 本计划锁定 |
| D2 | 自动化只调**只读**模式；`--apply` 永不进入任何定时任务（unit/cron 断言 `UNIT_EXEC_READONLY=1`） | 父 `P5` |
| D3 | 告警单标签**只有** `waitForConfirm`（无 `status:*`），确保永不进入流水线；同日期去重、同日只建 1 单 | 父 `P8` |
| D4 | 「对账失败」与「有差异」分流：失败建 `对账失败` 单并 `exit 2`；差异建 `对账差异` 单；无差异静默 | 父 `P7` |
| D5 | 升级规则 = 同类差异连续 2 天未下降 → 在既有单追加 1 条评论（不新建第二单） | 本计划锁定 |
| D6 | 归档落在 `${XDG_STATE_HOME:-$HOME/.local/state}/tuneloop/`，**不写仓库**（定时任务不得产生 Git 变更） | 本计划锁定 |
| D7 | 凭据经 `EnvironmentFile=%h/.config/tuneloop/reconcile.env` 提供；安装器只写路径、unit 内零密钥字面量 | 本计划锁定 |
| D8 | 真实宿主安装/首夜验证由运维执行（AI 无该机权限）→ closing 时建 `[Ops-Followup of #2625]`（`waitForConfirm`） | 父 `P8` |
| D9 | 本单不改 `scripts/reconcile-user-cache.sh`（如需 token 调整回 #2623） | 本计划锁定 |
| D10 | 时区默认宿主本地（Asia/Shanghai），跨时区宿主通过 `EnvironmentFile` 的 `TZ` 显式声明，文档写明 | 本计划锁定 |

### Out of Scope

- 差异判定逻辑 / token 契约（#2623）、任何修复动作（#2624）。
- 生产/预生产宿主上的实际安装与首夜运行（`[Ops-Followup of #2625]`）。
- 对账结果可视化看板、趋势图（如需另立 Issue）。
- 邮件/企业微信等其它通知渠道（本单只建 GitHub Issue）。
- 对账脚本本身的性能优化/分页（#2623 负责）。

### Dependencies

- #2623

### Workload

- Size: M
- Basis: 安装器 1 + 运行器 1 + 测试 1 + 文档 1 章节；3 条证据行（R1/R2/R3）；无 DB 读写、无 migration、无 Go/前端改动
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 缓存错误长期静默存在，只在某个功能突然 404/数据范围错乱时才被发现；无例行体检 | #2622 正文；#2620 实例 |
| 2 | 触发条件（trigger） | 写路径漏写/迁移/手工操作引入漂移后无人校验 | #2622 正文 |
| 3 | 根因（root cause） | 缺少「例行执行 + 差异可见 + 失败可见」的运行机制：一次性工具没人跑，跑了也没人看 | #2622 正文「L3」；本单 D4/D5 |
| 4 | 测试盲点（test blind spot） | 既有仓库无 timer 安装器与告警运行器的任何测试；运维脚本长期靠人工目测 | `scripts/install-*-timer.sh` 在仓库内 0 命中 |
| 5 | 同类扫描（same-class paths） | `~/.config/opencode/scripts/install-*-timer.sh` 有 5 个同族安装器；本单沿用其 `--print/--apply/--uninstall` + marker 范式，避免第二种风格 | 第 2 条事实 |
| 6 | 修复层（fix layer） | 运维层（调度 + 告警），不改业务代码；判定归 #2623、修复归 #2624 | 父 #2622 拓扑 |
| 7 | 红绿实证（red-green） | 红：仅落测试时两个脚本不存在 → r1/r2/r3 全 FAIL；绿：实现后三分支全绿 | 变更点 4 |
| 8 | 防复发（prevention） | ① unit 断言「只读 + 无密钥 + 03:30 + Persistent」；② 无差异 `GH_CALLS=0`（防空 Issue 噪声）；③ 同日去重；④ 失败必须 `exit 2` + 建单（防静默）；⑤ 连续 2 天升级 | R1/R2/R3 |
| 9 | 影响面（blast radius） | 只新增运维脚本 + 一条文档章节；不触碰业务代码、DB、API；定时任务只读 | D2、R1 的 unit 断言 |
| 10 | 未修边界（deferred） | 真实宿主安装/首夜验证 → `[Ops-Followup]`；判定与修复归 #2623/#2624；通知渠道与看板不在本单 | Out of Scope、D8 |

*Model: deepseek/deepseek-flash*

