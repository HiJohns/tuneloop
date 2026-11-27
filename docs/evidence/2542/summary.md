# Work Summary — #2542

## 修改内容

executeAction（merchant_admin 分支）在 IAM customer-permissions 写入失败时只 log.Printf 后继续返回 nil，因此三个入口（Confirm / IAMConfirmationCallback / SMSCallback）都按成功回执，会话保持 confirmed，而商户管理员的客户权限位为空——PC/H5 大量按 cus_perm 位判定的能力不可用且无任何错误提示（AGENTS.md 红线「API 调用链中静默吞错」）。

- **失败即回滚 + 回传**：SetUserCustomerPermissions 失败 → UnbindUserFromOrganization（不 purge：主体非本流程创建）→ 返回 error；回滚本身失败时记录 CRITICAL，并在错误信息中同时携带两条原因。
- **统一失败落库**：新增 failSession(db, session, actionErr)（单一实现），三个入口在 executeAction 返回 error 时把会话落为 status=failed + message；各自 HTTP/重定向形态保持不变，其中 IAM 回调由 status=error&message= 改为 status=failed&action=<action_type>。
- **契约文档（Core Update）**：docs/spec/api/api.md 新增 §2.8.2 确认执行与失败语义（三入口成败回执、merchant_admin 执行顺序、回滚与 CRITICAL 语义、failed 仍可重试）。

## 涉及文件

- backend/handlers/confirmation_session.go（1 处错误分支改造 + failSession helper + 3 个调用点接线）
- backend/handlers/confirmation_session_2542_test.go（新增 4 个 fake-IAM 运行时用例）
- docs/spec/api/api.md（§2.8.2 新增，Core Update）
- scripts/verify-2542.sh（r1–r6 + gate + red + ledger）
- 证据目录 `docs/evidence/2542/`（plan 快照、R1–R6 产物、红样、evidence.json、gate 报告）

Commit Hash：7ef3e0271（fix）；26b9f806f（Round 2 审计 B1 文档路径修正）；证据提交见「Remote」小节。

## 验证层级

- 本单为独立 Issue（无 Parent:），因此 gate 内跑 go build ./... + 本单定向用例 + 全包编译烟测（go test ./... -run '^$'），未重复全量 go test ./...（#2651：全量留给批次 gate）。
- 所有重命令均经 heavy-run.sh 取 host 锁执行。
- 定向回归：go test ./handlers -run '^TestConfirmationSessionHandler'（Create/Get/Confirm 既有用例）→ REG_FAIL=0。

## Remote

remote: origin/issue-2542@7ef3e0271; fix=7ef3e0271 pushed=yes; evidence=docs/evidence/2542 pushed=yes; round2-docs=26b9f806f pushed=yes

## 红绿实证

用例 TestConfirmationSession2542_CusPermFailureRollsBack：

- **红（缺陷版）**：在 fix 的父提交（7ef3e0271^）的旧 confirmation_session.go 上运行同一用例（bash scripts/verify-2542.sh red，临时 worktree）→ --- FAIL，断言消息即 Issue 症状：{"code":20000,"message":"successfully confirmed and action executed"}（RED_EXIT=1、RED_OK，产物 `docs/evidence/2542/R1-red-prefix.txt`）。
- **绿（修复版）**：同一用例 → --- PASS，STATUS=500 CODE=50000 SESSION_STATUS=failed UNBIND_CALLS=1 PURGE_CALLS=0（产物 `docs/evidence/2542/R1-cusperm-fail.txt`）。

## Accepted Plan 对照（7/7）

| 计划项 | 状态 |
|---|---|
| 变更点 A1：customer-permissions 失败 → 回滚 unbind（不 purge）+ 回传 error | 完成 |
| 变更点 A2：新增 failSession 统一失败落库 helper | 完成 |
| 变更点 A3：三个入口统一调用 failSession，回调重定向改 status=failed&action= | 完成 |
| 变更点 A4：不动 site_manager/site_staff、Reject、主体取值（#2543）、bind 关系角色（#2540） | 完成 |
| 变更点 B：新增 4 个用例（回滚 / 成功 / 补偿失败 / 回调整形），helper 带 2542 后缀 | 完成 |
| 变更点 C8：scripts/verify-2542.sh（r1–r6 + gate + red + ledger） | 完成 |
| 变更点 C9：docs/spec/api/api.md §2.8.2（Core Update） | 完成 |

### 契约三向核对

| 方向 | 内容 | 一致 |
|---|---|---|
| Plan ↔ API 文档 | 计划变更点 A1/A3 + Core Update ↔ docs/spec/api/api.md §2.8.2（三入口回执表、回滚不 purge、status=failed&action=、failed 可重试） | 是 |
| API 文档 ↔ handler | POST /api/confirmation-sessions/:id/confirm（Confirm）失败 → 500/50000 + failed；GET /api/iam/confirmation-callback（IAMConfirmationCallback）失败 → 302 status=failed&action=；SMS（SMSCallback）失败 → 500 文本 + failed | 是 |
| handler ↔ 测试 | 4 个用例逐一断言上述回执形态与副作用（BIND/CUSPERM/UNBIND/PURGE 调用计数、会话状态、Location） | 是 |

