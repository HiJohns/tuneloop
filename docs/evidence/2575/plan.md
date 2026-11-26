## Accepted Plan — #2575：确认会话 reject 守卫（confirmed / 已过期 / 非 waiting 不得改写）+ api.md §2.8.3 契约对齐

### 背景与现状核对（develop `68d2f4d0a` 只读核对）

| 事实 | 位置 |
|---|---|
| `Reject` 只按 `id + tenant_id + token` 取会话后直接 `session.Status = "rejected"; db.Save(&session)`，**无 status 守卫、无 expires_at 守卫**（实测 `REJECT_SAVE=1`、`REJECT_EXPIRY=0`） | `backend/handlers/confirmation_session.go:214-254` |
| `Confirm` 守卫顺序：先过期 → `400 + 40002`；再 `confirmed` 幂等 → `200 + 20000`；最后才写状态并执行动作 | `backend/handlers/confirmation_session.go:180-200` |
| `IAMConfirmationCallback` 的 `reject` 分支同样无守卫：`session.Status = "rejected"; db.Save(&session)` | `backend/handlers/confirmation_session.go:313-319` |
| `Create` 是唯一建会话入口；`confirm` / `reject` 均在 `authRequired` 组；`/iam/confirmation-callback` 无 JWT | `backend/main.go:307-310`、`:316` |
| 状态集合（模型 + 表文档）：`waiting` / `confirmed` / `rejected` / `failed` / `expired`（`expired` 由 IAM 24h 超时标记） | `backend/models/models.go:1170-1190`、`docs/spec/database/database.md:800-806` |
| api.md 只到 §2.8.1；**无 §2.8.2 / §2.8.3**（实测 `2.8.2_HITS=0`、`2.8.3_HITS=0`） | `docs/spec/api/api.md:795-832`（§2.8 段止于 `## 三、` 之前） |
| 仓库内**无**前端调用 `confirmation-sessions`（实测 `FRONT_CALLERS=0`）→ 本单无 UI 变更面 | `frontend-pc/src`、`frontend-mobile/src` |
| 既有回调 reject 用例只用 `status=waiting` 会话（语义不变，不会被本单翻转） | `backend/handlers/iam_integration_test.go:236-276` |
| 错误码 `40002` / `40901` 已存在于 PC + mobile `ERROR_CODE_MAP`（无需新增码/文案） | `frontend-pc/src/services/errorMessages.js:15,31`、`frontend-mobile/src/services/errorMessages.js:16,32` |

### 变更点

1. **契约先行（Core Update）**：`docs/spec/api/api.md` 在 §2.8.1 之后、`## 三、` 之前新增
   - `### 2.8.2 确认执行`：记录 `Confirm` **既有**行为（过期 `40002`、`confirmed` 幂等 `20000`），不改行为；
   - `### 2.8.3 拒绝`：本单语义（守卫顺序、HTTP/code、幂等、回调分支），并写明「`rejected` 只是会话状态标签，**不回收**已执行的 IAM 绑定 / 客户权限写入或本地 `site_members` 写入」。
2. **唯一写入口（条件更新 / CAS）**：`Reject` 改为
   - 先按 `id + tenant_id + token` 读取会话（仅用于 404 / token 校验）；
   - 唯一写入 = 条件更新 `WHERE id=? AND tenant_id=? AND token=? AND status='waiting' AND expires_at > now` → `RowsAffected==1` 成功返回 `200 + 20000`；
   - `RowsAffected==0` **必定不写**，改为**再读一次**（post-check）按守卫分类：过期 → `400/40002`；`rejected` → `200/20000` already rejected（幂等）；`confirmed` → `409/40901`；其它（`failed`/`expired`/未知/空）→ `409/40901`。
   - 删除 `Reject` 内的 `db.Save(&session)`（不再有「读-判-写」交错窗口，`Confirm` 与 `Reject` 并发时不会互相覆盖）。
