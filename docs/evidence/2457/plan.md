## Accepted Plan — #2457（v4：UpdateSite 改名先写 IAM + 名称规范化 + 锚点绕过封堵 + 全量 gate 证据）

> **v4 变更（采纳 2026-11-23T22:0x Round 1 审计 Findings A/B/C/D）**：
> ① **R1/R6 收紧**——`req.Name` 必须先 `TrimSpace` 再同时用于「校验 / IAM 请求 / 本地写入」，`nameChanged=false` 时不得覆盖 `name` 列（修复「前后空格制造漂移」与「锚点守卫被空格绕过」）；新增 `NO_DRIFT=1`、`ANCHOR_BYPASS=0` 断言。
> ② **R2 期望改为 502**（D3 默认映射 `502/50000`），并禁止用常量冒充真实状态码（`REAL_STATUS=1`）。
> ③ **R15 改为可执行的全量 gate 证据**：`--full-gate-cmd` 跑 `go test ./handlers|./services`，产物含 `FULL_SUITE_CMD / FULL_SUITE_EXIT=0 FAIL_COUNT=0 PANIC_COUNT=0`。
> ④ 新增 `### Delivery Self-Proof`，要求 Work Summary 的 artifact provenance 与实际首行逐字一致。
> R3–R5、R7–R12、R14 与 D1–D8 不变。

> **v3 变更（采纳 2026-11-23T21:08 Plan Gap）**：① `### Files` 补 `backend/handlers/membership_domain_explicit_2261_test.go`；② 新增 `### 被翻转的既有断言`；③ 新增 R14（既有 2261 多租户隔离用例在本单语义下仍绿）与 R15（gate 四源一致）；④ 存量 3 条对齐从矩阵移出为部署侧待办（不计入矩阵，避免不可产出的证据行）。R1–R12 与 Decision Lock D1–D7 不变。

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| `UpdateSite` 只写本地 `sites.name`，全程不调 IAM | `backend/handlers/site.go:547-557` |
| `CreateSite` 建网点时把 `req.Name` 传给 IAM `CreateOrganization`（建时一致） | `backend/handlers/site.go:337-357` |
| 后端 `IAMClient` **没有** `UpdateOrganization` 系列方法 | `backend/services/iam_client.go`（Create/Delete/Get/List 有，Update 无） |
| beaconiam 已有 `PUT /api/v1/organizations/:id`（`UpdateOrg`，可改 `name`），`RequirePermission(PermissionAdmin)` + `checkOrgNamespaceAccess` | `beaconiam/cmd/api/main.go:286`、`internal/handler/org_handler.go:586-646` |
| 唯一 IAM→本地方向是手动 `SyncOrganizations`（IAM wins：`existingSite.Name != org.Name` 时覆盖本地） | `backend/handlers/iam_proxy.go:702-806` |
| 预生产 3/3 网点名与 IAM 组织名不一致（卡丹萨乐器×2 vs 海淀店/卡丹萨西铁营；北京中转站 vs tuneloop） | #2457 正文证据 |

结论：`sites.name` 当前是「建时同步、改时不同步」的半镜像 ⇒ 改名必然漂移；`SyncOrganizations` 会把漂移静默回滚成 IAM 名，用户看到「改了又变回去」。

### 变更点

