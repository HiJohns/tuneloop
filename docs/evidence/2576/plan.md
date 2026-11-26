## Accepted Plan — #2576 [Bug] 确认会话 Confirm 缺 status 守卫：rejected/已过期会话可被改写为 confirmed 并执行动作

> **裁定摘要（Architect）**：Issue 正文的「二选一」由本计划裁定为 **Option 2（白名单显式重试语义）**：
> 唯一可翻转为 `confirmed` 的会话状态集合 = **`{waiting, failed}`**；`confirmed` 幂等只读、`rejected` 显式拒绝不可翻回、`expired`/未知/空一律 `409 40901` 且不写状态不执行动作。
> 理由：`failed` 的唯一来源是「IAM 已确认但动作执行失败」（`IAMConfirmationCallback` 的 `accept` 失败分支）——若禁止重试，用户已完成的确认事实被永久丢弃，只能靠人工重建会话；`rejected` 是用户的显式否决，任何路径不得翻回（这是本 Issue 的核心缺陷）。
> 配套（D4）：`Confirm` 自身动作失败时**必须回写 `status=failed` + `message`**，否则「`failed` 可重试」在 handler 侧不可达（现状是失败后停在 `confirmed`，重试命中的是幂等分支 → 动作永不执行，属静默失效）。
> 实现沿用 #2575 已定型的**条件更新（CAS）唯一写入口 + `RowsAffected=0` 只重读分类**，避免「读到 waiting → 并发双重执行」。本单不改 `Reject` / `SMSCallback` / `Create` / `Get` / `executeAction` 的动作语义。

### 背景与根因（代码为据）

| 事实 | 位置 |
|---|---|
| `Confirm` 只有「过期」与「`confirmed` 幂等」两道守卫，随后**无条件** `session.Status = "confirmed"` + `db.Save` + `executeAction` | `backend/handlers/confirmation_session.go:180-206` |
| `db.Save(&session)` 是**整行写回**，`rejected` / `failed` / `expired` 会被直接覆盖为 `confirmed` | 同上 `:194-200` |
| 回调 `accept` 分支同样无条件写 `confirmed` 并执行动作（`rejected` 会被翻回） | 同上 `:290-311` |
| 回调 `accept` 对**已 confirmed** 的会话会**重复执行动作**（重复 IAM bind / 重复本地写） | 同上 `:289-308`（无 status 分类） |
| `Confirm` 动作失败后**不回滚状态**（停在 `confirmed`），重试命中「already confirmed」→ 动作永不执行 | 同上 `:197-206` |
| 会话状态集合（含 `expired`）权威定义 | `docs/spec/database/database.md:787,795-800` |
| 当前契约只写 `Confirm` 的**既有**行为，并把本缺口 deferred 到已关闭的 #2543 / 不相关的 #2542 | `origin/issue-2575:docs/spec/api/api.md` §2.8.2 注（develop 尚无 §2.8.2） |
| 路由已注册（`authRequired`），回调无鉴权（IAM 重定向） | `backend/main.go:309`、`:316` |
| 仓库内无前端调用方（`confirmation-sessions` 0 命中）；`40901`/`40002` 两端 `ERROR_CODE_MAP` 已存在 | `grep -rn confirmation-sessions frontend-pc/src frontend-mobile/src` = 0；`frontend-pc/src/services/errorMessages.js:31` |

**根因**：`Confirm` / 回调 `accept` 都用「读到什么就写什么」的整行 `Save`，没有「会话状态是否允许转移到 `confirmed`」这一唯一判定，也没有把判定压进写操作本身（非 CAS）。因此任何非 `waiting` 的既有状态（用户拒绝、失败、过期、未知）都会被静默改写并执行授权动作。

### 变更点

