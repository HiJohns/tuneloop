## Accepted Plan — #2591 删除 `api.md` 中 §5.8.1–5.8.3 的陈旧重复副本（单行折叠块）

> **本计划是对 Issue 正文期望的修正版**（正文写「按 `\n` 展开」）。诊断修正与证据见本单上一条评论
> `诊断修正（Architect，接受本单前）`（comment 7204642881）。要点：折叠块**不是**被误折叠的现行内容，而是
> 现存 `#### 5.8.1 导入乐器信息` / `5.8.2 导出乐器列表` / `5.8.3 下载导入模板`（行 1679–1835，已由 #2578 更新）的
> **旧版拷贝**（编号仍是 5.6.x、缺 `sn` / `stock_status` / `category_id` / `exclude_status` / `base_daily_rate` / `archived|lost`）。
> **展开会把旧口径重新写回文档**（回退性错误）；正确修法是**删除该副本**，但保留其中唯一合法的一行
> —— 折叠行首段正是 §5.8.3「安全特性」列表的第 3 条 `- 严格数值验证: 价格字段必须为有效数字`（前两条在 1831–1832 已正常渲染）。

### 变更点

#### 1. `docs/spec/api/api.md`（唯一业务文件改动）

把行 1833（`develop 0d1c0aa9d` 处 3060 字符、含字面 `\n` 的单行）**替换为**一行：

```
- 严格数值验证: 价格字段必须为有效数字
```

替换后上下文（应为）：

```
1831| - Excel公式注入防护: 自动转义以`=`, `+`, `-`, `@`开头的值
1832| - 输入长度限制: 超过1000字符自动截断
1833| - 严格数值验证: 价格字段必须为有效数字
1834|
1835| ---
1836|
1837| ### 5.9 乐器照片存储 (Deprecated)
```

- 净变化：`-1` / `+1` 行（总行数、围栏数、标题数**全部不变**）。
- **禁止**把折叠块展开成 `#### 5.6.1/5.6.2/5.6.3` 小节（与现存 `### 5.6.1 阶梯定价方案 V2` / `### 5.6.2 批量设价` 编号冲突，且内容已被取代）。
- **禁止**改动 §5.8.1–5.8.3 现有正文（行 1679–1835 内其余行必须逐行不变，见 R2 `SEC_58_BLOCK_IDENTICAL`）。

#### 2. 新增证据 harness `scripts/verify-2591.sh r1..r4`

沿用 #2590/#2581 形态：每个子命令产出一个 `docs/evidence/2591/*.txt`，首行 provenance
（`# build=1.0.<fix 短码> # commit=<40 位 sha> # dirty=false`）。实现要点：

- 用 `python3 - <<'PY'` 内联脚本做结构判定，token 一律 `KEY=VALUE` 单行输出（机器可 grep 断言）；
- 「折叠行」定位：在 `git show <fix>^:docs/spec/api/api.md` 中找同时含字面 `\n` 与 `#### 5.6.1` 的行（基线定位，不硬编码行号）；
- `r3` 从 `git show <fix>^:…` 现场采集红样，禁止手写；
- 纯文档改动，无重命令，**不需要** `heavy-run.sh` 取锁，也不新增 `go build` 合同行。

#### 3. 陈旧副本逐行映射（rubric 产物，R4）

删除块中「全文别处不存在」的 **12** 行，必须在 R4 产物里逐行列出并标注其**现行取代行**：

| 陈旧行类别 | 行数 | 现行取代位置 |
|---|---|---|
| `#### 5.6.1/5.6.2/5.6.3`（旧编号小节标题） | 3 | `#### 5.8.1/5.8.2/5.8.3` 同名小节（行 1679/1750/1807） |
| `curl … \\  -H …`（双反斜杠变体） | 3 | 同名 `curl` 行（单 `\`，§5.8.1/5.8.2/5.8.3 内） |
| `| daily_rate | … 数字格式，如: 50 |` 等旧表格行 | 3 | §5.8.1「Excel模板字段」现行 3 行（含「旧模型」注记、`base_daily_rate`、`archived/lost`） |
| `- \`category\`: 分类筛选 (可选)` 等旧查询参数 | 3 | §5.8.2「查询参数」现行条目（含 `stock_status`/`sn`/`category_id`/`exclude_status`） |

判定 rubric：**每一行都必须是上述某现行行的子集/旧版**；出现无法映射的行（`STALE_UNMAPPED>0`）→ FAIL（说明误删了独有内容）。

### Files（变更文件清单）

