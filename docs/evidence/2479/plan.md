## Accepted Plan — #2479 v2（CreateSite 无负责人时跳过角色模板分配：nil UUID 不得作为 IAM 目标）

> **v2 说明**：v1 的验收断言写 `STATUS=200`，与 `CreateSite` **既有**成功契约 `201 Created`（`backend/handlers/site.go:508`；基线用例 `site_create_anchor_2266_test.go` 亦断言 201）矛盾。按「开发发现计划层矛盾 → 回 Architect」（§计划偏差升级）裁定：**改计划，不改实现**（把成功码改成 200 会变更既有 API 契约，属 Out of Scope）。本 v2 仅把 R1/R2 的 `STATUS=200` 统一为 `STATUS=201`，其余断言与变更点不变。

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 已判空 `if adminID != ""` —— 但 IAM 返回的 `admin_id` 在「未指定负责人」时是 **nil UUID 字符串**（`00000000-0000-0000-0000-000000000000`），非空串 | `backend/handlers/site.go:362-365`（`adminID := orgResp.AdminID`） |
| 因此进入模板分配分支 → `AssignRoleTemplateToUserWithToken(..., adminID, ...)` 3 次重试均 404，最终仅 `log.Printf` | `backend/handlers/site.go:368-391` |
| 同级已有 nil UUID 常量用法（`newAdmin == "" || newAdmin == nilUUID`） | `backend/handlers/merchant.go:79-84`、`:577-584` |
| 其它使用 `orgResp.AdminID` 的位置：`CreateMerchant` 仅在 `parseErr != nil && AdminID != ""` 时回填；响应字段 `iam_admin_id` 原样透出 | `backend/handlers/merchant.go:360-361`、`:492`；`site.go:509` |

**结论**：`site.go` 的判空只覆盖 `""`，未覆盖 nil UUID → 产生 3 次无谓 IAM 调用 + 3 条错误日志，且建网点返回 200（前端不知「管理员角色未分配」）。

### 变更点

1. `backend/handlers/site.go` `CreateSite`：
   - 新增局部判定 `hasAdmin := adminID != "" && adminID != uuid.Nil.String()`（与 `merchant.go` 同口径，避免再引入第二套常量）；
   - `!hasAdmin` 时**跳过** `ListRoleTemplates` 与 `AssignRoleTemplateToUserWithToken`（0 次 IAM 调用、0 条 error 日志），改打一条 INFO：`[CreateSite] skip role template assignment: no admin (site=%s org=%s)`；
   - `hasAdmin` 时保持既有 3 次重试逻辑与日志（回归不变）。
2. `backend/handlers/site.go` 响应：不改 `iam_admin_id` 字段语义（仍透出 IAM 原值）；不新增 4xx —— 与本单 D2 一致（可选负责人是既有产品语义，`manager_pending` 已表达「待指定」）。
3. 同类扫描结论（记录在证据里，不在本单改）：`merchant.go:360` 已有 `AdminID != ""` 判空且只用于回填 `adminUserID`，nil UUID 不会进入模板分配路径；`iam_admin_id` 为只读展示字段 → 无同类缺陷。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 nil UUID 负责人 → 跳过模板分配 | `backend/handlers/site.go` | `POST /api/merchant/sites` | fake IAM `CreateOrganization` 返回 `admin_id=00000000-0000-0000-0000-000000000000` | 后端 | 响应 **201 Created**（既有契约）；`AssignRoleTemplate` 调用 **0** 次；日志无 `AssignRoleTemplate … failed`；有 INFO `skip role template assignment` | R1 |
| R2 空串负责人 → 跳过（既有语义保持） | `backend/handlers/site.go` | 同上 | `admin_id=""` | 后端 | 响应 **201 Created**（既有契约）；IAM 模板调用 0 次；无 error 日志 | R2 |
| R3 正常负责人 → 仍分配（回归） | `backend/handlers/site.go` | 同上 | `admin_id=<有效 uuid>` | 后端 | `AssignRoleTemplate` 调用 **1** 次（首次成功，不重试）；`role` 模板 code=`site_admin` | R3 |
| R4 分配失败重试路径不被破坏 | `backend/handlers/site.go` | 同上 | `admin_id=<有效 uuid>`，前两次 404、第三次 200 | 后端 | 调用 **3** 次后成功；不产生 `failed after retries` | R4 |
| R5 gate 四源一致 + 全量套件 | `docs/evidence/2479/pre-ready-2479.json`、`docs/evidence/2479/plan.md` | `/work` 以 `--full-gate-cmd` 跑 pre-ready（`cd backend && go test ./handlers -count=1 -timeout 15m` 与 `./services`），随后 `plan-source.sh --issue 2479` | 修后树（干净） | 后端 | gate `result=PASS` 且含 `FULL_SUITE_EXIT=0 FAIL_COUNT=0 PANIC_COUNT=0`；`PLAN_SOURCE: PASS` | R5 |
| R6 构建与定向回归 | `backend` | `go build ./...` + 定向测试（2479 + 2266 + 站点相关） | 修后树 | 后端 | `BUILD_EXIT=0`、`TESTS_FAIL=0` | R6 |

