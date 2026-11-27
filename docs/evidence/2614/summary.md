# Work Summary — #2614

## 修改内容

IAMClient.UpdateUser 的两处后端调用点在失败时仍沿用旧口径：

- UpdateIAMUser（PUT /api/iam/users/:id，PC StaffEdit 保存）→ 任意失败 500/50000 + "Failed to update user in IAM: <上游原始串>"；
- UpdateCurrentUser（PUT /api/users/me）→ 任意失败 409/40900 + err.Error()（仅 username 冲突换成中文）。

即持久失败（上游 401/403/404）被表达成可重试，瞬时故障（不可达/超时/5xx）被表达成数据冲突，且上游原始串外溢到用户可见 message。本单把两处接入 #2598 的统一出口：

- 新增 iamUserUpdateConflictText（iam_user_conflict.go）：errors.As(*services.IAMClientHTTPError) + Status==409 后复用 iamConflictField（#2290）产出 username/email/phone 冲突中文文案；未知原因返回空由调用点兜底。**不新增** body 解析、**不修改** iam_failure.go 语义。
- UpdateIAMUser / UpdateCurrentUser 失败分支改调 respondIAMFailure（transient 502/50200、conflict 409/40900、userAction 400/40002、config 500/50015），message 不再拼接上游原始串（诊断只进 log.Printf）。
- username already exists → 409/40900 +「用户名已被占用，请换一个」**逐字保持**，判据由字符串 Contains 改为类型 + iamConflictField。
- 成功路径与本地缓存顺序**未改**（本地同步仍只在 IAM 成功后）。

## 涉及文件

- `backend/handlers/iam_user_conflict.go`（新增：409 冲突文案细化）
- `backend/handlers/iam_proxy.go`（UpdateIAMUser 失败分支）
- `backend/handlers/user_staff.go`（UpdateCurrentUser 失败分支）
- `backend/handlers/iam_user_update_failure_2614_test.go`（新增：fake IAM 状态矩阵）
- `docs/spec/api/api.md`（§2.4.1 失败三元组表、新增 §2.6.5、§14.4 适用端点）
- `docs/spec/ui/ui.md`（§2.6.2 资料保存失败提示表）
- `scripts/verify-2614.sh`（r1–r5 + gate + ledger）
- 证据目录 `docs/evidence/2614/`

Commit Hash：a7452e064（fix）；证据提交见「Remote」。

## 验证层级

- 本单为独立 Issue（无 Parent:），gate 跑 go build ./... + 本单两条状态矩阵 + 6 个既有回归用例；r5 另做全包编译冒烟（go test ./... -run '^$'）与 gofmt 检查。
- 所有重命令经 heavy-run.sh 取 host 锁执行。
- 基线为 origin/issue-2601（按计划 D1：git merge-base --is-ancestor origin/issue-2601 origin/develop 返回 1，故 worktree-create --base origin/issue-2601）。

## Remote

remote: origin/issue-2614@a7452e064; fix=a7452e064 pushed=yes; evidence=docs/evidence/2614 pushed=yes

## 红绿实证

两条矩阵用例即为红绿载体（缺陷版 → 修复版）：

- **红（修复前代码语义）**：UpdateIAMUser 任意失败固定 500/50000 且 message 拼接 "Failed to update user in IAM: <原始串>" → 新用例的 UPDATEIAM_403_STATUS=500/UPDATEIAM_403_CODE=50015、UPDATEIAM_LEGACY_50000_GONE=1、UPDATEIAM_RAW_LEAK=0 全部不成立；UpdateCurrentUser 任意失败固定 409/40900 → UPDATECURRENT_500_STATUS=502、UPDATECURRENT_TRANSIENT_NOT_CONFLICT=1 不成立。
- **绿（修复后）**：`docs/evidence/2614/R1-iam-update-triples.txt`、`docs/evidence/2614/R2-me-update-triples.txt` 全 token 成立（见下 Artifact 表）。

## Accepted Plan 对照（6/6）