1. `backend/services/iam_client.go`：新增 `UpdateOrganizationName(orgID, name string) error` 与 `UpdateOrganizationNameWithToken(token, orgID, name string) error`，走 `PUT /api/v1/organizations/{id}`，body `{"name": name}`；状态码映射为可判别错误（403 → `ErrIAMForbidden`、404 → `ErrIAMOrgNotFound`、409 → `ErrIAMOrgNameConflict`、其他 5xx → 原始错误）。
2. `backend/handlers/site.go` `UpdateSite`：
   - 先按 `id + tenant_id + deleted_at IS NULL` 取原 site（`SELECT ... FOR UPDATE` 不需要；仅取 `name/org_id/tenant_id`）。
   - **名称变更前置校验**（全部在 IAM 调用之前）：
     a. `req.Name` 去空白后为空 → `400/40002`；
     b. `site.OrgID == ""` → `400/40002`（无 IAM 组织绑定，禁止本地单边改名）；
     c. `site.OrgID == site.TenantID` → `400/40002`（商户/命名空间锚点网点，改名等于重命名商户组织）；
     d. 同租户重名（`tenant_id` 相同、`lower(name)` 相同、排除自己、`deleted_at IS NULL`）→ `409/40900`。
   - **名称规范化（v4，Findings A）**：`newName := strings.TrimSpace(req.Name)` 之后，**校验、IAM 请求体、本地写入三处必须使用同一个 `newName`**；`nameChanged = newName != site.Name`，未变时**不得覆盖 `name` 列**（只更新其它字段），否则用前后空格即可绕过锚点/org 绑定/重名守卫或制造 `sites.name` ≠ `organizations.name` 漂移。
   - 名称未变 → 跳过 IAM，直接更新其余字段（幂等，不发 IAM 请求）。
   - 名称变化 → **先** `UpdateOrganizationNameWithToken(userToken, ...)`；若返 `403` 且操作者已通过本地 `SysPermOrganizationUpdate`（bit 13）→ 用服务凭据 `UpdateOrganizationName(...)` 重试一次并 `log.Printf("[UpdateSite] IAM rename escalated to service credentials user=%s org=%s")`；仍失败则按错误映射返回（403/40301、404/40400、409/40900、其余 502/50000），**本地不写**。
   - IAM 成功后写本地；本地写失败 → `500/50000` + 文案含「IAM 已改名，本地同步失败，请重试」（重试幂等：此时同名分支跳过 IAM 只补本地）。
3. `backend/services/site_name_drift.go`：纯函数 `ComputeSiteNameDrift(local []SiteNameRef, iam []OrgNameRef) []DriftRow`（按 `org_id` 对齐，输出 `site_id/org_id/local_name/iam_name`），供漂移报告与单测共用。
4. `backend/main.go`：新增只读 flag `--site-name-drift`（打印漂移表，**仅 dry-run，无写路径**，退出码 0；无差异时打印 `DRIFT: 0`）。
5. `docs/spec/api/api.md`：`PUT /api/merchant/sites/:id` 段落补「先更新 IAM 组织名、成功后写本地缓存」；`sites.name` 语义 = IAM `organizations.name` 的本地镜像；商户锚点网点禁止改名。
6. 测试：`backend/handlers/site_update_iam_first_2457_test.go`（fake IAM `httptest` + `services.SetIAMInternalURLForTesting`，R1–R8）、`backend/services/iam_client_update_org_2457_test.go`（R9）、`backend/services/site_name_drift_2457_test.go`（R10）。
7. **既有用例适配（v3 新增）**：`backend/handlers/membership_domain_explicit_2261_test.go` 注入 fake IAM（事务见下「被翻转的既有断言」）；断言保持不变（自有租户 200 / 跨商户 404 / on-off 等价）。
   注意：该 fake 必须同时提供 `POST /api/v1/auth/token`（client-credentials）与 `PUT /api/v1/organizations/:id`，否则服务凭据升级路径会 401；并需 `t.Setenv("IAM_NAMESPACE","test-ns")` + `t.Setenv("IAM_SECRET","test-secret")`（`services.NewIAMClient()` 缺 env 会 `log.Fatal`）。
