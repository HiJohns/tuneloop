## Accepted Plan — #2631 [Hardening of #2625] 对账告警升级去重与「连续 2 天」基准口径加固

> Parent: #2625。本单只加固 #2625 运行器的**去重与基准口径**三项（H1/H2/H3），不改判定逻辑（#2623）、不做修复动作（#2624）、不改 timer 安装器。

## 0. 事实与基线（Implementer 必读）

| # | 事实 | 证据 |
|---|---|---|
| 1 | `scripts/reconcile-user-cache-cron.sh` **不存在于 `origin/develop`**，#2625 的修复仍在 `origin/issue-2625`（HEAD `f9b544d5`，fix `551079e0f`） | `git ls-tree origin/develop --name-only scripts/` 无命中；`git ls-tree origin/issue-2625 --name-only scripts/` 命中两脚本 |
| 2 | 升级比较对象是「最近一份更早的 `*.json`」而非严格昨日；措辞却写「连续 2 天未下降」 | 脚本 L301-306（`awk ... d < today` + `sort` + `tail -n 1`）、L326（标题） |
| 3 | 同日**新单**有去重（按标题搜索），同日**升级评论**无任何当日标记 | 脚本 L276-292（`find_issue_by_title`）vs L322-349（无 marker） |
| 4 | `find_issue_by_title` 依赖 `gh issue list --search`（最终一致 + 方括号特殊字符）；失败时降级「继续建单」 | 脚本 L95-102、L276、L173 |
| 5 | 归档目录 `${XDG_STATE_HOME:-$HOME/.local/state}/tuneloop/reconcile/` 同时存放 `<date>.json` / `<date>.txt`；`*.json` 是次日比较基准来源 | 脚本 L21-22、L104-112 |
| 6 | 既有离线夹具 `scripts/tests/test_reconcile_user_cache_cron_2625.sh`（r1/r2/r3）用 fake `gh` / fake reconcile / 沙箱 `HOME`+`XDG_STATE_HOME`，**零网络零真实库** | 该文件 L78-225 |
| 7 | #2625 r3 的「升级评论失败」场景与「升级成功」场景**复用同一日期 `2026-11-28`** | 该文件 L427、L463 |

事实 7 是本单的关键连带：H1 引入「同日升级成功 → 当日不再升级」后，r3 的第 2 个场景会因 marker 命中而**跳过**评论，原断言（`GH_COMMENT_FAIL_EXIT=2` / `ESCALATE_FAILED=1`）将失效。因此必须做**夹具日期隔离**（只改日期，不放宽断言），见 D7 与 R6。

## 1. 变更点

### H1 同日升级评论去重（每日最多 1 条升级评论）

- 新增当日 marker `${ARCHIVE_DIR}/.<date>.escalated`（内容含 `date` / `issue` / `baseline` / `reason` / `ts`）。
- 升级前：marker 存在 → **跳过**评论，输出 `ESCALATE_SKIPPED=already-escalated`，`ESCALATED=0`、`ESCALATE_COMMENTS=0`、不产生任何 `gh` 调用，`RESULT: PASS` + `exit 0`（当日升级已送达，无需重复动作）。
- 升级评论**成功之后**才写 marker；`gh issue comment` 失败 → 保持 `ESCALATE_FAILED=1` + `exit 2` 且**不写** marker（下轮仍会重试），既有 B2 语义不变。

### H2 基准口径（缺口不冒充「连续 2 天」）

- 基准选择：取「最近一份更早的**有效**归档」——跳过 `failed:true` 与无法解析的 JSON（`jq -e '.tokens | type == "object"'` 失败）；当前实现会把失败归档当基准（`tokens[...] // "0"` → 永远不升级），属掩盖。
- 新增 token：`ESCALATE_BASELINE=<basename|none>`、`ESCALATE_BASELINE_DAYS=<n|unknown>`、`ESCALATE_BASELINE_GAP=<0|1>`（`GAP=1` 表示基准日期 ≠ 昨日；天数用 `TZ=UTC date -d` 求差 / 86400，避免 DST 抖动）。
- 措辞（选定方案 (a)，见 D3）：`GAP=0` → 标题 `## 连续 2 天未下降（升级提醒）`；`GAP=1` → 标题 `## 较上次归档未下降（升级提醒）`，并追加一行「基准间隔：<n> 天（<base> → <today>）；昨日归档缺失或不可用，请检查例行任务是否漏跑」。
- 无任何有效基准 → `ESCALATE_BASELINE=none`、`ESCALATE_BASELINE_GAP=1`、**不升级**（无基准不得声称「未下降」），仍正常建差异单。