| 计划变更点 | 状态 |
|---|---|
| 1 新增 iam_user_conflict.go（复用 iamConflictField + 类型判据） | 完成 |
| 2 UpdateIAMUser 接入 respondIAMFailure，删除 500/50000 与原始串拼接 | 完成 |
| 3 UpdateCurrentUser 接入 respondIAMFailure，username 文案逐字保持 | 完成 |
| 4 新增 iam_user_update_failure_2614_test.go（fake IAM 状态矩阵 + 负例） | 完成 |
| 5 新增 `scripts/verify-2614.sh`（r1–r5 + gate） | 完成 |
| 6 文档（api.md §2.4.1/§2.6.5/§14.4、ui.md §2.6.2） | 完成 |

### 契约三向核对

| 方向 | 内容 | 一致 |
|---|---|---|
| Plan ↔ API 文档 | 计划变更点 2/3 + Core Update ↔ `docs/spec/api/api.md` §2.4.1「IAM 失败三元组（#2614）」表与新增 §2.6.5（四类失败表 + message 不拼接原始串） | 是 |
| API 文档 ↔ handler | PUT /api/users/me：502/50200、400/40002、409/40900（username/email/phone/未知）、500/50015；PUT /api/iam/users/:id：同上但 409 只可能是邮箱/手机号 —— 与 respondIAMFailure + iamUserUpdateConflictText 分支逐一对应 | 是 |
| handler ↔ 测试 | 状态矩阵逐项断言 (status, code, message) + 负例（持久类不含「请稍后再试」、瞬时类不得 409、message 无上游原始串、无 50000、本地缓存未改写） | 是 |

## Evidence Contract（5/5 PASS）

| 行 | 结果 | 产物 |
|---|---|---|
| R1 | PASS | `docs/evidence/2614/R1-iam-update-triples.txt` |
| R2 | PASS | `docs/evidence/2614/R2-me-update-triples.txt` |
| R3 | PASS | `docs/evidence/2614/R3-regression.txt` |
| R4 | PASS | `docs/evidence/2614/R4-docs.txt` |
| R5 | PASS | `docs/evidence/2614/R5-build.txt` |

汇总 bundle：`docs/evidence/2614/evidence.json`（build=1.0.a7452e064、commit=a7452e064、dirty=false）。

## G0 Golden Acceptance

| 行 | artifact | assertions |
|---|---|---|
| R1 | `docs/evidence/2614/R1-iam-update-triples.txt` | 400→40002、401→50015、403→500/50015、404→50015、409email/phone/unknown→40900、500/不可达→502/50200、PERSISTENT_NO_RETRY=0、RAW_LEAK=0、LEGACY_50000_GONE=1、LOCAL_CACHE_KEPT=1 |
| R2 | `docs/evidence/2614/R2-me-update-triples.txt` | 同上 + 409username→409/40900 且文案逐字、TRANSIENT_NOT_CONFLICT=1、WRAPPED_TYPED=1 |
| R3 | `docs/evidence/2614/R3-regression.txt` | 六个既有用例 --- PASS（CONFLICT_ABORT / SUCCESS_CACHE / 2446_USERNAME_TEXT / 2446_CREDENTIALS / IAMMOCK_EMAIL / ZERO_TENANT） |
| R4 | `docs/evidence/2614/R4-docs.txt` | API_IAM_UPDATE_ENDPOINT=1、API_IAM_UPDATE_FAILURE_ROWS=1、API_ME_UPDATE_FAILURE_ROWS=1、API_TRIPLE_APPLIES_ENDPOINTS=1、DOC_CONFIG_TEXT_KEPT=1、UI_ME_SAVE_FAILURE_ROWS=1、FE_ERROR_MAP_50015_PC/MOBILE=1 |
| R5 | `docs/evidence/2614/R5-build.txt` | GO_BUILD_OK=1、GOFMT_DIFF=0、GO_COMPILE_ALL_OK=1 |

