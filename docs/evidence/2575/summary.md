# Work Summary — #2575 确认会话 reject 守卫（confirmed / 已过期 / 非 waiting 不得改写）+ api.md §2.8.3 契约对齐

## 修改内容

| 文件 | 变更 |
|---|---|
| backend/handlers/confirmation_session.go | Reject 的唯一写入点改为条件 UPDATE：`WHERE id=? AND tenant_id=? AND token=? AND status='waiting' AND expires_at > now`；`RowsAffected=0` 只重读分类（过期 400/40002、rejected 幂等 200/20000、confirmed 与其它 409/40901），删除原先的 db.Save(&session)；IAMConfirmationCallback 的 `case "reject"` 同一口径（`status='waiting'` 条件更新，未命中只重定向 already_confirmed / not_rejectable / 幂等 rejected） |
| backend/handlers/confirmation_session_reject_2575_test.go | 新增 8 个用例：handler 5 个状态 + 错 token + 无动作执行断言；回调 confirmed / waiting / failed 三例（复用同包 setupConfirmationSessionTest） |
| docs/spec/api/api.md | 新增 `### 2.8.2 确认执行`（Confirm 既有语义）与 `### 2.8.3 拒绝`（守卫顺序、HTTP/code、回调分类、rejected 不回收已执行 IAM / 本地写入） |
| scripts/verify-2575.sh | 证据生成器（r1 结构红绿 / r2 行为用例 + go build ./... / r3 文档契约），产物首行写 provenance |

- fix commit：bc778f5e3
- 证据 commit：见下 Remote 行

## 验证层级（Issue #70 / #2651）

- 本次运行：go build ./...（BUILD_EXIT=0）+ 定向测试 go test ./handlers -run 'TestRejectConfirmationSession_2575|TestIAMCallbackReject_2575'（8/8 PASS）+ 既有相邻用例 TestConfirmationSessionHandler_ Create/Get/Confirm、TestIAMIntegration_ConfirmationCallback、TestIAMIntegration_ConfirmationCallback_Reject、TestIAMMock_ConfirmationCallback_Failed（同批 PASS，语义未翻转）。
- 全量回归：仓库根无 go.mod；单 Issue gate = go build ./... + 定向测试（R2）；全量回归由批次 gate 承担。

### 契约三向核对（Plan/API ↔ docs ↔ handler）

| 维度 | 内容 | 一致性 |
|---|---|---|
| Plan（Accepted Plan 变更点 2/3 + Decision Lock D1–D5） | 仅 `status='waiting'` 可转 rejected；先过期（40002）后状态（40901）；rejected 幂等 20000；confirmed 不得改写；回调不校验 expires_at 但必须守卫 confirmed/failed | 实现逐项一致（R1 结构断言 + R2 行为断言） |
| docs（docs/spec/api/api.md §2.8.2 / §2.8.3） | 守卫顺序表、HTTP/code 对应、回调重定向 message（already_confirmed / not_rejectable）、「不回收已执行写入」 | 与 handler 逐句一致（R3 断言 8 项；message 字符串与代码完全相同） |
| handler（backend/handlers/confirmation_session.go） | 条件更新为唯一写入点；`RowsAffected=0` 只重读分类、绝不写 | R1 的 GREEN_CAS_PREDICATE / GREEN_SINGLE_WRITE_POINT / NO_SAVE_IN_REJECT_PATHS / GREEN_POSTCHECK_REREAD 全部为 1 |
| 前端 / client | 仓库内无 confirmation-sessions 调用方（FRONT_CALLERS=0，计划 Decision Lock D7）；40002 / 40901 已在 PC 与 mobile 的 ERROR_CODE_MAP | 无前端改动面，未改任何前端文件 |

## 红绿实证

- 红（缺陷版 FAIL）：把 backend/handlers/confirmation_session.go 临时还原为修复前版本（bc778f5e3^）后运行同一批用例 → `--- FAIL: TestRejectConfirmationSession_2575`、`--- FAIL: TestIAMCallbackReject_2575`（CB_CONFIRMED_STATUS 期望 confirmed 实际 rejected；CB_FAILED 期望 not_rejectable 实际被改写）。原始输出：docs/evidence/2575/R2-red-prefix.txt（首行已注明本次为临时还原的运行态）。
- 绿（修复版 PASS）：bc778f5e3 上同一批用例 8/8 PASS → docs/evidence/2575/R2-reject-behavior.txt。
- 结构红绿：docs/evidence/2575/R1-reject-guards.txt 的 RED_NO_EXPIRY_GUARD=1 / RED_NO_STATUS_GUARD=1 / RED_CALLBACK_NO_GUARD=1（修前无守卫）与 GREEN_CAS_PREDICATE=1 / GREEN_SINGLE_WRITE_POINT=1 / NO_SAVE_IN_REJECT_PATHS=1（修后单条条件更新、无 db.Save）。

