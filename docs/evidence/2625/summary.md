## Work Summary — #2625 [Sub-task of #2622 L3] 用户缓存对账例行化与告警

### 修改内容

1. **新增调度安装器** `scripts/install-user-cache-reconcile-timer.sh`
   - `--print`（默认，零写入）/ `--apply` / `--uninstall`，`--backend auto|systemd|cron`、`--at HH:MM`（默认 03:30）、`--repo`。
   - systemd user timer 优先（opencode-user-cache-reconcile.{service,timer}），失败可按 `auto` 回退；cron 兜底带 marker `# opencode-user-cache-reconcile`，**幂等**（重复 apply 仍只有一行）。
   - unit 硬约束：`OnCalendar=*-*-* 03:30:00` + `Persistent=true`；`ExecStart` 指向运行器且**不含** `--apply`；凭据走 EnvironmentFile=%h/.config/tuneloop/reconcile.env（unit 内零密钥字面量）；日志 append 到 ${XDG_STATE_HOME:-$HOME/.local/state}/tuneloop/user-cache-reconcile.log。
2. **新增薄运行器** `scripts/reconcile-user-cache-cron.sh`（**不复制判定逻辑**，只解析 #2623 的 tokens/rows）
   - 无差异（6 类计数全 0）→ 只归档，`GH_CALLS=0`、不建 Issue、不通知。
   - 有差异 → 归档 + 建 `[Ops] 用户缓存对账差异 <date>`（标签**仅** `waitForConfirm`，正文含汇总 token + 前 20 条明细 + 人工处置指引）；同日已存在则复用，不重复建单。
   - 连续 2 天同类差异未下降 → 在既有单追加**恰好 1 条**升级评论（`ESCALATED=1`/`ESCALATE_COMMENTS=1`），不新建第二单。
   - 对账失败（`exit != 0` / 输出非法 / 缺必需 token）→ 建 `[Ops] 用户缓存对账失败 <date>`（`waitForConfirm`）并 `exit 2`，**禁止**静默或冒充「0 差异」。
   - 缺 `gh` / 缺 env / 归档目录不可写 → `exit 2` + `MISSING_ENV=<name>`。
   - 归档落 `${XDG_STATE_HOME}/tuneloop/reconcile/<date>.{json,txt}`（**不写仓库**）；`--dry-run` 只打印动作（`GH_CALLS=0`、`FILES_WRITTEN=0`）。
3. **新增离线测试** `scripts/tests/test_reconcile_user_cache_cron_2625.sh`（fake `gh` 记录调用与正文、fake 对账脚本、沙箱 `HOME`/`XDG_STATE_HOME`、fake `systemctl`/`crontab`）。
4. **文档** `docs/ops/user-cache-reconcile.md` 追加「8. 例行任务与告警」：安装/卸载、env 文件字段清单、运行器分支表、归档与 `--dry-run`、标签与升级规则、自动化只读声明。

### 涉及文件

- `scripts/install-user-cache-reconcile-timer.sh`（新增）
- `scripts/reconcile-user-cache-cron.sh`（新增）
- `scripts/tests/test_reconcile_user_cache_cron_2625.sh`（新增）
- `docs/ops/user-cache-reconcile.md`（追加章节；该文件由 #2623 创建，本单从 `origin/issue-2623` 取同源版本后追加）
- `docs/evidence/2625/plan.md`、`docs/evidence/2625/R1-timer-install.txt`、`docs/evidence/2625/R2-cron-alert-branches.txt`、`docs/evidence/2625/R3-escalation-and-failure.txt`、`docs/evidence/2625/evidence.json`、`docs/evidence/2625/pre-ready-2625.json`、`docs/evidence/2625/summary.md`、`docs/evidence/2625/changed-files.txt`

### Commit

- fix: `72ca1bcbc`（`fix(issue-2625): nightly user cache reconcile timer + read-only alert runner`）
- evidence: `docs(evidence): issue-2625 …`（fix commit 之后从该 commit 重新生成，`dirty=false`）

### 实现前差异核对（计划「Input Truth & Tool Preflight」逐条）

| 检查 | 期望 | 实测 |
|---|---|---|
| `ls scripts/install-*-timer.sh` | 修前 0 | 0 ✅ |
| `grep -rn 'reconcile-user-cache-cron' scripts/` | 修前 0 | 0 ✅ |
| `gh issue list --label waitForConfirm --state open --json number -q length` | 记录基线 | 30（本单**未新增任何真实 Issue**，测试全部走 fake `gh`） ✅ |

### 验证层级（Issue #70）