1. **`backend/handlers/confirmation_session.go`（唯一判定源）**：新增包级白名单与谓词，全文件只此一处定义可确认状态：

   ```go
   // confirmableStatuses 是「可被翻转为 confirmed」的唯一白名单（#2576 D1）：
   // waiting = 等待确认；failed = IAM 已确认但动作失败，允许重试。
   var confirmableStatuses = map[string]bool{"waiting": true, "failed": true}

   // ConfirmableStatuses 返回白名单切片（供 CAS 谓词使用，禁止在调用点写字面量）。
   func ConfirmableStatuses() []string { ... }

   // IsConfirmableStatus 报告会话状态是否允许转移到 confirmed。
   func IsConfirmableStatus(status string) bool { return confirmableStatuses[status] }
   ```

2. **`Confirm` 改为 CAS 唯一写入口 + 事后分类**（删除 `db.Save(&session)`）：
   - 读会话（`id + tenant_id + token`）→ 404 不变；
   - `res := db.Model(&models.ConfirmationSession{}).Where("id = ? AND tenant_id = ? AND token = ? AND status IN ? AND expires_at > ?", id, tenantID, token, ConfirmableStatuses(), now).Updates(map[string]any{"status": "confirmed", "confirmed_at": now})`；
   - `res.RowsAffected == 1` → `executeAction`；失败则**第二次写**（`status=failed` + `message=Action execution failed: …`）并返回 `500 50000`（可重试，D4）；成功返回 `200 20000 successfully confirmed and action executed`；
   - `res.RowsAffected == 0` → **只重读分类、不写**：`now > expires_at` → `400 40002 confirmation link has expired`；`status=confirmed` → `200 20000 already confirmed`；`status=rejected` → `409 40901 session already rejected`；其它（`expired`/未知/空）→ `409 40901 session is not confirmable`（判定顺序即文档表顺序，D3）。
