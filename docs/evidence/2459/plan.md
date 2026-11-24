## Accepted Plan — #2459（v2：商户改名先写 IAM + 仅商户管理员可改 + namespace 内唯一 + 移除运维行）

> **v2 变更（采纳 2026-11-24 Plan Gap + Owner 裁定）**：
> ① **Gap 1**：原 R12「预生产改名回执（运维执行）」从 `Verification Matrix / Evidence Contract / G0 / Files` **移出**，改为 `### Deployment-side Follow-ups`；
> ② **Gap 2 / Owner O1=A**：前置 #2457 先审后合并；合并前允许 stacked base（`--base origin/issue-2457`）开发，**合并顺序 #2457 → #2459**，最终证据在合并后的 develop 基线重跑；
> ③ **Owner O3**：改名**仅商户管理员（OWNER）可执行**，平台/系统管理员改商户名 → 403；商户名在 **namespace 内唯一**（tuneloop 预检 409）；
> ④ 新增 R12（权限门控）、R13（命名空间唯一预检）、R14（gate 读校验）。R1–R11 语义不变。

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| `UpdateMerchant` 仅在 `input.Name != ""` 时改 `merchant.Name`，随后 `db.Save`，全程无 IAM 组织改名 | `backend/handlers/merchant.go:557-559`、`:612-619` |
| 同函数内 IAM 只用于 `admin_uid` 变更（Bind/UpdateRole），**名称**从不回写 | `backend/handlers/merchant.go:585-607` |
| `merchants.org_id` 为 NOT NULL，直接对应 IAM 组织 | `backend/models/models.go:1045` |
| IAM 侧 `PUT /api/v1/organizations/:id` 已存在（可改 `name`） | `beaconiam/cmd/api/main.go:286`、`internal/handler/org_handler.go:586-646` |
| 下游展示依赖商户名（员工上下文「商户/组织名 + 网点名」） | #2455 需求 2 |

与 #2457 同模式；本单**复用** #2457 引入的 `UpdateOrganizationName*` 客户端方法，不另写 HTTP 调用。

### 变更点

1. `backend/handlers/merchant.go` `UpdateMerchant`：
   - **权限门控（Owner O3）**：仅当 `middleware.GetBusinessRole(ctx) == BusinessRoleMerchantAdmin` 时才允许**名称变更**；平台/系统管理员发起改名 → `403/40301`「仅商户管理员可修改商户名称」（其它字段更新不受影响）。
   - **namespace 内唯一预检（Owner O3）**：名称变化时用 IAM 列出同 namespace 组织（`ListOrganizationsWithToken`），排除自身 `org_id` 后按 `lower(trim(name))` 比较，命中 → `409/40900`「商户名在命名空间内已存在」，**不调 IAM 改名、不写本地**。
   - 在改名分支前取 `origName := merchant.Name`；
   - `input.Name` 去空白后为空 → **保持现有语义**（不改名，不报错；部分更新兼容）；非空且与 `origName` 相同 → 跳过 IAM；
   - 名称变化 → **先** `UpdateOrganizationNameWithToken(userToken, merchant.OrgID, input.Name)`；`403` 且本地已过 `SysPermTenantUpdate`（bit 8）→ 服务凭据 `UpdateOrganizationName(...)` 重试一次并记日志；
   - IAM 失败（403/404/409/5xx）→ 按映射返回（40301/40400/40900/50000），**不执行 `db.Save`**（admin_uid 变更也不落库，避免半更新）；
   - IAM 成功 → 应用本地字段并 `db.Save`；本地保存失败 → `500/50000` + 「IAM 已改名，本地同步失败，请重试」（同名重试幂等）。