- 子任务（`Parent: #2622`）：只跑**定向**离线测试 r1/r2/r3（不涉及 Go/前端/DB，无全量 gate）；计划 D8 已把真实宿主安装与首夜验证划给运维。
- 全部证据在沙箱产生：未写真实 ~/.config/systemd、未改真实 crontab、未调用真实 `gh`、未连任何数据库。

### Remote

remote: origin/issue-2625; fix=72ca1bcbc pushed=yes; evidence 提交 pushed=yes

### 红绿实证

红（实现前）：两个脚本不存在，`bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r1|r2|r3` 三行均无法执行（`No such file`，非 0 退出），所有 token 缺失。

绿（fix commit `72ca1bcbc`）：三行 `TEST_EXIT=0` / `RESULT: PASS`：

```
R1: PRINT_FILES_WRITTEN=0 APPLY_FILES_WRITTEN=2 UNIT_ONCALENDAR_0330=1 UNIT_PERSISTENT=1
    UNIT_EXEC_READONLY=1 UNIT_ENVFILE=1 UNIT_NO_SECRET=1 IDEMPOTENT=1
    CRON_MARKER_PRESENT=1 CRON_IDEMPOTENT=1 UNINSTALL_REMOVED=2 BAD_AT_EXIT=2
R2: DIFFS0_GH_CALLS=0 DIFFS0_ISSUE_CREATED=0 DIFFS_ISSUE_CREATED=1
    ISSUE_LABELS_EXACT=waitForConfirm ISSUE_BODY_HAS_TOKENS=1 ISSUE_BODY_DETAIL_LIMIT=1
    SAME_DAY_DUPLICATE=0 ARCHIVE_WRITTEN=1 DRY_RUN_GH_CALLS=0
R3: ESCALATED=1 ESCALATE_COMMENTS=1 ESCALATE_NEW_ISSUE=0 FAILURE_ISSUE_CREATED=1
    FAILURE_EXIT=2 FAILURE_NOT_SILENT=1 MISSING_ENV_EXIT=2
```

### Accepted Plan 对照（逐条完成）

| 计划项 | 状态 |
|---|---|
| 变更点 1 安装器（print/apply/uninstall + backend/at/repo + unit 硬约束 + 幂等） | ✅ |
| 变更点 2 运行器（无差异静默 / 有差异建单 / 同日去重 / 连续 2 天升级 / 失败告警 / 缺 env / dry-run / 归档不写仓库） | ✅ |
| 变更点 3 文档章节（安装/卸载、env 字段、分支表、升级规则、只读声明） | ✅ |
| 变更点 4 离线测试（fake gh / fake reconcile / 沙箱 HOME / fake systemctl+crontab） | ✅ |
| Decision Lock D1–D10 | ✅（D1 只解析 #2623 输出；D2 unit 断言只读；D3 标签仅 waitForConfirm；D4 失败与差异分流；D5 升级只加 1 条评论；D6 归档不进仓库；D7 EnvironmentFile + 零密钥；D9 未改 scripts/reconcile-user-cache.sh；D10 时区默认宿主本地并在文档写明） |
| Out of Scope 未越界 | ✅ 未改判定逻辑/token、未做修复动作、未做看板/其它通知渠道 |

### 依赖顺序说明（需 Auditor 知悉）

- 依赖 `#2623` 当前为 `status:ready`（未合入 `develop`）。本单需要在其创建的 `docs/ops/user-cache-reconcile.md` 上**追加**章节，因此从 `origin/issue-2623` 取同源文件后追加（文件内容 = #2623 版本 + 本单 §8）。
- 合并顺序建议：**#2623 先合，再合 #2625**（同文件追加，若有 add/add 冲突只需保留 §8）。

### Evidence Contract（3/3 行，全部 PASS）

