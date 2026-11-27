## Fix Summary — #2625 [Sub-task of #2622 L3] 例行任务与告警（Round 1 audit 返工）

### 审计意见 → 修复映射

| 审计项 | 矩阵行 | 根因分类 | 修复后证据 |
|---|---|---|---|
| **B1** 有差异分支 `gh issue create` 失败仍 `RESULT: PASS` + `exit 0`（告警静默丢失） | R2（负例扩展） | 执行（外部调用失败被 `\|\| true` 吞掉） | `scripts/reconcile-user-cache-cron.sh:266-290`；证据 `R2-cron-alert-branches.txt`：`GH_CREATE_FAIL_EXIT=2`、`GH_CREATE_FAIL_NO_PASS=0`、`GH_CREATE_FAIL_ISSUE=none`、`GH_CREATE_FAIL_TOKEN=1`、`GH_CREATE_FAIL_RC_SHOWN=1` |
| **B2** 升级评论失败仍声称 `ESCALATED=1` + `exit 0` | R3（负例扩展） | 执行（同类未对称） | `scripts/reconcile-user-cache-cron.sh:317-345`；证据 `R3-escalation-and-failure.txt`：`GH_COMMENT_FAIL_EXIT=2`、`ESCALATE_CLAIMED_ON_FAILURE=0`、`ESCALATE_COMMENTS_ON_FAILURE=0`、`ESCALATE_FAIL_TOKEN=1` |
| **补充** `handle_failure` 建失败单也吞错（`FAILURE_ISSUE=unknown` 无法区分） | R3（负例扩展） | 执行（同类未对称） | `scripts/reconcile-user-cache-cron.sh:166-184`；证据：`FAILURE_CREATE_FAIL_EXIT=2`、`FAILURE_CREATE_FAIL_TOKEN=1`、`FAILURE_CREATE_FAIL_ISSUE=none` |
| 同类路径扫描（`gh issue list` 去重调用亦被吞） | R2/R3 | 执行 | `find_issue_by_title` 现返回 rc=2；调用方记 `GH_LIST_FAILED=1` 并**继续建单**（宁可重复也不丢告警），文档 §8.3 已写明 |
| 语义挑战 1（`ORG_UNRESOLVED`/`IAM_MISSING` 不触发告警） | —（非阻断） | 计划（判定口径 owner = #2623，D1/D9） | 文档 §8.3 增加说明：这些类别属 #2623「只计数、不判差异」口径，仍写入归档与告警正文；如需并入阈值须回 #2623 裁定。**本单未改判定逻辑** |
| 语义挑战 2（同日去重依赖 GitHub 搜索最终一致） | —（非阻断） | 执行（已知边界） | 未改：夜间单次运行风险低；`gh issue list` 失败时已记 `GH_LIST_FAILED=1` 且继续建单 |

### 变更详情

- 修改文件：`scripts/reconcile-user-cache-cron.sh`（告警送达失败的显式失败与 token）、`scripts/tests/test_reconcile_user_cache_cron_2625.sh`（fake `gh` create/comment 失败注入 + 5 组负例断言）、`docs/ops/user-cache-reconcile.md`（§8.3 告警送达失败口径 + 阈值说明）
- fix commit：`fab810a8b`（`fix(issue-2625): 告警送达失败不得静默（create/comment/failure-create 三处对称）`）
- 分支 / 远端：`issue-2625` → `origin/issue-2625`

### 红绿实证（修复类必填）

- 红（缺陷版 `72ca1bcbc` 的 runner + 本轮新测试）：
  - `r2` → `GH_CREATE_FAIL_EXIT=MISMATCH(expected=2 actual=0)`、`GH_CREATE_FAIL_NO_PASS=MISMATCH(expected=0 actual=1)`、`GH_CREATE_FAIL_ISSUE=MISMATCH(expected=none actual=unknown)`、`TEST_EXIT=1`
  - `r3` → `GH_COMMENT_FAIL_EXIT=MISMATCH(expected=2 actual=0)`、`ESCALATE_CLAIMED_ON_FAILURE=MISMATCH(expected=0 actual=1)`、`TEST_EXIT=1`