2. `backend/services/site_name_drift.go`（由 #2457 创建）：新增 `ComputeMerchantNameDrift(local []MerchantNameRef, iam []OrgNameRef) []DriftRow`，复用同一比较核心（同一循环/同一 `DriftRow`），不复制第二套比对逻辑。
3. `backend/main.go`：`--site-name-drift` 报告同时输出 `MERCHANT_DRIFT` 段（仍为只读 dry-run）。
4. `docs/spec/api/api.md`（L6590 起 `PUT /api/merchants/:id`）：补「名称变更先写 IAM 组织名；`merchants.name` 是 IAM 组织名镜像；IAM 失败不落库」。
5. 测试：`backend/handlers/merchant_update_iam_first_2459_test.go`（fake IAM，R1–R7 + R12 权限 + R13 唯一预检）、`backend/services/merchant_name_drift_2459_test.go`（R8）。
6. **IAM 侧唯一性缺口（跟随跟踪）**：beaconiam `UpdateOrg` 无重名校验、也无 `(namespace_id, lower(name))` 唯一索引 → tuneloop 预检只能覆盖常规路径（并发仍可绕过）。按 Owner O3，另立 beaconiam Issue（幂等迁移 + 唯一索引 + 冲突 409），本单不阻塞但必须跟踪。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 改名 IAM 优先 | `backend/handlers/merchant.go` | `PUT /api/merchants/:id` | name 变化、IAM 200 | 后端 | IAM 先调用且 body 为新名，随后本地改名 | R1 |
| IAM 失败不落库 | `backend/handlers/merchant.go` | 同上 | IAM 500 且同时改 admin_uid | 后端 | 返回 5xx；`merchants.name` 与 `admin_uid` 均未变 | R2 |
| 同名幂等 | `backend/handlers/merchant.go` | 同上 | name 未变、phone 变化 | 后端 | IAM 0 次；phone 落库 | R3 |
| 403 服务凭据升级 | `backend/handlers/merchant.go` | 同上 | 用户 token 403、服务凭据 200 | 后端 | IAM 调用 2 次、第二次非用户 token、最终 200 | R4 |
| IAM 重名映射 | `backend/handlers/merchant.go` | 同上 | IAM 409 | 后端 | 409/40900；本地不变 | R5 |
| IAM 组织不存在 | `backend/handlers/merchant.go` | 同上 | IAM 404 | 后端 | 404/40400；本地不变 | R6 |
| 空名称兼容 | `backend/handlers/merchant.go` | 同上 | name 为 `""` 或空白 | 后端 | 不改名、返回 200、IAM 0 次 | R7 |
| 复用客户端 | `backend/handlers/merchant.go` | grep | — | 后端 | 引用 `UpdateOrganizationName` 系列；无新增裸 http 请求 | R8 |
| 商户漂移清单 | `backend/services/site_name_drift.go` | 单元测试 | 2 条不一致 / 0 条 | 后端 | 2 行（org_id + 两侧名）/ 0 行 | R9 |
| 文档同步 | `docs/spec/api/api.md` | grep | — | 文档 | 含 IAM 优先、镜像、失败不落库三项 | R10 |
| 构建与静态检查 | `backend` | `go build ./...` + `go vet` | — | 后端 | exit 0 | R11 |
| 仅商户管理员可改名 | `backend/handlers/merchant.go` | `PUT /api/merchants/:id` | 平台/系统管理员发起 `name` 变更 | 后端 | `403/40301`；IAM 改名 0 次；`merchants.name` 不变 | R12 |
| 命名空间内商户名唯一预检 | `backend/handlers/merchant.go` | 同上 | 同 namespace 已有其它组织用该名（大小写/空白不同） | 后端 | `409/40900`；IAM 改名 0 次；本地不变 | R13 |
| gate 四源一致 + 全量套件（读校验） | `docs/evidence/2459/pre-ready-2459.json`、`docs/evidence/2459/plan.md` | `/work` 以 `--full-gate-cmd` 跑一次 pre-ready（`cd backend && go test ./handlers -count=1 -timeout 15m` 与 `./services`），随后 `plan-source.sh --issue 2459` | 修后树（干净） | 后端 | gate `result=PASS` 且含 `FULL_SUITE_EXIT=0 FAIL_COUNT=0 PANIC_COUNT=0`（P5 不得为 N/A）；`PLAN_SOURCE: PASS` | R14 |

## Bug Reflection Matrix