3. **回调分支同口径**：`IAMConfirmationCallback` 的 `case "reject"` 用同一 `status='waiting'` 条件更新（`status` 与 `message` 同一次 UPDATE 写入）；`RowsAffected==0` 时再读分类并**只重定向不写**：`confirmed` → `/confirmation-result?status=error&message=already_confirmed`；`rejected` → `status=rejected`（幂等）；其它 → `status=error&message=not_rejectable`。
4. **新增测试** `backend/handlers/confirmation_session_reject_2575_test.go`（复用同包 `setupConfirmationSessionTest`），8 个用例：`waiting` 正例、`confirmed` 冲突不改写、`expired` 40002 不改写、`rejected` 幂等不改写、`failed` 冲突不改写、错误 token 404、回调 `confirmed` 不改写、回调 `waiting`/`failed` 分类。
5. **新增证据生成器** `scripts/verify-2575.sh`（r1–r4，artifact 首行写 provenance；`FIX_REF` 传入锚点）。
6. **证据账本**：`docs/evidence/2575/{plan.md,summary.md,changed-files.txt,evidence.json}`；`summary.md` 必须含（a）Work Summary、（b）**Verification Matrix 全行覆盖表**（P2：每行 `PASS`/`N/A`，禁止 `RISK`）、（c）P5 跳过原因原文「仓库根无 go.mod；单 Issue gate = `go build ./...` + 定向测试（R2）；全量回归由批次 gate 承担」。

不改：`Confirm` / `Create` / `Get` / `SMSCallback` / `executeAction` / 路由 / 模型 / 迁移 / 前端。

## Core Update

- 文档：`docs/spec/api/api.md`
  - 当前状态：§2.8 仅有 `### 2.8.1 查询确认会话`；无 §2.8.2 / §2.8.3；`reject` 端点与守卫在契约中完全缺失（`2.8.3_HITS=0`）。
  - 修改内容：新增 `### 2.8.2 确认执行`（`POST /api/confirmation-sessions/:id/confirm?token=` → 过期 `400/40002`、`confirmed` 幂等 `200/20000`、写 `status=confirmed` + `confirmed_at`）；新增 `### 2.8.3 拒绝`（`POST /api/confirmation-sessions/:id/reject {token}` → 守卫表：`expired_at` 已过 `400/40002`、`status=confirmed` `409/40901`、`status=rejected` 幂等 `200/20000`、其它非 `waiting` `409/40901`、`waiting` 且未过期 `200/20000` 且 `status → rejected`；回调 `result=reject` 的分类重定向与 `message` 值 `already_confirmed` / `not_rejectable`；`rejected` 不回滚已执行 IAM / 本地写入）。
  - 原因：`docs/spec/api/api.md` 是本项目 API 契约唯一权威（instructions §7.4）；当前端点无契约，且 #2543 分支曾写出与代码相反的说法。本单以「新增段落 = 实现行为」的形态落定，避免同一语义在代码与文档间继续漂移。

## Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 已 `confirmed`（动作已执行）的会话可被翻成 `rejected`：动作发生但记录显示被拒绝，审计/对账无法还原真实结果；已过期会话同样可被改写 | `backend/handlers/confirmation_session.go:214-254` |
| 2 | 触发条件（trigger） | 持原 `token` 调 `POST /api/confirmation-sessions/:id/reject`，会话 `status=confirmed` 或 `now > expires_at` | 同上（无任何守卫分支） |
| 3 | 根因（root cause） | 写状态前缺「状态 / 过期」判定，且唯一写入点是「读-判-写」`db.Save`：判定完全缺失 + 与 `Confirm` 并发时存在覆盖窗口 | `:214-254` vs `:180-200` |
| 4 | 测试盲点（test blind spot） | 既有用例只覆盖 `Confirm`（成功 / 错 token / 过期），`Reject` 与回调 reject 分支**零用例** | `backend/handlers/confirmation_session_test.go`（无 `Reject`）、`iam_integration_test.go:236-276`（仅 waiting） |
| 5 | 同类扫描（same-class paths） | 同文件 `IAMConfirmationCallback` 的 `reject` 分支同样无守卫（`rejected` 直接 `db.Save`）；`SMSCallback` 已按 `status='waiting'` 查询（天然守卫，不改）；`Confirm` 的失败不回滚属 #2543/#2542 | `:313-319`、`:418` |
| 6 | 修复层（fix layer） | 修在唯一写入口（handler 条件更新），不下沉到模型钩子、不改路由/表；文档同步到契约唯一权威 `docs/spec/api/api.md` | 本计划 R1–R3 |
| 7 | 红绿实证（red-green） | 红：`git show FIX^:backend/handlers/confirmation_session.go` 的 `Reject` 体内无 `ExpiresAt`、无状态守卫、含 `db.Save`（`REJECT_SAVE=1`、`REJECT_EXPIRY=0`）；绿：修后同函数含 CAS 谓词 + `RowsAffected` 分类且无 `db.Save` | R1（结构红绿）、R2（行为） |
| 8 | 防复发（prevention） | 结构断言锁死：`GREEN_CAS_PREDICATE=1`、`GREEN_SINGLE_WRITE_POINT=1`、`GREEN_POSTCHECK_REREAD=1`、`CAS_PREDICATES=2`、`NO_SAVE_IN_REJECT_PATHS=1`；行为断言锁死 5 个状态分支 + 3 个回调分支 | R1/R2 |
| 9 | 影响面（blast radius） | 仅 `Reject` + 回调 `reject` 分支；`Confirm`/`Create`/`Get`/`SMSCallback`/`executeAction` 不改；仓库内无前端调用方，无 UI 面 | `FRONT_CALLERS=0` |
| 10 | 未修边界（deferred） | `Confirm` 的 confirmed-先落库后执行 + 失败不回滚 → #2543/#2542（已立）；`executeAction` 的 IAM 主体域 → #2543（已立）；accept 分支守卫不改（`FirstOrCreate`/bind 幂等） | 本计划 Out of Scope |

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 reject 守卫结构 + 条件更新唯一写入口（红绿对照） | `backend/handlers/confirmation_session.go` | `POST /api/confirmation-sessions/:id/reject`、`GET /api/iam/confirmation-callback?result=reject` | 修前（`FIX^`）vs 修后 | backend | `RED_NO_EXPIRY_GUARD=1`、`RED_NO_STATUS_GUARD=1`、`RED_CALLBACK_NO_GUARD=1`、`GREEN_EXPIRY_GUARD=1`、`GREEN_CONFIRMED_40901=1`、`GREEN_IDEMPOTENT_REJECTED=1`、`GREEN_CAS_PREDICATE=1`、`GREEN_SINGLE_WRITE_POINT=1`、`GREEN_POSTCHECK_REREAD=1`、`GREEN_CALLBACK_CLASSIFY=1`、`CAS_PREDICATES=2`、`NO_SAVE_IN_REJECT_PATHS=1`；负例：修后仍存在 `db.Save(&session)` 于 reject 路径、或 classification 前先写状态 → FAIL | R1 |
| R2 行为用例（5 handler + 3 回调）+ 编译 | `backend/handlers/confirmation_session_reject_2575_test.go` | `POST .../reject`、`GET /api/iam/confirmation-callback` | `waiting` / `confirmed` / `expired` / `rejected` / `failed` / 错 token | backend | `BUILD_EXIT=0`、`CASE_WAITING=20000`、`CASE_WAITING_STATUS=rejected`、`CASE_CONFIRMED=40901`、`CASE_CONFIRMED_STATUS=confirmed`、`CASE_EXPIRED=40002`、`CASE_EXPIRED_STATUS=waiting`、`CASE_REJECTED_IDEMPOTENT=1`、`CASE_FAILED=40901`、`CASE_BAD_TOKEN=404`、`CB_CONFIRMED=already_confirmed`、`CB_CONFIRMED_STATUS=confirmed`、`CB_WAITING_STATUS=rejected`、`CB_FAILED=not_rejectable`、`NO_ACTION_EXECUTED=1`；负例：任一状态被改写或动作被执行 → FAIL | R2 |
| R3 api.md §2.8.2 / §2.8.3 契约与实现一致 | `docs/spec/api/api.md` | `docs/spec/api/api.md` §2.8 | 修后文档 | doc | `DOC_282=1`、`DOC_283=1`、`DOC_EXPIRED_40002=1`、`DOC_CONFIRMED_40901=1`、`DOC_IDEMPOTENT_REJECTED=1`、`DOC_NO_ROLLBACK=1`、`DOC_CALLBACK_ALREADY_CONFIRMED=1`、`DOC_CALLBACK_NOT_REJECTABLE=1`；负例：文档仍称已确认/已过期会话会被改写、或缺「不回收」语句 → FAIL | R3 |
| R4 pre-ready gate（四源一致 + 证据完整 + P13 已推送） | `docs/evidence/2575/pre-ready-2575.json` | `pre-ready-check.sh 2575` | 修后干净树 + 最新 Accepted Plan | backend | `GATE_RESULT=PASS`、`jq -e '.result == "PASS"'`、每条合同行 `pass=true`、`dirty=false`；负例：任一 P 项失败 / evidence 缺行 / 未推送 → gate 非 0，禁止 ready | R4 |