8. 不改 `SyncOrganizations` 的 IAM→本地方向；不改 beaconiam 代码。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| 改名 IAM 优先 + 名称规范化 | `backend/handlers/site.go` | `PUT /api/merchant/sites/:id` | ①`name="B"` ②`name=" B "`（前后空格），IAM 200 | 后端 | 先调 IAM `PUT /organizations/{org_id}` 再写本地；**IAM body 与本地列均为 trim 后的 `B`**（`NO_DRIFT=1`） | R1 |
| IAM 失败不改本地 | `backend/handlers/site.go` | 同上 | IAM 500（未匹配 sentinel → D3 默认分支） | 后端 | 返回 **HTTP 502 + code 50000**（真实响应码，不得用常量冒充）；`sites.name` 保持旧值 | R2 |
| IAM 重名映射 | `backend/handlers/site.go` | 同上 | IAM 409 | 后端 | 返回 409/40900，本地不变 | R3 |
| 403 服务凭据升级 | `backend/handlers/site.go` | 同上 | 用户 token 403、服务凭据 200 | 后端 | 第二次调用带服务凭据，返回 200 且本地改名 | R4 |
| 同名幂等 | `backend/handlers/site.go` | 同上 | name 未变 | 后端 | 不调用 IAM，其余字段仍更新 | R5 |
| 商户锚点守卫（含空格绕过） | `backend/handlers/site.go` | 同上 | ①`org_id == tenant_id`，`name="B"` ②锚点 `name="<当前名> "`（trim 后等于现值） | 后端 | 两种都 400/40002；不调 IAM、不写库；`ANCHOR_BYPASS=0`（本地名不变） | R6 |
| 同商户重名预检 | `backend/handlers/site.go` | 同上 | 租户内已有同名 | 后端 | 409/40900；不调 IAM | R7 |
| 空名称拒绝 | `backend/handlers/site.go` | 同上 | name 为空白 | 后端 | 400/40002；不调 IAM | R8 |
| IAM 客户端契约 | `backend/services/iam_client.go` | IAM `PUT /organizations/:id` | 200/403/404/409/500 | 后端 | 路径与 body 正确；错误可判别 | R9 |
| 漂移清单纯函数 | `backend/services/site_name_drift.go` | 单元测试 | 3 条不一致 / 0 条不一致 | 后端 | 3 行（含 org_id 与两侧名称）/ 0 行 | R10 |
| 语义文档同步 | `docs/spec/api/api.md` | grep | — | 文档 | 含 IAM 优先、镜像语义、锚点禁区三项表述 | R11 |
| 构建与静态检查 | `backend` | `go build ./...` + `go vet` | — | 后端 | exit 0 | R12 |
| R14 既有 2261 多租户隔离用例仍绿 | `backend/handlers/membership_domain_explicit_2261_test.go` | `PUT /api/merchant/sites/:id`（该用例自带 fixture） | siteA/siteB `org_id` 为随机 uuid、无同名冲突；fake IAM 200 | 后端 | 自有租户改名 200、跨商户 404、on/off 等价四项断言全部保持（未 skip、未放宽） | R14 |
| R15 gate 四源一致 + 全量套件 | `docs/evidence/2457/pre-ready-2457.json`、`docs/evidence/2457/plan.md` | `/work` 以 `--full-gate-cmd` 跑一次 pre-ready（含 `cd backend && go test ./handlers -count=1 -timeout 15m` 与 `./services`），随后读校验 `plan-source.sh --issue 2457` | 修后树（干净） | 后端 | gate 报告 `result=PASS` **且含全量套件结果**（`FULL_SUITE_EXIT=0 FAIL_COUNT=0 PANIC_COUNT=0`，P5 不得为 N/A）；`PLAN_SOURCE: PASS`（四源同 sha） | R15 |

### 被翻转的既有断言（AGENTS.md §语义变更，强制）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `backend/handlers/membership_domain_explicit_2261_test.go:119-120` | `PUT /merchant/sites/{siteA}` body `{"name":"renamed-A"}` 在无 IAM stub 下**恒 200**（`UpdateSite` 只写本地） | 同一请求在注入 fake IAM 后仍 **200**；未注入 IAM 时返回 5xx（因为 IAM-first，D3） | 本单把 `UpdateSite` 改为 IAM 优先（D1/D3）；该用例的 fixture `org_id` 为随机 uuid 且无 IAM stub，属于「旧语义下才成立」的隐含前提，必须显式化而不是放宽断言 |
| `backend/handlers/membership_domain_explicit_2261_test.go:121` | 跨商户更新 **404** | 保持 **404**（不翻转） | 新实现先按 `id + tenant_id` 取 site，跨商户在 IAM 调用之前即 404，语义更严格且一致 |
| 该文件其余断言（ownOff/crossOff/on-off 等价） | 200/404/等价 | 保持不翻转 | 本单不改成员列表与租户作用域语义 |
| `docs/evidence/2457/R2-iam-fail.txt`（v3 合同） | `STATUS=500`（测试常量冒充） | `STATUS=502` + `REAL_STATUS=1`（取自真实 `w.Code`） | v4 Findings B：D3 默认映射为 `502/50000`，计划期望写错且测试未测量真实值 |
| `docs/evidence/2457/R1-iam-first.txt`（v3 合同） | 仅 `IAM_CALLS=1` + `ORDER=iam_before_local` | 追加 `IAM_BODY`（trim 后）+ `NO_DRIFT=1` | v4 Findings A：需证明空格输入下 IAM 与本地同名 |
| `docs/evidence/2457/R6-anchor-guard.txt`（v3 合同） | 仅锚点 `name="B"` → 400 | 追加锚点 `name="<现值> "` → 400 + 本地名不变（`ANCHOR_BYPASS=0`） | v4 Findings A：空格可绕过锚点守卫 |