| # | 缺陷面 | 触发条件 | 根因 | 修复点 | 覆盖行 | 负例断言 |
|---|---|---|---|---|---|---|
| B1 | 商户改名后 IAM 组织名陈旧 | 任何商户改名 | UpdateMerchant 不回写 IAM | 变更点 1 | R1 | 只在本地改名 → R1 失败 |
| B2 | IAM 失败仍落库（含 admin_uid 半更新） | IAM 5xx | 无 IAM 分支 | 变更点 1 错误映射 | R2 | 返回 5xx 但本地已变 → R2 失败 |
| B3 | 同名保存触发无谓 IAM 调用 | 原样保存表单 | 无同名短路 | 变更点 1 同一名分支 | R3 | IAM 被调用 → R3 失败 |
| B4 | 商户管理员被 IAM 403 卡死 | 操作者 IAM 角色不足 | 无降级策略 | 变更点 1 服务凭据升级 | R4 | 403 直接失败 → R4 失败 |
| B5 | IAM 重名被当 500 | 目标名已存在 | 无 409 映射 | 变更点 1 错误映射 | R5 | 500 → R5 失败 |
| B6 | IAM 组织缺失被静默忽略 | 数据不一致（org_id 指向已删组织） | 无 404 分支 | 变更点 1 错误映射 | R6 | 200 且本地改名 → R6 失败 |
| B7 | 空名称把商户名清空 | 前端部分更新 | 未定义空值语义 | 变更点 1 空名分支 | R7 | IAM 收到空名 → R7 失败 |
| B8 | 网点/商户两处各写一套 IAM 调用 | 并行开发 | 无共享客户端方法 | 变更点 1 复用 #2457 方法 | R8 | 出现第二套裸 http 请求 → R8 失败 |
| B9 | 商户名漂移不可见 | 存量数据 | 报告只覆盖网点 | 变更点 2/3 | R9 | 2 条输入返回 0 行 → R9 失败 |
| B10 | 文档仍写「只改本地」 | 后续实现回退 | 语义未落文档 | 变更点 4 | R10 | 缺 IAM 优先表述 → R10 失败 |

## Input Truth & Tool Preflight

- **Source of truth**：IAM `organizations.name`；`merchants.name` 与 `sites.name` 同为镜像（与 #2457 D1 一致）。
- **差异核对**：`grep -rn "merchant.Name = \|merchants.name" backend --include=*.go` 确认写 `merchants.name` 只有 `UpdateMerchant`（`merchant.go:558`）与 `CreateMerchant`（建时同步）。
- **损坏输入拒绝**：空白 name = 不改名（保持现状，避免部分更新清空商户名）；OrgID 缺失 → 400/40002。
- **工具盲区**：fake IAM 验证不了真实 `checkOrgNamespaceAccess` 与 IAM 组织列表分页 → 预生产实测转入 `### Deployment-side Follow-ups`（不计入矩阵）。

## Decision Lock

| ID | 项 | 裁定 | authority |
|---|---|---|---|
| D1 | `merchants.name` 语义 | IAM `organizations.name` 的本地镜像，写路径 IAM 优先（与 #2457 D1 同口径） | target_semantics |
| D2 | 空名称 | **不改名、返回 200**（部分更新兼容）；与 #2457 的网点 D5（400）不同，理由是该接口按字段可选更新 | target_semantics |
| D3 | 失败原子性 | IAM 失败 → 不执行 `db.Save`，admin_uid 变更一并回滚（不做半更新） | target_semantics |
| D4 | 权限降级 | 用户 token 403 → 服务凭据重试一次并记日志（同 #2457 D3） | target_semantics |
| D5 | 复用边界 | 复用 #2457 的 `UpdateOrganizationName*`，禁止新写 HTTP 调用 | target_semantics |
| D6 | 运维行位置（Owner 批准 Gap 1） | 「预生产改名回执」不作为矩阵行，执行转入 `[Ops-Followup of #2459]`（`waitForConfirm`，无 `status:*`） | owner_approved |
| D7 | 改名权限（Owner O3） | **仅商户管理员（OWNER）** 可改商户名；平台/系统管理员改 `name` → 403/40301；其它字段沿用现有 `SysPermTenantUpdate` 语义 | owner_approved |
| D8 | namespace 内唯一（Owner O3） | tuneloop 侧按 `lower(trim(name))` + 同 namespace 组织列表预检 → 409/40900；IAM 侧缺 DB 唯一索引 → 另立 beaconiam Issue（不阻塞本单） | owner_approved |
| D9 | 前置落地方式（Owner O1=A / O4） | #2457 先审后合并；合并前允许 stacked base 开发，**合并顺序 #2457 → #2459**，最终证据在合并后的 develop 基线重跑 | owner_approved |