## Input Truth & Tool Preflight

- **Source of truth**：
  - 行为契约 = `backend/handlers/confirmation_session.go` 的 `Confirm`（`:180-200`）与 `Reject`（`:214-254`）现存分支顺序；
  - 状态集合 = `backend/models/models.go:1170-1190` + `docs/spec/database/database.md:800-806`（`waiting/confirmed/rejected/failed/expired`）；
  - 错误码 = 仓库既有 `40002` / `40901`（不复用其它语义、不新增码）；
  - 文档权威 = `docs/spec/api/api.md` §2.8（唯一 owner，禁止另建第二份契约）。
- **差异核对（/analyze 只读实测）**：
  - `sed -n '214,254p' backend/handlers/confirmation_session.go | grep -c 'db.Save'` → `1`；`grep -c 'ExpiresAt'` → `0`（守卫完全缺失）；
  - `grep -c '2\.8\.3' docs/spec/api/api.md` → `0`；`grep -c '2\.8\.2'` → `0`（段落不存在，非「需要修一句话」）；
  - `grep -rn 'confirmation-sessions' frontend-pc/src frontend-mobile/src | wc -l` → `0`（无前端调用方，故无 UI/文案映射面）；
  - `grep -rn 'confirmation' backend/handlers/confirmation_session_test.go` → 仅 `Create`/`Get`/`Confirm` 用例，`Reject` 零覆盖（新增用例不是重复覆盖）；
  - `sed -n '314,316p' backend/handlers/confirmation_session.go` → 回调 reject 分支三行直写（无守卫）。
- **损坏输入拒绝规则**：
  - 会话 `status` 为空串/未知值 → 归入「非 waiting」→ `409/40901`，绝不写（脏数据不得被「修好成 rejected」）；
  - `expires_at` 为零值（`0001-01-01`）→ 视为已过期 → `40002`（不允许把无过期字段的行当成永久有效）；
  - `token` 不匹配 / 会话不存在 → `404/40400`（不泄漏「存在但状态不符」）；回调找不到会话 → 既有 `session_not_found` 重定向，不改。
- **工具盲区声明**：
  - `scripts/verify-2575.sh` 为**文本级**结构断言（函数体切片 + 计数），不能证明运行时并发行为；并发安全由「唯一写入口 = 单条条件 UPDATE」这一结构事实表达，并由 R2 的「状态未被改写」行为断言兜底。
  - 条件更新后的 post-check 重读分支在串行测试中由「CAS 不命中」路径覆盖（`confirmed`/`expired`/`rejected`/`failed` 用例均走该分支），真实并发交错无法在单测中确定性复现 —— 属已知盲区，不假装已覆盖。
  - 红样依赖 `git show FIX^:backend/handlers/confirmation_session.go`（脚本内 `$(git rev-parse "$FIX_REF^")`，禁止手写 sha）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 语义取 Issue 的**方案 1 严格态**：仅 `status='waiting'` 可转 `rejected`；`rejected` 走幂等 `200/20000`（不做「confirmed 幂等提示」，`confirmed` 必须 4xx） | Architect | 本计划锁定（Issue「期望」二选一，方案 1 + 幂等子项） | 阻塞实现；若要改成方案 2 → 回 Architect 重写矩阵 |