## Accepted Plan 对照（6/6）

| 计划项 | 状态 |
|---|---|
| 1 契约先行：api.md 新增 §2.8.2 / §2.8.3（含「不回收已执行写入」） | 完成（R3） |
| 2 Reject 改为条件更新唯一写入口 + post-check 分类，删除 db.Save | 完成（R1/R2） |
| 3 IAMConfirmationCallback 的 reject 分支同口径（条件更新 + 不写入的分类重定向） | 完成（R1/R2） |
| 4 新增 8 个测试用例 | 完成（R2） |
| 5 新增 scripts/verify-2575.sh（r1–r3） | 完成 |
| 6 证据账本 docs/evidence/2575/ 下 plan.md、summary.md、changed-files.txt、evidence.json + 产物 | 完成 |

不改：Confirm / Create / Get / SMSCallback / executeAction / 路由 / 模型 / 迁移 / 前端（与计划一致）。

## Verification Matrix 全行覆盖（4/4）

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 reject 守卫结构 + 条件更新唯一写入口（红绿对照） | PASS | docs/evidence/2575/R1-reject-guards.txt（R1_OK、CAS_PREDICATES=2、NO_SAVE_IN_REJECT_PATHS=1） |
| R2 行为用例（5 handler + 3 回调）+ 编译 | PASS | docs/evidence/2575/R2-reject-behavior.txt（BUILD_EXIT=0、TEST_EXIT=0、CASE_ 与 CB_ 全标记、NO_ACTION_EXECUTED=1）；红跑 docs/evidence/2575/R2-red-prefix.txt |
| R3 api.md §2.8.2 / §2.8.3 契约与实现一致 | PASS | docs/evidence/2575/R3-doc-contract.txt（DOC_282=1、DOC_283=1、DOC_NO_ROLLBACK=1 等 8 项） |
| R4 pre-ready gate（四源一致 + 证据完整 + P13 已推送） | PASS | docs/evidence/2575/pre-ready-2575.json（GATE_RESULT=PASS；报告顶层字段为 overall=PASS；P5=N/A 且记录原因；P12 四行齐全；P13 origin/issue-2575） |

## Pre-ready gate

- 命令：pre-ready-check.sh 2575 --plan-file docs/evidence/2575/plan.md --summary-file docs/evidence/2575/summary.md --changed-files docs/evidence/2575/changed-files.txt --evidence-file docs/evidence/2575/evidence.json --no-full-gate --report docs/evidence/2575/pre-ready-2575.json
- P5 跳过原因（原文）：仓库根无 go.mod；单 Issue gate = go build ./... + 定向测试（R2）；全量回归由批次 gate 承担
- Result：见 docs/evidence/2575/pre-ready-2575.json（RESULT: PASS）

## Evidence Contract

| 行 | 结果 | 产物 |
|---|---|---|
| R1 | PASS | docs/evidence/2575/R1-reject-guards.txt |
| R2 | PASS | docs/evidence/2575/R2-reject-behavior.txt |
| R3 | PASS | docs/evidence/2575/R3-doc-contract.txt |
| R4 | PASS | docs/evidence/2575/pre-ready-2575.json |

汇总账本：docs/evidence/2575/evidence.json

## Dependency Order / 验收规则

- 依赖：无（#2543 已 done 关闭；work_deps.sh 2575 返回 PROCEED）。
- 本单未修改任何验收工具/规则（pre-ready-check.sh 与 evidence-artifacts.sh 及 matrix_parser.sh 均未改动）；只新增本单证据生成器 scripts/verify-2575.sh。
- 语义翻转核对：既有 TestIAMIntegration_ConfirmationCallback_Reject（fixture status=waiting）同批 PASS，未翻转；仓库内无「confirmed / 已过期会话可被 reject」的正向断言。

## 同类路径对称扫描

- grep -rn '"rejected"' --include=*.go backend（排除 _test.go）：confirmation_sessions 的状态写入点只有本单修改的两处（handler 条件更新 + 回调条件更新）；其余 rejected 命中属独立域（人脸采集与审核、员工邀请、证件审核、报价、支付分），不含确认会话状态机，未改。
- SMSCallback 查询已含 status = 'waiting'（天然守卫）→ 按计划不改。

## 人工/部署侧

- 无（本单不涉及部署动作、迁移或人工回执）。

## Remote

- remote: origin/issue-2575@095a753ed; fix=bc778f5e3 pushed=yes; evidence=12e1b84b7 pushed=yes; gate-report=095a753ed pushed=yes

*Model: DeepSeek V4.1 Flash 1017*