## Out of Scope

- 不改 `CreateMerchant`/`DeleteMerchant`；不改 IAM（beaconiam）代码。
- 不做商户倒闭/合并等批量迁移；不做 `merchants.name` 多语言/别名。
- 存量商户名漂移的实际修改（Owner 已授权方向 IAM→本地）由 ops 在预生产执行 → `### Deployment-side Follow-ups`，代码只出只读清单。
- 网点侧改名（#2457）与唯一约束/UI（#2455）不在本单。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 商户 `org_id=ORG1` 旧名 A，请求 B | `PUT /api/merchants/:id` | `docs/evidence/2459/R1-iam-first.txt` | IAM 1 次、body `{"name":"B"}`；本地 name=B | IAM 0 次 → FAIL |
| R2 | 同上 + admin_uid 同时变更，IAM 500 | 同上 | `docs/evidence/2459/R2-iam-fail.txt` | 5xx；name 与 admin_uid 均未变 | 任一被改 → FAIL |
| R3 | 请求 name=A、phone 变更 | 同上 | `docs/evidence/2459/R3-idempotent.txt` | IAM 0 次；phone 已更新 | IAM 被调 → FAIL |
| R4 | 用户 token 403、服务凭据 200 | 同上 | `docs/evidence/2459/R4-escalate.txt` | IAM 2 次；`ESCALATED=1`；200 | 返回 403 → FAIL |
| R5 | IAM 409 | 同上 | `docs/evidence/2459/R5-conflict.txt` | 409/40900；本地不变 | 500 → FAIL |
| R6 | IAM 404 | 同上 | `docs/evidence/2459/R6-not-found.txt` | 404/40400；本地不变 | 200 → FAIL |
| R7 | name=`""` 与 `"   "` | 同上 | `docs/evidence/2459/R7-empty-name.txt` | 200；IAM 0 次；name 仍 A | 商户名被清空 → FAIL |
| R8 | 源码 grep | grep | `docs/evidence/2459/R8-reuse.txt` | 命中 `UpdateOrganizationNameWithToken` 与 `UpdateOrganizationName(`；无新增 `UpdateOrganizationName` 之外的裸 http 调用 | 出现第二套实现 → FAIL |
| R9 | 商户 2 条不一致 + 0 条一致 | `services` 单测 | `docs/evidence/2459/R9-drift.txt` | 不一致组 2 行、一致组 0 行 | 2 条输入 0 行 → FAIL |
| R10 | `docs/spec/api/api.md` | grep | `docs/evidence/2459/R10-doc.txt` | 命中「先更新 IAM」「镜像」「不落库」 | 缺任一 → FAIL |
| R11 | HEAD 源码 | build/vet | `docs/evidence/2459/R11-build.txt` | `go build ./...` 与 `go vet ./handlers ./services` 均 exit 0 | 非 0 → FAIL |
| R12 | 平台管理员 actor（`business_role=system_admin`）+ body `{name:"新名"}` | `PUT /api/merchants/:id` | `docs/evidence/2459/R12-perm.txt` | `STATUS=403`、`CODE=40301`、`IAM_CALLS=0`、`DB_NAME` 不变 | 返回 200 或 IAM 被调用 → FAIL |
| R13 | 同 namespace 已有组织名 `卡丹萨`；商户当前 `旧名`；body `{name:" 卡丹萨 "}` | 同上 | `docs/evidence/2459/R13-unique.txt` | `STATUS=409`、`CODE=40900`、`IAM_CALLS=0`、`DB_NAME` 不变 | 改成功或 IAM 被调 → FAIL |
| R14 | 修后干净树 + `--full-gate-cmd` 产出的 gate 报告 | 读校验（`plan-source.sh` + gate 解析） | `docs/evidence/2459/R14-gate.txt` | `FULL_SUITE_EXIT=0`、`FAIL_COUNT=0`、`PANIC_COUNT=0`、`PLAN_SOURCE=PASS`、`GATE_PLAN_SHA_MATCH=1`、`P5!=NA` | P5=NA 或 sha 不一致 → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2459.sh r1` | docs/evidence/2459/R1-iam-first.txt | exit 0；含 `IAM_CALLS=1` 与 `ORDER=iam_before_local` | PASS | IAM 0 次 → FAIL | dev |
| R2 | `bash scripts/verify-2459.sh r2` | docs/evidence/2459/R2-iam-fail.txt | exit 0；含 `STATUS=500` 与 `LOCAL_UNCHANGED=1` | PASS | 本地被改 → FAIL | dev |
| R3 | `bash scripts/verify-2459.sh r3` | docs/evidence/2459/R3-idempotent.txt | exit 0；含 `IAM_CALLS=0` 与 `PHONE=updated` | PASS | IAM 被调 → FAIL | dev |
| R4 | `bash scripts/verify-2459.sh r4` | docs/evidence/2459/R4-escalate.txt | exit 0；含 `IAM_CALLS=2` 与 `ESCALATED=1` | PASS | 未重试 → FAIL | dev |
| R5 | `bash scripts/verify-2459.sh r5` | docs/evidence/2459/R5-conflict.txt | exit 0；含 `STATUS=409` 与 `CODE=40900` | PASS | 500 → FAIL | dev |
| R6 | `bash scripts/verify-2459.sh r6` | docs/evidence/2459/R6-not-found.txt | exit 0；含 `STATUS=404` 与 `CODE=40400` | PASS | 200 → FAIL | dev |
| R7 | `bash scripts/verify-2459.sh r7` | docs/evidence/2459/R7-empty-name.txt | exit 0；含 `IAM_CALLS=0` 与 `NAME=unchanged` | PASS | 名称被清空 → FAIL | dev |
| R8 | `bash scripts/verify-2459.sh r8` | docs/evidence/2459/R8-reuse.txt | exit 0；含 `REUSE_HITS=2` 与 `NEW_HTTP=0` | PASS | 新写 HTTP → FAIL | dev |
| R9 | `bash scripts/verify-2459.sh r9` | docs/evidence/2459/R9-drift.txt | exit 0；含 `MERCHANT_DRIFT_ROWS=2` 与 `MERCHANT_DRIFT_EMPTY=0` | PASS | 计数不符 → FAIL | dev |
| R10 | `bash scripts/verify-2459.sh r10` | docs/evidence/2459/R10-doc.txt | exit 0；含 `DOC_TOKENS=3` | PASS | 少于 3 → FAIL | dev |
| R11 | `bash scripts/verify-2459.sh r11` | docs/evidence/2459/R11-build.txt | exit 0；含 `BUILD_OK` 与 `VET_OK` | PASS | 构建失败 → FAIL | dev |
| R12 | `bash scripts/verify-2459.sh r12` | docs/evidence/2459/R12-perm.txt | exit 0；含 `STATUS=403`、`CODE=40301`、`IAM_CALLS=0` | PASS | 平台管理员改名成功 → FAIL | dev |
| R13 | `bash scripts/verify-2459.sh r13` | docs/evidence/2459/R13-unique.txt | exit 0；含 `STATUS=409`、`CODE=40900`、`IAM_CALLS=0` | PASS | `STATUS=200` → FAIL | dev |
| R14 | `bash scripts/verify-2459.sh r14` | docs/evidence/2459/R14-gate.txt | exit 0；含 `FULL_SUITE_EXIT=0`、`PLAN_SOURCE=PASS` | PASS | `P5=NA` 或 sha 不一致 → FAIL | dev |

## Artifact Provenance

- R1–R11（脚本生成，首行 provenance）：写 `# build=1.0.<短码>`、`# commit=<40 位 sha>`、`# dirty=false` 三个字段（`git rev-parse --short=8 HEAD` + `git rev-parse HEAD` + `git status --porcelain`），同一 provenance 写入 `docs/evidence/2459/evidence.json`。
  - docs/evidence/2459/R1-iam-first.txt
  - docs/evidence/2459/R2-iam-fail.txt
  - docs/evidence/2459/R3-idempotent.txt
  - docs/evidence/2459/R4-escalate.txt
  - docs/evidence/2459/R5-conflict.txt
  - docs/evidence/2459/R6-not-found.txt
  - docs/evidence/2459/R7-empty-name.txt
  - docs/evidence/2459/R8-reuse.txt
  - docs/evidence/2459/R9-drift.txt
  - docs/evidence/2459/R10-doc.txt
  - docs/evidence/2459/R11-build.txt