3. **`IAMConfirmationCallback` 的 `case "accept"` 同口径**：同一 CAS 谓词但**不含 `expires_at`**（IAM 是确认事实权威，D5）；`RowsAffected=0` 只重读重定向：`confirmed` → `/confirmation-result?status=error&message=already_confirmed`（**不重复执行动作**）；`rejected` 与其它 → `?status=error&message=not_confirmable`；成功 → `?status=success&action=<action_type>`；动作失败 → 写 `failed` 后 `?status=error&message=<err>`（现状保持）。
4. **`backend/handlers/confirmation_session_confirm_2576_test.go`（新增）**：handler 与回调两组用例，覆盖 `waiting` / `failed` / `confirmed` / `rejected` / `expired` / 未知 / 过期时间 / 错 token；负例断言「状态未变」「动作未执行（`site_members` 增量 0）」「IAM 调用 0 次」（复用同包 `installFakeIAM2540` 的 `snapshot()`）；用例名均含 `2576`，可用 `-run '2576'` 一次选中（不使用 `|` 交替）。
5. **`scripts/verify-2576.sh`（新增）**：5 个模式：`r1` 结构红绿 / `r2` handler 行为 / `r3` 回调行为 / `r4` 文档契约 / `affected`（`go build ./...` + `go test ./handlers -run '2576'`，供 pre-ready `--affected-cmd` 复用，避免合同命令内联 `go` 命令）；首行 provenance 由 `git` 派生（公式同 `scripts/verify-2575.sh`，锚定 fix commit）。
6. **`docs/spec/api/api.md` §2.8.2（Core Update）**：以「替换或新建」方式落定 `Confirm` 守卫表（含 `failed` 可重试、`40901` 分类、失败回写 `failed`），新增回调 `accept` 分类表，并**删除/替换 #2575 引入的过期 deferral 注**（其指向已 CLOSED 的 #2543 与不相关的 #2542）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 唯一判定源 + CAS 唯一写入口（结构红绿对照） | `backend/handlers/confirmation_session.go` | `bash scripts/verify-2576.sh r1` | 修前（`FIX_SHA^`）与修后函数体逐段文本对照 | backend | `RED_NO_STATUS_GUARD=1`、`RED_NO_CALLBACK_GUARD=1`、`GREEN_WHITELIST_DEFS=1`、`GREEN_WHITELIST_USES=2`、`GREEN_CAS_PREDICATE=1`、`GREEN_NO_FULL_ROW_SAVE=1`、`GREEN_POSTCHECK_REREAD=1`、`CALLBACK_CAS_NO_EXPIRY=1`、`NO_INLINE_WAITING_IN_CONFIRM=1` | R1 |
| R2 `Confirm` 状态矩阵行为（含拒绝/失败/过期/幂等/重试 + 零副作用） | `backend/handlers/confirmation_session_confirm_2576_test.go` | `POST /api/confirmation-sessions/:id/confirm?token=` | `waiting`/`failed`/`confirmed`/`rejected`/`expired`/未知 + 过期时间 + 错 token | backend（测试库 + fake IAM） | 正例 `CASE_WAITING=20000`、`CASE_FAILED_RETRY=20000`；幂等 `CASE_CONFIRMED_IDEMPOTENT=20000` + `CASE_CONFIRMED_NO_DUP=1`；负例 `CASE_REJECTED=40901`、`CASE_EXPIRED_FLAG=40901`、`CASE_UNKNOWN=40901`、`CASE_PAST=40002`、`CASE_BAD_TOKEN=404`、`CASE_REJECTED_NO_WRITE=1`、`CASE_REJECTED_IAM_CALLS=0`、`BUILD_EXIT=0`、`TEST_EXIT=0` | R2 |
| R3 IAM 回调 `accept` 分类行为（不得把 `rejected` 翻回，不得重复执行） | `backend/handlers/confirmation_session_confirm_2576_test.go` | `GET /api/iam/confirmation-callback?result=accept` | `waiting`/`failed`/`confirmed`/`rejected`/`expired` | backend（测试库 + fake IAM） | `CB_WAITING_STATUS=confirmed`、`CB_FAILED_STATUS=confirmed`、`CB_CONFIRMED=already_confirmed`、`CB_CONFIRMED_STATUS=confirmed`、`CB_CONFIRMED_NO_DUP=1`、`CB_REJECTED=not_confirmable`、`CB_REJECTED_STATUS=rejected`、`CB_EXPIRED_FLAG=not_confirmable`、`TEST_EXIT=0` | R3 |
| R4 契约文档与实现逐句一致（含过期 deferral 清除） | `docs/spec/api/api.md` | `bash scripts/verify-2576.sh r4` | 修后文档 + handler 源码文本 | docs | `DOC_SECTION_282=1`、`DOC_REJECTED_40901=1`、`DOC_FAILED_RETRY=1`、`DOC_EXPIRED_40002=1`、`DOC_CALLBACK_ALREADY_CONFIRMED=1`、`DOC_CALLBACK_NOT_CONFIRMABLE=1`、`DOC_STALE_DEFERRAL=0`、`DOC_2543_REF=0` | R4 |
| R5 pre-ready gate（四源一致 + 证据账本 + 已推送） | `docs/evidence/2576/evidence.json` | `~/.config/opencode/scripts/pre-ready-check.sh 2576` | 修后分支 `issue-2576`，fix + 证据均已推送 | backend | 报告 `overall=PASS`，`P12=PASS`（4 行 `pass=true`、`dirty=false`）、`P13=PASS`、`P14=PASS`、`P5=N/A`（原因见 Decision Lock D8） | R5 |

## Input Truth & Tool Preflight

- **Source of truth**：
  - 状态集合与语义 = `docs/spec/database/database.md:787,795-800`（`waiting`/`confirmed`/`failed`/`rejected`/`expired`）；
  - 守卫顺序与 `40901`/`40002` 码 = `origin/issue-2575` 已落定的 `Reject` 同源口径（先过期 `40002`，后状态 `40901`），本单只把 `Confirm` 侧补成对偶；
  - 白名单集合裁定 = 本计划 `## Decision Lock` D1。
- **差异核对（实施前先跑，只读）**：
  - `grep -n 'session.Status = "confirmed"' backend/handlers/confirmation_session.go` → 期望修前命中 `Confirm:194`、回调 `:291`、`SMSCallback:425`（第三处天然有 `status='waiting'` 查询守卫，本单不改）；
  - `grep -rn 'confirmation-sessions' frontend-pc/src frontend-mobile/src` → 期望 0（无 UI 面，故无 weapp/H5 可达性要求）；
  - `git merge-base --is-ancestor $(git rev-parse origin/issue-2575) origin/develop` → 预期 **否**（#2575 未合并 → develop 的 `api.md` 无 §2.8.2，按 D6「新建」处理）；
  - `gh issue view 2575 --json state,labels` → 记录 `rejected`（合并顺序只影响 `api.md` 段落，见 D6）。