- `docs/spec/api/api.md`
- `scripts/verify-2591.sh`
- `docs/evidence/2591/plan.md`
- `docs/evidence/2591/R1-structure-restored.txt`
- `docs/evidence/2591/R2-no-side-effect.txt`
- `docs/evidence/2591/R3-red-baseline.txt`
- `docs/evidence/2591/R4-stale-mapping.txt`
- `docs/evidence/2591/evidence.json`
- `docs/evidence/2591/pre-ready-2591.json`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| 陈旧折叠副本被移除，§5.8.3 安全特性恢复 3 条，目标行变短 | `docs/spec/api/api.md` | `bash scripts/verify-2591.sh r1` | 修复后工作树 | CLI（文档静态结构） | `TARGET_LINE_LEN=21`、`LITERAL_BSLASH_N_LINES=0`、`SECURITY_BULLETS_58_3=3`、`HEADINGS_58X=3`、`STALE_58_TITLES=0` | R1 |
| 除目标行外零副作用：行数/围栏/标题不变，前后文与 §5.8 正文逐行不变，diff 仅 -1/+1 | `docs/spec/api/api.md` | `bash scripts/verify-2591.sh r2` | 修复后工作树 vs `git show <fix>^` | CLI（diff 归一化） | `LINE_COUNT_UNCHANGED=1`、`FENCE_COUNT_UNCHANGED=1`、`HEADING_COUNT_UNCHANGED=1`、`PREFIX_IDENTICAL=1`、`SUFFIX_IDENTICAL=1`、`SEC_58_BLOCK_IDENTICAL=1`、`DIFF_ADDED_LINES=1`、`DIFF_DELETED_LINES=1` | R2 |
| 红样：修复前该行确为 3060 字符折叠块，且 §5.8.3 第 3 条不可见 | `docs/spec/api/api.md`（`git show <fix>^:…`） | `bash scripts/verify-2591.sh r3` | 修改前 blob（现场采集） | CLI | `RED_LITERAL_BSLASH_N_LINES=1`、`RED_TARGET_LINE_LEN=3060`、`RED_SECURITY_BULLETS_58_3=2`、`RED_STALE_TITLE_MAP=3` | R3 |
| 删除块 12 行全部可映射到现行 §5.8 行（无独有内容丢失） | `docs/spec/api/api.md`（fix^ 折叠块展开 vs 修复后全文） | `bash scripts/verify-2591.sh r4` | 修复后工作树 + fix^ blob | CLI（集合差 + rubric 表） | `STALE_ONLY_LINES=12`、`STALE_TITLE_MAP=3`、`STALE_UNMAPPED=0`、`RETAINED_BULLET_PRESENT=1` | R4 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R1 | static | 目标行长度/字面 `\n` 计数/§5.8.3 bullet 数/旧编号小节数的结构断言 |
| R2 | static | `fix^` vs 修复后的 diff 归一化：行数·围栏·标题不变、前后文逐行相等、`git diff --numstat` = 1/1 |
| R3 | static | 红样：`fix^` blob 上同算法判定（3060 字符、字面 `\n`、§5.8.3 只有 2 条可见 bullet） |
| R4 | static | 删除块「别处不存在行」集合差 + 逐行 rubric 映射表（12/12 可映射） |

> 纯文档内容收敛：无 UI 交互、无运行时行为，矩阵不设 `integration`/`ui` 行；
> 「渲染是否正常」用**结构代理断言**（目标行变短 + 无字面 `\n` + 3 条 bullet + 无旧编号小节）表达，
> 环境无 Markdown 渲染器（`markdown`/`commonmark`/`pandoc` 均不可用），**不伪造截图**。

### Input Truth & Tool Preflight

- **Source of truth**：`docs/spec/api/api.md`（`develop 0d1c0aa9d`）；判定依据是同文件的**现存** §5.8.1–5.8.3
  （行 1679–1835）——它由 #2578 更新到最新口径，是本文档对该组接口的唯一权威描述。
- **改前核对（dev 必须先跑，结果贴进 Work Summary）**：
  - `awk 'NR>=1679 && NR<=1836' docs/spec/api/api.md` → 确认 §5.8.1–5.8.3 已存在且内容为新口径；
  - `awk 'NR==1833{print length($0)}' docs/spec/api/api.md` → `3060`，且该行含字面 `\n` 与 `#### 5.6.1`；
  - `grep -c '^#### 5\.6\.[123] ' docs/spec/api/api.md` → `0`（旧编号小节从未行首出现，故渲染中完全不可见）；
  - `grep -c '^- ' docs/spec/api/api.md` 等无需比对；若 1833 行已不存在（他人已修）→ 本单转为核验 + 记录，**不得**改出等价但形态不同的结果。
