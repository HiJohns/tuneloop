## Accepted Plan — #2470 v2（孤儿面板接上游「孤儿端点」+ 旧 IAM 回退 + 清理路径同源）｜Rev.3

> **v2 说明（修 /work 报出的计划缺口）**：v1 只把 `List` 切到新端点，`Purge` 的实时复核仍用 `ListPendingUsersWithToken`（受 IAM 活跃关系过滤）→ 面板**能列出**无关系孤儿，但点「清理」会 `not an orphan` 100% 失败。v2 把 `Purge` 复核切到同一新方法（含同一回退），并新增 D5 + R7（清理成功）。同时把上游引用从重复单 `beaconiam#505` 更正为既有单 **beaconiam#502**。

### 背景与裁定

`GET /api/admin/iam-orphans` 现通过 `ListPendingUsersWithToken` 调 IAM `GET /api/v1/users?status=pending`，而 IAM 对非全局管理员**按活跃组织关系过滤**（`internal/handler/user_handler.go:128-134`）——「bind 失败」型孤儿没有关系，永远不返回。实测：IAM 有 2 个 pending、接口 `scanned=1`。

**Architect 裁定（D1）**：正确修法在 IAM 侧提供不被关系过滤的孤儿查询端点（已开 **HiJohns/beaconiam#502**），tuneloop 侧改造消费 + **旧 IAM 回退**，保证「先发 tuneloop 后发 IAM」也不炸。

### 变更点

1. `backend/services/iam_client.go`：新增 `ListOrphanCandidatesWithToken(token string, minAgeMinute int)` → `GET /api/v1/users/orphans?min_age_minute=…&page=…&page_size=…`；返回 `users[]` + `total` + `scanned_pending`；
2. 同一方法内**能力探测**：若 IAM 返回 `404/405`（旧版本无该端点）→ 回退调用既有 `ListPendingUsersWithToken`，并 `log.Printf("[IAMOrphan] orphan endpoint unsupported, fallback to scoped pending list")`，返回 `Unsupported=true` 标记；
3. `backend/handlers/iam_orphan.go`：`List` 与 **`Purge` 的实时复核** 均改用新方法（`Purge` 失败时同样回退到旧方法，且回退路径下不得把「不在旧列表」直接判为 `not an orphan`——改为按四条判定本地复算，见 D5）；`scanned` 取 `scanned_pending`（新端点）或 `len(pending)`（回退路径）；响应新增 `data.fallback_unsupported`（bool）与 `data.endpoint`（`orphans|pending`）便于前端/回执区分；
4. 前端（`frontend-pc/src/pages/System/UserManagement.jsx`）：当 `fallback_unsupported=true` 时在弹窗顶部显示一行中性提示「当前 IAM 版本不支持全量孤儿扫描（仅显示可见 pending）」；
5. 不改 `orphanFromPending` 的四条判定语义（仍以 `pending + org_id 空 + 本地无行 + ≥min_age` 为准）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 新端点消费 | `backend/services/iam_client.go` | `GET /api/v1/users/orphans` | fake IAM 返回 1 条无关系孤儿 | 后端 | 请求路径含 `/users/orphans`、`min_age_minute=60`；解析 `users`/`total`/`scanned_pending` | R1 |
| 旧 IAM 回退 | 同上 | 同上 | fake IAM 返回 404 | 后端 | 回退调 `/users?status=pending`；`Unsupported=true`；日志含 `fallback` | R2 |
| 面板可见性（核心回归） | `backend/handlers/iam_orphan.go` | `GET /api/admin/iam-orphans` | IAM 侧存在**无关系** pending 用户 | 后端 | 该用户出现在 `data.list`（修复前为空） | R3 |
| 回退标记透出（evidence_type: static，Owner 豁免：文案接线层；浏览器走 ops follow-up） | 同文件 + PC 页面 | 同上 | `Unsupported=true` | 后端 + PC | 响应含 `fallback_unsupported=true`；页面渲染「当前 IAM 版本不支持全量孤儿扫描」中性提示 | R4 |
| 判定语义不变 | `backend/handlers/iam_orphan.go` | 同上 | 有本地行 / 有 org_id / 未满 60 分钟 | 后端 | 三类均被排除（`localIAMSubs` / `org_id` / `age`） | R5 |
| 清理路径同源（v2 新增） | `backend/handlers/iam_orphan.go` | `POST /api/admin/iam-orphans/purge` | 无关系 pending 孤儿 | 后端 | `purged=1`；不出现 `not an orphan` | R7 |
| 构建与测试 | `backend` + PC | `go build ./...` + 定向测试 + `npm run build` | — | 后端 + PC | 全绿 | R6 |
| 全量后端回归（独立 Issue gate，**基线差异口径**） | `backend` | `go test ./... -timeout 20m`（HEAD 与 `origin/develop` 基线各跑一次） | 修后树 | 后端 | `FULL_SUITE_EXIT` 允许为 1（基线已有 3 个与本单无关的失败）、`FULL_GATE_NEW_FAILURES=0`（HEAD 失败集 ⊆ baseline）、`PANIC_COUNT=0` | R8 |
| 接口/镜像语义文档 | `docs/spec/api/api.md`、`docs/topics/iam/iam-notes.md` | grep | 修后树 | 文档 | 含 `users/orphans`、`fallback_unsupported`、`scanned_pending` 三项表述 | R9 |