| 行 | 命令 | 产物 | 关键 token | 状态 |
|---|---|---|---|---|
| R1 | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r1` | `docs/evidence/2625/R1-timer-install.txt` | TEST_EXIT=0 / PRINT_FILES_WRITTEN=0 / UNIT_ONCALENDAR_0330=1 / UNIT_PERSISTENT=1 / UNIT_EXEC_READONLY=1 / UNIT_ENVFILE=1 / UNIT_NO_SECRET=1 / IDEMPOTENT=1 | PASS |
| R2 | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r2` | `docs/evidence/2625/R2-cron-alert-branches.txt` | TEST_EXIT=0 / DIFFS0_GH_CALLS=0 / DIFFS0_ISSUE_CREATED=0 / DIFFS_ISSUE_CREATED=1 / ISSUE_LABELS_EXACT=waitForConfirm / ISSUE_BODY_HAS_TOKENS=1 / SAME_DAY_DUPLICATE=0 / ARCHIVE_WRITTEN=1 | PASS |
| R3 | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r3` | `docs/evidence/2625/R3-escalation-and-failure.txt` | TEST_EXIT=0 / ESCALATED=1 / ESCALATE_COMMENTS=1 / ESCALATE_NEW_ISSUE=0 / FAILURE_ISSUE_CREATED=1 / FAILURE_EXIT=2 / MISSING_ENV_EXIT=2 | PASS |

依据 `docs/evidence/2625/evidence.json`（schema_version=1、issue=2625、build=1.0.72ca1bcb、commit=72ca1bcb…、dirty=false、plan_sha256=92b1a823…）。

### G0 Golden Acceptance

| 行 | artifact | assertions | negative |
|---|---|---|---|
| R1 | `docs/evidence/2625/R1-timer-install.txt` | 8 项合同 token + `CRON_MARKER_PRESENT=1`、`CRON_IDEMPOTENT=1`、`UNIT_LOG_APPEND=1`、`UNIT_CONTENT_STABLE=1`、`UNINSTALL_REMOVED=2`、`BAD_AT_EXIT=2` | `--print` 写文件、unit 含 `--apply`/密钥、缺 EnvironmentFile、重复安装产生第二份 cron 行、卸载残留 → 任一即 FAIL（实测均为 0 命中） |
| R2 | `docs/evidence/2625/R2-cron-alert-branches.txt` | 8 项合同 token + `ISSUE_BODY_DETAIL_LIMIT=1`、`DIFFS_GH_CALLS=2`、`DRY_RUN_FILES_WRITTEN=0` | 无差异仍建单、Issue 带 `status:*`、正文缺 token、同日重复建单、未归档 → 任一即 FAIL（实测 create 次数/标签/正文均已断言） |
| R3 | `docs/evidence/2625/R3-escalation-and-failure.txt` | 7 项合同 token + `ESCALATE_COMMENT_CALLS=1`、`ESCALATE_REASON_IN_BODY=1`、`FAILURE_NO_PASS=0`、`FAILURE_LABEL_EXACT=1`、`MISSING_ENV_TOKEN=BEACONIAM_DB_NAME` | 连续 2 天不降未升级、升级时重复建单、失败被当成功、缺 env 静默 → 任一即 FAIL（实测失败路径无 `RESULT: PASS`） |

### Verification Matrix（矩阵 3/3）

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 timer 安装器（print/apply/uninstall + unit 硬约束 + 幂等） | PASS | `docs/evidence/2625/R1-timer-install.txt` |
| R2 运行器分支（无差异静默 / 有差异建单 / 同日去重） | PASS | `docs/evidence/2625/R2-cron-alert-branches.txt` |
| R3 连续 2 天升级 + 对账失败必须告警 | PASS | `docs/evidence/2625/R3-escalation-and-failure.txt` |

### 证据真实性 / 持久化

- 三份 artifact 位于 `docs/evidence/2625/`（随 `docs(evidence)` 提交），首行 provenance `# build=1.0.72ca1bcb # commit=72ca1bcbcb22645de848b33d86e89a8706cf6daa # dirty=false`，与 evidence.json 一致；证据在 fix commit **之后**生成。
- 三条证据对应不同脚本/断言面，无跨行复用：R1 = 安装器（unit/cron 文件断言）；R2 = 运行器正常分支；R3 = 升级与失败分支。

### 测试保真

- fake `gh` 把 `--body-file` 内容复制到沙箱目录，因此「正文含 token」「明细 ≤20 条」「标签只有 waitForConfirm」是**对真实命令行与真实正文**的断言，而非对脚本源码的 grep。
- 「不建单」用 `grep -c 'issue create' $GH_LOG` = 0 断言；「升级不新建第二单」用同一计数 + `issue comment` 计数 = 1 断言。
- 安装器断言读取沙箱里**真实写出的 unit 文件**（`sha256sum` 比对幂等性），`--print` 的零写入用沙箱目录文件数判定。

### 依赖顺序 / 验收规则

- 计划 `## Dependencies`：`#2623`（ready）——已满足；本单未修改 #2623 的脚本或 token 契约（D9）。
- 未修改任何既有验收工具/规则；未出现连续两轮验收失败；无延期项。

### 人工 / 部署侧（不由 Developer 完成）

| 事项 | 负责方 | 回执要求 |
|---|---|---|
| 真实宿主安装（`--apply`）与首夜运行验证（`NEXT_ELAPSE`、归档文件、告警分流） | 运维 | 关单时按 `[Ops-Followup of #2625]`（`waitForConfirm`，无 `status:*`）另记；需回评首夜归档路径与 token 摘要 |

*Model: deepseek/deepseek-flash*