### H3 当日单号本地 marker（搜索双保险）

- 新增 `${ARCHIVE_DIR}/.<date>.issue`（差异单）与 `${ARCHIVE_DIR}/.<date>.failure-issue`（失败单，对称加固）。
- 解析顺序：本地 marker（纯数字）→ `ISSUE_SOURCE=local-marker`，**跳过** `gh issue list`（`GH_CALLS=0`）；否则搜索（`ISSUE_SOURCE=search`）→ 命中即写 marker；再否则 create（`ISSUE_SOURCE=create`）→ **成功才写** marker，失败不写（下轮重试，不得伪造「当天已建单」）。
- 搜索失败仍按既有降级建单（`GH_LIST_FAILED=1`），但之后写好 marker，使后续同轮/次日重跑不再依赖搜索索引。
- marker 写失败不致命（去重降级），但必须显式输出 `MARKER_WRITE_FAILED=<kind>`，不得静默。

### 文档

- `docs/ops/user-cache-reconcile.md` §8.3 分支表 + §8.4 升级规则：补双措辞、`ESCALATE_BASELINE_GAP`、失败/损坏归档不作基准、两种当日 marker、`already-escalated`；删除旧的无条件「不低于昨日」表述。

## 2. 计划步骤（/work 执行顺序）

1. `scripts/worktree-create.sh --project tuneloop --issue 2631 --base origin/issue-2625`（**必须**以 `origin/issue-2625` 为基线，见 D1）。
2. 改 `scripts/reconcile-user-cache-cron.sh`：marker 助手（`marker_path` / `read_num_marker` / `marker_exists` / `write_marker`）、H1/H2/H3 三处接入、`emit_tokens` 新增 token（`ISSUE_SOURCE` / `ISSUE_MARKER` / `FAILURE_ISSUE_SOURCE` / `ESCALATE_BASELINE` / `ESCALATE_BASELINE_DAYS` / `ESCALATE_BASELINE_GAP` / `ESCALATE_SKIPPED` / `ESCALATE_MARKER`）。
3. 新增 `scripts/tests/test_reconcile_user_cache_cron_2631.sh`（自包含离线夹具：fake `gh` 支持 `FAKE_GH_LIST_RC` / `FAKE_GH_CREATE_RC` / `FAKE_GH_COMMENT_RC` / `FAKE_GH_LIST_JSON` / `FAKE_GH_CREATE_NUMBER`，fake reconcile 三档 `zero` / `diff3` / `fail`，沙箱 `HOME`+`XDG_STATE_HOME`）；子行 `h1` / `h2` / `h2b` / `h3` / `guard` / `regress` / `docs`，每子行首行 provenance `# build=1.0.<fix> # commit=<fix> # dirty=<bool>`，末行 `TEST_EXIT=<0|1>` + `RESULT: PASS|FAIL`。
4. 改 `scripts/tests/test_reconcile_user_cache_cron_2625.sh`：**仅**把 r3「升级评论失败」场景日期从 `2026-11-28` 改为 `2026-12-31`，并加一行注释说明原因（同日已升级 marker）；**禁止**改动任何 `expect` 断言的期望值或删除断言。
5. 改 `docs/ops/user-cache-reconcile.md`（见「文档」）。
6. 逐条跑 Evidence Contract 命令（contract 行）→ 生成 `docs/evidence/2631/R*.txt` + `evidence.json`（post-fix、`dirty=false`）。
7. `bash ~/.config/opencode/scripts/pre-ready-check.sh 2631 --summary-file … --changed-files … --evidence-file docs/evidence/2631/evidence.json --report docs/evidence/2631/pre-ready-2631.json` → `RESULT: PASS` 才可 `status:ready`。

## Dependencies

- #2625

## 基线分支与合入顺序

