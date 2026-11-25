## Accepted Plan — #2513 S5 维修服务：剔除 `oid ↔ site_id` 误用（服务单无 site 维度，按 RS-API-2 显式化）

> Parent: #2509（同族 ID 域审计）｜关联：#2122（网点脱离维修服务）、#2134（`{tid,oid}` 归属集合）、#1974 T1（师傅直属商户）

> **Rev.2 勘误（2026-11-25，Round 1 审计 Bug 1；仅断言文本，不改验收规则）**
> gate 报告 `docs/evidence/2513/pre-ready-2513.json` 的顶层键是 `overall`，**不存在** `result` / `GATE_RESULT`；
> 原 R7 断言不可执行（`jq -e '.result == "PASS"'` → exit 1）。本修订**只**把断言文本对齐 artifact schema：
> Verification Matrix R7 期望结果、G0 R7 assertions 的 `GATE_RESULT=PASS` → `overall=PASS`；
> Evidence Contract R7 断言改为 `jq -e '.overall == "PASS"'`。验收语义（gate PASS、每行 `pass=true`、
> `dirty=false`、P13 在 `origin/issue-2513`）与所有验收工具/规则**均未改变**。
> 另：R1 采纳 Round 1 审计非阻断建议，增补 `MERCHANT_STAFF_ROWS=2`（商户直属员工 `Role=STAFF` +
> `oid=商户 org` 覆盖 `repairServiceMerchantLevel` 的 DB 分支），合同断言同步增补该项。
> `evidence.json` 的 `build/commit/dirty` 改由 `bash scripts/verify-2513.sh ledger` 从最新
> `fix(issue-2513)` commit 派生并写回，取消手工修改。

### 背景与现状核对（代码 + 规格为据）

| 事实 | 位置 |
|---|---|
| `ListPendingDispatch`：`if orgID != "" { site_id = orgID }` —— `repair_requests.site_id` 是 `sites.id` 域，`orgID` 是 `sites.org_id` 域 → **对任何带 oid 的账号恒空** | `backend/handlers/repair_service.go:1831-1835` |
| `ListTasks scope=site`：`(site_id = orgID OR site_id IS NULL)` → 商户账号只命中 `IS NULL` 行；**带 site_id 的存量行被静默漏掉** | `backend/handlers/repair_service.go:1882-1887` |
| `repairServiceStaffAllowed`：`rr.SiteID != oid → 拒绝` → 带 site_id 的行**对所有人恒拒**（ID 域不同） | `backend/handlers/repair_service.go:335-339` |
| 规格裁定：**服务单归属 = 维修师直属商户，`tenant_id` 回填，不再有 site 维度** | `docs/cases/repair-service.md:65` |
| 规格裁定：`scope=site` **仅商户层级**，`#2122` 商户组织闸门，**网点账号返回空** | `docs/cases/repair-service.md:271`（RS-API-2） |
| 规格裁定：`pending-dispatch` 是 `GET /repair-services?scope=site&status=done_repair` 的**兼容入口** → 必须同口径 | `docs/cases/repair-service.md:140`（RS-05） |
| 归属集合权威：`repairServiceTenantScope` 已处理共享 tenant/叶组织/商户 org 锚（#2248/#2338 D8） | `backend/handlers/repair_service.go:183-260` |
| 现有测试已锁定：网点账号 `scope=site` → 0；商户层级可见；跨租户 → 0 | `backend/handlers/repair_service_test.go:443-475`；`repair_service_receive_2116_test.go:321-350` |

**Issue 内部矛盾与裁定（必读）**：#2513「期望 4」要求「网点账号可见带 `site_id` 的那张（修复后 PASS）」，但 RS-API-2（`docs/cases/repair-service.md:271`）与已落地 #2122 明确规定**网点账号返回空**，且服务单**无 site 维度**（`:65`）。本计划以**规格 + 已关闭决策 #2122** 为准：