- **损坏输入拒绝**：会话夹具若不显式设置 `status`（GORM 默认 `waiting`）→ 该用例不成立，判 FAIL；负例若只断言 HTTP 码而不校验「状态未变 + 动作未执行」→ 视为无效负例（本单要求三件套齐全）。
- **工具盲区**：`go test ./handlers -run '2576'` 只证明本单用例；`executeAction` 的 IAM 幂等性（重复 bind）由 fake IAM 计数覆盖，真实 IAM 冲突语义未在本单验证（见 Out of Scope）。
- **测试库**：沿用 `docs/ops/test-db-lifecycle.md`（`POSTGRES_HOST/PORT` + `TUNELOOP_TEST_DB` 可覆盖），与 `scripts/verify-2575.sh` 同一组默认值。

## Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 可翻转为 `confirmed` 的状态集合 | **`{waiting, failed}`**（Option 2）；`rejected`/`expired`/未知/空 → `409 40901` 且不写、不执行；`confirmed` 幂等只读。依据见裁定摘要 |
| D2 | 写入方式 | 条件更新（CAS）为**唯一写入点**：`WHERE id AND tenant_id AND token AND status IN ? AND expires_at > now`；`RowsAffected=0` 只重读分类；禁止整行 `db.Save` |
| D3 | 分类顺序 | 过期 `400/40002` → `confirmed` 幂等 `200/20000` → `rejected` `409/40901 session already rejected` → 其它 `409/40901 session is not confirmable`（与 #2575 `Reject` 表同构） |
| D4 | 动作失败语义 | `Confirm` 动作失败 → 回写 `status=failed` + `message` 并返回 `500 50000`（使 D1 的 `failed` 重试在 handler 侧可达，消除「confirmed 但动作未执行」的静默陷阱） |
| D5 | 回调 `accept` | 同一白名单；**不校验** `expires_at`（IAM 为确认事实权威）；`confirmed` → `already_confirmed` 且**不重复执行动作**；`rejected`/其它 → `not_confirmable` |
| D6 | `api.md` 与 #2575 的合并顺序 | 本单拥有 §2.8.2 的**最终内容**：若 #2575 先合并则**替换**其守卫表并删除过期 deferral 注；若本单先合并则**新建** §2.8.2（不新增 §2.8.3）。两序结果必须一致（R4 断言覆盖） |
| D7 | 前端 | 不改（仓库内 0 调用方；`40901` 两端已映射）。`40002` 文案为「信息不完整，请补充必填项」（`errorMessages.js:15/16`），因无调用方不构成用户可见缺陷 → 见 Out of Scope |
| D8 | 验证层级（#2651） | 单 Issue = `go build ./...`（全包编译）+ 定向测试（`go test ./handlers -run '2576'`）；全量 `go test ./...` 与全部测试包编译由批次 gate 承担，故 pre-ready `P5=N/A` 且原因写入 Work Summary |
| D9 | 验收工具/规则 | 不修改 `pre-ready-check.sh` / `matrix_parser.sh` / `plan-lint.sh` / `evidence-artifacts.sh`；`scripts/verify-2576.sh` 为本单私有生成器 |

### 既有断言核对（语义变更必查）

