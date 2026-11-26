## Accepted Plan — #2590 `docs/spec/api/api.md` 围栏配对修复（3 个断点）+ 文档围栏结构守卫｜Rev.2

> **诊断修正（本计划与 Issue 正文的关键差异，先读这段）**
> Issue 正文把缺陷定位为「§12.4 之前有一行孤立围栏」，并据此要求「删除该行 → 围栏计数变偶数」。
> 本单用**结构扫描**（逐行奇偶 + 分节奇偶 + 标题位置）复核后确认：**孤立围栏是症状，不是根因**。
> 真正的断点有 **3 处**，且最早一处在 §8.7（第 3840 行），**远早于 §12.4**：
>
> | # | 位置（develop `0d1c0aa9d`） | 断点形态 | 前因 | 单独删 12.4 那行能否修好 |
> |---|---|---|---|---|
> | A | `### 8.7 收藏列表`，行 3840–3846 | **残缺重复片段**：`**接口**`/`**响应**`/```json 起的 JSON 只写到 `"data": {` 就断了，**没有闭合围栏** | `eb805171cf`（2026-04-09）插入的截断副本，完整块在其下方 3848–3866 | ❌ 不能。A 使全文件奇偶从 3843 起**整体反转**，§12.4 之后的「孤立围栏」只是反转后的表象 |
> | B | `### 12.2 租户管理`，行 6667–6679 | ```json 起的租户列表响应块**没有闭合围栏**，直接接 `### 12.3 商户管理`（6681） | 漏写闭合行 | ❌ 不能 |
> | C | `##### 12.3.5.5 转让所有权`，行 6952 | **孤立闭合围栏**（无对应开启行），紧跟 6951 的引用块、紧邻 6954 `### 12.4 审计日志` | 多余闭合行（即 Issue 正文指认的那行） | ⚠️ 只能让总数为偶数，§12.4 仍会被 B（及 A）反转到代码块内 |
>
> **三处齐修后的实测（已在分析阶段用同算法模拟验证）**：总围栏 597（奇）→ **596（偶）**；
> 「标题落在代码块内」的事件数 **135 → 0**；文件结束态 `inside_fence=False`。
> 因此本单验收**不接受「只让计数变偶数」**，必须满足「**全文件零标题落在围栏内**」这一结构断言（见 R2）。

### 变更点

#### 1. `docs/spec/api/api.md` — 三处断点（按内容锚定，禁止按行号盲改）

1. **站点 A**（§8.7 收藏列表）：删除**截断的重复片段**共 7 行——第二个 `**接口**: \`GET /api/user/favorites\``、空行、
   第二个 `**响应**:`、```json、`{`、`  "code": 20000,`、`  "data": {`（即行 3840–3846）。
   保留其下方**完整**的块（3848–3866：接口 → 响应 → 完整 JSON → 闭合围栏）。
   选择「删除残缺副本」而非「补一个闭合围栏」：补围栏会留下一个**渲染成残缺 JSON 的代码块**，且与下方完整块内容重叠。
2. **站点 B**（§12.2 租户管理）：在租户列表响应 JSON 的收尾 `}` 之后补一行闭合围栏 ```` ``` ````（行 6679 与 6681 之间）。
3. **站点 C**（§12.3.5.5 转让所有权）：删除孤立闭合围栏（行 6952，位于 6951 引用块之后、6954 `### 12.4 审计日志` 之前）。

**不改任何正文/表格/代码内容**：diff 只包含「删除 1 个围栏行 + 插入 1 个围栏行 + 删除 7 行重复片段」（见 R4 内容守恒断言）。

#### 2. 新增结构守卫 `scripts/check-doc-fences.sh`（可复用，不是一次性脚本）

CLI（约定即契约，`verify-2590.sh` 与后续 Issue 复用）：

```
check-doc-fences.sh [--root DIR]... [--file PATH]... [--require-heading-outside TEXT]... [--self-test]
```

- 扫描规则：递归 `*.md`；围栏行 = 行首 **0–3 个空格** + 至少三个反引号；开启行允许 info string，闭合行不得带 info string。
- 对每个文件输出一行：
  `FILE=<path> FENCES=<n> EVEN=<0|1> HEADINGS_INSIDE=<n> EOF_INSIDE=<0|1>`
- 命中缺陷时输出：`VIOLATION file=<path> kind=<odd_fence|heading_inside|eof_inside|tagged_close> line=<n>`
- 标题断言输出：`HEADING_STATE=<text>=<outside|inside|missing>`
- 汇总行：`DOC_FILES_SCANNED=<n> ODD_FENCE_FILES=<n> HEADINGS_INSIDE_FILES=<n> EOF_INSIDE_FILES=<n> RESULT=<PASS|FAIL>`
- 退出码：全部 `EVEN=1` 且 `HEADINGS_INSIDE=0` 且 `EOF_INSIDE=0` 且所有标题断言为 `outside` → `0`，否则 `1`。
- `--self-test`：在 `tmp/fence-selftest/` 下**现场生成** 4 个 fixture（不外提交）：① 正常文件；② 奇数围栏；③ 偶数围栏但标题落在块内（上下两处互补缺陷）；④ EOF 仍在块内。输出
  `SELFTEST_CASES=4 SELFTEST_DETECTED=3 SELFTEST_FALSE_POSITIVE=0 RESULT=<PASS|FAIL>`，用于证明「守卫真的会红」。

#### 3. 新增证据 harness `scripts/verify-2590.sh r0..r4`

沿用 #2581/#2587 形态：每个子命令产出一个 `docs/evidence/2590/*.txt`，首行写 provenance
（`# build=1.0.<fix 短码> # commit=<40 位 sha> # dirty=<bool>`）。本单为**纯文档**改动，无重命令，**不需要** `heavy-run.sh` 取锁，
也不新增 `go build` 合同行。

#### 4. 同批扫描（Issue 期望第 3 条）

`--root docs` 全量扫：修复后 `ODD_FENCE_FILES=0`。分析阶段实测（`develop 0d1c0aa9d`）全仓 `docs/**/*.md` 中**只有**
`docs/spec/api/api.md` 一个奇数围栏文件（597），其余 `docs/spec/{ui/ui.md:82, database/database.md:14, permissions.md:6, features.md:2}` 均为偶数且零标题入块——
即本单的同批扫描预期是**不需要改其他文档**，但必须由 R1 的扫描产物证明，而不是口头声明。

### Files（变更文件清单）

- `docs/spec/api/api.md`
- `scripts/check-doc-fences.sh`
- `scripts/verify-2590.sh`
- `docs/evidence/2590/plan.md`
- `docs/evidence/2590/R0-checker-selftest.txt`
- `docs/evidence/2590/R1-fence-sweep.txt`
- `docs/evidence/2590/R2-api-structure.txt`
- `docs/evidence/2590/R3-red-prefix.txt`
- `docs/evidence/2590/R4-content-preservation.txt`
- `docs/evidence/2590/evidence.json`
- `docs/evidence/2590/pre-ready-2590.json`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| 结构守卫本身可用（能红能绿） | `scripts/check-doc-fences.sh` | `bash scripts/verify-2590.sh r0` | 现场生成 4 个 fixture（2 正常 / 1 奇数围栏 / 1 偶数但标题入块 / 1 EOF 未闭合） | CLI（Linux，bash + grep/awk） | `SELFTEST_CASES=4`、`SELFTEST_DETECTED=3`、`SELFTEST_FALSE_POSITIVE=0` | R0 |
| 同批扫描：`docs/` 无围栏失衡文件（**范围**：`docs/**/*.md`，排除 `docs/evidence/**`；标题判定 = 围栏内的行首 `#` 不计为标题） | `docs/spec/api/api.md` 及 docs 目录下全部 Markdown | `bash scripts/verify-2590.sh r1` | 修复后工作树 | CLI（文档静态结构） | `DOC_FILES_SCANNED>=9`、`ODD_FENCE_FILES=0`、`HEADINGS_INSIDE_FILES=0`、`EOF_INSIDE_FILES=0` | R1 |
| `api.md` 三处断点修复：全文件零标题入块，§12.4 回到正常渲染层 | `docs/spec/api/api.md` | `bash scripts/verify-2590.sh r2` | 修复后工作树 | GitHub / 任意 CommonMark 渲染器（结构代理断言） | `API_EVEN=1`、`API_HEADINGS_INSIDE=0`、`API_EOF_INSIDE=0`、`HEADING_STATE_12_4=outside` | R2 |
| 红样：修复前（fix^）确实失衡且 §12.4 在块内 | `docs/spec/api/api.md`（`git show <fix>^:…`） | `bash scripts/verify-2590.sh r3` | 修改前 blob（由 `git show` 采集，不手写） | CLI | `RED_CHECKER_EXIT=1`、`RED_EVEN=0`、`RED_HEADING_STATE_12_4=inside`、`RED_FENCES=597` | R3 |
| 内容守恒：只删围栏行与 7 行重复片段，正文/标题零丢失，§12.4 正文逐字不变 | `docs/spec/api/api.md`（fix^ vs 修复后） | `bash scripts/verify-2590.sh r4` | 修复后工作树 vs `git show <fix>^` | CLI（diff 归一化） | `HEADINGS_DELTA=0`、`TOTAL_DELTA=-7`、`NONFENCE_DELTA=-6`、`REMOVED_FENCE_LINES=1`、`REMOVED_LINES_ARE_DUPLICATES=1`、`SEC_12_4_UNCHANGED=1` | R4 |

### Evidence Types

| 行 ID | evidence_type | 说明 |
|---|---|---|
| R0 | static | 守卫自检：现场 fixture 上验证「奇数围栏 / 标题入块 / EOF 未闭合」都能被检出，正常文件零误报 |
| R1 | static | `docs/**/*.md` 全量结构扫描产物（逐文件 `EVEN/HEADINGS_INSIDE/EOF_INSIDE`） |
| R2 | static | 目标文件结构断言 + `### 12.4 审计日志` 的围栏位置断言（Issue 验收表指定的结构判定方式） |
| R3 | static | 红样：`git show <fix>^:docs/spec/api/api.md` 原始 blob 上的同算法判定（证明修复前为红） |
| R4 | static | 归一化 diff 守恒断言（**归一化对比**：`TOTAL_DELTA` = 原始行数差（含围栏行，应为 -7）、`NONFENCE_DELTA` = 仅非围栏行的行数差（应为 -6）；两者同时成立才算守恒 + §12.4 段落逐字相等） |

> 本单为**纯文档结构**改动：无 UI 交互行、无运行时行为行，故矩阵不设 `integration`/`ui` 行；
> 「§12.4 在渲染器里可见」按 Issue 验收表的口径使用**结构代理断言**（到该标题时围栏状态为 closed），
> 不伪造截图，也不以「字符串存在于文件」代替该断言。

### Input Truth & Tool Preflight

- **Source of truth**：文件自身（`docs/spec/api/api.md`，develop `0d1c0aa9d`）；缺陷清单由**结构扫描算法**给出，而非人工抽样：
  - 逐行奇偶：`grep -c '^```' docs/spec/api/api.md` → `597`（奇数，基线红态）；
  - 分节奇偶（定位断点）：按 `^#{2,5} ` 切节统计围栏数 → 奇数节恰为 `L3838 §8.7(3)`、`L6662 §12.2(1)`、`L6912 §12.3.5.5(5)`；
  - 标题入块：顺序扫描，`### 12.4 审计日志` 等 **135** 个标题落在「已开启未闭合」状态内；
  - 同批扫描：`find docs -name '*.md'` 全量 → 仅 `api.md` 奇数。
- **改前差异核对（dev 必须先跑再改，结果贴进 Work Summary）**：
  - `awk 'NR>=3840 && NR<=3846' docs/spec/api/api.md` → 确认残缺重复片段仍在（行号可能随其他 Issue 合并漂移，**以内容为准**）；
  - `awk 'NR>=6674 && NR<=6682' docs/spec/api/api.md` → 确认 `}` 与 `### 12.3 商户管理` 之间**无**闭合围栏；
  - `awk 'NR>=6949 && NR<=6956' docs/spec/api/api.md` → 确认 6951 引用块后有一行孤立 ```` ``` ````；
  - `bash scripts/check-doc-fences.sh --root docs` → 必须 **exit 1**（若已 exit 0，说明他人已合入等价修复：本单转为核验 + 记录，不得重复改）。
- **损坏/异常输入拒绝规则（守卫脚本必须显式失败，不得静默通过）**：`--file` 不存在 → exit 2；`--root` 不存在或无 `*.md` → exit 2；
  同一文件重复传入 → 去重后只报一次；围栏行带 info string 却出现在闭合位置（`tagged_close`）→ 记 VIOLATION。
- **工具盲区（显式声明，避免「守卫通过 = 文档完美」的错觉）**：
  1. 只识别反引号围栏，不识别 `~~~`（本仓 `docs/` 无 `~~~` 围栏，实测 0 处）；
  2. 只识别行首 0–3 空格的围栏（CommonMark 上限），**4 空格及以上缩进属缩进代码块**，不计入；
  3. **检测不到**「整段折叠为单行 + 字面 `\n`」这类结构损坏（`api.md:1833` 正是此形态）→ 已另立 **#2591**，不在本单；
  4. 检测不到表格管道符失衡、标题层级错乱等其他 Markdown 结构问题。
- **批次/单 Issue 边界**：#2590 无 `Parent`，按 #2651 口径由 `/work` 执行 `pre-ready-check.sh` 的全量门（本单无代码改动，
  P5 由 gate 适配器判定）；本计划不新增 `go build`/`npm build` 合同行。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 修 3 处而非只删 §12.4 前那行 | **锁定**。Issue 正文的单行结论经结构扫描证伪：仅删 6952 后 §12.4 仍被 A/B 反转到代码块内；验收以「零标题入块」为准 |
| D2 | 站点 A 选「删残缺重复片段」而非「补闭合围栏」 | **锁定**。补围栏会留下渲染成残缺 JSON 的块并与 3848+ 完整块重复；删除后内容零丢失（R4 证明被删 7 行均为下方完整块的重复前缀） |
| D3 | 站点 C 直接删除孤立闭合围栏 | **锁定**。其前最近的块（6919–6921 请求体、6930–6938 响应）均已闭合，该行无开启者 |
| D4 | 不改任何正文/表格/代码块内容 | **锁定**。diff 仅：删 1 围栏行、插 1 围栏行、删 7 行重复片段；§12.4 正文逐字不变（R4） |
| D5 | 新增可复用守卫 `scripts/check-doc-fences.sh` 并用它产出证据 | **锁定**。Issue 验收表要求「奇偶性结构断言」而非 token grep；守卫的 CLI/输出 token 必须按「变更点 2」实现，保证后续 Issue 可复用 |
| D6 | §5.6.1–5.6.3 折叠单行（`api.md:1833`）不在本单 | **锁定**。属另一类结构缺陷（字面 `\n`），会扩大 Issue 验收面；已立 **#2591** 并回填证据 |
| D7 | 证据全部为结构性静态证据，不伪造渲染截图 | **锁定**。环境无 Markdown 渲染器（`markdown`/`commonmark`/`pandoc` 均不可用），Issue 验收表已指定结构判定口径 |

### Out of Scope

- `docs/spec/api/api.md:1833` 的「§5.6.1–5.6.3 折叠为单行（字面 `\n`）」→ **#2591**（同文件另一类结构缺陷）。
- `~~~` 波浪号围栏、缩进 ≥4 空格的代码块、行内单反引号代码 → 守卫不覆盖。
- Markdown 表格管道符失衡、标题层级（`####` 跨级）、锚点/目录生成 → 不在本单。
- 非 `docs/**` 目录的 Markdown（`README.md`、`AGENTS.md`、`docs/evidence/**`）→ 本单扫描范围外；若后续需要，直接用守卫 `--root .` 另行立单。
- API 契约语义（端点/字段/错误码）本身的正确性 → 与围栏结构无关。
- 任何代码（`backend/`、`frontend-*/`）改动 → 本单纯文档 + 守卫脚本。

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R0 | `--self-test` 现场生成：`good.md`（1 对围栏）、`odd.md`（1 个孤立开启）、`inside.md`（2 处互补缺陷使偶数但标题入块）、`eof.md`（开启后到 EOF 未闭合） | `bash scripts/verify-2590.sh r0`（内部调 `check-doc-fences.sh --self-test`） | `docs/evidence/2590/R0-checker-selftest.txt` | `SELFTEST_CASES=4`、`SELFTEST_DETECTED=3`、`SELFTEST_FALSE_POSITIVE=0`、`RESULT=PASS` | 任一坏 fixture 未被检出（`DETECTED<3`）、或 `good.md` 被误报（`FALSE_POSITIVE>0`）→ FAIL（守卫不可信，R1/R2 随之作废） |
| R1 | 修复后 `docs/` 全量 `*.md`（含 `docs/spec/api/api.md` 与 4 个兄弟文档） | `bash scripts/verify-2590.sh r1` | `docs/evidence/2590/R1-fence-sweep.txt` | `DOC_FILES_SCANNED>=9`、`ODD_FENCE_FILES=0`、`HEADINGS_INSIDE_FILES=0`、`EOF_INSIDE_FILES=0`、`RESULT=PASS` | 任一文件 `EVEN=0` 或 `HEADINGS_INSIDE>0` → FAIL；只报 `api.md` 通过而兄弟文档未扫（`SCANNED<9`）→ FAIL |
| R2 | 修复后 `docs/spec/api/api.md`（596 围栏，偶数） | `bash scripts/verify-2590.sh r2` | `docs/evidence/2590/R2-api-structure.txt` | `API_FENCES>=590`、`API_EVEN=1`、`API_HEADINGS_INSIDE=0`、`API_EOF_INSIDE=0`、`HEADING_STATE_12_4=outside`、`SEC_12_4_LINE>0` | `API_EVEN=0`、任一标题 `inside`、`HEADING_STATE_12_4=inside`（§12.4 仍被吞）→ FAIL |
| R3 | 修改前 blob：`git show <fix>^:docs/spec/api/api.md`（脚本内采集，不手写） | `bash scripts/verify-2590.sh r3` | `docs/evidence/2590/R3-red-prefix.txt` | `RED_CHECKER_EXIT=1`、`RED_EVEN=0`、`RED_FENCES=597`、`RED_HEADING_STATE_12_4=inside` | 修复前 `RED_CHECKER_EXIT=0`（说明红样采集错/改的是已修版本，红绿失效）或 `RED_HEADING_STATE_12_4!=inside` → FAIL |
| R4 | `git show <fix>^:…` vs 修复后工作树 | `bash scripts/verify-2590.sh r4` | `docs/evidence/2590/R4-content-preservation.txt` | `HEADINGS_PRE=HEADINGS_POST`（`HEADINGS_DELTA=0`）、`TOTAL_DELTA=-7`、`NONFENCE_DELTA=-6`、`REMOVED_FENCE_LINES=1`、`REMOVED_LINES_ARE_DUPLICATES=1`、`SEC_12_4_UNCHANGED=1` | 标题数变化（误删标题）、`TOTAL_DELTA` 非 -7 或 `NONFENCE_DELTA` 非 -6（误改正文）、被删行不全是重复行、§12.4 段落有任何字节差异 → FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R0 | `bash scripts/verify-2590.sh r0` | `docs/evidence/2590/R0-checker-selftest.txt` | exit 0 且含 `SELFTEST_CASES=4` `SELFTEST_DETECTED=3` `SELFTEST_FALSE_POSITIVE=0` `RESULT=PASS` | PASS（4 项 token 全为期望值） | 坏 fixture 漏检或正常 fixture 误报 → FAIL（守卫能力未被证明） | dev |
| R1 | `bash scripts/verify-2590.sh r1` | `docs/evidence/2590/R1-fence-sweep.txt` | exit 0 且含 `ODD_FENCE_FILES=0` `HEADINGS_INSIDE_FILES=0` `EOF_INSIDE_FILES=0` `RESULT=PASS` 且 `DOC_FILES_SCANNED>=9` | PASS（四项 token 全为期望值且扫描文件数达标） | 任一文档奇数围栏或标题入块 → FAIL；只扫单文件（`DOC_FILES_SCANNED<9`）→ FAIL | dev |
| R2 | `bash scripts/verify-2590.sh r2` | `docs/evidence/2590/R2-api-structure.txt` | exit 0 且含 `API_EVEN=1` `API_HEADINGS_INSIDE=0` `API_EOF_INSIDE=0` `HEADING_STATE_12_4=outside` | PASS（四项 token 全为期望值） | §12.4 仍在代码块内（`inside`）或任一标题入块 → FAIL | dev |
| R3 | `bash scripts/verify-2590.sh r3` | `docs/evidence/2590/R3-red-prefix.txt` | exit 0 且含 `RED_CHECKER_EXIT=1` `RED_EVEN=0` `RED_FENCES=597` `RED_HEADING_STATE_12_4=inside` | PASS（四项 token 全为期望值，即修复前确为红） | 修复前被判 PASS（红绿失效）、`RED_FENCES` 不是 597 → FAIL | dev |
| R4 | `bash scripts/verify-2590.sh r4` | `docs/evidence/2590/R4-content-preservation.txt` | exit 0 且含 `HEADINGS_DELTA=0` `TOTAL_DELTA=-7`、`NONFENCE_DELTA=-6`、`REMOVED_FENCE_LINES=1` `REMOVED_LINES_ARE_DUPLICATES=1` `SEC_12_4_UNCHANGED=1` | PASS（四项 token 全为期望值） | 标题数变化、正文被误改（`NONFENCE_DELTA` 非 -7）、被删行含非重复内容、§12.4 正文有 diff → FAIL | dev |

### Artifact Provenance

- 生成器：`scripts/verify-2590.sh`（子命令 `r0`–`r4`），内部调用 `scripts/check-doc-fences.sh`；每个产物首行
  `# build=1.0.<fix 短码> # commit=<40 位 sha> # dirty=false`（工作树干净，证据在 fix commit 之后生成），
  另起一行 `GENERATED_AT_HEAD=<40 位 sha>`。
- **证据必须在 fix commit 之后生成**；`r3` 的红样通过 `git show <fix>^:docs/spec/api/api.md` 现场采集，禁止手写。
- 逐条清单：
  - `docs/evidence/2590/R0-checker-selftest.txt`
  - `docs/evidence/2590/R1-fence-sweep.txt`
  - `docs/evidence/2590/R2-api-structure.txt`
  - `docs/evidence/2590/R3-red-prefix.txt`
  - `docs/evidence/2590/R4-content-preservation.txt`
- `docs/evidence/2590/evidence.json`：`schema_version=1`、`issue=2590`、`dirty=false`、
  `commit`=fix commit 全 sha、`plan_sha256`=本计划评论对应 `plan.md`；`rows` 为 R0–R4 各一条且 `pass=true`。
  evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2590 --summary-file tmp/pre-ready-2590-summary.md --changed-files tmp/pre-ready-2590-changed.txt --evidence-file docs/evidence/2590/evidence.json --report docs/evidence/2590/pre-ready-2590.json`。

### Dependencies

无。

### Workload

- Size: S
- Basis: 文档 1 个文件（删 1 围栏 + 插 1 围栏 + 删 7 行重复片段）+ 1 个守卫脚本（约 120 行 bash）+ 1 个证据 harness（5 子命令）+ 5 个静态证据产物；无代码、无 migration、无前端、无外部依赖
- Confidence: high

*Model: DeepSeek V4.1 Flash 1017*