- 受影响方不是「网点账号」，而是**商户层级账号**（`oid` = 商户 org）——它们今天同样被 `site_id = oid` 打成空清单（这才是用户可见缺陷）；
- 因此本单交付：**商户层级**看到全部本商户服务单（含历史带 `site_id` 行）、**网点账号保持空**（显式化，不再靠 ID 域事故）；`site_id` 彻底退出维修服务的可见性/守卫维度。

### 变更点（`backend/handlers/repair_service.go`）

1. `ListPendingDispatch`（`:1823-1842`）：改为与 `ListTasks scope=site&status=done_repair` 同口径——

```go
if !repairServiceMerchantLevel(db, ctx) {           // #2122：纯网点账号不参与
	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": gin.H{"list": []interface{}{}, "total": 0}})
	return
}
scopes := repairServiceTenantScope(ctx)
if len(scopes) == 0 {
	c.JSON(http.StatusForbidden, gin.H{"code": 40300, "message": "access denied"})
	return
}
query := db.WithContext(database.IdentityCtx(ctx)).
	Where("type = ? AND status = ?", repairServiceTypeVal, models.RepairReqStatusDoneRepair).
	Where("tenant_id IN ?", scopes)                 // 无 site 过滤（#1974 T1 / RS-13）
```
2. 新增 `repairServiceMerchantLevel(db, ctx) bool`（唯一判定，供两处复用）：`business_role ∈ {system_admin, platform_staff, merchant_admin}` → true；否则 `oid != ""` 且 `SELECT 1 FROM merchants WHERE org_id = oid` 命中 → true；其余 false。（把 `ListTasks :1874-1880` 的既有内联判定抽出来复用，语义不变。）
3. `ListTasks scope=site`（`:1872-1894`）：用 `repairServiceMerchantLevel` 替换内联闸门；删除 `(site_id = ? OR site_id IS NULL)` 条件，保留 `tenant_id IN ?`（历史带 `site_id` 行不再被漏掉；NULL 行照旧可见）。
4. `repairServiceStaffAllowed`（`:311-341`）：删除 `rr.SiteID != oid` 分支（ID 域误用、且该维度按规格不存在），保留 `rr.TenantID` 非空 + `repairServiceTenantScope` 命中判定；补注释说明 #2513 与规格出处。

**行为变化（显式声明，D3）**：带 `site_id` 的历史服务单此前「任何人操作都被拒」，修复后「归属范围内的员工/师傅可操作」——这是**修复**而非扩权：跨租户/跨商户仍被 `repairServiceTenantScope` 拦住（R4 负例覆盖）。网点账号的可见性不变（列表仍空）。

**测试（新增 `backend/handlers/repair_service_2513_test.go`，自带夹具）**：夹具必须 `sites.id ≠ sites.org_id`（#2508/#2510 口径）；`merchants.org_id = 商户 org`；两行服务单 `rrLegacy`（`site_id = aID`）与 `rrNull`（`site_id` 空）；账号：商户层级（`tid = oid = 商户 org`）、纯网点（`tid = 商户 org`、`oid = aOrg`）、他租户商户。

- `TestRepairService2513_PendingDispatchMerchantSeesLegacy`：红（旧谓词 `site_id = oid` → 0）→ 绿（2 行）；并输出 `OLD_SITE_EQ_OID=0`；
- `TestRepairService2513_ListTasksSiteIncludesLegacy`：红（旧谓词 → 仅 `rrNull` 1 行）→ 绿（2 行）；
- `TestRepairService2513_StaffAllowedLegacySiteRow`：红（旧分支 `rr.SiteID != oid` → false）→ 绿（true）；
- `TestRepairService2513_Negative`：他租户商户 → 两接口 0 行、`staffAllowed` false；纯网点账号 → 两接口 0 行（#2122 保持）、`staffAllowed` 仍 true（与今日 NULL 行一致，D3）。

**文档**：`docs/cases/repair-service.md` RS-API-2 / RS-05 各补一句「`site_id` 非可见性维度：历史带 `site_id` 行不按 site 过滤；`pending-dispatch` 与 `scope=site` 同口径（#2513）」。