- R12/R13/R14（脚本生成，规则同 R1–R11）：`docs/evidence/2459/R12-perm.txt`、`docs/evidence/2459/R13-unique.txt`、`docs/evidence/2459/R14-gate.txt`，首行同样写 `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`。
- `docs/evidence/2459/evidence.json` 的 `build` 字段须匹配 `evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$`。

## Dependencies

- #2457

### 依赖与关联说明（非硬依赖）

- **前置 #2457（Owner O1=A / O4）**：先审后合并；`status:ready` 视为「已实现」可放行下游开发，但**合并顺序 #2457 → #2459**，最终证据在合并后的 `origin/develop` 基线上重跑。落地校验：`git merge-base --is-ancestor origin/issue-2457 origin/develop` 且 `grep -q 'func.*UpdateOrganizationName' backend/services/iam_client.go`。
- **下游 #2455**（员工上下文显示商户/组织名）消费 `merchants.name` 镜像语义。
- **beaconiam 唯一索引**：按 D8 已在 `HiJohns/beaconiam#501` 立项（`(namespace_id, lower(name))` 唯一索引 + `UpdateOrg`/`Create` 409 校验；含迁移 fail-fast 与启动断言）。本单只做客户端预检，不阻塞。

### Deployment-side Follow-ups（部署侧，不计入矩阵）