| 既有断言 | 旧期望 | 本单后 | 处理 |
|---|---|---|---|
| `TestConfirmationSessionHandler_Confirm/ConfirmConfirmationSession_Success` | `200` + `confirmed` + 用户 `active` + `site_members` 建立 | 不变（`waiting` 命中白名单） | 不翻转 |
| `TestConfirmationSessionHandler_Confirm/ConfirmConfirmationSession_Expired` | `400` + `40002` | 不变（过期优先） | 不翻转 |
| `TestConfirmationSessionHandler_Confirm/ConfirmConfirmationSession_InvalidToken` | `404` | 不变 | 不翻转 |
| `TestIAMIntegration_ConfirmationCallback`（`result=accept` + `waiting`） | `302 success` + `confirmed` + 用户 `active` | 不变 | 不翻转 |
| `TestIAMMock_ConfirmationCallback_Failed` / `_Reject` | 各自状态 | 不变（`failed`/`reject` 分支不改；`failed` 仅新增可确认语义） | 不翻转 |
| 任何断言「动作失败后仍停在 `confirmed`」 | —— | 本单改为 `failed` | `grep -rn 'failed to execute action' backend --include=*_test.go` = 0（无既有断言冲突，D4 属新增可观测行为） |

## Out of Scope

- `Reject` 守卫与回调 `reject` 分类：由 **#2575** 负责（本单不动其代码与 §2.8.3）。
- `executeAction` 内 `SetUserCustomerPermissions` 失败仅 `log.Printf` 后继续（`confirmation_session.go:368-370`）：由 **#2542** 负责（silent degrade），本单不改动作实现。
- `SMSCallback`：其查询已含 `status='waiting'`（天然守卫），**排除**在本单改动面之外；若后续统一为 `ConfirmableStatuses()` 需另立 Issue。
- `40002` 的用户面文案（「信息不完整，请补充必填项」与「链接已过期」不符）：无前端调用方，**排除**；出现调用方时另立 Issue。
- 真实 IAM 对重复 `BindUserToOrganization` 的幂等/冲突语义：本单只用 fake IAM 覆盖，**排除**真实环境验证。
- 存量脏数据（生产/预生产已停在 `confirmed` 但动作未执行的会话）盘点与修复：需 Owner 授权，**排除**。
- 前端/小程序任何改动与 `build:weapp`：无 UI 面，**排除**。

## Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 触发条件 | 持原 `token` 对 `status=rejected` / `expired` / `failed` 的会话调 `confirm`，或 IAM 对已 `rejected` 会话推 `accept` | `confirmation_session.go:180-206`、`:290-311` |
| 2 | 根因 | 状态转移没有唯一判定：读到即写整行 + 无条件 `executeAction`；判定与写入未合并为 CAS | 同上 `:194-200` |
| 3 | 为何长期未发现 | 既有用例只有 `waiting` 成功、`waiting` 过期、错 token 三态；`rejected`/`failed`/`expired` 三态在 `Confirm` 侧无用例，`api.md` 也只记录「既有行为」 | `confirmation_session_test.go:242-389` |
| 4 | 同类扫描 | 状态写入点仅三处：`Confirm`、回调 `accept`、`SMSCallback`（后者查询已带 `status='waiting'`）；`Reject` 侧由 #2575 覆盖 | `grep -n 'Status = "confirmed"'` |
| 5 | 防复发（代码） | 白名单 `confirmableStatuses` 单一定义 + `ConfirmableStatuses()` 供 CAS 使用；调用点禁止字面量 | 变更点 1–3、R1 |
| 6 | 防复发（测试） | `rejected`/`expired`/未知/过期 + 回调 `rejected` 全部负例，断言「状态未变 + 动作未执行 + IAM 0 次」 | 变更点 4、R2/R3 |
| 7 | 防复发（文档） | `api.md` §2.8.2 守卫表 + 回调分类表与实现逐句一致；清除指向 CLOSED Issue 的 deferral | 变更点 6、R4 |
| 8 | 假阴性反例 | 若任一非白名单状态被写成 `confirmed`、或 `confirmed` 幂等仍重复执行动作、或文档仍保留 `见 #2543` → R2/R3/R4 FAIL | R2/R3/R4 负例 |
| 9 | 回归面 | `waiting` 成功、过期 `40002`、错 token `404`、回调 `accept` 成功、`failed`/`reject` 回调分支语义不变 | 既有断言核对表、R2/R3 |
| 10 | 未修边界 | 真实 IAM 重复 bind 幂等、存量脏数据、`SetUserCustomerPermissions` 静默降级 → Out of Scope（#2542 在册） | Out of Scope |

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 修前 `FIX_SHA^` 与修后 `backend/handlers/confirmation_session.go` 函数体文本 | `bash scripts/verify-2576.sh r1` | `docs/evidence/2576/R1-confirm-guards.txt` | `RED_NO_STATUS_GUARD=1`、`RED_NO_CALLBACK_GUARD=1`、`GREEN_WHITELIST_DEFS=1`、`GREEN_WHITELIST_USES=2`、`GREEN_CAS_PREDICATE=1`、`GREEN_NO_FULL_ROW_SAVE=1`、`GREEN_POSTCHECK_REREAD=1`、`CALLBACK_CAS_NO_EXPIRY=1`、`NO_INLINE_WAITING_IN_CONFIRM=1`、`R1_OK` | 修前已带状态守卫（红例不成立）、或修后仍存在整行 `db.Save`、或回调 CAS 带上 `expires_at` → FAIL |
| R2 | 8 个会话夹具（`waiting`/`failed`/`confirmed`/`rejected`/`expired`/未知状态/过期时间/错 token）+ fake IAM | `bash scripts/verify-2576.sh r2` | `docs/evidence/2576/R2-confirm-behavior.txt` | `BUILD_EXIT=0`、`TEST_EXIT=0`、`CASE_WAITING=20000`、`CASE_WAITING_STATUS=confirmed`、`CASE_WAITING_ACTION=1`、`CASE_FAILED_RETRY=20000`、`CASE_CONFIRMED_IDEMPOTENT=20000`、`CASE_CONFIRMED_NO_DUP=1`、`CASE_REJECTED=40901`、`CASE_REJECTED_STATUS=rejected`、`CASE_REJECTED_NO_WRITE=1`、`CASE_REJECTED_IAM_CALLS=0`、`CASE_EXPIRED_FLAG=40901`、`CASE_UNKNOWN=40901`、`CASE_PAST=40002`、`CASE_BAD_TOKEN=404` | `rejected` 返回 `20000`、或 `rejected`/`expired` 状态被改写、或 `site_members` 增量为 1、或 fake IAM 调用数 >0 → FAIL |
| R3 | 回调 5 态夹具（`waiting`/`failed`/`confirmed`/`rejected`/`expired`） | `bash scripts/verify-2576.sh r3` | `docs/evidence/2576/R3-callback-behavior.txt` | `CB_WAITING_STATUS=confirmed`、`CB_FAILED_STATUS=confirmed`、`CB_CONFIRMED=already_confirmed`、`CB_CONFIRMED_STATUS=confirmed`、`CB_CONFIRMED_NO_DUP=1`、`CB_REJECTED=not_confirmable`、`CB_REJECTED_STATUS=rejected`、`CB_EXPIRED_FLAG=not_confirmable`、`TEST_EXIT=0` | `rejected` 被翻成 `confirmed`、或 `confirmed` 再次执行动作（`site_members`/IAM 计数增加）、或返回 `status=success` → FAIL |
| R4 | 修后 `docs/spec/api/api.md` §2.8.2 + handler 文本 | `bash scripts/verify-2576.sh r4` | `docs/evidence/2576/R4-doc-contract.txt` | `DOC_SECTION_282=1`、`DOC_REJECTED_40901=1`、`DOC_FAILED_RETRY=1`、`DOC_EXPIRED_40002=1`、`DOC_CALLBACK_ALREADY_CONFIRMED=1`、`DOC_CALLBACK_NOT_CONFIRMABLE=1`、`DOC_STALE_DEFERRAL=0`、`DOC_2543_REF=0`、`R4_OK` | 文档缺 `40901` 白名单行、或无 `failed` 重试语义、或仍含 `见 #2543`/`#2542` deferral → FAIL |
| R5 | 修后 `issue-2576` 分支 + `docs/evidence/2576/` 全部产物 | `bash scripts/verify-2576.sh affected` 作为 `pre-ready-check.sh 2576 --affected-cmd`（完整参数见 Evidence Contract R5） | `docs/evidence/2576/pre-ready-2576.json` | `jq -e '.overall=="PASS"'`；`P12`/`P13`/`P14` 均 PASS；`P5` 为 `N/A` 且 Work Summary 记录 D8 原因 | `overall` 非 PASS、或证据行 `pass=false`、或 `dirty=true`、或 fix commit 未推送 → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2576.sh r1 > docs/evidence/2576/R1-confirm-guards.txt` | `docs/evidence/2576/R1-confirm-guards.txt` | exit 0；含 `RED_NO_STATUS_GUARD=1`、`RED_NO_CALLBACK_GUARD=1`、`GREEN_CAS_PREDICATE=1`、`GREEN_WHITELIST_DEFS=1`、`GREEN_WHITELIST_USES=2`、`GREEN_NO_FULL_ROW_SAVE=1`、`GREEN_POSTCHECK_REREAD=1`、`CALLBACK_CAS_NO_EXPIRY=1`、`R1_OK` | PASS | 修前样本已含状态守卫，或修后仍有整行 Save / 回调 CAS 含 expires_at → FAIL | dev |
| R2 | `bash scripts/verify-2576.sh r2 > docs/evidence/2576/R2-confirm-behavior.txt` | `docs/evidence/2576/R2-confirm-behavior.txt` | exit 0；含 `BUILD_EXIT=0`、`TEST_EXIT=0`、`CASE_REJECTED=40901`、`CASE_REJECTED_STATUS=rejected`、`CASE_REJECTED_NO_WRITE=1`、`CASE_EXPIRED_FLAG=40901`、`CASE_FAILED_RETRY=20000`、`CASE_CONFIRMED_NO_DUP=1`、`CASE_PAST=40002` | PASS | `rejected` 返回 20000 或状态被改写或副作用非 0 → FAIL | dev |
| R3 | `bash scripts/verify-2576.sh r3 > docs/evidence/2576/R3-callback-behavior.txt` | `docs/evidence/2576/R3-callback-behavior.txt` | exit 0；含 `CB_REJECTED=not_confirmable`、`CB_REJECTED_STATUS=rejected`、`CB_CONFIRMED_NO_DUP=1`、`CB_WAITING_STATUS=confirmed`、`CB_FAILED_STATUS=confirmed`、`TEST_EXIT=0` | PASS | `rejected` 被翻回 `confirmed` 或已 `confirmed` 重复执行 → FAIL | dev |
| R4 | `bash scripts/verify-2576.sh r4 > docs/evidence/2576/R4-doc-contract.txt` | `docs/evidence/2576/R4-doc-contract.txt` | exit 0；含 `DOC_SECTION_282=1`、`DOC_REJECTED_40901=1`、`DOC_FAILED_RETRY=1`、`DOC_CALLBACK_NOT_CONFIRMABLE=1`、`DOC_STALE_DEFERRAL=0`、`DOC_2543_REF=0`、`R4_OK` | PASS | 文档保留旧 deferral 或缺少 40901/失败重试语义 → FAIL | dev |
| R5 | `~/.config/opencode/scripts/pre-ready-check.sh 2576 --summary-file tmp/pre-ready-summary-2576.md --changed-files tmp/changed-2576.txt --affected-cmd "bash scripts/verify-2576.sh affected" --evidence-file docs/evidence/2576/evidence.json --report docs/evidence/2576/pre-ready-2576.json` | `docs/evidence/2576/pre-ready-2576.json` | 报告 `.overall` 为 `PASS`，`P12`/`P13`/`P14` 均 `PASS`，`P5` 为 `N/A`（原因已记录） | PASS | `overall` 非 PASS 或证据行缺失/`pass=false` 或 fix commit 未推送 → FAIL | dev |

## Artifact Provenance

- R1–R4 均由 `scripts/verify-2576.sh` 生成，首行 provenance：`# build=1.0.<fix 短码> # commit=<fix 40 位 sha> # dirty=false`（公式同 `scripts/verify-2575.sh`，锚定 fix commit 而非证据提交；`dirty` 计算忽略 `docs/evidence/2576/` 自身 churn）。
  - `docs/evidence/2576/R1-confirm-guards.txt`
  - `docs/evidence/2576/R2-confirm-behavior.txt`
  - `docs/evidence/2576/R3-callback-behavior.txt`
  - `docs/evidence/2576/R4-doc-contract.txt`