## Input Truth & Tool Preflight

- **Source of truth**：IAM `CreateOrganization` 级联管理员的返回字段 `admin_id`（`services.CreateOrganizationResponse.AdminID`）；未指定负责人时 IAM 返回 nil UUID（预生产日志实证）。
- **差异核对**：`site.go` 现有守卫为 `adminID != ""`，**不能**覆盖 nil UUID；`merchant.go` 已有 `nilUUID` 判空范式可对齐，避免新增第二套常量。
- **损坏输入拒绝**：`admin_id` 为空白、非 UUID、nil UUID 一律视为「无负责人」→ 跳过（不把非法值传给 IAM）；`hasAdmin=true` 时仍按既有重试逻辑。
- **工具盲区**：fake IAM 只能验证调用次数与 payload，真实 IAM 的 404 语义由部署侧回归覆盖；`CreateSite` 依赖 `managerPending` 等本地字段，测试需按既有 `site_create_anchor_2266_test.go` 的 fixture 方式构造。
- **成功响应码基线**：`CreateSite` 固定 `201 Created`（`site.go:508`），基线用例 `site_create_anchor_2266_test.go` 亦断言 201 → 本单**不得**为迁就计划而改码，也不得把 201 写成 200（伪造证据）。
- **影响面预检**：`grep -rn "AdminID" backend/handlers/*.go` → 仅 `site.go`（本次）与 `merchant.go`（只读回填/展示，无同类缺陷）→ 无需全仓扩面。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 语义选择：**跳过**模板分配（不发 IAM、不打 error 日志、打 INFO），**不**改为 400 强制负责人 | Owner / Architect | 采用 Issue 期望 1（推荐项）；`CreateSite` 既有「可选负责人 + `manager_pending`」语义保持不变 | 若 Owner 要求「建网点必须有负责人」→ 回 Architect 改计划（新增 400/40002 与前端必填） |
| D2 | 判定口径：`adminID != "" && adminID != uuid.Nil.String()`，不新增自定义常量（与 `merchant.go` 同口径） | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 不改响应字段 `iam_admin_id` 与 `manager_pending` 语义 | Architect | 本计划锁定 | 阻塞实现 |
| D4 | 成功响应码保持 **201 Created**（既有契约，`site.go:508` + 基线用例 `site_create_anchor_2266_test.go`）；本单不改 HTTP 状态码，证据必须记录真实 201 | Architect | 本计划锁定（v2 修正） | 若 Owner 要求 200 → 另立 Issue（改前端/测试契约），本单不阻塞 |

## Out of Scope

- 强制要求网点必须有负责人（若 Owner 选 D1 备选，另立 Issue 改前端必填 + 后端 400）。
- `CreateMerchant` 的管理员分配路径（已正确判空）。
- `AssignRoleTemplateToUserWithToken` 的 404 语义与重试策略本身（本单只避免无效目标）。
- 前端「网点管理员未分配」的提示 UI（现有 `manager_pending` 已表达，展示改造另议）。
- 全量 `go test ./...` 作为合同行（由 R5 gate 承担）。

## G0 Golden Acceptance

| 变更点 | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） |
|---|---|---|---|
| R1 | 商户管理员 actor；fake IAM `CreateOrganization` → `{"org_id":"ORG-NEW","admin_id":"00000000-0000-0000-0000-000000000000"}`；body `{name:"测试网点 A"}` | `CreateSite`（`POST /api/merchant/sites`） | `docs/evidence/2479/R1-nil-admin.txt` |
| R2 | 同上但 `admin_id=""` | 同上 | `docs/evidence/2479/R2-empty-admin.txt` |
| R3 | `admin_id=<valid-uuid>`，fake IAM 模板分配 200 | 同上 | `docs/evidence/2479/R3-with-admin.txt` |
| R4 | `admin_id=<valid-uuid>`，模板分配前两次 404、第三次 200 | 同上 | `docs/evidence/2479/R4-retry.txt` |
| R5 | 修后干净树 + `--full-gate-cmd` gate 报告 | 读校验（`plan-source.sh` + gate 解析） | `docs/evidence/2479/R5-gate.txt` |
| R6 | 修后 backend | `go build ./...` + 定向测试 | `docs/evidence/2479/R6-build.txt` |

### Assertions