- `#2625` 的 `status:done` 只表示其**分支**已验收，代码尚未进 `develop`（事实 1）。因此本单工作树**必须以 `--base origin/issue-2625` 创建**；若基线里 `scripts/reconcile-user-cache-cron.sh` 缺失 → `/work` **HOLD**（禁止在 develop 上自建同名脚本，否则会造成两份实现）。
- 合入顺序：`issue-2625` → `develop` → `issue-2631`（堆叠分支）。`origin/issue-2625` 若在 `/work` 期间被更新，先 `git fetch` 再 rebase，禁止静默分叉。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|--------|-----------|-----------|----------|----------|----------|------------------------|
| H1 同日升级评论去重 | `scripts/reconcile-user-cache-cron.sh` | 运行器 `--date 2027-01-06`（经 `test_reconcile_user_cache_cron_2631.sh h1`） | 同日两次有差异运行（前一日归档同类差异同值） | Linux bash | 第 1 次 `ESCALATED=1` 与 `ESCALATE_COMMENTS=1`；第 2 次 `ESCALATE_SKIPPED=already-escalated` 与 `ESCALATE_COMMENTS=0` 与 `GH_CALLS=0`；累计 `issue comment` 调用恰 1 | R1 |
| H2 基准缺口措辞与 token | `scripts/reconcile-user-cache-cron.sh` | 运行器 `--date 2027-01-13`（`h2`） | 昨日归档缺失（基准间隔 3 天） | Linux bash | `ESCALATE_BASELINE=2027-01-10.json`、`ESCALATE_BASELINE_DAYS=3`、`ESCALATE_BASELINE_GAP=1`；评论标题「较上次归档未下降」且正文 0 处「连续 2 天」；昨日连续场景 `GAP=0` 且保留「连续 2 天未下降」 | R2 |
| H2 失败/损坏归档不作基准 | `scripts/reconcile-user-cache-cron.sh` | 运行器 `--date 2027-01-17` 与 `--date 2027-01-02`（`h2b`） | 前一日归档为 `failed:true`；另有无任何更早有效归档 | Linux bash | 基准跳过失败归档取 `2027-01-15.json`（`DAYS=2` 与 `GAP=1`）；无基准时 `ESCALATE_BASELINE=none`、`ESCALATED=0`、`GAP=1`、仍建差异单且 exit 0 | R3 |
| H3 当日单号本地 marker | `scripts/reconcile-user-cache-cron.sh` | 运行器 `--date 2027-02-01` 与 `2027-02-02` 与 `2027-02-03`（`h3`） | 搜索命中；搜索失败且 marker 命中；搜索失败且 create 成功/失败 | Linux bash | 搜索命中复用不建单；marker 命中 `ISSUE_SOURCE=local-marker` 与 `GH_CALLS=0` 与 `H3_LIST_CALLS=0`；create 成功写 marker 且重跑不再建单；create 失败**不写** marker 且下轮重试 | R4 |
| dry-run 零写入（含 marker） | `scripts/reconcile-user-cache-cron.sh` | 运行器 `--date 2027-03-01 --dry-run`（`guard`） | 有差异的干跑；零差异实跑 | Linux bash | `FILES_WRITTEN=0`、当日 marker 文件数 0、`GH_CALLS=0`；零差异实跑只写 2 个归档文件、marker 数 0 | R5 |
| #2625 回归全绿与夹具仅日期隔离 | `scripts/tests/test_reconcile_user_cache_cron_2625.sh` | `test_reconcile_user_cache_cron_2625.sh r1` 与 `r2` 与 `r3`（`regress`） | 既有三条用例全量离线跑 | Linux bash | 三行均 `TEST_EXIT=0`；断言调用数不低于基线（`HARNESS_ASSERT_PARITY=1`）；对 `origin/issue-2625` 的 diff 仅含日期/注释行（`HARNESS_DIFF_NON_DATE_LINES=0`） | R6 |
| 文档口径同步 | `docs/ops/user-cache-reconcile.md` | §8.3 分支表与 §8.4 升级规则（`docs`） | 静态文本 | 文档 | 含「较上次归档未下降」与「连续 2 天」（按 GAP 分流）、`ESCALATE_BASELINE_GAP`、两种 marker、`already-escalated`；旧「不低于昨日」表述已移除 | R7 |

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|--------|----------------------|-------------------|-----------------------|
| R1 H1 去重 | 沙箱 `ARCHIVE_DIR` 预置 `2027-01-05.json`（`TENANT_MISMATCH=2`,`ORG_MISMATCH=1`）；fake reconcile `diff3`；fake `gh` list=`[]`、create=`777`；同日运行两次 | `bash scripts/reconcile-user-cache-cron.sh --date 2027-01-06` | `docs/evidence/2631/R1-escalation-dedup.txt`（离线 token 报告） |
| R2 H2 缺口措辞 | 预置 `2027-01-10.json`（差异同值）后运行 `2027-01-13`；另预置 `2027-01-20.json`（同值）后运行 `2027-01-21` | 同上（`--date 2027-01-13`、`--date 2027-01-21`） | `docs/evidence/2631/R2-baseline-gap-wording.txt` |
| R3 H2 失败归档 | 预置 `2027-01-15.json`（差异）与 `2027-01-16.json`（`failed:true`）后运行 `2027-01-17`；另运行 `2027-01-02`（早于所有预置归档） | 同上 | `docs/evidence/2631/R3-failed-archive-baseline.txt` |
| R4 H3 marker | `2027-01-31.json` 全零；运行 `2027-02-01`（list=`[{"number":811}]`）后再跑（`FAKE_GH_LIST_RC=1`）；`2027-02-02`（list 失败、create=`812`）后再跑；`2027-02-03`（list 失败、create 失败）后再跑（create 成功） | 同上 | `docs/evidence/2631/R4-local-issue-marker.txt` |
| R5 dry-run 零写入 | 预置 `2027-02-28.json`（差异）；`--date 2027-03-01 --dry-run`；`--date 2027-03-02`（zero 模式实跑） | 同上 | `docs/evidence/2631/R5-dry-run-zero-markers.txt` |
| R6 #2625 回归 | 既有 r1/r2/r3 夹具（fake systemctl/crontab 与沙箱 HOME） | `bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r1`、`r2`、`r3` | `docs/evidence/2631/R6-regression-2625.txt` |
| R7 文档同步 | 仓库当前 `docs/ops/user-cache-reconcile.md` | `grep` 断言（经 `test_reconcile_user_cache_cron_2631.sh docs`） | `docs/evidence/2631/R7-doc-sync.txt` |