- 台账：`docs/evidence/2576/evidence.json`（5 行 `pass=true`、`dirty=false`、`plan_sha256` 与 Accepted Plan 一致）。
- 计划快照（P17）：`docs/evidence/2576/plan.md`（`scripts/plan-source.sh --issue 2576 --fresh` 生成，随证据一起提交）。
- gate（/work 执行）：`docs/evidence/2576/pre-ready-2576.json`；`evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$`。

## Core Update

`[Core Update Required]` — 本单必须同步修改以下核心文档（AGENTS.md §Core Documents）：

| 文档 | 当前状态（develop） | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §2.8 只有 `2.8.1 查询确认会话`；`Confirm` 的守卫语义未成文（`origin/issue-2575` 曾把本缺口 deferred 到已 CLOSED 的 #2543 与不相关的 #2542） | 新建/替换 `### 2.8.2 确认执行`：`POST /api/confirmation-sessions/:id/confirm?token=` 的守卫顺序表（`404 40400 invalid session or token` → `400 40002 confirmation link has expired` → `200 20000 already confirmed`（幂等）→ `409 40901 session already rejected` → `409 40901 session is not confirmable` → `200 20000 successfully confirmed and action executed` → 动作失败 `500 50000` 且回写 `status=failed`），并新增「IAM 回调 `result=accept` 分类」表（`already_confirmed` / `not_confirmable` / `success`，回调不校验 `expires_at`）；删除指向 `#2543`/`#2542` 的过期 deferral 注 | 契约是 #2575 Round 1 审计的阻断项来源（deferral 未落盘）；守卫语义必须与 handler 逐句一致，否则下一次审计仍无法从文档判断「哪些状态可确认」 |