- 绿（fix commit `fab810a8b`）：`r1`/`r2`/`r3` 均 `TEST_EXIT=0` / `RESULT: PASS`
- Auditor 原命令复跑（等价于审计机的 fake gh 场景，本轮由 r2 场景 5 / r3 场景 4 固定）：
  - fake `gh` `issue create` → 401：修复前 `RESULT: PASS` + `RUNNER_RC=0`；修复后 `RESULT: FAIL` + `RUNNER_RC=2` + `ISSUE=none` + `ISSUE_CREATE_FAILED=1`
  - fake `gh` `issue comment` → 403：修复前 `ESCALATED=1` + `RUNNER_RC=0`；修复后 `ESCALATE_FAILED=1` + `ESCALATED=0` + `RUNNER_RC=2`

### Evidence Contract（3/3 行，全部 PASS）

| 行 | 命令 | 产物 | 关键 token | 状态 |
|---|---|---|---|---|
| R1 | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r1` | `docs/evidence/2625/R1-timer-install.txt` | TEST_EXIT=0 / PRINT_FILES_WRITTEN=0 / UNIT_ONCALENDAR_0330=1 / UNIT_PERSISTENT=1 / UNIT_EXEC_READONLY=1 / UNIT_ENVFILE=1 / UNIT_NO_SECRET=1 / IDEMPOTENT=1 | PASS |
| R2 | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r2` | `docs/evidence/2625/R2-cron-alert-branches.txt` | TEST_EXIT=0 / DIFFS0_GH_CALLS=0 / DIFFS0_ISSUE_CREATED=0 / DIFFS_ISSUE_CREATED=1 / ISSUE_LABELS_EXACT=waitForConfirm / ISSUE_BODY_HAS_TOKENS=1 / SAME_DAY_DUPLICATE=0 / ARCHIVE_WRITTEN=1 / **GH_CREATE_FAIL_EXIT=2 / GH_CREATE_FAIL_NO_PASS=0 / GH_CREATE_FAIL_ISSUE=none / GH_CREATE_FAIL_TOKEN=1** | PASS |
| R3 | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r3` | `docs/evidence/2625/R3-escalation-and-failure.txt` | TEST_EXIT=0 / ESCALATED=1 / ESCALATE_COMMENTS=1 / ESCALATE_NEW_ISSUE=0 / FAILURE_ISSUE_CREATED=1 / FAILURE_EXIT=2 / MISSING_ENV_EXIT=2 / **GH_COMMENT_FAIL_EXIT=2 / ESCALATE_CLAIMED_ON_FAILURE=0 / ESCALATE_FAIL_TOKEN=1 / FAILURE_CREATE_FAIL_EXIT=2 / FAILURE_CREATE_FAIL_TOKEN=1** | PASS |

依据 `docs/evidence/2625/evidence.json`（schema_version=1、issue=2625、build=1.0.fab810a8、commit=fab810a8…、dirty=false、plan_sha256=92b1a823…）。

### 证据真实性 / 持久化

- 三份 artifact 位于 `docs/evidence/2625/`，首两行 `# build=1.0.fab810a8 # commit=fab810a8b0d6e337e4c44cb3e18699a5fece251d # dirty=false` + `GENERATED_AT_HEAD=<相同 sha>`，均在 fix commit **之后**生成。
- 全部在沙箱（`HOME`/`XDG_STATE_HOME` → `tmp/` 下临时目录）产生：未写真实 `~/.config/systemd`、未改真实 crontab、未调用真实 `gh` 建 Issue/评论、未连任何数据库。

### 范围声明

- 仅修复审计 B1/B2 + 同类路径（失败单建单、去重查询）的静默吞错；未引入新功能，未改判定/token 契约（D1/D9 保持），未改 #2623 的 `scripts/reconcile-user-cache.sh`。

### 未完成行

- 无。真实宿主安装与首夜验证按 D8 归 `[Ops-Followup of #2625]`（`waitForConfirm`，无 `status:*`），关单时落盘。

*Model: DeepSeek V4.1 Flash 1017*