### Assertions

- provenance：7 个 artifact 首行均为 `# build=1.0.<fix short> # commit=<fix sha> # dirty=false`，且 `<fix>` 是本单最新 fix commit（post-fix 生成）。
- R1：`H1_RUN1_ESCALATE_COMMENTS=1`、`H1_RUN1_ESCALATED=1`、`H1_RUN2_ESCALATE_SKIPPED=already-escalated`、`H1_RUN2_ESCALATE_COMMENTS=0`、`H1_RUN2_GH_CALLS=0`、`H1_COMMENT_CALLS_TOTAL=1`、`H1_COMMENT_BODIES=1`、`TEST_EXIT=0`。
- R2：`H2_GAP=1`、`H2_BASELINE=2027-01-10.json`、`H2_DAYS=3`、`H2_GAP_BODY_HAS_GAP_TITLE=1`、`H2_GAP_BODY_CONTIGUOUS_CLAIMS=0`、`H2_CONTIGUOUS_GAP=0`、`H2_CONTIGUOUS_BODY_HAS_CONTIGUOUS_TITLE=1`、`H2_CONTIGUOUS_BODY_GAP_CLAIMS=0`、`TEST_EXIT=0`。
- R3：`H2B_BASELINE=2027-01-15.json`、`H2B_DAYS=2`、`H2B_GAP=1`、`H2B_FAILED_BASELINE_USED=0`、`H2B_NO_BASELINE=1`、`H2B_NO_BASELINE_ESCALATED=0`、`H2B_NO_BASELINE_ISSUE_CREATED=1`、`H2B_NO_BASELINE_EXIT=0`、`TEST_EXIT=0`。
- R4：`H3_SEARCH_ISSUE=811`、`H3_SEARCH_CREATES=0`、`H3_SEARCH_MARKER_EXISTS=1`、`H3_MARKER_ISSUE=811`、`H3_MARKER_SOURCE=local-marker`、`H3_MARKER_GH_CALLS=0`、`H3_MARKER_LIST_CALLS=0`、`H3_DEGRADE_ISSUE=812`、`H3_DEGRADE_LIST_FAILED=1`、`H3_DEGRADE_CREATES=1`、`H3_DEGRADE_RERUN_ISSUE=812`、`H3_FAIL_ISSUE=none`、`H3_FAIL_MARKER_EXISTS=0`、`H3_FAIL_RETRY_CREATES=2`、`TEST_EXIT=0`。
- R5：`GUARD_DRY_RUN_FILES_WRITTEN=0`、`GUARD_DRY_RUN_MARKERS=0`、`GUARD_DRY_RUN_GH_CALLS=0`、`GUARD_ZERO_MARKERS=0`、`GUARD_ZERO_ARCHIVES=2`、`TEST_EXIT=0`。
- R6：`R2625_R1_TEST_EXIT=0`、`R2625_R2_TEST_EXIT=0`、`R2625_R3_TEST_EXIT=0`、`HARNESS_ASSERT_PARITY=1`、`HARNESS_DIFF_NON_DATE_LINES=0`、`TEST_EXIT=0`。
- R7：`DOC_GAP_WORDING=1`、`DOC_CONTIGUOUS_WORDING=1`、`DOC_GAP_TOKEN=1`、`DOC_ESCALATE_MARKER=1`、`DOC_ISSUE_MARKER=1`、`DOC_ALREADY_ESCALATED=1`、`DOC_OLD_UNCONDITIONAL_ROW_GONE=1`、`TEST_EXIT=0`。
- 负例（每行至少 1 条，详见 Evidence Contract「负例」列）：同日重复升级评论 ≥2、缺口场景出现「连续 2 天」、失败归档被当基准、create 失败仍写 marker 或 marker 命中仍调用 `gh issue list`、dry-run 写入 marker、既有断言被删减、文档保留旧无条件表述 —— 任一出现即 FAIL。
- language：artifact 与文档为中文口径 + 英文 token 名，不得混入英文句式替代既有中文措辞。
- 语义权威：`target_semantics` = Decision Lock D2/D3/D5；`historical_baseline` = #2625 现存行为（同值比较 + 无条件「连续 2 天」），两者不得混用。