| D2 | 守卫顺序：**先过期**（`40002`）**后状态**（`40901`），与 `Confirm` 对称；`confirmed` 且已过期 → `40002` | Architect | 本计划锁定（以 `Confirm:180-184` 为对称基准） | 阻塞实现 |
| D3 | 错误码：过期 `400/40002`（复用 `Confirm`），非 waiting `409/40901`（Issue 建议码）；message 为英文机器串，不新增码 | Architect | 本计划锁定 | 若 Owner 要求新码 → 回 Architect 并同步 PC/mobile `ERROR_CODE_MAP` |
| D4 | `failed` 不可转 `rejected`（`failed` 可能已含部分 IAM 写入，禁止被 `rejected` 覆盖）；`failed` 处置为 re-confirm 或人工清理 | Architect | 本计划锁定 | 阻塞实现 |
| D5 | 回调 `reject` 分支**不校验** `expires_at`：IAM 是确认事实权威，且 reject 不执行动作；但**必须**守卫 `confirmed`（本单 AC）与 `failed`/`expired`（统一分类） | Architect | 本计划锁定（理由见上） | 阻塞实现 |
| D6 | `rejected` 只是会话状态标签：**不回收**已执行的 IAM `BindUserToOrganization` / `SetUserCustomerPermissions` 或本地 `site_members` 写入；撤销须走 IAM 显式解绑（非本接口语义） | Architect | 本计划锁定，并在 api.md §2.8.3 明写 | 阻塞实现 |
| D7 | 无 UI / 无前端映射变更：仓库内无调用方（`FRONT_CALLERS=0`），`40002`/`40901` 已在两张 `ERROR_CODE_MAP` | Architect | 本计划锁定（实测为据） | 若新增前端调用方 → 另立 Issue |
| D8 | 并发口径：唯一写入口 = 条件 UPDATE（`status='waiting' [AND expires_at > now]`）；`RowsAffected==0` 只允许重读分类，禁止写 | Architect | 本计划锁定（消除读-判-写覆盖窗口） | 阻塞实现 |
| D9 | 与未合入的 #2543 分支**不设阻塞依赖**：本单在 develop 上自洽；若 #2543 先合入，其 §2.8.0–2.8.3 与本单冲突处以**本单 §2.8.3 语义为准**（合并时手工对齐，代码以本单为唯一写入口） | Architect | 本计划锁定 | 合并冲突时回 Architect 裁定 |
| D10 | gate 口径按 #2651：单 Issue 不跑全量 `go test ./...`（仓库根无 `go.mod`，`pre-ready` 的 P5 自动探测亦不适用）。R4 用 `--no-full-gate`，Summary 必须原样写明「仓库根无 go.mod；单 Issue gate = `go build ./...` + 定向测试（R2）；全量回归由批次 gate 承担」；全量 gated 证据由批次 gate 产出，不在本单 | Architect | 本计划锁定（#2651 吞吐规则） | 若 Owner 要求本单跑全量 → 回 Architect 并补 `### Batch Gate Reference` |

## Out of Scope