## Input Truth & Tool Preflight

- **Source of truth**：IAM 侧过滤事实（`beaconiam/internal/handler/user_handler.go:128-134`）+ 实测 `scanned=1`（IAM 2 个 pending）；消费侧现状 `backend/handlers/iam_orphan.go:117`。
- **差异核对**：改造后 `scanned` 语义必须写清来源（`scanned_pending` 或回退的 `len(pending)`），前端文案随 `endpoint` 变化。
- **损坏输入拒绝**：端点缺 `users` 字段 → 视为协议错误并回退到旧路径（不是返回空列表）。
- **工具盲区**：跨仓端点不可在本地验证 → 由 **HiJohns/beaconiam#502** 交付后，用预生产回执（`waitForConfirm`）验证真实数据；本单测试用 fake IAM 覆盖协议与回退。

## Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 修法层级 | IAM 侧新增孤儿端点（beaconiam#505）为**唯一正确修法**；tuneloop 不自建绕过可见性的查询 |
| D2 | 部署顺序 | tuneloop 先发（带回退），IAM 后发；回退期面板显示「能力未启用」提示，不静默 |
| D3 | `scanned` 语义 | 必须反映「全量 pending 数」；回退路径下显式标注 `fallback_unsupported` |
| D4 | 判定语义 | 四条孤儿判定不变（`orphanFromPending` 不改） |
| D5 | List/Purge 同源 | `List` 与 `Purge` 复核必须用同一新方法（同一回退）；回退路径下 `Purge` 按四条判定本地复算，**不得**因「旧列表未命中」而拒绝真正的无关系孤儿 |

## Out of Scope

- 不改 IAM 的用户可见性模型本身（仅新增专用只读端点）；
- 不改 `beaconiam` 其他 API；
- 不做孤儿自动清理（仍由管理员在面板显式操作）；
- 不处理「平台 owner 无 namespace」的既有全局分支。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | fake IAM：`/users/orphans` 返回 `{users:[{id:pending-norel}],total:1,scanned_pending:2}` | `ListOrphanCandidatesWithToken` | `docs/evidence/2470/R1-client-orphans.txt` | 请求路径/参数正确；`Unsupported=false`；解析出 1 条 | 走到旧路径 → FAIL |
| R2 | fake IAM：`/users/orphans` → 404 | 同上 | `docs/evidence/2470/R2-client-fallback.txt` | 回退调用 `/users?status=pending`；`Unsupported=true`；日志含 `fallback` | 直接失败/空列表 → FAIL |
| R3 | fake IAM 返回 1 条无关系 pending（65 分钟）+ 1 条有本地行 | `GET /api/admin/iam-orphans` | `docs/evidence/2470/R3-panel-visibility.txt` | `data.list` 含无关系那条；有本地行那条被排除 | 无关系那条缺失 → FAIL |
| R4 | 回退场景 | 同上 | `docs/evidence/2470/R4-fallback-flag.txt` | 响应 `fallback_unsupported=true` | 静默无标记 → FAIL |
| R5 | 三条负例（有本地行 / org_id 非空 / 30 分钟） | 同上 | `docs/evidence/2470/R5-filter-semantics.txt` | 三者均不在 `list` | 任一被列出 → FAIL |
| R7 | fake IAM 返回 1 条无关系孤儿 | `POST /api/admin/iam-orphans/purge` | `docs/evidence/2470/R7-purge.txt` | `purged=1`；该 IAM 用户被删除；不出现 `not an orphan` | 清理被拒 → FAIL |
| R6 | HEAD 源码 | `bash scripts/verify-2470.sh r6` | `docs/evidence/2470/R6-build.txt` | `BUILD_OK` + `GO_TEST_OK` + `NPM_BUILD_OK` | 任一失败 → FAIL |
| R8 | 修后树 + `origin/develop` 基线 | `go test ./... -timeout 20m`（两棵树） | `docs/evidence/2470/R8-gate.txt` | `FULL_GATE_NEW_FAILURES=0`、`BASELINE_FAIL_COUNT=3`（已知 `instrument_levels` / `merchant_members` 缺失类环境耦合）、`PANIC_COUNT=0` | 出现**基线之外**的新失败 → FAIL |
| R9 | 两篇文档 | grep | `docs/evidence/2470/R9-docs.txt` | `DOC_TOKENS=3` | 缺项 → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2470.sh r1` | docs/evidence/2470/R1-client-orphans.txt | exit 0；含 `PATH=/api/v1/users/orphans` 与 `UNSUPPORTED=0` | PASS | 走旧路径 → FAIL | dev |
| R2 | `bash scripts/verify-2470.sh r2` | docs/evidence/2470/R2-client-fallback.txt | exit 0；含 `UNSUPPORTED=1` 与 `FALLBACK=1` | PASS | 未回退 → FAIL | dev |
| R3 | `bash scripts/verify-2470.sh r3` | docs/evidence/2470/R3-panel-visibility.txt | exit 0；含 `LIST_HAS_NOREL=1` 与 `LIST_EXCLUDES_LOCAL=1` | PASS | 缺失 → FAIL | dev |
| R4 | `bash scripts/verify-2470.sh r4` | docs/evidence/2470/R4-fallback-flag.txt | exit 0；含 `FALLBACK_FLAG=1` | PASS | 无标记 → FAIL | dev |
| R5 | `bash scripts/verify-2470.sh r5` | docs/evidence/2470/R5-filter-semantics.txt | exit 0；含 `EXCLUDED_ALL=3` | PASS | 任一被列出 → FAIL | dev |
| R7 | `bash scripts/verify-2470.sh r7` | docs/evidence/2470/R7-purge.txt | exit 0；含 `PURGED=1` 与 `NOT_ORPHAN=0` | PASS | 清理被拒 → FAIL | dev |
| R6 | `bash scripts/verify-2470.sh r6` | docs/evidence/2470/R6-build.txt | exit 0；含 `BUILD_OK` 与 `GO_TEST_OK` 与 `NPM_BUILD_OK` | PASS | 任一失败 → FAIL | dev |
| R8 | `bash scripts/verify-2470.sh r8` | docs/evidence/2470/R8-gate.txt | exit 0；含 `FULL_GATE_NEW_FAILURES=0`、`PANIC_COUNT=0`、`BASELINE_FAIL_COUNT=` | PASS | 新增失败 → FAIL | dev |
| R9 | `bash scripts/verify-2470.sh r9` | docs/evidence/2470/R9-docs.txt | exit 0；含 `DOC_TOKENS=3` | PASS | 缺项 → FAIL | dev |

## Artifact Provenance

- R1–R6（脚本生成，首行 provenance）：写 `# build=1.0.<短码>`、`# commit=<40 位 sha>`、`# dirty=false`。
  - docs/evidence/2470/R1-client-orphans.txt
  - docs/evidence/2470/R2-client-fallback.txt
  - docs/evidence/2470/R3-panel-visibility.txt
  - docs/evidence/2470/R4-fallback-flag.txt
  - docs/evidence/2470/R5-filter-semantics.txt
  - docs/evidence/2470/R6-build.txt
  - docs/evidence/2470/R8-gate.txt
  - docs/evidence/2470/R9-docs.txt
  - docs/evidence/2470/R7-purge.txt