| 待办 | 责任人 | 前置 | 验收 |
|---|---|---|---|
| 预生产改名回执（原 R12） | ops / Owner | 本单合入并部署；beaconiam `UpdateOrg` 可用 | 回执列出「旧名 → 新名」、IAM 组织名与 `merchants.name` 一致、namespace 内无重复；`RESULT: PASS`；由 `[Ops-Followup of #2459]`（`waitForConfirm`）承载 |
| 存量商户名漂移只读清单核对 | ops | 本单 `--site-name-drift` 的 `MERCHANT_DRIFT` 段 | 清单与实际一致；改名由 Owner 授权后执行 |

### Workload

- Size: M
- Basis: 1 个 handler 改造（含权限门控与唯一预检）+ 1 个漂移函数扩展 + 1 篇接口文档 + 2 个测试文件 + 1 个验证脚本 + **14 条证据行（R1–R14）**；无 migration、无新 API。
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/merchant.go`
- `backend/services/site_name_drift.go`
- `backend/main.go`
- `backend/handlers/merchant_update_iam_first_2459_test.go`
- `backend/services/merchant_name_drift_2459_test.go`
- `scripts/verify-2459.sh`
- `docs/spec/api/api.md`
- `docs/evidence/2459/R1-iam-first.txt`
- `docs/evidence/2459/R2-iam-fail.txt`
- `docs/evidence/2459/R3-idempotent.txt`
- `docs/evidence/2459/R4-escalate.txt`
- `docs/evidence/2459/R5-conflict.txt`
- `docs/evidence/2459/R6-not-found.txt`
- `docs/evidence/2459/R7-empty-name.txt`
- `docs/evidence/2459/R8-reuse.txt`
- `docs/evidence/2459/R9-drift.txt`
- `docs/evidence/2459/R10-doc.txt`
- `docs/evidence/2459/R11-build.txt`
- `docs/evidence/2459/R12-perm.txt`
- `docs/evidence/2459/R13-unique.txt`
- `docs/evidence/2459/R14-gate.txt`
- `docs/evidence/2459/pre-ready-2459.json`
- `docs/evidence/2459/plan.md`
- `docs/evidence/2459/evidence.json`

*Model: DeepSeek V4.1 Flash 1017*