## Input Truth & Tool Preflight

**Source of truth**

1. 脚本与夹具真值 = `origin/issue-2625`（`git show origin/issue-2625:scripts/reconcile-user-cache-cron.sh`，HEAD `f9b544d5`）；`origin/develop` 无该文件（事实 1），故不得以 develop 为输入。
2. #2625 计划 D5（升级 = 同类差异连续 2 天未下降 → 追加 1 条评论，不新建单）= 本单 `historical_baseline`；本单把「连续」与「较上次归档」拆成两套措辞（D3）。
3. 归档位置与格式真值 = `docs/ops/user-cache-reconcile.md` §8.3/§8.4。

**差异核对**

| 来源 | 差异 | 处理 |
|---|---|---|
| `origin/develop` vs `origin/issue-2625` | 运行器/安装器/夹具/文档章节仅存在于 2625 分支 | 基线固定 `origin/issue-2625`（D1），缺文件即 HOLD |
| #2625 计划 D5 措辞 vs 实现 | 实现比较「最近更早归档」，措辞恒为「连续 2 天」 | 本单引入 `ESCALATE_BASELINE_GAP` 分流措辞（D3） |
| `docs §8.3` 表 vs 实现 | 表里写「今日 >0 且不低于昨日」，实现是「最近更早归档」 | 文档同步为「昨日 → 连续 2 天 / 非昨日 → 较上次归档未下降」 |

**损坏输入拒绝规则**

- 归档 JSON 无法解析或不含 `tokens` 对象 → 不作为比较基准（跳过，继续找更早的有效归档）。
- 归档 `failed:true` → 不作为基准（失败归档没有可比计数）。
- 本地 marker 内容非纯数字 → 视为不存在（回退搜索，不猜单号）。
- `--date` 非 `YYYY-MM-DD` → `exit 2`（既有行为，不变）。

**Tool Preflight（离线夹具能力与盲区）**

| 工具 | 覆盖 | 盲区 | 替代验证 |
|---|---|---|---|
| fake `gh`（本单新增 `FAKE_GH_LIST_RC`） | list / create / comment 三条路径 + 失败码 | 真实 GitHub 搜索索引最终一致性、方括号标题匹配 | 本单 H3 本地 marker 即为该盲区的生产级消除手段；真机行为由既有 `[Ops-Followup of #2625]` 首夜验证覆盖 |
| fake reconcile（`zero`/`diff3`/`fail`） | token 全 0 / 有差异 / 失败三档 | 真实库不可达与 SQL 语义（#2623 owner） | 不在本单范围（D9 继承） |
| `date -d`（`TZ=UTC`） | 基准间隔天数 | 宿主 `date` 非 GNU 的极端发行版 | 取不到 epoch → `ESCALATE_BASELINE_DAYS=unknown` 且 `GAP=1`（保守措辞，不冒充连续） |

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|----|--------|--------|----------|----------------|
| D1 | 基线分支 = `origin/issue-2625`，工作树 `--base origin/issue-2625`；基线缺运行器 → HOLD，禁止在 develop 自建同名脚本 | Architect（本计划锁定） | 计划接受即锁定 | 缺失则 `/work` HOLD，不得绕过 |
| D2 | H1 去重键 = **当日日期**（`.<date>.escalated`）；基线信息写入 marker 供审计但不参与键 | Architect（锁定） | 同日重复运行不得重复打扰 | 若需「换基准可再升级」，另立 Issue（本单不做） |
| D3 | H2 采用**方案 (a)**：基准非昨日仍升级，但措辞改「较上次归档未下降」与 `ESCALATE_BASELINE_GAP=1`；**不采用**方案 (b)（丢弃升级会掩盖停机期间持续未下降） | Architect（锁定） | 计划接受即锁定；#2625 D5 的「连续 2 天」仅在 `GAP=0` 时成立 | 未锁定 → 阻塞实现（不得两种措辞混用） |
| D4 | 失败（`failed:true`）或损坏归档不作基准；无有效基准 → `ESCALATE_BASELINE=none` 且不升级 | Architect（锁定） | 计划接受即锁定 | 阻塞实现 |
| D5 | marker 只在成功路径写入（create 成功或 comment 成功）；失败不写以保证下轮重试；写失败只降级并显式 `MARKER_WRITE_FAILED=<kind>` | Architect（锁定） | 计划接受即锁定 | 阻塞实现 |
| D6 | marker 落在 `${XDG_STATE_HOME}/tuneloop/reconcile/`（不写仓库）；`--dry-run` 一律零写入（含 marker） | Architect（锁定） | 沿 #2625 D6 | 阻塞实现 |
| D7 | 允许修改 `test_reconcile_user_cache_cron_2625.sh` 的 r3「升级评论失败」场景日期（`2026-11-28` → `2026-12-31`）并加注释；**禁止**改期望值或删断言；R6 以断言计数对齐与非日期行 diff 为 0 守门 | Architect（锁定） | 计划接受即锁定 | 若需放宽既有断言 → 回 Architect |
| D8 | 语义权威：`target_semantics` = 本计划 D2/D3/D5（双措辞与当日 marker）；`historical_baseline` = #2625 实现的「最近更早归档 + 无条件连续 2 天」；矩阵与文档不得混用两种口径 | Architect（锁定） | 计划接受即锁定 | 阻塞审计 PASS |
| D9 | 不改判定口径与 token 契约（#2623 owner）、不做修复动作（#2624）、不改 timer 安装器 | 继承 #2625 D1/D2/D9 | 计划接受即锁定 | 越界 → 回 Architect |