- gate（由 /work 执行）：`~/.config/opencode/scripts/pre-ready-check.sh 2470 --summary-file … --changed-files … --evidence-file docs/evidence/2470/evidence.json --report docs/evidence/2470/pre-ready-2470.json`；报告 `build` 须匹配 `evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$`。

## Dependencies

- none

### 跨仓前置（非本仓 Issue 号，不参与依赖解析）

- **HiJohns/beaconiam#502**：孤儿查询端点。不阻塞本单编码与 fake IAM 测试；真实数据可见性在预生产由 `waitForConfirm` 覆盖。

### 依赖说明（非声明）

- tuneloop 侧必须在 IAM 端点缺失时回退（D2），因此本单可先合入；真实可见性回归依赖 beaconiam#505 部署后再执行。
- 关联：#2369（面板来源）、#2382（预生产回执被本缺陷阻塞）、#2469（同面板空态/告警 UX）。

### Workload

- Size: M
- Basis: 1 个 IAM 客户端方法（新端点 + 回退）+ 1 个 handler 改造 + 1 个前端提示 + 2 个测试文件 + 1 个验证脚本 + 6 条证据行；跨仓依赖 1 个
- Confidence: high

### Files（变更文件清单）

- `backend/services/iam_client.go`
- `backend/handlers/iam_orphan.go`
- `backend/services/iam_orphan_client_2470_test.go`
- `backend/handlers/iam_orphan_visibility_2470_test.go`
- `frontend-pc/src/pages/System/UserManagement.jsx`
- `docs/spec/api/api.md`
- `docs/topics/iam/iam-notes.md`
- `scripts/verify-2470.sh`
- `docs/evidence/2470/R1-client-orphans.txt`
- `docs/evidence/2470/R2-client-fallback.txt`
- `docs/evidence/2470/R3-panel-visibility.txt`
- `docs/evidence/2470/R4-fallback-flag.txt`
- `docs/evidence/2470/R5-filter-semantics.txt`
- `docs/evidence/2470/R6-build.txt`
- `docs/evidence/2470/R8-gate.txt`
- `docs/evidence/2470/R9-docs.txt`
- `docs/evidence/2470/R7-purge.txt`
- `docs/evidence/2470/pre-ready-2470.json`
- `docs/evidence/2470/evidence.json`

*Model: DeepSeek V4.1 Flash 1017*




