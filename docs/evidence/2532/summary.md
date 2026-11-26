# Work Summary — #2532：SubmitAssessment 回退分支禁止把组织 ID 写入 instruments.current_site_id

关联（非硬依赖）：#2509（同族 ID 域审计父任务）、#2511（`resolveSiteIDByOrg` 唯一实现 owner）

## 修改内容

| # | 改动 | 文件 |
|---|------|------|
| 1 | `SubmitAssessment`（`hasDamage=true`）回退分支改为「order.OrgID → sites.id 反查」：调用 #2511 的 `resolveSiteIDByOrg`（单一实现，未复制）；不可解析时**不写入**（保留原值）并打印含 instrument id 与 org 的 `[WARN] #2532` | `backend/handlers/assessment.go` |
| 2 | 新增自带夹具（`sites.id ≠ sites.org_id`，操作者无 `site_members` 确定性进入回退分支）的 3 个用例：回退写站点、不可解析保留原值、主分支不变 | `backend/handlers/assessment_2532_test.go` |
| 3 | 证据生成器 `r1`–`r4` + `gate` + `red`（预修复红跑）+ `ledger`（从 fix commit 与 gate 报告派生 `build/commit/dirty` 和 R5 `actual`，禁止手改）（首行 provenance 锚定 fix commit） | `scripts/verify-2532.sh` |
| 4 | Accepted Plan 快照（四源校验） | `docs/evidence/2532/plan.md` |

**Commit**：fix `0275ebc84` → evidence（本提交）；证据 provenance 锚定 `0275ebc84`。

**依赖合入方式**：#2511 的 `status:done` 代码位于 `origin/issue-2511`（未进 develop），本分支以 merge `e77597a7d` 引入其已审计实现（`resolveSiteIDByOrg`），merge 内容不属于本单改动、也不在本次 docs/evidence/2532/changed-files.txt 范围内；R4 的 `TREE_ORG_ASSIGN=0` 依赖该合入，未合入时脚本会 FAIL（未放宽断言）。

## 验证层级

- 独立 Issue（body 无 `Parent:` 行；#2509 为关联父任务）：按计划 R5 的 `--affected-cmd`/`--full-gate-cmd` 执行受影响包定向套件（`go build ./...` + `go test ./handlers -run '…2532|…1889 主分支|…2511'`），无前端改动、无 migration。
- 全量 `go test ./...` 不在本单范围（计划 R5 只要求 affected gate；全量属父任务 #2509 验收）。

## Remote

remote: `origin/issue-2532`; fix=`0275ebc84` pushed=yes; evidence 提交在其后并已 push（P13 校验 fix commit 在远端）

## 红绿实证

**R1（回退写站点 ID）—— 缺陷版 FAIL（HTTP 500 + 外键违例）→ 修复版 PASS**

缺陷版（预修复源码 `e77597a7d:backend/handlers/assessment.go` 在工作树中还原后运行，用例与夹具不变，同一测试库；等价于 `git stash` 掉修复）：

```
STATUS=500
CURRENT_IS_SITE=0
CURRENT_NOT_ORG=0
OLD_VALUE_IS_ORG=1
RED_CONFIRMED=0
--- FAIL: TestSubmitAssessment2532_FallbackWritesSiteNotOrg (0.82s)
    Error:  Not equal: expected: 200 / actual: 500
FAIL	tuneloop-backend/handlers
```

红跑已持久化（非 `tmp/`）：`docs/evidence/2532/R1-red-prefix.txt`，由 `bash scripts/verify-2532.sh red` 生成，首行 provenance 为 `commit=e77597a7d`（预修复 HEAD）+ 说明「fix 0275ebc84 在工作树中被还原后运行」。

修复版（`0275ebc84`）：`docs/evidence/2532/R1-assessment-fallback.txt` → `STATUS=200`、`OPERATOR_SITE=none`、`RESOLVED_IS_SITE=1`、`CURRENT_IS_SITE=1`、`CURRENT_NOT_ORG=1`、`OLD_VALUE_IS_ORG=1`、`RED_CONFIRMED=1`、`--- PASS`、`R1_OK`。

> 说明：测试库确实带 `instruments_current_site_id_fkey`（缺陷版直接 500），比计划 D4 预期的「测试库可能无 FK」更强——红证据同时覆盖「值域错误」与「真实 500」。

## Accepted Plan 对照（6/6）

1. 变更点 A1：回退分支改「组织 → 站点反查」，不可解析不写入 + WARN → 完成（assessment.go:132-140）
2. 变更点 A2：禁止本文件新增解析函数，只消费 #2511 的 `resolveSiteIDByOrg` → 完成（`LOCAL_RESOLVER_DEF=0`、`RESOLVER_CALL=1`）
3. 变更点 A3：主分支语义不变、不改其它 `current_site_id` 消费方、不改 `GetAssessmentData` → 完成（R3 + 定向回归）
4. 变更点 B4：夹具 `sites.id ≠ sites.org_id` 且操作者无 `site_members` → 完成
5. 变更点 B5：3 个用例（回退红绿 / 不可解析负例 / 主分支负例）→ 完成
6. 变更点 C6：`scripts/verify-2532.sh`（`r1`–`r4` + `gate`，首行 provenance）→ 完成