## Evidence Contract（6/6 PASS）

| 行 | 结果 | 产物 |
|---|---|---|
| R1 | PASS | `docs/evidence/2542/R1-cusperm-fail.txt` |
| R2 | PASS | `docs/evidence/2542/R2-cusperm-ok.txt` |
| R3 | PASS | `docs/evidence/2542/R3-compensation-fail.txt` |
| R4 | PASS | `docs/evidence/2542/R4-callback-failed.txt` |
| R5 | PASS | `docs/evidence/2542/R5-neighbor-scan-build.txt` |
| R6 | PASS | `docs/evidence/2542/R6-docs.txt` |

汇总 bundle：`docs/evidence/2542/evidence.json`（build=1.0.7ef3e0271、commit=7ef3e0271、dirty=false）。

## G0 Golden Acceptance

| 行 | artifact | assertions |
|---|---|---|
| R1 | `docs/evidence/2542/R1-cusperm-fail.txt` | GO_TEST_EXIT=0、STATUS=500、CODE=50000、SESSION_STATUS=failed、BIND_CALLS=1、CUSPERM_CALLS=1、UNBIND_CALLS=1、PURGE_CALLS=0、R1_OK；负例（20000 / confirmed / 无 unbind）→ FAIL |
| R2 | `docs/evidence/2542/R2-cusperm-ok.txt` | STATUS=200、CODE=20000、SESSION_STATUS=confirmed、UNBIND_CALLS=0、CUS_PERM_BIT0=1、CUS_PERM_BIT7=1；负例（成功路径 unbind / 位为 0）→ FAIL |
| R3 | `docs/evidence/2542/R3-compensation-fail.txt` | STATUS=500、SESSION_STATUS=failed、UNBIND_CALLS=1、UNBIND_FAILED=1、COMP_ERROR_PRESENT=1、CRITICAL_LOGGED=1 |
| R4 | `docs/evidence/2542/R4-callback-failed.txt` | REDIRECT_CODE=302、LOCATION_HAS_STATUS_FAILED=1、SESSION_STATUS=failed；负例（status=success / confirmed）→ FAIL |
| R5 | `docs/evidence/2542/R5-neighbor-scan-build.txt` | CONFIRM_CUSPERM_SITES=1、CONFIRM_LOG_ONLY=0、CONFIRM_UNBIND_AFTER=1、CONFIRM_RETURN_ERR=1、NEIGHBOR_CALL_SITES=5、NEIGHBOR_UNHANDLED=0、BUILD_RC=0、REG_FAIL=0 |
| R6 | `docs/evidence/2542/R6-docs.txt` | DOC_SESSION_FAILED=1、DOC_UNBIND_COMPENSATION=1、DOC_CALLBACK_FAILED=1 |

## 矩阵 n/n

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 客户权限位失败 → 回滚绑定 + 回执失败 + 会话 failed（红绿） | PASS | `docs/evidence/2542/R1-cusperm-fail.txt` |
| R2 成功路径不受影响（负例） | PASS | `docs/evidence/2542/R2-cusperm-ok.txt` |
| R3 补偿本身失败必须显式 | PASS | `docs/evidence/2542/R3-compensation-fail.txt` |
| R4 IAM 回调回执 status=failed | PASS | `docs/evidence/2542/R4-callback-failed.txt` |
| R5 邻居静态扫描 + 构建 + 定向回归 | PASS | `docs/evidence/2542/R5-neighbor-scan-build.txt` |
| R6 契约文档同步 | PASS | `docs/evidence/2542/R6-docs.txt` |
| R7 pre-ready gate（四源 + 定向测试 + 构建） | PASS | `docs/evidence/2542/pre-ready-2542.json` |

## 同类路径对称扫描

grep -rn "SetUserCustomerPermissions(" backend --include=*.go（排除 iam_client.go 定义与测试文件）= 6 处：

| 调用点 | 失败处理 | 结论 |
|---|---|---|
| handlers/confirmation_session.go:388 | 本单改造：unbind + return error | 已修 |
| handlers/permission_manage.go:136 | 500/50000 + return | 已正确（豁免） |
| handlers/user_staff.go:719 | unbind + purge + 500/50000 | 已正确（豁免） |
| handlers/user_staff.go:872 | errs = append 收集 | 已正确（豁免） |
| handlers/user_staff.go:931 | compensateSiteTransfer + 502 + 显式 message | 已正确（豁免） |
| services/bulk_import_accounts.go:322 | LogImportError + unbind + delete + 该行记失败 | 已正确（豁免） |

→ NEIGHBOR_CALL_SITES=5、NEIGHBOR_UNHANDLED=0，除本处外无同类吞错，无需扩面。

## 语义变更（被翻转的既有断言）