- **损坏/异常输入拒绝**：`git show <fix>^:…` 失败 → 契约命令 exit 非 0（红样不可得，视为失败）；折叠行定位不到（0 行或多行命中） → FAIL，不猜。
- **工具盲区**：
  1. 结构/逐行断言**不能**证明「读者是否觉得可读」，只能证明「陈旧副本不在了、合法 bullet 在、无字面 `\n`」；
  2. `python3` 需可用（本机已用于分析；若缺失则 `verify-2591.sh` 显式 exit 2，不得静默跳过）；
  3. 折叠块内**没有**行首围栏，所以本单不改变围栏结构——「全文件围栏配对」属 #2590，本单不重复其断言（保持两单可独立合并）。
- **批次/单 Issue 边界**：#2591 无 `Parent`，`/work` 按 #2651 口径执行 `pre-ready-check.sh` 全量门（本单无代码改动）。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 删除陈旧副本而非展开 | **锁定**。展开会造出与 `### 5.6.1 阶梯定价方案 V2` / `### 5.6.2 批量设价` 编号冲突的小节，并把 #2578 之前的旧字段口径写回文档（回退性错误）；证据见修正评论 |
| D2 | 保留 `- 严格数值验证: 价格字段必须为有效数字` 一行 | **锁定**。它是 §5.8.3「安全特性」第 3 条（前两条在 1831–1832），删除会丢真实内容 |
| D3 | 只改这一行，不动 §5.8.1–5.8.3 正文 | **锁定**。`SEC_58_BLOCK_IDENTICAL=1` 与 `DIFF_ADDED_LINES=1 / DIFF_DELETED_LINES=1` 强制 |
| D4 | 不引入「围栏配对」断言（属 #2590） | **锁定**。避免同文件两单互相卡依赖；本单验收只看行内容 |
| D5 | 证据形态 = 结构断言 + 逐行 rubric 映射，不用截图/渲染器 | **锁定**（环境无渲染器；不伪造证据） |
| D6 | 与新口径的差异**不**在本单补齐（如把 `sn` 等写回旧块） | **锁定**。旧块已废弃；接口口径以现存 §5.8.x 为准，本单只做减法 |

### Out of Scope