## Out of Scope

- 差异判定逻辑、token 契约、`scripts/reconcile-user-cache.sh`（owner = #2623；本单只解析其输出）。
- 修复动作与 `--apply`（#2624）；自动化永不调用 `--apply`。
- timer 安装器 `scripts/install-user-cache-reconcile-timer.sh` 与调度时间（#2625 已验收，本单不改）。
- 额外通知渠道（邮件 / IM / 看板）。
- 真实宿主安装与首夜验证：沿既有 `[Ops-Followup of #2625]`（运维执行，本单不重复建单、不写成已完成）。
- `ROLE_UNVERIFIED` / `CONTEXT_AMBIGUOUS` / `MEMBER_ONLY` 等「只计数」类是否并入告警阈值（需先改 #2623 口径，另立）。

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|-------|------|----------------------|--------------------|--------|------|--------|
| R1 | `bash scripts/tests/test_reconcile_user_cache_cron_2631.sh h1 > docs/evidence/2631/R1-escalation-dedup.txt 2>&1` | `docs/evidence/2631/R1-escalation-dedup.txt` | 产物逐行含以下 KEY=VALUE 且值相等；`RESULT: PASS` 恰 1 行 | `H1_RUN1_ESCALATED=1`、`H1_RUN1_ESCALATE_COMMENTS=1`、`H1_RUN2_ESCALATE_SKIPPED=already-escalated`、`H1_RUN2_ESCALATE_COMMENTS=0`、`H1_RUN2_GH_CALLS=0`、`H1_COMMENT_CALLS_TOTAL=1`、`H1_COMMENT_BODIES=1`、`TEST_EXIT=0` | `H1_COMMENT_CALLS_TOTAL` 出现 2 或 `H1_COMMENT_BODIES` 出现 2（同日重复升级评论）；`H1_RUN2_ESCALATE_COMMENTS=1` 即 FAIL | dev |
| R2 | `bash scripts/tests/test_reconcile_user_cache_cron_2631.sh h2 > docs/evidence/2631/R2-baseline-gap-wording.txt 2>&1` | `docs/evidence/2631/R2-baseline-gap-wording.txt` | 产物逐行含以下 KEY=VALUE 且值相等；`RESULT: PASS` 恰 1 行 | `H2_GAP=1`、`H2_BASELINE=2027-01-10.json`、`H2_DAYS=3`、`H2_GAP_BODY_HAS_GAP_TITLE=1`、`H2_GAP_BODY_CONTIGUOUS_CLAIMS=0`、`H2_CONTIGUOUS_GAP=0`、`H2_CONTIGUOUS_BODY_HAS_CONTIGUOUS_TITLE=1`、`H2_CONTIGUOUS_BODY_GAP_CLAIMS=0`、`TEST_EXIT=0` | 缺口场景评论正文出现「连续 2 天」（`H2_GAP_BODY_CONTIGUOUS_CLAIMS` 大于 0）；或连续场景改用「较上次归档」措辞（`H2_CONTIGUOUS_BODY_GAP_CLAIMS` 大于 0） | dev |
| R3 | `bash scripts/tests/test_reconcile_user_cache_cron_2631.sh h2b > docs/evidence/2631/R3-failed-archive-baseline.txt 2>&1` | `docs/evidence/2631/R3-failed-archive-baseline.txt` | 产物逐行含以下 KEY=VALUE 且值相等；`RESULT: PASS` 恰 1 行 | `H2B_BASELINE=2027-01-15.json`、`H2B_DAYS=2`、`H2B_GAP=1`、`H2B_FAILED_BASELINE_USED=0`、`H2B_NO_BASELINE=1`、`H2B_NO_BASELINE_ESCALATED=0`、`H2B_NO_BASELINE_ISSUE_CREATED=1`、`H2B_NO_BASELINE_EXIT=0`、`TEST_EXIT=0` | 基准取到失败归档（`H2B_BASELINE=2027-01-16.json` 或 `H2B_FAILED_BASELINE_USED=1`）；无基准仍升级（`H2B_NO_BASELINE_ESCALATED=1`） | dev |
| R4 | `bash scripts/tests/test_reconcile_user_cache_cron_2631.sh h3 > docs/evidence/2631/R4-local-issue-marker.txt 2>&1` | `docs/evidence/2631/R4-local-issue-marker.txt` | 产物逐行含以下 KEY=VALUE 且值相等；`RESULT: PASS` 恰 1 行 | `H3_SEARCH_ISSUE=811`、`H3_SEARCH_CREATES=0`、`H3_SEARCH_MARKER_EXISTS=1`、`H3_MARKER_ISSUE=811`、`H3_MARKER_SOURCE=local-marker`、`H3_MARKER_GH_CALLS=0`、`H3_MARKER_LIST_CALLS=0`、`H3_DEGRADE_ISSUE=812`、`H3_DEGRADE_LIST_FAILED=1`、`H3_DEGRADE_CREATES=1`、`H3_DEGRADE_RERUN_ISSUE=812`、`H3_FAIL_ISSUE=none`、`H3_FAIL_MARKER_EXISTS=0`、`H3_FAIL_RETRY_CREATES=2`、`TEST_EXIT=0` | marker 命中仍调用 `gh issue list`（`H3_MARKER_LIST_CALLS` 大于 0）或 `H3_MARKER_GH_CALLS` 大于 0；create 失败后仍写 marker（`H3_FAIL_MARKER_EXISTS=1`）或下轮不再建单（`H3_FAIL_RETRY_CREATES` 小于 2） | dev |
| R5 | `bash scripts/tests/test_reconcile_user_cache_cron_2631.sh guard > docs/evidence/2631/R5-dry-run-zero-markers.txt 2>&1` | `docs/evidence/2631/R5-dry-run-zero-markers.txt` | 产物逐行含以下 KEY=VALUE 且值相等；`RESULT: PASS` 恰 1 行 | `GUARD_DRY_RUN_FILES_WRITTEN=0`、`GUARD_DRY_RUN_MARKERS=0`、`GUARD_DRY_RUN_GH_CALLS=0`、`GUARD_ZERO_MARKERS=0`、`GUARD_ZERO_ARCHIVES=2`、`TEST_EXIT=0` | dry-run 产生 marker（`GUARD_DRY_RUN_MARKERS` 大于 0）或出现 gh 调用（`GUARD_DRY_RUN_GH_CALLS` 大于 0）；零差异实跑写 marker（`GUARD_ZERO_MARKERS` 大于 0） | dev |
| R6 | `bash scripts/tests/test_reconcile_user_cache_cron_2631.sh regress > docs/evidence/2631/R6-regression-2625.txt 2>&1` | `docs/evidence/2631/R6-regression-2625.txt` | 产物逐行含以下 KEY=VALUE 且值相等；`RESULT: PASS` 恰 1 行 | `R2625_R1_TEST_EXIT=0`、`R2625_R2_TEST_EXIT=0`、`R2625_R3_TEST_EXIT=0`、`HARNESS_ASSERT_PARITY=1`、`HARNESS_DIFF_NON_DATE_LINES=0`、`TEST_EXIT=0` | 任一 r1/r2/r3 非 0；断言调用数低于 `origin/issue-2625` 基线（`HARNESS_ASSERT_PARITY=0`）；夹具 diff 出现非日期行（`HARNESS_DIFF_NON_DATE_LINES` 大于 0，即放宽或改写既有断言） | dev |
| R7 | `bash scripts/tests/test_reconcile_user_cache_cron_2631.sh docs > docs/evidence/2631/R7-doc-sync.txt 2>&1` | `docs/evidence/2631/R7-doc-sync.txt` | 产物逐行含以下 KEY=VALUE 且值相等；`RESULT: PASS` 恰 1 行 | `DOC_GAP_WORDING=1`、`DOC_CONTIGUOUS_WORDING=1`、`DOC_GAP_TOKEN=1`、`DOC_ESCALATE_MARKER=1`、`DOC_ISSUE_MARKER=1`、`DOC_ALREADY_ESCALATED=1`、`DOC_OLD_UNCONDITIONAL_ROW_GONE=1`、`TEST_EXIT=0` | 文档仍保留旧的「同类差异今日 >0 且不低于昨日」无条件表述（`DOC_OLD_UNCONDITIONAL_ROW_GONE=0`）；或缺少 `ESCALATE_BASELINE_GAP` 与 marker 说明 | dev |