## Verification Matrix（5/5）

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 回退分支写站点 ID（红绿） | PASS | `docs/evidence/2532/R1-assessment-fallback.txt` |
| R2 不可解析 → 保留原值（负例） | PASS | `docs/evidence/2532/R2-unresolvable-keeps.txt` |
| R3 主分支不变（负例） | PASS | `docs/evidence/2532/R3-primary-unchanged.txt` |
| R4 同类扫描 + 构建 + 定向回归 | PASS | `docs/evidence/2532/R4-scan-build.txt` |
| R5 pre-ready gate（四源 + 定向测试 + 构建） | PASS | `docs/evidence/2532/pre-ready-2532.json` |

## Evidence Contract

| 行 | 结果 | 产物 |
|---|------|------|
| R1 | PASS → `STATUS=200、OPERATOR_SITE=none、RESOLVED_IS_SITE=1、CURRENT_IS_SITE=1、CURRENT_NOT_ORG=1、OLD_VALUE_IS_ORG=1、RED_CONFIRMED=1、R1_OK` | `docs/evidence/2532/R1-assessment-fallback.txt` |
| R2 | PASS → `STATUS=200、KEPT_ORIGINAL=1、NO_CROSS_DOMAIN_WRITE=1、RESOLVED_EMPTY=1、OLD_WOULD_WRITE_ORG=1、R2_OK` | `docs/evidence/2532/R2-unresolvable-keeps.txt` |
| R3 | PASS → `STATUS=200、PRIMARY_EQUALS_OPERATOR_SITE=1、PRIMARY_NOT_ORG=1、R3_OK` | `docs/evidence/2532/R3-primary-unchanged.txt` |
| R4 | PASS → `ASSESSMENT_ORG_ASSIGN=0、TREE_ORG_ASSIGN=0、RESOLVER_CALL=1、LOCAL_RESOLVER_DEF=0、WARN_LINE_PRESENT=1、BUILD_RC=0、REG_RC=0、REG_PASS_COUNT=22、REG_FAIL=0、R4_OK` | `docs/evidence/2532/R4-scan-build.txt` |
| R5 | PASS → gate `overall=PASS`（P1–P18 逐项，R5 行 pass=true） | `docs/evidence/2532/pre-ready-2532.json` |

机器可判定记录：`docs/evidence/2532/evidence.json`（5 行，全部 `pass=true`、`dirty=false`、build=1.0.0275ebc84、`commit=0275ebc84…`、`plan_sha256=4add6ab5…`）。

## G0 Golden Acceptance

| 行 | artifact | assertions | 负例（未被触发） |
|---|---|---|---|
| R1 | `docs/evidence/2532/R1-assessment-fallback.txt` | `STATUS=200`／`OPERATOR_SITE=none`／`RESOLVED_IS_SITE=1`／`CURRENT_IS_SITE=1`／`CURRENT_NOT_ORG=1`／`OLD_VALUE_IS_ORG=1`／`RED_CONFIRMED=1`／`R1_OK` | 写入 `aOrg` → `CURRENT_NOT_ORG=0` 且 500 → `R1_FAIL` |
| R2 | `docs/evidence/2532/R2-unresolvable-keeps.txt` | `KEPT_ORIGINAL=1`／`NO_CROSS_DOMAIN_WRITE=1`／`RESOLVED_EMPTY=1`／`OLD_WOULD_WRITE_ORG=1`／`R2_OK` | 覆盖为 org/空 → `KEPT_ORIGINAL=0` → `R2_FAIL` |
| R3 | `docs/evidence/2532/R3-primary-unchanged.txt` | `PRIMARY_EQUALS_OPERATOR_SITE=1`／`PRIMARY_NOT_ORG=1`／`STATUS=200`／`R3_OK` | 主分支被 org 覆盖 → `PRIMARY_NOT_ORG=0` → `R3_FAIL` |
| R4 | `docs/evidence/2532/R4-scan-build.txt` | `ASSESSMENT_ORG_ASSIGN=0`／`TREE_ORG_ASSIGN=0`／`RESOLVER_CALL=1`／`LOCAL_RESOLVER_DEF=0`／`WARN_LINE_PRESENT=1`／`BUILD_RC=0`／`REG_FAIL=0`／`R4_OK` | 残留跨域写入或自建解析函数 → `R4_FAIL` |
| R5 | `docs/evidence/2532/pre-ready-2532.json` | overall=PASS、evidence 每行 `pass=true`、`dirty=false`、P13 在 `origin/issue-2532` | 任一 P 项失败 → 不得 ready |

## Pre-ready gate