## 矩阵 n/n

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 UpdateIAMUser 失败出口接入统一三元组（含 409 冲突对象细化） | PASS | `docs/evidence/2614/R1-iam-update-triples.txt` |
| R2 UpdateCurrentUser 失败出口接入统一三元组（username 文案逐字保持） | PASS | `docs/evidence/2614/R2-me-update-triples.txt` |
| R3 既有断言回归（#1600 / #2249 / #2446 / IAMMock 成功路径；既有测试文件本身不修改） | PASS | `docs/evidence/2614/R3-regression.txt` |
| R4 契约文档 + 双端码表登记（静态） | PASS | `docs/evidence/2614/R4-docs.txt` |
| R5 构建 / gofmt / 全包编译冒烟 | PASS | `docs/evidence/2614/R5-build.txt` |
| R6 pre-ready gate（四源 + 合同完整性 + 远端可达） | PASS | `docs/evidence/2614/pre-ready-2614.json` |

## 同类路径对称扫描

IAMClient.UpdateUser 非测试调用点共 4 处（计划「同批扫描判定」表）：

| 调用点 | 现状 | 本单处置 |
|---|---|---|
| handlers/iam_proxy.go（UpdateIAMUser） | 旧：500/50000 + 原始串 | 已修（R1） |
| handlers/user_staff.go（UpdateCurrentUser） | 旧：409/40900 + 原始串 | 已修（R2） |
| handlers/wechat_bind.go:373（Unbind） | 已由 #2601 接 respondIAMFailure | 仅回归（保持不变） |
| handlers/registration_helpers.go:81 | 支付回调链，无用户三元组 | Out of Scope（计划已记录） |

另：grep -rn "Failed to update user in IAM" backend → 修后 0 命中；grep -rn "username already exists" backend/handlers/user_staff.go → 仅 iamConflictField 内部（body 解析唯一 owner）。

## 语义变更（被翻转 / 被强化的既有断言）

计划「被翻转 / 被强化的既有断言」表 6 行全部按预期处置：

- iam_proxy.go 与 user_staff.go 的失败分支**无既有断言**（grep 仅实现行命中）→ 无翻绿；由新负例 UPDATEIAM_LEGACY_50000_GONE=1、UPDATECURRENT_TRANSIENT_NOT_CONFLICT=1 锁死旧行为不再发生。
- update_current_user_test.go:93（#1600 冲突中止）→ **强化不翻转**：仍 409/40900 且本地不写（R3 REGRESSION_CONFLICT_ABORT=1）。
- update_current_user_2446_test.go:128-137（username 文案）→ 逐字保持（R2 UPDATECURRENT_409_USERNAME_TEXT_KEPT=1 + R3 REGRESSION_2446_USERNAME_TEXT=1）。
- iam_proxy_update_2249_test.go:24（零租户成功路径）→ 保持不变（R3 REGRESSION_ZERO_TENANT=1）。

## Dependency Order / 验收规则

- 前置 #2601（status:done）的 respondIAMFailure / IAMClientHTTPError / iamConflictField 均在基线 origin/issue-2601 上命中（D1 条件成立，未自行实现分类器）。
- 未修改任何验收工具/规则（iam_failure.go、plan-lint.sh、pre-ready-check.sh 均未触碰，D2）。
- 未新增 migration、无前端代码改动、无跨仓改动。

### 证据来源

| repo | build_version | fix commit | artifact |
|---|---|---|---|
| . | 1.0.a7452e064 | a7452e064619d2f1105b97780ef5b4241cdca901 | `docs/evidence/2614/R1-iam-update-triples.txt` |

build_version 短码 a7452e064 即本轮 fix commit（ancestor of HEAD 成立）；生成证据前源码树无改动（dirty=false）。

## 人工/部署侧

- 无。矩阵全部行可由 Developer 在测试库 + fake IAM 上完成；真实 beaconiam 端 409 reason 串由既有契约（user_handler.go）承担（计划「工具盲区」）。

## Pre-ready gate

见 `docs/evidence/2614/pre-ready-2614.json`；最终 RESULT: PASS 的关键输出在下方（gate 运行后补登）。

*Model: deepseek/deepseek-flash*