## Artifact Provenance

| artifact | first_line |
|----------|------------|
| docs/evidence/2631/R1-escalation-dedup.txt | # build=1.0.<fix> # commit=<fix> # dirty=false |
| docs/evidence/2631/R2-baseline-gap-wording.txt | # build=1.0.<fix> # commit=<fix> # dirty=false |
| docs/evidence/2631/R3-failed-archive-baseline.txt | # build=1.0.<fix> # commit=<fix> # dirty=false |
| docs/evidence/2631/R4-local-issue-marker.txt | # build=1.0.<fix> # commit=<fix> # dirty=false |
| docs/evidence/2631/R5-dry-run-zero-markers.txt | # build=1.0.<fix> # commit=<fix> # dirty=false |
| docs/evidence/2631/R6-regression-2625.txt | # build=1.0.<fix> # commit=<fix> # dirty=false |
| docs/evidence/2631/R7-doc-sync.txt | # build=1.0.<fix> # commit=<fix> # dirty=false |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

- 首行模板由新夹具的 provenance 块统一打印（`# build=1.0.<fix short> # commit=<fix sha> # dirty=<bool>`）；7 个子行共用同一模板，`<fix>` 必须是本单最新 fix commit。
- `evidence.json`：`schema_version=1`、`issue=2631`、`dirty=false`、7 行 `pass=true`，`command` 与 `assert` 与 `expected` 与 `negative` 与本合同逐字一致。