**禁止**：删除/跳过该用例、把 404 放宽为 4xx、或用 `t.Skip` 掩盖；只允许「注入 fake IAM + 断言不变」。

### Deployment-side Follow-ups（部署侧，不计入矩阵）

| 待办 | 责任人 | 前置 | 验收 |
|---|---|---|---|
| 预生产 3 条 `sites.name`（卡丹萨乐器×2、北京中转站）按 IAM 组织名对齐 | Owner 授权 → ops | 本单合入并部署；先跑 `--site-name-drift` 只读清单（R10 复用） | 回执列出 3 行「旧名 → 新名」+ `RESULT: PASS`；由关闭本单时创建的 `[Ops-Followup of #2457]` 跟踪 Issue（`waitForConfirm`，无 `status:*`）承载 |

### Delivery Self-Proof（/work 交付必填，v4 新增）

Work Summary 必须包含四节且可核验：
1. **红绿实证**：R1/R2/R6 各给出「缺陷版断言失败输出 → 修复版通过输出」（去掉 trim 归一化 / 去掉锚点守卫 / 恢复常量化，三处红例）。
2. **Accepted Plan 对照**：14 行逐行 `完成` + 证据路径，不得合并/改行名。
3. **测试保真**：说明每个断言键的取值来源（真实 `w.Code` / DB 查询 / 纯函数返回值），**禁止常量**（D10）。
4. **端到端验收**：本单为 API 语义变更，给出「改名前 IAM 组织名 → 改名后 IAM 组织名 → 本地 `sites.name`」三元一致的实际输出。
5. **provenance 一致性**：Summary 中声明的 artifact 首行（build/commit/dirty）必须与文件实际首行**逐字一致**；`pushed=yes/待 push` 必须与 `git ls-remote` 实际状态一致（v3 曾出现 2adc04f88 vs d823a4f7 的错报）。

## Bug Reflection Matrix

| # | 缺陷面 | 触发条件 | 根因 | 修复点 | 覆盖行 | 负例断言 |
|---|---|---|---|---|---|---|
| B1 | 改名后本地与 IAM 漂移 | 任何网点改名 | UpdateSite 不调 IAM | 变更点 2 名称分支 | R1 | 只写本地不调 IAM → R1 失败 |
| B2 | IAM 失败仍改本地（fail-open） | IAM 5xx/超时 | 无 IAM 调用即无失败分支 | 变更点 2 错误映射 | R2 | IAM 500 后本地 name 变化 → R2 失败 |
| B3 | IAM 重名被当 500 | 目标组织名已存在 | 客户端无 409 映射 | 变更点 1 错误映射 | R3 | 返回 500 而非 409/40900 → R3 失败 |
| B4 | 商户管理员改名被 IAM 403 卡死 | 操作者 IAM 角色低于 ADMIN | 未定义降级策略 | 变更点 2 服务凭据升级 | R4 | 403 直接失败且不重试 → R4 失败 |
| B5 | 重复提交无谓 IAM 调用 | 表单原样保存 | 无同名短路 | 变更点 2 同名分支 | R5 | 同名仍发 IAM 请求 → R5 失败 |
| B6 | 重命名商户/命名空间组织 | 编辑锚点网点 | 未识别 `org_id == tenant_id` | 变更点 2 守卫 c | R6 | 锚点网点改名成功 → R6 失败 |
| B7 | 同商户两个同名网点 | 租户内重名提交 | 无预检（DB 约束归 #2455） | 变更点 2 守卫 d | R7 | 重名写入成功 → R7 失败 |
| B8 | 组织名被清空 | name 传空串/空白 | 无入参校验 | 变更点 2 守卫 a | R8 | 空名调用 IAM → R8 失败 |
| B9 | 客户端方法缺失导致无法同步 | 任意调用 | IAMClient 无 Update 系列 | 变更点 1 | R9 | 方法不存在/路径错 → R9 失败 |
| B10 | 存量漂移不可见 | 预生产 3 条不一致 | 无只读报告 | 变更点 3/4 | R10 | 3 条输入返回 0 行 → R10 失败 |
| B11 | 文档仍描述「只改本地」 | 后续实现回退 | 语义未落文档 | 变更点 5 | R11 | 缺 IAM 优先表述 → R11 失败 |
| B12 | 既有 2261 隔离用例被 IAM-first 打破 | 跑既有 handlers 套件 | 用例隐含「UpdateSite 不依赖 IAM」前提，未 stub | 变更点 7 适配 | R14 | 用例被 skip/放宽 404 → R14 失败 |
| B13 | 前后空格制造漂移 / 绕过锚点守卫 | `{"name":" B "}`、锚点 `{"name":"<现值> "}` | 校验用 trim 值、写库用原始值，`nameChanged` 与写入口径不一致 | 变更点 2 名称规范化 | R1 / R6 | `NO_DRIFT=0` 或 `ANCHOR_BYPASS=1` → 失败 |
| B14 | 证据键为常量（假通过） | 测试 `t.Logf` 写死状态码 | 断言未取真实响应 | 变更点 6 测试 | R2 | `REAL_STATUS=0` → 失败 |