- `Confirm` 的「先落 `confirmed` 再执行动作、失败不回滚」→ **#2543 / #2542**（均已立 Issue，不夹带）。
- `executeAction` 的 IAM 主体域（本地 `users.id` 当作 IAM 主体）→ **#2543**（已立 Issue）。
- `SetUserCustomerPermissions` 失败静默降级 → **#2542**（已立 Issue，当前 `status:holdon`）。
- `IAMConfirmationCallback` 的 `accept` 分支识别/过期守卫 → 不改：IAM 回调为确认事实权威，且 `executeAction` 对 `site_members` 是 `FirstOrCreate`、对 IAM bind 幂等，重复 accept 无写入放大。
- `SMSCallback` → 不改：其查询已含 `status = 'waiting'`（`:418`），天然具备守卫。
- `docs/cases/organization.md:326` 的 `confirmation_sessions(waiting) → cancelled`（合并账号场景的 `cancelled` 状态）→ 归合并账号 Issue，本单不新增该状态值。
- `confirmation_sessions` 建表迁移 / 启动校验豁免（`backend/database/db.go`）→ #2543 范围，本单不动。
- 前端 UI、错误文案映射、`ERROR_MESSAGE_MAP` → 无仓库内调用方（D7）。
- 迁移、模型字段、路由、审日志（`audit_logger`）→ 不改。

## G0 Golden Acceptance