`~/.config/opencode/scripts/pre-ready-check.sh 2532 …` 报告：`docs/evidence/2532/pre-ready-2532.json`（`overall=PASS`，`plan_sha256=4add6ab58b866c52d0a84d49b2c98e7d4fe4052ef6f3b4166774969400abd24f`）。关键行：P12 Evidence Contract 5 行齐全、P14 artifact provenance 与 fix commit 一致、P17 计划快照与 Accepted Plan 一致、P13 fix commit 在 `origin/issue-2532`。

## 同类路径对称扫描

`grep -rhE 'current_site_id"\] = .*OrgID' backend/handlers/` → **0**（本单 1 处 + #2511 两处均已收敛）。仓库级其它 `current_site_id` 写点（inventory.go 等）不涉及 `OrgID`，未改。

## Round 2 修复（/fixreview：Round 1 审计 3 项 → 修复 → 证据）

| 审计项 | 问题 | 修复 | 修复后证据 |
|---|---|---|---|
| Bug 1（高） | 计划变更点 C6 / Input Truth 要求 `evidence.json` 的 `build/commit/dirty` 由脚本 `ledger` 从最新 fix commit 派生，但脚本只有 `r1`–`r4` / `gate`，账本靠手写 | 新增 `ledger` 子命令：从最新 `fix(issue-2532)` commit 派生 build / commit / dirty，用 `jq` 只写回顶层三字段（保留 rows）；报告缺失、overall 非 PASS、或报告 provenance.commit 与 fix commit 不一致时非 0 退出 | 见下方 Round 2 证据块（LEDGER_OK） |
| Bug 2（高） | `evidence.json` 的 R5 行 `actual` 仍是首轮占位 `PENDING:first-gate-run`，与产物 overall=PASS 自相矛盾 | `ledger` 从 `docs/evidence/2532/pre-ready-2532.json` 派生 R5 `actual`（overall / plan_sha256 / provenance.build / provenance.commit / dirty / P12 / P13 + 来源路径），占位串清零 | `docs/evidence/2532/evidence.json` 的 R5 `actual` |
| Bug 3（中） | 红证据只指向不可追溯的 `tmp/red-2532.log`（gitignored），账本引用不可核验路径 | 新增 `red` 子命令：把 fix commit 的父提交 `e77597a7d` 的 `backend/handlers/assessment.go` 还原到工作树后运行同一 R1 用例（等价 stash 掉修复），输出持久化到 `docs/evidence/2532/R1-red-prefix.txt`（首行 provenance 为预修复 commit + 还原说明，`trap` 保证工作树复原）；R1 `negative_result` 改指该文件 | `docs/evidence/2532/R1-red-prefix.txt`（STATUS=500 / CURRENT_NOT_ORG=0 / RED_CONFIRMED=0 / --- FAIL / RED_OK） |

### Round 2 证据（命令输出）

```
$ bash scripts/verify-2532.sh ledger
== ledger: derive build/commit/dirty + R5 actual into evidence.json ==
build=1.0.0275ebc84
commit=0275ebc843ea3087dcf9a9f316adb33619baadf2
dirty=false
R5_ACTUAL=PASS：overall=PASS / plan_sha256=4add6ab58b866c52d0a84d49b2c98e7d4fe4052ef6f3b4166774969400abd24f / provenance.build=1.0.0275ebc84 / provenance.commit=0275ebc843ea3087dcf9a9f316adb33619baadf2 / dirty=false / P12=PASS / P13=PASS（source: docs/evidence/2532/pre-ready-2532.json，ledger 派生）
ROWS=5 PASS_ROWS=5
LEDGER_OK

$ bash scripts/verify-2532.sh red
# build=1.0.e77597a7d # commit=e77597a7d2560a3ba562e0d31d7804d06db84575 # dirty=false
== R1 red run: pre-fix SubmitAssessment fallback writes order.OrgID ==
STATUS=500
CURRENT_IS_SITE=0
CURRENT_NOT_ORG=0
RED_CONFIRMED=0
--- FAIL: TestSubmitAssessment2532_FallbackWritesSiteNotOrg
RED_EXIT=1
RED_OK
```

- 本轮只修复上述审计项：业务代码 `backend/handlers/assessment.go` 与 `backend/handlers/assessment_2532_test.go` 仍与 fix `0275ebc84` 逐字一致，未新增功能、未改验收口径。
- 审计附带的「计划层观察」（R3 断言强度、P5/P18 两项 WARN）未作为本轮 REJECT 依据，本单不扩面修改；如需加强由 Architect 出新版计划。

## 未纳入本单（Out of Scope，计划已登记）

- 存量 `current_site_id` 域污染数据修复（需 Owner 授权改数据；#2511 已留只读排查 SQL 形态）。
- assessment.go 中 `orders`/`instruments` 更新缺 `tenant_id` 条件的隔离加固（另立）。
- #2509 父任务验收与其余同族子任务（#2512 / #2511 已 done）。

*Model: DeepSeek V4.1 Flash 1017*