计划「语义变更」表列出 4 行：Confirm 失败分支、IAM 回调重定向、SMS 回调、TestConfirmationSessionHandler_Confirm/Success。前三行旧行为没有任何既有断言锁定（grep 确认），因此无需翻绿；第四行按计划保持不变并纳入 R5 定向回归（REG_FAIL=0）。新增「旧行为不再发生」断言：CONFIRM_LOG_ONLY=0（R5）、SESSION_STATUS=failed（R1/R3/R4）、UNBIND_CALLS=0（R2 成功路径）。

## Dependency Order / 验收规则

- 前置 #2540（status:done）已合入 origin/develop（275c96a94 Merge issue-2540）→ 本单基线含 bind 关系角色 OWNER，未越界改动。
- 未修改任何验收工具/规则（plan-lint.sh、pre-ready-check.sh、matrix_parser.sh、scripts/verify-2540.sh 等均未触碰，D8）。
- 未新增 migration、未改前端、未改 #2567/#2543 范围（Out of Scope 遵守）。

### 证据来源

| repo | build_version | fix commit | artifact |
|---|---|---|---|
| . | 1.0.7ef3e0271 | 7ef3e027143c3ec41896c79a2c68aa87b9f1357f | `docs/evidence/2542/R1-cusperm-fail.txt` |

build_version 的短码 7ef3e0271 即本轮 fix commit（git merge-base --is-ancestor 7ef3e0271 HEAD 成立）；生成证据前 git status --porcelain -- ':!docs/evidence' 为空（dirty=false）。

## 人工/部署侧

- 无。本单所有矩阵行均可由 Developer 在测试库 + fake IAM 上完成；真实 beaconiam 端权限位效果由 #2506 的 services 单测与 beaconiam 端点承担（计划「工具盲区」）。

## Round 2 修复（审计 Round 1 · B1）

| 审计项 | 问题 | 修复 | 证据 |
|---|---|---|---|
| B1（High） | `docs/spec/api/api.md` §2.8.2 的「短信回调」行把入口写成 GET /api/iam/confirmation-callback?token=&reply=Y；实际短信回调是 GET /api/confirmation/callback/sms（路由注册在 `backend/main.go` 第 313 行 → SMSCallback，读 token/reply），而 IAM 回调（同文件第 316 行）只读 session/result/confirm_type → 按旧行联调会命中 sessionID 为空 → 302 /confirmation-result?status=error&message=missing_parameters，短信确认永不生效 | 该行路径改为 GET /api/confirmation/callback/sms?token=&reply=Y；同表另两行复核：POST /api/confirmation-sessions/:id/confirm（同文件第 309 行，成功 200/20000、失败 500/50000）与 GET /api/iam/confirmation-callback?session=&result=&confirm_type=（同文件第 316 行）与实现一致，未改 | `docs/spec/api/api.md`（§2.8.2 表格）；重生成 `docs/evidence/2542/R6-docs.txt` |

**R6 形态声明（审计 B1 要求的选项说明）**：R6 目前是**存在性**断言（DOC_SESSION_FAILED / DOC_UNBIND_COMPENSATION / DOC_CALLBACK_FAILED 三个 token），**不**校验「文档路径 ↔ `backend/main.go` 路由注册」是否一致——B1 正是从该盲区漏过的。把 R6 升级为「从路由注册表抽取已注册 route，逐行断言 §2.8.2 表中每个 METHOD /api/... 都存在 + 负例」属**验收规则强化**，需 Architect 在计划 R6 行登记（instructions §6.5.2）；本次按审计给出的最小路径执行：只做文档修正 + R6 重跑落盘 + 本声明，未自行加断言据以过关。路径正确性本轮由人工核对（`grep -n "confirmation" backend/main.go` 命中第 307–316 行；SMSCallback 读 token/reply，IAMConfirmationCallback 读 session/result/confirm_type）确认。

## Pre-ready gate

见 `docs/evidence/2542/pre-ready-2542.json`（`overall: PASS`）。关键输出：

```
P1   PASS   plan has Verification Matrix / Input Truth & Tool Preflight / Decision Lock / Out of Scope
P2   PASS   Work Summary covers all 7 matrix rows with PASS/N/A
P3   PASS   all referenced evidence paths exist and are durable
P4   PASS   all 1 affected command(s) passed
P12  PASS   Evidence Contract complete: 7 row(s) match the plan and have durable artifacts
P14  PASS   artifact provenance matches evidence (7 artifacts)
P17  PASS   plan snapshot and Accepted Plan comment match the gated plan
P5   WARN   baseline gate also failed (rc=127); no parseable baseline failures, cannot prove subset
P6   PASS   all changed files are declared in the Accepted Plan
P7   PASS   contract three-way section present
P9   PASS   build provenance verified for 1 version(s)
P11  PASS   fix commit(s) are ancestors of HEAD; canonical worktree; worktree guard: PASS
P13  PASS   fix commit and evidence artifacts are on origin/issue-2542
RESULT: PASS
```

P5 的 WARN 说明：baseline（merge-base）上不存在本单新增的 scripts/verify-2542.sh，因此 baseline gate 以 rc=127（command not found）退出，无法证明「失败集合是修复版的子集」；修复版 gate 本身 `exit=0`（`[gate-run] source=run ... exit=0`），该项不阻断。

*Model: deepseek/deepseek-flash*