## Input Truth & Tool Preflight

- **Source of truth**：IAM `organizations.name` 为唯一权威（#2457 正文 + #2455 Accepted Plan「`sites.name` = IAM 组织名镜像」）；`backend/handlers/site.go` 现状为被修复对象。
- **差异核对**：`grep -rn "models.Site{}" backend --include=*.go` 确认写 `sites.name` 的路径只有 `UpdateSite`（`site.go:548`）与 `SyncOrganizations`（`iam_proxy.go:790`，IAM→本地方向，保留）；`transit_site_admin.go:138` 只改中转网点其它字段，不在本单。
- **损坏输入拒绝**：空白 name → 400/40002；`org_id` 缺失或等于 `tenant_id` → 400/40002；租户内重名 → 409/40900；均为 IAM 调用前拒绝。
- **工具盲区**：fake IAM 只能验证调用顺序与 body，不能证明真实 beaconiam 的 `checkOrgNamespaceAccess` 行为 → 由部署侧 follow-up 在预生产实测一条（不计入矩阵）。
- **既有用例真值**：`membership_domain_explicit_2261_test.go` 的 `sites.org_id` 为随机 uuid（≠`tenant_id`，故不触发锚点守卫）、无同租户重名（故不触发 409 预检）；`backend/.env` 的 `BEACONIAM_INTERNAL_URL` 在测试进程内非空 → 不打 stub 会真实外呼并超时（实测 `expected 200 / actual 502`，31.07s）。
- **权限口径**：本地 `RequireSysPerm(SysPermOrganizationUpdate=13)` 已存在（`backend/main.go:528`），本单不改路由权限。

## Decision Lock

| ID | 项 | 裁定 | authority |
|---|---|---|---|
| D1 | `sites.name` 语义 | **IAM `organizations.name` 的本地镜像（写路径 IAM 优先；读路径不回查 IAM，避免列表 N+1）**；与 #2455 的「镜像」口径一致 | target_semantics |
| D2 | 锚点网点 | `sites.org_id == sites.tenant_id` 的网点禁止改名（改名等于重命名商户/命名空间组织），返回 400/40002 | target_semantics |
| D3 | 权限降级 | 用户 token 403 且本地 bit 13 已通过 → 服务凭据重试一次并记日志；不做静默只改本地。备选（直接 403 失败）已否：会让现有商户管理员改名流程整体不可用 | target_semantics |
| D4 | 失败语义 | IAM 成功、本地失败 → 500 + 「IAM 已改名，本地同步失败，请重试」；重试走同名分支补本地（幂等） | target_semantics |
| D5 | 空名称 | 400/40002，不做「忽略空值」 | target_semantics |
| D6 | 存量 3 条 | 不在代码里静默改名；只出只读清单（R10）；方向由 Owner 裁定，执行转入 `[Ops-Followup of #2457]` 跟踪 Issue（`waitForConfirm`，无 `status:*`），**不作为矩阵行** | target_semantics |
| D8 | 既有 2261 用例 | 只允许「注入 fake IAM + 断言不变」；`t.Skip`、放宽 404、删除用例一律视为验收规则破坏 → REJECT | target_semantics |
| D9 | 名称规范化 | `TrimSpace` 后的名称是唯一口径：守卫校验、IAM 请求体、本地写入三者必须一致；`nameChanged=false` 时不得覆盖 `name` 列 | target_semantics |
| D10 | 证据必须测量真实值 | 合同断言的键必须取自真实响应/真实 DB 行（`w.Code`、DB 查询）；禁止 `t.Logf` 写死常量；`STATUS=502` 为 D3 默认映射的唯一期望 | target_semantics |
| D11 | 全量 gate | 本单为独立 Issue：`/work` 必须在 ready 前以 `--full-gate-cmd` 跑一次 `go test ./handlers` + `./services` 并把结果写入 gate 报告与 R15 产物；`P5=N/A` 视为缺证据 | process |
| D7 | 唯一约束 | DB 唯一索引归 #2455；本单只做服务端预检 + 40900，不新增迁移 | target_semantics |

## Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | `PUT /api/merchant/sites/:id`（L997 起）只描述字段更新 | 补「名称变更先调用 IAM `PUT /organizations/{id}`，成功后写本地缓存；`sites.name` 是 IAM 组织名镜像；锚点网点与租户内重名分别返回 400/409」 | 本单改变写路径顺序与错误码，接口文档须与实现一致 |

## Out of Scope

- 不改 `SyncOrganizations`（IAM→本地）方向与清理逻辑；不新增写路径 CLI（只有 dry-run 报告）。
- 不改 beaconiam：`PUT /api/v1/organizations/:id` 已存在（2024 年起）。
- 商户改名（`UpdateMerchant` 只改本地 `merchants.name`，同模式缺陷）→ 已另立 Issue 跟踪，不在本单。
- DB 唯一索引 / 员工上下文展示（归 #2455）；空组织 token 登录回弹（归 #2454）。
- 中转网点（`transit_sites`）命名与同步；移动端站点编辑 UI（复用同一 API，无需改）。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | ①网点 `org_id=ORG1` 旧名 A，请求 `name=B` ②同网点请求 `name=" B "`；fake IAM 记录顺序与 body | `PUT /api/merchant/sites/:id` | `docs/evidence/2457/R1-iam-first.txt` | 两例 IAM 调用各 1 次且 body=`{"name":"B"}`；本地 name=`B`；调用序号 IAM < 本地；`NO_DRIFT=1` | IAM 0 次、本地先写、或本地为 `" B "` → FAIL |
| R2 | 同上，fake IAM 返回 500（未匹配 sentinel → D3 默认分支） | 同上 | `docs/evidence/2457/R2-iam-fail.txt` | 真实 `w.Code=502`、code=50000、本地 name 仍 A；`REAL_STATUS=1` | `STATUS` 取自常量、本地变 B、或返回 200 → FAIL |
| R3 | fake IAM 返回 409 | 同上 | `docs/evidence/2457/R3-conflict.txt` | 响应 409 且 code=40900；本地 name 仍 A | 500 或本地改名 → FAIL |
| R4 | fake IAM：带用户 token 403、带服务凭据 200 | 同上 | `docs/evidence/2457/R4-escalate.txt` | IAM 调用 2 次；第二次 Authorization 非用户 token；最终 200、本地 name=B | 只调 1 次或返回 403 → FAIL |
| R5 | 请求 name 与现值相同 | 同上 | `docs/evidence/2457/R5-idempotent.txt` | IAM 调用 0 次；`address` 等字段仍更新 | IAM 被调用 → FAIL |
| R6 | ①锚点网点（`org_id == tenant_id`）`name=B` ②锚点网点 `name="<现值> "` | 同上 | `docs/evidence/2457/R6-anchor-guard.txt` | 两例均 400/40002；IAM 0 次；本地名不变；`ANCHOR_BYPASS=0` | 任一例 200 / IAM 被调用 / 本地名变化 → FAIL |
| R7 | 同租户已有另一网点名 C | 同上 | `docs/evidence/2457/R7-duplicate.txt` | 409/40900；IAM 0 次 | 200 或 IAM 被调 → FAIL |
| R8 | name 为 `"   "` | 同上 | `docs/evidence/2457/R8-empty-name.txt` | 400/40002；IAM 0 次 | 200 或 IAM 被调 → FAIL |
| R9 | fake IAM 各状态码 | `services` 单测 | `docs/evidence/2457/R9-client.txt` | 请求方法 PUT、路径 `/api/v1/organizations/ORG1`、body 含 `"name"`；403/404/409 映射为对应错误 | 路径错误或错误不可判别 → FAIL |
| R10 | 3 条不一致 + 0 条不一致两组输入 | `services` 单测 | `docs/evidence/2457/R10-drift.txt` | 不一致组输出 3 行且字段齐全；一致组输出 0 行 | 3 条输入输出 0 行 → FAIL |
| R11 | `docs/spec/api/api.md` | grep | `docs/evidence/2457/R11-doc.txt` | 命中「先更新 IAM」「镜像」「锚点」三处表述 | 缺任一 → FAIL |
| R12 | HEAD 源码 | 构建/静态检查 | `docs/evidence/2457/R12-build.txt` | `go build ./...` exit 0；`go vet ./handlers ./services` exit 0 | 任一非 0 → FAIL |
| R14 | 既有 2261 fixture（siteA/siteB，随机 `org_id`，tenant 不同）+ fake IAM 200 | 该用例原样运行 | `docs/evidence/2457/R14-legacy-regression.txt` | 4 项断言全绿（own 200 / cross 404 / on-off 等价 ×2）；输出含 `LEGACY_2261=PASS` | 任一断言放宽、`SKIP` 出现 → FAIL |
| R15 | 修后干净树 + plan.md/evidence.json + `--full-gate-cmd` 产出的 gate 报告 | 读校验（`plan-source.sh` + gate 报告解析） | `docs/evidence/2457/R15-gate.txt` | `FULL_SUITE_EXIT=0`、`FAIL_COUNT=0`、`PANIC_COUNT=0`、`PLAN_SOURCE=PASS`、`GATE_PLAN_SHA_MATCH=1`、`P5!=NA` | P5=N/A（未跑全量）、gate 缺失、sha 不一致 → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2457.sh r1` | docs/evidence/2457/R1-iam-first.txt | exit 0；含 `IAM_CALLS=1`、`ORDER=iam_before_local`、`NO_DRIFT=1` | PASS | 缺 IAM 调用或 `NO_DRIFT=0` → FAIL | dev |
| R2 | `bash scripts/verify-2457.sh r2` | docs/evidence/2457/R2-iam-fail.txt | exit 0；含 `STATUS=502`、`REAL_STATUS=1`、`LOCAL_NAME=A` | PASS | `REAL_STATUS=0`（常量化）或本地被改 → FAIL | dev |
| R3 | `bash scripts/verify-2457.sh r3` | docs/evidence/2457/R3-conflict.txt | exit 0；含 `STATUS=409` 与 `CODE=40900` | PASS | 500 → FAIL | dev |
| R4 | `bash scripts/verify-2457.sh r4` | docs/evidence/2457/R4-escalate.txt | exit 0；含 `IAM_CALLS=2` 与 `ESCALATED=1` | PASS | 未重试 → FAIL | dev |
| R5 | `bash scripts/verify-2457.sh r5` | docs/evidence/2457/R5-idempotent.txt | exit 0；含 `IAM_CALLS=0` 与 `ADDRESS=updated` | PASS | IAM 被调用 → FAIL | dev |
| R6 | `bash scripts/verify-2457.sh r6` | docs/evidence/2457/R6-anchor-guard.txt | exit 0；含 `STATUS=400`、`IAM_CALLS=0`、`ANCHOR_BYPASS=0` | PASS | 空格绕过成功（`ANCHOR_BYPASS=1`）→ FAIL | dev |
| R7 | `bash scripts/verify-2457.sh r7` | docs/evidence/2457/R7-duplicate.txt | exit 0；含 `STATUS=409` 与 `IAM_CALLS=0` | PASS | 重名成功 → FAIL | dev |
| R8 | `bash scripts/verify-2457.sh r8` | docs/evidence/2457/R8-empty-name.txt | exit 0；含 `STATUS=400` 与 `IAM_CALLS=0` | PASS | 空名调用 IAM → FAIL | dev |
| R9 | `bash scripts/verify-2457.sh r9` | docs/evidence/2457/R9-client.txt | exit 0；含 `METHOD=PUT` 与 `PATH=/api/v1/organizations/` | PASS | 方法/路径错 → FAIL | dev |
| R10 | `bash scripts/verify-2457.sh r10` | docs/evidence/2457/R10-drift.txt | exit 0；含 `DRIFT_ROWS=3` 与 `DRIFT_ROWS_EMPTY=0` | PASS | 计数不符 → FAIL | dev |
| R11 | `bash scripts/verify-2457.sh r11` | docs/evidence/2457/R11-doc.txt | exit 0；含 `DOC_TOKENS=3` | PASS | 少于 3 → FAIL | dev |
| R12 | `bash scripts/verify-2457.sh r12` | docs/evidence/2457/R12-build.txt | exit 0；含 `BUILD_OK` 与 `VET_OK` | PASS | 构建失败 → FAIL | dev |
| R14 | `bash scripts/verify-2457.sh r14` | docs/evidence/2457/R14-legacy-regression.txt | exit 0；含 `LEGACY_2261=PASS` 与 `SKIPPED=0` | PASS | SKIP 或 404 放宽 → FAIL | dev |
| R15 | `bash scripts/verify-2457.sh r15` | docs/evidence/2457/R15-gate.txt | exit 0；含 `FULL_SUITE_EXIT=0`、`FAIL_COUNT=0`、`PLAN_SOURCE=PASS`、`P5=NA_ABSENT` | PASS | `P5=NA`（全量未跑）→ FAIL | dev |

## Artifact Provenance

- R1–R12（脚本生成，首行 provenance）：写 `# build=1.0.<短码>`、`# commit=<40 位 sha>`、`# dirty=false` 三个字段（`git rev-parse --short=8 HEAD` + `git rev-parse HEAD` + `git status --porcelain`），并把同一 provenance 写入 `docs/evidence/2457/evidence.json`。
  - docs/evidence/2457/R1-iam-first.txt
  - docs/evidence/2457/R2-iam-fail.txt
  - docs/evidence/2457/R3-conflict.txt
  - docs/evidence/2457/R4-escalate.txt
  - docs/evidence/2457/R5-idempotent.txt
  - docs/evidence/2457/R6-anchor-guard.txt
  - docs/evidence/2457/R7-duplicate.txt
  - docs/evidence/2457/R8-empty-name.txt
  - docs/evidence/2457/R9-client.txt
  - docs/evidence/2457/R10-drift.txt
  - docs/evidence/2457/R11-doc.txt
  - docs/evidence/2457/R12-build.txt