### Files（变更文件清单）

- `scripts/reconcile-user-cache-cron.sh`
- `scripts/tests/test_reconcile_user_cache_cron_2631.sh`
- `scripts/tests/test_reconcile_user_cache_cron_2625.sh`
- `docs/ops/user-cache-reconcile.md`
- `docs/evidence/2631/R1-escalation-dedup.txt`
- `docs/evidence/2631/R2-baseline-gap-wording.txt`
- `docs/evidence/2631/R3-failed-archive-baseline.txt`
- `docs/evidence/2631/R4-local-issue-marker.txt`
- `docs/evidence/2631/R5-dry-run-zero-markers.txt`
- `docs/evidence/2631/R6-regression-2625.txt`
- `docs/evidence/2631/R7-doc-sync.txt`
- `docs/evidence/2631/evidence.json`
- `docs/evidence/2631/plan.md`
- `docs/evidence/2631/pre-ready-2631.json`

### Workload

- Size: S
- Basis: 4 个实现/文档文件（运行器 ~90 净行 + 新夹具 ~230 行 + 既有夹具 1 行日期隔离 + 文档 2 小节）+ 7 条离线证据行；无 migration、无外部依赖、无真实库/宿主操作；拆分会让同一脚本的 H1/H2/H3 三处 marker/基准逻辑分散到两个夹具，反而增加重复与漂移风险。
- Confidence: high

## 风险与回退

| 风险 | 影响 | 处理 |
|---|---|---|
| marker 与真实评论不一致（人工删评论） | 当日不再升级 | 文档写明「marker 为准；如需重新升级删除 `.<date>.escalated`」；不自动补偿（避免重复打扰） |
| 宿主 `date` 不支持 `-d` | 天数 unknown | `GAP=1` 保守措辞，不影响告警送达 |
| `origin/issue-2625` 未合入 develop 期间堆叠分支漂移 | 合入冲突 | 合入顺序 2625 → develop → 2631；`/work` 开始 `git fetch` 并 rebase 到最新 `origin/issue-2625` |
| 修改 #2625 夹具日期被视为放宽验收 | 审计 REJECT | D7 限定范围 + R6 断言计数对齐 + 非日期行 diff 断言 |

*Model: deepseek/deepseek-flash*