- R1：`STATUS=201`、`ASSIGN_CALLS=0`、`ERR_LOG=0`、`INFO_SKIP=1`；负例：`ASSIGN_CALLS=3`（仍打 nil UUID）→ FAIL；`STATUS=200`（成功码被改成 200）→ FAIL（触发既有契约变更，需另立 Issue）。
- R2：`STATUS=201`、`ASSIGN_CALLS=0`；负例：`ASSIGN_CALLS>0` → FAIL；`STATUS!=201` → FAIL。
- R3：`ASSIGN_CALLS=1`、`TEMPLATE_CODE=site_admin`；负例：`ASSIGN_CALLS=0`（误跳）→ FAIL。
- R4：`ASSIGN_CALLS=3`、`RETRY_OK=1`、`ERR_LOG_FAILED_AFTER=0`；负例：放弃重试 → FAIL。
- R5：`FULL_SUITE_EXIT=0`、`FAIL_COUNT=0`、`PANIC_COUNT=0`、`PLAN_SOURCE=PASS`；负例：`P5=NA` → FAIL。
- R6：`BUILD_EXIT=0`、`TESTS_FAIL=0`；负例：任一非 0 → FAIL。
- 通用 provenance：每个 artifact 首行 `# build=1.0.<fix> # commit=<sha> # dirty=false`。
- 负例（跨行）：任何 artifact 出现 `AssignRoleTemplate … 00000000-0000-0000-0000-000000000000` → FAIL。

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2479/R1-nil-admin.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2479/R2-empty-admin.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2479/R3-with-admin.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2479/R4-retry.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2479/R5-gate.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2479/R6-build.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| symptom | trigger | root cause | test blind spot | same-class paths | fix layer | red-green | prevention |
|---|---|---|---|---|---|---|---|
| 建网点刷 3 条 404 错误日志 | 不指定负责人 | 判空只覆盖 `""`，未覆盖 nil UUID | 无「无负责人」fixture | `CreateMerchant` 管理员回填 | handler | R1 红绿 | D2 统一 nil UUID 判定 |
| 静默降级（200 但管理员角色未分配） | 同上 | 失败仅日志 | 无角色模板调用计数断言 | 其它「可选负责人」路径 | handler | R1/R3 对照 | 显式 INFO + 计数断言 |
| 正常负责人路径被误跳 | 过度判定 | 判空条件过宽 | 无正向用例 | — | handler | R3 红绿 | 双态 fixture |
| 重试逻辑被破坏 | 改动守卫位置 | 未保留 retry | 无重试用例 | 其它 IAM 重试路径 | handler | R4 红绿 | 保留原循环 |
| 验收断言与既有响应契约不符（本轮实际发生） | 计划写 `STATUS=200` | 计划未核对 `CreateSite` 现有成功码 | 无「响应码基线」核对项 | 其它写接口的 200/201 | 计划层（Architect） | 本轮已红（developer 实测 201） | D4 + Input Truth 基线核对 |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2479.sh r1` | `docs/evidence/2479/R1-nil-admin.txt` | exit 0；grep `STATUS=201`、`ASSIGN_CALLS=0`、`INFO_SKIP=1` | PASS | `ASSIGN_CALLS=3` → FAIL | dev |
| R2 | `bash scripts/verify-2479.sh r2` | `docs/evidence/2479/R2-empty-admin.txt` | exit 0；grep `STATUS=201`、`ASSIGN_CALLS=0` | PASS | `ASSIGN_CALLS>0` → FAIL | dev |
| R3 | `bash scripts/verify-2479.sh r3` | `docs/evidence/2479/R3-with-admin.txt` | exit 0；grep `ASSIGN_CALLS=1`、`TEMPLATE_CODE=site_admin` | PASS | `ASSIGN_CALLS=0` → FAIL | dev |
| R4 | `bash scripts/verify-2479.sh r4` | `docs/evidence/2479/R4-retry.txt` | exit 0；grep `ASSIGN_CALLS=3`、`RETRY_OK=1` | PASS | 放弃重试 → FAIL | dev |
| R5 | `bash scripts/verify-2479.sh r5` | `docs/evidence/2479/R5-gate.txt` | exit 0；grep `FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | PASS | `P5=NA` → FAIL | dev |
| R6 | `bash scripts/verify-2479.sh r6` | `docs/evidence/2479/R6-build.txt` | exit 0；grep `BUILD_EXIT=0`、`TESTS_FAIL=0` | PASS | 任一非 0 → FAIL | dev |

### Workload

- Size: S
- Basis: 1 个后端源文件（守卫 + INFO 日志 ~10 行）+ 1 个测试文件 + 1 个 verify 脚本 + 6 条证据行；无 migration、无前端
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/site.go`
- `backend/handlers/site_create_admin_guard_2479_test.go`
- `scripts/verify-2479.sh`
- `docs/evidence/2479/plan.md`
- `docs/evidence/2479/pre-ready-2479.json`
- `docs/evidence/2479/R1-nil-admin.txt`
- `docs/evidence/2479/R2-empty-admin.txt`
- `docs/evidence/2479/R3-with-admin.txt`
- `docs/evidence/2479/R4-retry.txt`
- `docs/evidence/2479/R5-gate.txt`
- `docs/evidence/2479/R6-build.txt`
- `docs/evidence/2479/evidence.json`

## Dependencies

- 无

### 依赖与关联说明（非硬依赖）

- **#2266**（CreateSite 归属锚）：同一函数，本单只加守卫，不改归属逻辑。
- **#2477**（CreateSite 重名预检 + 名称归一化）：同函数不同变更点，互不冲突。
- **#2377**（创建网点/成员角色分配口径）：本单补「无负责人」分支的静默降级问题。
- **部署侧**：预生产不指定负责人建网点 → 日志无 3 次 404 的回归由 `[Ops-Followup of #2479]`（`waitForConfirm`）承载。

*Model: DeepSeek V4.1 Flash 1017*