- 围栏配对 / 标题落在代码块内的全文件结构问题 → **#2590**（本单已刻意不共享断言，避免依赖）。
- §5.8.x 现行内容本身的准确性（字段/参数/错误码是否符合后端实现）→ 不在本单；如发现口径不符另立 Issue。
- `docs/spec/api/api.md` 其他超长行（如行 30 的 758 字符）→ 属排版优化，不在本单（本单只要求**目标行**变短）。
- 其他文档目录的 Markdown 结构 → #2590 的 `--root docs` 扫描覆盖。
- 代码改动（`backend/`、`frontend-*/`）→ 无。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 修复后 `docs/spec/api/api.md` | `bash scripts/verify-2591.sh r1` | `docs/evidence/2591/R1-structure-restored.txt` | `TARGET_LINE_LEN=21`、`LITERAL_BSLASH_N_LINES=0`、`SECURITY_BULLETS_58_3=3`、`HEADINGS_58X=3`、`STALE_58_TITLES=0` | 目标行仍 >100 字符、仍含字面 `\n`、§5.8.3 bullet 仍为 2 → FAIL |
| R2 | `git show <fix>^:docs/spec/api/api.md` vs 修复后工作树 | `bash scripts/verify-2591.sh r2` | `docs/evidence/2591/R2-no-side-effect.txt` | `LINE_COUNT_UNCHANGED=1`、`FENCE_COUNT_UNCHANGED=1`、`HEADING_COUNT_UNCHANGED=1`、`PREFIX_IDENTICAL=1`、`SUFFIX_IDENTICAL=1`、`SEC_58_BLOCK_IDENTICAL=1`、`DIFF_ADDED_LINES=1`、`DIFF_DELETED_LINES=1` | 触及 §5.8 正文、行数/围栏/标题变化、diff 非 1/1 → FAIL |
| R3 | `git show <fix>^:docs/spec/api/api.md` | `bash scripts/verify-2591.sh r3` | `docs/evidence/2591/R3-red-baseline.txt` | `RED_LITERAL_BSLASH_N_LINES=1`、`RED_TARGET_LINE_LEN=3060`、`RED_SECURITY_BULLETS_58_3=2`、`RED_STALE_TITLE_MAP=3` | 修复前被判「已正常」（计数/长度与 3060 不符）→ FAIL（红绿失效） |
| R4 | fix^ 折叠块展开 vs 修复后全文 | `bash scripts/verify-2591.sh r4` | `docs/evidence/2591/R4-stale-mapping.txt` | `STALE_ONLY_LINES=12`、`STALE_TITLE_MAP=3`、`STALE_UNMAPPED=0`、`RETAINED_BULLET_PRESENT=1` | 出现无法映射的独有内容（`STALE_UNMAPPED>0`）、保留 bullet 丢失 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2591.sh r1` | `docs/evidence/2591/R1-structure-restored.txt` | exit 0 且含 `TARGET_LINE_LEN=21` `LITERAL_BSLASH_N_LINES=0` `SECURITY_BULLETS_58_3=3` `HEADINGS_58X=3` `STALE_58_TITLES=0` | PASS（5 项 token 全为期望值） | 目标行仍含字面 `\n`、或 §5.8.3 只剩 2 条 bullet、或出现旧编号 5.6.x 小节 → FAIL | dev |
| R2 | `bash scripts/verify-2591.sh r2` | `docs/evidence/2591/R2-no-side-effect.txt` | exit 0 且含 `LINE_COUNT_UNCHANGED=1` `FENCE_COUNT_UNCHANGED=1` `HEADING_COUNT_UNCHANGED=1` `PREFIX_IDENTICAL=1` `SUFFIX_IDENTICAL=1` `SEC_58_BLOCK_IDENTICAL=1` `DIFF_ADDED_LINES=1` `DIFF_DELETED_LINES=1` | PASS（8 项 token 全为期望值） | 改动溢出到 §5.8 正文或其他行、diff 非 1/1 → FAIL | dev |
| R3 | `bash scripts/verify-2591.sh r3` | `docs/evidence/2591/R3-red-baseline.txt` | exit 0 且含 `RED_LITERAL_BSLASH_N_LINES=1` `RED_TARGET_LINE_LEN=3060` `RED_SECURITY_BULLETS_58_3=2` `RED_STALE_TITLE_MAP=3` | PASS（4 项 token 全为期望值，即修复前确为红） | 修复前被判定为正常（`RED_TARGET_LINE_LEN` 非 3060）→ FAIL | dev |
| R4 | `bash scripts/verify-2591.sh r4` | `docs/evidence/2591/R4-stale-mapping.txt` | exit 0 且含 `STALE_ONLY_LINES=12` `STALE_TITLE_MAP=3` `STALE_UNMAPPED=0` `RETAINED_BULLET_PRESENT=1` | PASS（4 项 token 全为期望值） | 存在无法映射到现行 §5.8 行的独有内容、或保留的 bullet 未被写入 → FAIL | dev |

### Artifact Provenance

- 生成器：`scripts/verify-2591.sh`（子命令 `r1`–`r4`）；每个产物首行
  `# build=1.0.<fix 短码> # commit=<40 位 sha> # dirty=false`，另起一行 `GENERATED_AT_HEAD=<40 位 sha>`。
- **证据必须在 fix commit 之后生成**；`r3`/`r4` 的基线通过 `git show <fix>^:docs/spec/api/api.md` 现场采集。
- 逐条清单：
  - `docs/evidence/2591/R1-structure-restored.txt`
  - `docs/evidence/2591/R2-no-side-effect.txt`
  - `docs/evidence/2591/R3-red-baseline.txt`
  - `docs/evidence/2591/R4-stale-mapping.txt`
- `docs/evidence/2591/evidence.json`：`schema_version=1`、`issue=2591`、`dirty=false`、`commit`=fix commit 全 sha、
  `plan_sha256`=本计划对应 `plan.md`；`rows` 为 R1–R4 各一条且 `pass=true`。
  evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2591 --summary-file tmp/pre-ready-2591-summary.md --changed-files tmp/pre-ready-2591-changed.txt --evidence-file docs/evidence/2591/evidence.json --report docs/evidence/2591/pre-ready-2591.json`。

### Dependencies

无。

### Workload

- Size: S
- Basis: 1 个文档文件（-1/+1 行，等价于删除 145 行陈旧副本并保留 1 行合法 bullet）+ 1 个证据 harness（4 子命令）+ 4 个静态证据产物；无代码、无 migration、无前端
- Confidence: high

*Model: DeepSeek V4.1 Flash 1017*