**证据**：新增 `scripts/verify-2513.sh`（`r1`–`r6` + `gate`）。

- Tags: bug, code:go, scope:backend

### Workload

- Size: M
- Basis: 1 个 handler 文件（3 处 + 1 个新 helper）+ 1 个新测试文件 + 1 个文档 + 1 个证据脚本 + 7 条证据行；无 migration、无前端改动
- Confidence: high

## Core Update

| 文档路径 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/cases/repair-service.md` | RS-API-2 只写「scope=site 仅商户层级，网点账号返回空」，未说明 `site_id` 非可见性维度；RS-05 未写 `pending-dispatch` 与 `scope=site` 的过滤一致性 | RS-API-2 补「`site_id` 不参与过滤（历史带 site 行不遗漏）」；RS-05 补「`pending-dispatch` 与 `scope=site&status=done_repair` 同口径（#2513）」 | 本轮 bug 根因是「规格写了无 site 维度、代码仍按 site 过滤」；把口径写进用例文档可防复发 |

## Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 触发条件 | 商户管理员/商户直属员工打开维修服务「待发回清单」（`GET /repair-services/pending-dispatch`）→ 恒空；`GET /repair-services?scope=site` 漏掉带 `site_id` 的存量单 | `repair_service.go:1831-1835`、`:1882-1887` |
| 2 | 根因 | 三处把 JWT `oid`（`sites.org_id`）当 `repair_requests.site_id`（`sites.id`）比较/过滤；而规格明确服务单**无 site 维度** | `docs/cases/repair-service.md:65,271` |
| 3 | 为何长期未发现 | `site_id IS NULL` 分支掩盖了 `ListTasks`；`pending-dispatch` 无集成测试；夹具此前把 site 与 org 混为同值 | `repair_service_test.go:443-475`（未覆盖带 site 行） |
| 4 | 同类扫描 | 同族：#2510（instrument_loss 写侧）、#2511/#2512（warehouse/user_rental）、S1 #2508（personnel，已修） | #2509 |
| 5 | 防复发（代码） | `site_id` 彻底退出维修服务可见性/守卫；唯一闸门 `repairServiceMerchantLevel` 复用；过滤仅 `tenant_id IN repairServiceTenantScope` | 变更点 1–4 |
| 6 | 防复发（测试） | 4 个新用例，夹具 `sites.id ≠ sites.org_id`，含带 `site_id` 存量行与红证据计数 | 变更点 5 |
| 7 | 防复发（文档） | RS-API-2 / RS-05 明确「`site_id` 非可见性维度」与两接口同口径 | Core Update |
| 8 | 假阴性反例 | 若任一接口仍留 `site_id = orgID` / `(site_id = ? OR site_id IS NULL)` / `rr.SiteID != oid` → R6 扫描 FAIL；若网点账号变为可见 → R4 FAIL | R4/R6 负例 |
| 9 | 回归面 | #2122 网点账号 scope=site 仍 0；#2134 叶组织行可见；#2248 共享 tenant 不泄漏；#2116 流程测试 | R5 |
| 10 | 未修边界 | `pending-dispatch` 与 `ListTasks` 的**响应形态**统一（当前一款返回 `total`、另一款 `len(list)`）、分页化 → 不在本单 | Out of Scope |

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 待发回清单：商户层级见历史带 site 行 | `backend/handlers/repair_service.go`、`backend/handlers/repair_service_2513_test.go` | `GET /repair-services/pending-dispatch` | 商户层级账号；行 `rrLegacy(site_id=aID)` + `rrNull` | backend（Postgres 测试库） | `MERCHANT_ROWS=2`、`MERCHANT_STAFF_ROWS=2`（STAFF 走 DB 分支）、`LEGACY_VISIBLE=1`、`OLD_SITE_EQ_OID_ROWS=0`（红证据） | R1 |
| R2 任务列表 `scope=site` 不再漏历史行 | 同上 | `GET /repair-services?scope=site` | 同上夹具 | backend | `SITE_SCOPE_ROWS=2`、`OLD_FILTER_ROWS=1`（红证据） | R2 |
| R3 单条守卫：历史行可操作 | 同上 | `repairServiceStaffAllowed`（经 `/repair-services/:id/quote` 等入口的公共判定） | 商户层级 + `rrLegacy` | backend | `ALLOWED_LEGACY=1`、`OLD_DIRECT_OID_COMPARE=0` | R3 |
| R4 负例：跨租户 / 网点账号口径 | 同上 | 上两接口 + `staffAllowed` | 他租户商户；纯网点账号 | backend | `OTHER_TENANT_ROWS=0`、`SITE_STAFF_ROWS=0`（#2122 保持）、`OTHER_TENANT_ALLOWED=0` | R4 |
| R5 既有维修服务套件回归 | 既有套件（`Repair*` / `ListTasks*`） | `scripts/verify-2513.sh r5` | #2116/#2122/#2134/#2248/#2255 全部用例 | backend | `REGRESSION_PASS=1`、`FAIL_COUNT=0` | R5 |
| R6 构建 + 同类扫描 | `scripts/verify-2513.sh` | `cd backend` 构建 + grep | 修后工作树 | backend | `BUILD_RC=0`、`SITE_ID_EQ_ORG=0`、`SITE_FILTER_IN_SERVICE=0`、`MERCHANT_GATE_SHARED=1` | R6 |
| R7 pre-ready gate（四源 + 定向测试 + 构建） | `docs/evidence/2513/pre-ready-2513.json` | `pre-ready-check.sh 2513` | 修后干净树 + 最新 Accepted Plan | backend | `overall=PASS`、P12 完整、P13 在 `origin/issue-2513` | R7 |

## Input Truth & Tool Preflight

- **Source of truth（口径唯一）**：`docs/cases/repair-service.md:65`（无 site 维度）、`:271`（RS-API-2：scope=site 仅商户层级）、`:140`（pending-dispatch 为兼容入口）；`backend/handlers/repair_service.go:183-260`（`repairServiceTenantScope` 归属集合）；`docs/cases/cases.md:289`（ID 域定义）。
- **差异核对（只读，已于 /analyze 执行）**：
  - `grep -n "site_id = ?\|site_id = orgID\|rr.SiteID != oid" backend/handlers/repair_service.go` → `:335-338`、`:1832`、`:1883` 三处；
  - `grep -rn "scope=site\|pending-dispatch" backend/handlers/*_test.go` → 现有断言只覆盖「网点账号 0 行 / 商户可见 / 跨租户 0 行」，**无带 `site_id` 存量行用例**；
  - `grep -n "SiteID" backend/handlers/repair_request.go | head` → 租赁维修（`repair_requests` 非 service 类型）仍用 `site_members.site_id` 写 `site_id`，故该列对 service 单恒空、对存量/异常行可能非空（正是被漏掉的行）。
- **损坏输入拒绝**：`repairServiceTenantScope` 为空（无 tid/oid 锚）→ `403/40300`（不放宽为「无条件放行」）；共享 tenant 值不进入范围（#2248 既有逻辑不动）；`rr.TenantID == ""` → `staffAllowed` 仍 false（既有规则）。
- **工具盲区声明**：
  - `go test ./handlers` 需 Postgres 测试库（`docs/ops/test-db-lifecycle.md`），本单只跑**定向**套件（`-run 'Repair|ListTasks'`），全量由 pre-ready P5 baseline 差分承担；
  - R6 grep 为文本级，不能识别动态拼接条件；以「三处特征串为 0 + `repairServiceMerchantLevel` 被两处调用」作为最小防漂移断言；
  - 生产库里「带 `site_id` 的 service 单」实际条数本单无法核实（dev 无样本），由 R1–R3 夹具证明语义，生产抽样留 Out of Scope。
- **证据时序**：fix commit 之后重跑 `bash scripts/verify-2513.sh r1` … `r6`；`evidence.json` 的 `build/commit/dirty` 由 `bash scripts/verify-2513.sh ledger` 从最新 `fix(issue-2513)` commit 派生并写回（禁止手改）。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | **服务单无 site 维度**：`site_id` 退出维修服务的可见性与守卫（不改为「站点集合成员判定」，因为该维度按规格不存在） | Architect | 本计划锁定（`docs/cases/repair-service.md:65,271` + 已关闭 #2122） | 若 Owner 要求「网点账号可见本站服务单」→ 那将反转 #2122/RS-API-2，须先改规格并另立 Issue，本单不猜做 |
| D2 | `pending-dispatch` 与 `GET /repair-services?scope=site` **同口径**（规格 `:140` 定义为兼容入口）；商户层级可见全商户（含历史带 `site_id` 行），网点账号返回空 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 守卫放宽的边界：移除 `rr.SiteID != oid` 后，**归属范围内**员工/师傅可操作带 `site_id` 的历史单（此前恒拒）；跨租户/跨商户仍拒 | Architect | 本计划锁定（仅「原本不可用的行」变为可用，不新增跨域访问） | 若审计要求「网点账号不得操作」→ 需 Owner 决策并另立 Issue（属 #2122 后端门禁补齐，超出本单） |
| D4 | Issue「期望 4」以 #2122 规格为准修订（网点账号保持空；红绿实证由**商户层级**账号承载） | Architect | 本计划锁定（见「Issue 内部矛盾与裁定」） | Owner 异议 → 回 Architect |
| D5 | 生产库「带 site_id 的 service 单」抽样与历史数据清理 → 不在本单 | Architect | 本计划锁定 | 审计若要求实测 → 转 Ops Follow-up |

## Out of Scope

- 反转 #2122（网点员工参与维修服务）→ 需 Owner + 规格变更，另立 Issue。
- `pending-dispatch` 与 `ListTasks` 的响应形态/分页统一（`{list,total}` 一致化、`len(list)` → count）→ 不在本单。
- 租赁维修（`type <> service`）的 `site_id` 语义与 `repair_request.go` 归属校验 → 不属 S5 范围。
- 历史数据清理（把存量 `site_id` 归一/清空）→ 涉及 DB 写入，需 Owner 授权另立。
- 生产数据抽样（`repair_requests WHERE type='service' AND site_id IS NOT NULL` 条数）→ 部署侧回执（D5）。
- 前端任何改动（H5/小程序/PC）；`docs/cases/repair-service.md` 之外的文档。
- 父任务 #2509 的其余子任务（S3 #2511、S4 #2512）。

## G0 Golden Acceptance

| 行 ID | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） | assertions（含负例） |
|---|---|---|---|---|
| R1 | 商户 M(`merchants.org_id=M`)；site A(`id=aID, org_id=aOrg`)，`aID≠aOrg`；`rrLegacy(site_id=aID, type=service, status=done_repair)`、`rrNull(site_id='')`；账号商户层级(`tid=oid=M`) | `GET /repair-services/pending-dispatch` | `docs/evidence/2513/R1-pending-dispatch.txt` | `MERCHANT_ROWS=2`、`LEGACY_VISIBLE=1`、`OLD_SITE_EQ_OID_ROWS=0`；负例：修复后仍 0 行或漏掉 `rrLegacy` → FAIL |
| R2 | 同 R1 | `GET /repair-services?scope=site&status=done_repair` | `docs/evidence/2513/R2-list-tasks-site.txt` | `SITE_SCOPE_ROWS=2`、`OLD_FILTER_ROWS=1`（旧谓词只命中 NULL 行）；负例：修复后仍 1 行 → FAIL |
| R3 | 同 R1 + 商户层级账号 | `repairServiceStaffAllowed`（经 quote 入口） | `docs/evidence/2513/R3-staff-allowed.txt` | `ALLOWED_LEGACY=1`、`OLD_DIRECT_OID_COMPARE=0`、`QUOTE_STATUS=200`；负例：`rrLegacy` 仍 403 → FAIL |
| R4 | 同 R1 + 他租户商户(`tid=other`) + 纯网点账号(`tid=M, oid=aOrg`) | 上两接口 + 守卫 | `docs/evidence/2513/R4-negative.txt` | `OTHER_TENANT_ROWS=0`、`OTHER_TENANT_ALLOWED=0`、`SITE_STAFF_ROWS=0`（#2122 保持）；负例：他租户看到 >0 行 → FAIL |
| R5 | 既有维修服务套件（`Repair*`/`ListTasks*`） | `scripts/verify-2513.sh r5` | `docs/evidence/2513/R5-regression.txt` | `REGRESSION_PASS=1`、`FAIL_COUNT=0`、`ok` 行存在；负例：任一既有用例 FAIL → 脚本非 0 |
| R6 | 修后工作树 | `cd backend` 构建 + grep 扫描 | `docs/evidence/2513/R6-build-scan.txt` | `BUILD_RC=0`、`SITE_ID_EQ_ORG=0`、`SITE_FILTER_IN_SERVICE=0`、`MERCHANT_GATE_SHARED=1`；负例：残留 `site_id = orgID` 或 `rr.SiteID != oid` → FAIL |
| R7 | 修后干净树 + 最新 Accepted Plan + evidence.json | `pre-ready-check.sh 2513` | `docs/evidence/2513/pre-ready-2513.json` | `overall=PASS`、每行 `pass=true`、`dirty=false`、P13 在 `origin/issue-2513`；负例：任一 P 失败/evidence 缺行 → 不得 ready |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2513.sh r1 > docs/evidence/2513/R1-pending-dispatch.txt` | `docs/evidence/2513/R1-pending-dispatch.txt` | exit 0；`grep -q '^R1_OK$'` 且 `grep -q 'MERCHANT_ROWS=2'` 且 `grep -q 'MERCHANT_STAFF_ROWS=2'` | `LEGACY_VISIBLE=1`、`OLD_SITE_EQ_OID_ROWS=0` | 修复后仍 0 行/漏 `rrLegacy` → `R1_FAIL` 且非 0 退出 | dev |
| R2 | `bash scripts/verify-2513.sh r2 > docs/evidence/2513/R2-list-tasks-site.txt` | `docs/evidence/2513/R2-list-tasks-site.txt` | exit 0；`grep -q '^R2_OK$'` 且 `grep -q 'SITE_SCOPE_ROWS=2'` | `OLD_FILTER_ROWS=1`（红证据） | 修复后仍 1 行 → `R2_FAIL` 且非 0 退出 | dev |
| R3 | `bash scripts/verify-2513.sh r3 > docs/evidence/2513/R3-staff-allowed.txt` | `docs/evidence/2513/R3-staff-allowed.txt` | exit 0；`grep -q '^R3_OK$'` 且 `grep -q 'ALLOWED_LEGACY=1'` | `OLD_DIRECT_OID_COMPARE=0`、quote 入口 200 | 历史行仍 403 → `R3_FAIL` 且非 0 退出 | dev |
| R4 | `bash scripts/verify-2513.sh r4 > docs/evidence/2513/R4-negative.txt` | `docs/evidence/2513/R4-negative.txt` | exit 0；`grep -q '^R4_OK$'` 且 `grep -q 'OTHER_TENANT_ROWS=0'` | `OTHER_TENANT_ALLOWED=0`、`SITE_STAFF_ROWS=0` | 他租户/网点账号出现 >0 行 → `R4_FAIL` 且非 0 退出 | dev |
| R5 | `bash scripts/verify-2513.sh r5 > docs/evidence/2513/R5-regression.txt` | `docs/evidence/2513/R5-regression.txt` | exit 0；`grep -q '^R5_OK$'` 且 `grep -q 'FAIL_COUNT=0'` | 既有 #2116/#2122/#2134/#2248/#2255 用例全绿 | 任一既有用例 FAIL → `R5_FAIL` 且非 0 退出 | dev |
| R6 | `bash scripts/verify-2513.sh r6 > docs/evidence/2513/R6-build-scan.txt` | `docs/evidence/2513/R6-build-scan.txt` | exit 0；`grep -q '^R6_OK$'` 且 `grep -q 'BUILD_RC=0'` | `SITE_ID_EQ_ORG=0`、`SITE_FILTER_IN_SERVICE=0`、`MERCHANT_GATE_SHARED=1` | 残留 ID 域比较或闸门未复用 → `R6_FAIL` 且非 0 退出 | dev |
| R7 | `~/.config/opencode/scripts/pre-ready-check.sh 2513 --summary-file docs/evidence/2513/summary.md --changed-files docs/evidence/2513/changed-files.txt --evidence-file docs/evidence/2513/evidence.json --affected-cmd "bash scripts/verify-2513.sh r5" --full-gate-cmd "bash scripts/verify-2513.sh gate" --report docs/evidence/2513/pre-ready-2513.json` | `docs/evidence/2513/pre-ready-2513.json` | exit 0；`jq -e '.overall == "PASS"' docs/evidence/2513/pre-ready-2513.json` 且 evidence 每行 `pass=true` | `overall=PASS`、`dirty=false`、P13 提交在 `origin/issue-2513` | 任一 P 项失败 / evidence 缺行 / `dirty=true` → gate 非 0，禁止 ready | dev |

### Files（变更文件清单）

| 类别 | 路径 |
|---|---|
| Handler 修复 | `backend/handlers/repair_service.go` |
| 新增测试 | `backend/handlers/repair_service_2513_test.go` |
| 证据生成器（新增） | `scripts/verify-2513.sh` |
| 用例文档 | `docs/cases/repair-service.md` |
| 证据（计划） | `docs/evidence/2513/plan.md` |
| 证据（摘要） | `docs/evidence/2513/summary.md` |
| 证据（清单） | `docs/evidence/2513/changed-files.txt` |
| 证据（合同汇总） | `docs/evidence/2513/evidence.json` |
| 证据 R1 | `docs/evidence/2513/R1-pending-dispatch.txt` |
| 证据 R2 | `docs/evidence/2513/R2-list-tasks-site.txt` |
| 证据 R3 | `docs/evidence/2513/R3-staff-allowed.txt` |
| 证据 R4 | `docs/evidence/2513/R4-negative.txt` |
| 证据 R5 | `docs/evidence/2513/R5-regression.txt` |
| 证据 R6 | `docs/evidence/2513/R6-build-scan.txt` |
| Gate 报告 | `docs/evidence/2513/pre-ready-2513.json` |

## Artifact Provenance

- 生成器：`scripts/verify-2513.sh`（`r1`–`r6` + `gate` + `ledger`），每个文本 artifact 首行必须是
  `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`（锚定本单最新 `fix(issue-2513)` commit；证据收尾提交不改变锚点）。
- 逐条清单（全部位于 `docs/evidence/2513/`）：
  - `docs/evidence/2513/R1-pending-dispatch.txt`
  - `docs/evidence/2513/R2-list-tasks-site.txt`
  - `docs/evidence/2513/R3-staff-allowed.txt`
  - `docs/evidence/2513/R4-negative.txt`
  - `docs/evidence/2513/R5-regression.txt`
  - `docs/evidence/2513/R6-build-scan.txt`
  - `docs/evidence/2513/pre-ready-2513.json`（gate 报告，JSON 自带 `.provenance.{build,commit,dirty}`）
- `docs/evidence/2513/evidence.json` 字段口径：`build` 形如 `1.0.<7-40 位短码>`，
  `commit` 为 40 位 sha，`dirty:false`；evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$
- 证据重生成纪律：fix commit 之后统一重跑 `bash scripts/verify-2513.sh r1` … `r6`，禁止只改 `build/commit` 冒充新证据。

*Model: DeepSeek V4.1 Flash 1017*