`docs/spec/database/database.md` §2.27 **无需修改**：状态集合（`waiting`/`confirmed`/`failed`/`rejected`/`expired`）不变，本单只新增「状态之间哪些转移合法」的接口层契约。

## Dependencies

- 无

### 依赖说明

- **#2575（OPEN，`status:rejected`，分支 `origin/issue-2575`）**：镜像方向（`Reject` 守卫 + 回调 `reject` 分类），与本单**无语义前置**，故不列为硬依赖；唯一交叉面是 `docs/spec/api/api.md` §2.8.2 的**同一段落**，合并顺序规则见 Decision Lock D6（replace-or-create，两序结果一致）。本单不修改 `Reject` 与 §2.8.3。
- **#2543（已 CLOSED）**：提供 `confirmation_sessions` 表与 `iam_sub` 改造，本单只在其状态机上补守卫。
- **对下游**：本单关闭后，#2575 的 `api.md` deferral 阻断项（Round 1 Bug 1）获得在册 Issue 回链。

### Workload

- Size: S
- Basis: 1 个产品文件（`confirmation_session.go` 两个函数 + 1 个白名单 helper）+ 1 个新增测试文件（约 8–9 个用例）+ 1 个证据脚本 + 1 个核心文档段落；无 migration、无前端、无新增路由
- Confidence: high（缺陷行与守卫口径已由 #2575 同源定型，`expired` 未写入路径已用 grep 证实，白名单集合由本计划裁定）

### Files（变更文件清单）

- `backend/handlers/confirmation_session.go`
- `backend/handlers/confirmation_session_confirm_2576_test.go`
- `scripts/verify-2576.sh`
- `docs/spec/api/api.md`
- `docs/evidence/2576/plan.md`
- `docs/evidence/2576/R1-confirm-guards.txt`
- `docs/evidence/2576/R2-confirm-behavior.txt`
- `docs/evidence/2576/R3-callback-behavior.txt`
- `docs/evidence/2576/R4-doc-contract.txt`
- `docs/evidence/2576/evidence.json`
- `docs/evidence/2576/pre-ready-2576.json`

*Model: DeepSeek V4.1 Flash 1017*