- R14/R15（脚本生成，同 R1–R12 provenance 规则）：`docs/evidence/2457/R14-legacy-regression.txt`、`docs/evidence/2457/R15-gate.txt`，首行同样写 `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`。
- `docs/evidence/2457/evidence.json` 的 `build` 字段须匹配 `evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$`。
- 全部产物仓库可见：`docs/evidence/2457/`。

## Dependencies

- 前置：无代码依赖；beaconiam `PUT /api/v1/organizations/:id` 自查已存在（`beaconiam` `org_handler.go` 自 Phase 4 起，路由 `main.go:286`）。
- 下游：**#2455**（`status:holdon`，deps #2457）——其 D1「`sites.name` = IAM 组织名镜像」由本单落地后解锁；本单不改索引/UI。
- 关联但不在本单：#2454（空组织 token 登录回弹）、#2456（PC idle timeout）、商户改名同模式缺陷（另立 Issue）。

### Workload

- Size: M
- Basis: 2 个实现文件（`site.go`、`iam_client.go`）+ 1 个新纯函数文件 + `main.go` flag + 3 个测试文件 + 1 个既有用例适配（2261）+ 1 个验证脚本 + 1 篇接口文档 + 14 条证据行（R1–R12、R14、R15）；v4 追加名称规范化与全量 gate 两项（无新增文件、无 migration、无外部 API 变更）。
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/site.go`
- `backend/services/iam_client.go`
- `backend/services/site_name_drift.go`
- `backend/main.go`
- `backend/handlers/site_update_iam_first_2457_test.go`
- `backend/services/iam_client_update_org_2457_test.go`
- `backend/services/site_name_drift_2457_test.go`
- `backend/handlers/membership_domain_explicit_2261_test.go`
- `scripts/verify-2457.sh`
- `docs/spec/api/api.md`
- `docs/evidence/2457/R1-iam-first.txt`
- `docs/evidence/2457/R2-iam-fail.txt`
- `docs/evidence/2457/R3-conflict.txt`
- `docs/evidence/2457/R4-escalate.txt`
- `docs/evidence/2457/R5-idempotent.txt`
- `docs/evidence/2457/R6-anchor-guard.txt`
- `docs/evidence/2457/R7-duplicate.txt`
- `docs/evidence/2457/R8-empty-name.txt`
- `docs/evidence/2457/R9-client.txt`
- `docs/evidence/2457/R10-drift.txt`
- `docs/evidence/2457/R11-doc.txt`
- `docs/evidence/2457/R12-build.txt`
- `docs/evidence/2457/R14-legacy-regression.txt`
- `docs/evidence/2457/R15-gate.txt`
- `docs/evidence/2457/pre-ready-2457.json`
- `docs/evidence/2457/plan.md`
- `docs/evidence/2457/evidence.json`

*Model: DeepSeek V4.1 Flash 1017*