| 行 ID | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） | assertions（含负例） |
|---|---|---|---|---|
| R1 | `FIX^`（修前）与 `FIX`（修后）两个版本的 `backend/handlers/confirmation_session.go`，按 `Reject` / 回调 `case "reject"` 函数体切片 | 生产入口相同（同一路由），只换版本 | `docs/evidence/2575/R1-reject-guards.txt` | `RED_NO_EXPIRY_GUARD=1`、`RED_NO_STATUS_GUARD=1`、`RED_CALLBACK_NO_GUARD=1`、`GREEN_EXPIRY_GUARD=1`、`GREEN_CONFIRMED_40901=1`、`GREEN_IDEMPOTENT_REJECTED=1`、`GREEN_CAS_PREDICATE=1`、`GREEN_SINGLE_WRITE_POINT=1`、`GREEN_POSTCHECK_REREAD=1`、`GREEN_CALLBACK_CLASSIFY=1`、`CAS_PREDICATES=2`、`NO_SAVE_IN_REJECT_PATHS=1`；负例：修后 reject 路径仍含 `db.Save(&session)`、或 CAS 前先赋 `session.Status` → `R1_FAIL` 非 0 退出 |
| R2 | 5 个 handler fixture（`waiting` 未过期 / `confirmed` / `expired` / `rejected` / `failed` / 错 token）+ 3 个回调 fixture（`confirmed` / `waiting` / `failed`），均带本地 `users` 行与 tenant/site FK 种子 | `POST /api/confirmation-sessions/:id/reject`、`GET /api/iam/confirmation-callback?result=reject`（httptest 直调 handler） | `docs/evidence/2575/R2-reject-behavior.txt` | `BUILD_EXIT=0`、`CASE_WAITING=20000`、`CASE_WAITING_STATUS=rejected`、`CASE_CONFIRMED=40901`、`CASE_CONFIRMED_STATUS=confirmed`、`CASE_EXPIRED=40002`、`CASE_EXPIRED_STATUS=waiting`、`CASE_REJECTED_IDEMPOTENT=1`、`CASE_FAILED=40901`、`CASE_BAD_TOKEN=404`、`CB_CONFIRMED=already_confirmed`、`CB_CONFIRMED_STATUS=confirmed`、`CB_WAITING_STATUS=rejected`、`CB_FAILED=not_rejectable`、`NO_ACTION_EXECUTED=1`；负例：任一「应不变」的状态被改写、或 `no_action_executed` 断言失败 → `R2_FAIL` 非 0 退出 |
| R3 | 修后 `docs/spec/api/api.md`（§2.8.2/§2.8.3 段落） | 契约文档（人工审计入口） | `docs/evidence/2575/R3-doc-contract.txt` | `DOC_282=1`、`DOC_283=1`、`DOC_EXPIRED_40002=1`、`DOC_CONFIRMED_40901=1`、`DOC_IDEMPOTENT_REJECTED=1`、`DOC_NO_ROLLBACK=1`、`DOC_CALLBACK_ALREADY_CONFIRMED=1`、`DOC_CALLBACK_NOT_REJECTABLE=1`；负例：§2.8.3 缺「不回收」语句或未写 `40901`/`40002` → `R3_FAIL` 非 0 退出 |
| R4 | 修后树 + 最新 Accepted Plan 评论 + `docs/evidence/2575/evidence.json` + 已推送 `origin/issue-2575` | `pre-ready-check.sh 2575` | `docs/evidence/2575/pre-ready-2575.json` | `GATE_RESULT=PASS`、`jq -e '.result == "PASS"'`、每行 `pass=true`、`dirty=false`、P13 提交在 `origin/issue-2575`；负例：任一 P 项失败 / 合同缺行 → gate 非 0，禁止 ready |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2575.sh r1 > docs/evidence/2575/R1-reject-guards.txt` | `docs/evidence/2575/R1-reject-guards.txt` | exit 0；`grep -q '^R1_OK$'`；`grep -q 'GREEN_CAS_PREDICATE=1'`；`grep -q 'NO_SAVE_IN_REJECT_PATHS=1'` | `RED_NO_EXPIRY_GUARD=1`、`RED_NO_STATUS_GUARD=1`、`RED_CALLBACK_NO_GUARD=1`、`GREEN_EXPIRY_GUARD=1`、`GREEN_CONFIRMED_40901=1`、`GREEN_IDEMPOTENT_REJECTED=1`、`GREEN_SINGLE_WRITE_POINT=1`、`GREEN_POSTCHECK_REREAD=1`、`GREEN_CALLBACK_CLASSIFY=1`、`CAS_PREDICATES=2` | 修后 reject 路径仍含 `db.Save`、无 CAS 谓词、或回调分支缺分类 → `R1_FAIL` 非 0 退出 | dev |
| R2 | `bash scripts/verify-2575.sh r2 > docs/evidence/2575/R2-reject-behavior.txt` | `docs/evidence/2575/R2-reject-behavior.txt` | exit 0；`grep -q '^R2_OK$'`；`grep -q 'BUILD_EXIT=0'`；`grep -q 'CASE_CONFIRMED=40901'`；`grep -q 'NO_ACTION_EXECUTED=1'` | 8 用例全跑（`CASE_*` 5 例 + `CB_*` 3 例）；`CASE_CONFIRMED_STATUS=confirmed`、`CASE_EXPIRED_STATUS=waiting`、`CASE_WAITING_STATUS=rejected`、`CASE_REJECTED_IDEMPOTENT=1`、`CASE_BAD_TOKEN=404` | 任一守卫缺失导致状态被改写 / 动作被执行 / 用例 FAIL → `R2_FAIL` 非 0 退出 | dev |
| R3 | `bash scripts/verify-2575.sh r3 > docs/evidence/2575/R3-doc-contract.txt` | `docs/evidence/2575/R3-doc-contract.txt` | exit 0；`grep -q '^R3_OK$'`；`grep -q 'DOC_NO_ROLLBACK=1'`；`grep -q 'DOC_CONFIRMED_40901=1'` | `DOC_282=1`、`DOC_283=1`、`DOC_EXPIRED_40002=1`、`DOC_IDEMPOTENT_REJECTED=1`、`DOC_CALLBACK_ALREADY_CONFIRMED=1`、`DOC_CALLBACK_NOT_REJECTABLE=1` | 文档缺 §2.8.3、缺 `40901`/`40002`、或缺「不回收已执行 IAM 写入」→ `R3_FAIL` 非 0 退出 | dev |
| R4 | `/home/coder/.config/opencode/scripts/pre-ready-check.sh 2575 --plan-file docs/evidence/2575/plan.md --summary-file docs/evidence/2575/summary.md --changed-files docs/evidence/2575/changed-files.txt --evidence-file docs/evidence/2575/evidence.json --no-full-gate --report docs/evidence/2575/pre-ready-2575.json` | `docs/evidence/2575/pre-ready-2575.json` | exit 0；`GATE_RESULT=PASS`；`jq -e '.result == "PASS"'`；`jq -e 'any(.checks[]; .id == "P5" and .status == "N/A")'` | 每条合同行 `pass=true`、`dirty=false`、P5 为 `N/A`（`--no-full-gate` + Summary 记录原因）、P13 `origin/issue-2575` 已含 fix+证据、P12 合同行齐全 | 任一 P 项失败 / evidence.json 缺行 / `dirty=true` / 未推送 / P5 无记录原因 → gate 非 0，禁止 ready | dev |

### Files（变更文件清单）

| 类别 | 路径 |
|---|---|
| Handler（守卫 + CAS） | `backend/handlers/confirmation_session.go` |
| 测试（新增） | `backend/handlers/confirmation_session_reject_2575_test.go` |
| 契约文档（Core Update） | `docs/spec/api/api.md` |
| 证据生成器（新增） | `scripts/verify-2575.sh` |
| 证据（计划快照） | `docs/evidence/2575/plan.md` |
| 证据（摘要） | `docs/evidence/2575/summary.md` |
| 证据（改动清单） | `docs/evidence/2575/changed-files.txt` |
| 证据（合同汇总） | `docs/evidence/2575/evidence.json` |
| 证据 R1 | `docs/evidence/2575/R1-reject-guards.txt` |
| 证据 R2 | `docs/evidence/2575/R2-reject-behavior.txt` |
| 证据 R3 | `docs/evidence/2575/R3-doc-contract.txt` |
| Gate 报告 | `docs/evidence/2575/pre-ready-2575.json` |

## Artifact Provenance

- 生成器：`scripts/verify-2575.sh`（r1–r3）；每个文本 artifact 首行必须为
  `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`，锚定本单 fix commit（`FIX_REF` 传入；`dirty` 计算忽略 `docs/evidence/2575/` 自churn）。
- 逐条清单（全部位于 `docs/evidence/2575/`，均需带上述首行）：
  - `docs/evidence/2575/R1-reject-guards.txt`（修前/修后函数体结构断言输出）
  - `docs/evidence/2575/R2-reject-behavior.txt`（`cd backend && go build ./...` + `go test ./handlers -run 'TestRejectConfirmationSession_2575|TestIAMCallbackReject_2575' -count=1` 输出与标记）
  - `docs/evidence/2575/R3-doc-contract.txt`（`docs/spec/api/api.md` §2.8.2/§2.8.3 契约标记）
  - `docs/evidence/2575/pre-ready-2575.json`（gate 报告，JSON 自带 `.provenance.{build,commit,dirty}`）
- `docs/evidence/2575/evidence.json`：`build` 形如 `1.0.<7-40 位短码>`、`commit` 为 40 位 sha、`dirty:false`。
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$
- 证据重生成纪律：fix commit 之后统一重跑 `r1`→`r3`，禁止只改 `evidence.json` 的 `build/commit` 冒充新证据。

### Workload

- Size: M
- Basis: 3 个产品/文档文件（handler + 新增测试 + api.md）+ 1 个证据生成器；4 条证据行；无 migration / 无跨仓 / 无外部依赖
- Confidence: high

## Dependencies

- 无

## 语义翻转核对（旧断言是否需翻转）

| 既有断言 | 位置 | 结论 |
|---|---|---|
| 回调 `result=reject` → `status=rejected`（fixture 为 `status=waiting`） | `backend/handlers/iam_integration_test.go:236-276` | **保持**：`waiting` 仍是唯一可转 `rejected` 的状态，断言继续成立（不翻转） |
| 回调 `result=failed` → `status=failed` | `backend/handlers/iam_integration_test.go:596` | **保持**：本单只改 `reject` 分支 |
| `Confirm` 成功 / 错 token / 过期三例 | `backend/handlers/confirmation_session_test.go:242-389` | **保持**：`Confirm` 代码不改 |
| 全仓是否存在「`confirmed`/已过期会话可被 reject」的正向断言 | `grep -rn "Reject" backend/handlers/*_test.go`（仅命中无确认会话版本） | **0 处**：无与新语义相反的旧断言需要翻转，也无兼容口径需要保留 |

*Model: DeepSeek V4.1 Flash 1017*

