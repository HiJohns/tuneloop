## Accepted Plan — #2512 S4 游客下单：`orders.org_id` 回退链改为「站点 → `sites.org_id`」反查（禁止写站点 ID）｜Rev.2｜Rev.3

> Parent: #2509（同族 ID 域审计）｜关联：#2508（夹具口径）、#2511（S3 warehouse 同类，另单）

### 修订记录（Rev.1 → Rev.2，仅验收侧，产品代码不变）

来源：Round 1 Audit（Bug 2 阻断 + Observation 1/2）+ Investigation Packet（选项 1）。

| # | 修订项 | Rev.1 | Rev.2 |
|---|---|---|---|
| 1 | R7 断言 | `jq -e '.result == "PASS"'`（`pre-ready` 报告无 `result` 键 → 断言不可执行） | `jq -e '.overall == "PASS"'`（报告根键）；三处（Matrix / G0 / Evidence Contract）同文 |
| 2 | R5 合同 | 只登记 `SQL_PRESENT=1` | 补登 `RESOLVER_CALLS=2`、`SQL_SET_SEMANTICS=1`（AC4 集合语义：`orders.org_id::text NOT IN (sites.org_id ∪ merchants.org_id)`），缺失即 `R5_FAIL` |
| 3 | R1 红侧 | 仅 `RED_CONFIRMED=1`（常量推导） | 追加 `PRE_FIX_SITE_ID_AS_ORG_LINES>=1`：产物含 `--- pre-fix code ---` 段并命中旧实现行；`RED_CONFIRMED` 键名不变 |
| 4 | Provenance | 仅首行 `# build= # commit= # dirty=` | 首行格式不变，另起一行 `GENERATED_AT_HEAD=<40 位 sha>`（产物生成树），区分锚点与生成树 |

不在本次修订范围：产品代码（`user_rental.go` 已按 Rev.1 D1/D2/D4 实现且审计通过）、R5 只读 SQL 集合语义实现（已在 `origin/issue-2512` 落地：`d89212234` + `b27a959ee`）。

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 游客下单（`tid`/`oid` 为空）时 `effectiveOrgID` 回退链把 `instruments.site_id` / `current_site_id`（**`sites.id` 域**）直接写入 `orders.org_id`（**`sites.org_id` 域**） | `backend/handlers/user_rental.go:430-440`（CreateOrder）、`:1037-1047`（CreateBatchOrders） |
| `orders.org_id` 被该值直接使用（订单落库 + 结算 breakdown + 支付期限） | `user_rental.go:639`、`:752`、`:775`、`:921`、`:1227`、`:1247`、`:1267`、`:1343` |
| `instruments.site_id` / `current_site_id` 为 `*uuid.UUID`（`sites.id`）；`org_id` 为 `*string`（组织域） | `backend/models/models.go:87,98,99` |
| 数据归属权威口径：业务数据 `tenant_id` 写**商户 org**，写入归属从对象推导 | `docs/spec/api/api.md:91`；`docs/cases/cases.md:289`（ID 域定义） |
| 下游按 `orders.org_id` 过滤/反查（污染即漏单/解析失败） | `backend/handlers/warehouse.go:52-56`、`merchant_order.go`、`repair_service.go`（商户 org 锚） |
| 现有测试未覆盖回退链：`user_rental_1762_test.go` 等夹具把 `ContextKeyOrgID = tenantID` 注入 → 走的是「非游客」分支 | `backend/handlers/user_rental_1762_test.go:44-52` |
| `instruments.org_id` 为空的存量乐器确实存在（导入/历史数据） | 父任务 #2509 S4 描述；本单以夹具证明语义 |

**根因**：游客分支在 `instruments.org_id` 缺失时用**物理位置/归属站点 ID** 顶替**组织 ID**，把域错误持久化进 `orders.org_id`；下游所有按 org 的过滤与商户组织反查都会因此失准或失败。

### 变更点

**A. 代码（`backend/handlers/user_rental.go`）**
1. 新增唯一解析函数（含注释指向 #2509/#2512 与 ID 域权威）：

```go
// resolveOrderOrgID 解析订单归属组织（sites.org_id 域，不得写入 sites.id）：
// instrument.OrgID → 按 site_id / current_site_id 反查 sites.org_id → 解析不到返回 ""。
func resolveOrderOrgID(db *gorm.DB, inst *models.Instrument) string {
	if inst.OrgID != nil && *inst.OrgID != "" {
		return *inst.OrgID
	}
	for _, sid := range []*uuid.UUID{inst.SiteID, inst.CurrentSiteID} {
		if sid == nil {
			continue
		}
		var site models.Site
		if err := db.Select("org_id").Where("id = ?", *sid).First(&site).Error; err == nil && site.OrgID != "" {
			return site.OrgID
		}
	}
	return ""
}
```
2. `CreateOrder`（`:430-440`）：游客分支改为

```go
if effectiveTenantID == "" {
	effectiveTenantID = instrument.TenantID
	org := resolveOrderOrgID(db, &instrument)
	if org == "" {
		log.Printf("[CreateOrder] WARN: instrument %s has no resolvable org (org_id/site_id/current_site_id empty or site missing org)", instrument.ID)
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "乐器归属信息不完整，暂时无法下单，请联系门店"})
		return
	}
	effectiveOrgID = org
}
```
3. `CreateBatchOrders`（`:1037-1047`）：同一函数替换回退链；任一乐器解析失败 → 同样 `400/40002`（**在落库前**返回，消息含 instrument id），不得以站点 ID 兜底。
4. 不改动非游客分支（`effectiveOrgID = oid`）与其它 6 处 `effectiveOrgID` 消费点。

**B. 测试（新增 `backend/handlers/user_rental_2512_test.go`，自带夹具）**
5. 夹具必须 `sites.id ≠ sites.org_id`（#2508/#2510 口径）；游客上下文 = `ContextKeyUserID` 有值、`TenantID`/`OrgID` 为空；用户经 `testutil.SeedVerifiedCustomer` 满足实名前置。
6. 用例：
   - `TestCreateOrder2512_GuestOrgFromSite`：`instruments.org_id = NULL`、`site_id = aID`（`aOrg`）→ 落库 `orders.org_id == aOrg`；红证据：旧逻辑值 `= aID`（`OLD_FALLBACK_IS_SITE_ID=1`、`RED_CONFIRMED=1`）；
   - `TestCreateOrder2512_GuestOrgFromCurrentSite`：`org_id NULL`、`site_id NULL`、`current_site_id = bID` → `orders.org_id == bOrg`；
   - `TestCreateOrder2512_UnresolvableRejected`：三者全空 → `HTTP 400 / code 40002`，且 `orders` 无新行（`ORDERS_AFTER=0`）；
   - `TestBatchCreateOrder2512_GuestOrgFromSiteAndReject`：批量为 2 件（首件 `org_id NULL + site_id=aID`）→ `orders.org_id == aOrg` 且每单同值；另一批含不可解析乐器 → `400/40002` 且**零落库**。

**C. 文档**
7. `docs/spec/api/api.md` §6.2 / §6.3 游客说明补：`org_id` 派生顺序 = `instruments.org_id` → `sites.org_id`（按 `site_id`/`current_site_id` 反查）；均不可解析 → `400/40002`（不得写入 `sites.id`）。

**D. 证据**
8. 新增 `scripts/verify-2512.sh`（`r1`–`r6` + `gate`，首行写 provenance）。
9. `r1` 产物除既有键外，追加 `--- pre-fix code ---` 段与 `PRE_FIX_SITE_ID_AS_ORG_LINES=<n>`（从 `origin/develop` 的旧实现中 grep `effectiveOrgID = instrument.SiteID.String()`，`n>=1` 即通过），使红侧不再只靠 `RED_CONFIRMED` 常量推导。
10. `r5` 输出**只读**存量排查 SQL（不执行写操作），必须同时包含两类查询：① `orders.org_id` 命中某个 `sites.id` 的细分项；② AC4 集合语义 `orders.org_id::text NOT IN (sites.org_id ∪ merchants.org_id)`；脚本输出可判定键 `SQL_PRESENT=1` 与 `SQL_SET_SEMANTICS=1`（缺②即 `R5_FAIL`）。
11. `r7`/gate 判定谓词统一为 `pre-ready` 报告的 `.overall == "PASS"`（报告不写 `.result` 键）。

- Tags: bug, code:go, scope:backend

### Workload

- Size: M
- Basis: 1 个 handler 文件（1 新函数 + 2 处替换）+ 1 个新测试文件 + 1 个文档 + 1 个证据脚本 + 7 条证据行；无 migration、无前端改动
- Confidence: high

## Core Update

| 文档路径 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` | §6.2/§6.3 只写「`tenant_id` 和 `org_id` 从乐器记录中派生」 | 补明派生顺序（`instruments.org_id` → `sites.org_id` 反查）、不可解析时的 `400/40002` 拒绝语义，并声明**禁止**写入 `sites.id` | 契约未写清派生链 → 实现以站点 ID 兜底（本 bug）；契约必须可判定 |

## Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 触发条件 | 游客（`tid`/`oid` 空）对 `instruments.org_id` 为空的存量乐器下单 → `orders.org_id` 落 `sites.id` | `user_rental.go:433-439` |
| 2 | 根因 | 回退链以「站点 ID」顶替「组织 ID」，把 domain 错误持久化；无失败出口 | `models.go:87,98,99`、`user_rental.go:430-440` |
| 3 | 为何长期未发现 | 现有用例都把 `org_id` 注入 context（走非游客分支），游客分支零覆盖；`instruments.org_id` 非空的正常数据掩盖缺陷 | `user_rental_1762_test.go:44-52` |
| 4 | 同类扫描 | 同族：S3 `warehouse.go:697,872`（#2511，另单）、S5 `repair_service.go`（#2513，已 accepted）、S1/S2 已修 | #2509 |
| 5 | 防复发（代码） | 唯一解析函数 `resolveOrderOrgID`（两处复用），解析失败**拒绝下单**而非兜底 | 变更点 1–3 |
| 6 | 防复发（测试） | 4 个用例（org 缺失 + site / current_site / 全空拒绝 / 批量），夹具 `sites.id ≠ sites.org_id` | 变更点 5–6 |
| 7 | 防复发（文档） | §6.2/§6.3 写清派生顺序与拒绝码 | Core Update |
| 8 | 假阴性反例 | 若落库等于 `sites.id`、或全空仍成功下单、或批量部分落库 → R1/R3/R4 FAIL | R1/R3/R4 负例 |
| 9 | 回归面 | 正常乐器（`org_id` 有值）路径不变；非游客分支不变；批量 6.3 行为一致 | R2/R4/R6 |
| 10 | 未修边界 | 存量 `orders.org_id` 污染的清理（需 Owner 授权）→ 本单只出**只读**排查 SQL | R5、Out of Scope |

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 游客下单 org 取 `sites.org_id`（红绿） | `backend/handlers/user_rental.go`、`backend/handlers/user_rental_2512_test.go` | `POST /api/user/orders` | 游客（tid/oid 空）+ `org_id NULL` + `site_id=aID` | backend（Postgres 测试库） | `ORDER_ORG_MATCHES_SITE_ORG=1`（`order.OrgID == f.orgA` 派生，非占位符）、`SITE_ID_IS= aID`、`OLD_FALLBACK_IS_SITE_ID=1`、`RED_CONFIRMED=1`、`PRE_FIX_SITE_ID_AS_ORG_LINES>=1`（产物含 `--- pre-fix code ---` 段） | R1 |
| R2 `current_site_id` 回退同样按 org 反查 | 同上 | 同上 | `org_id NULL`、`site_id NULL`、`current_site_id=bID` | backend | `ORDER_ORG_MATCHES_CURRENT_SITE_ORG=1`（`order.OrgID == f.orgB`）、`RESOLVED_FROM_CURRENT_SITE=1` | R2 |
| R3 不可解析 → 拒绝且不落库（负例） | 同上 | 同上 | 三者全空 | backend | `STATUS=400`、`CODE=40002`、`ORDERS_AFTER=0` | R3 |
| R4 批量下单同口径（含拒绝不落库） | 同上 | `POST /api/user/orders/batch` | 首件 `org_id NULL + site_id=aID`；另批含不可解析件 | backend | `BATCH_ORG_MATCHES_SITE_ORG=1`、`BATCH_ORGS_UNIQUE=1`、`BATCH_SITE_ID_AS_ORG=0`（R4 谓词拆分：站点 ID 域独立判定）、`REJECT_STATUS=400`、`BATCH_ORDERS_AFTER=0` | R4 |
| R5 同类扫描 + 只读存量排查 SQL | `scripts/verify-2512.sh` | grep + SQL 文本 | 修后工作树 | backend | `SITE_ID_AS_ORG=0`（无 `SiteID.String()` 赋给 org）、`CURRENT_SITE_AS_ORG=0`、`RESOLVER_CALLS=2`、`SQL_PRESENT=1`、`SQL_SET_SEMANTICS=1` | R5 |
| R6 构建 + 定向回归 | `scripts/verify-2512.sh` | `cd backend` 构建 + 定向套件 | 修后工作树 | backend | `BUILD_RC=0`、`REG_RC=0`（`CreateOrder`/`BatchCreateOrder` 既有用例） | R6 |
| R7 pre-ready gate（四源 + 定向测试 + 构建） | `docs/evidence/2512/pre-ready-2512.json` | `pre-ready-check.sh 2512` | 修后干净树 + 最新 Accepted Plan | backend | `GATE_RESULT=PASS`（报告 `.overall == "PASS"`）、P12 完整、P13 在 `origin/issue-2512`；禁止引用不存在的 `.result` 键 | R7 |

## Input Truth & Tool Preflight

- **Source of truth（口径唯一）**：`docs/cases/cases.md:289`（`sites.org_id` = 组织域；`sites.id` = 站点主键）；`docs/spec/api/api.md:91`（业务数据 `tenant_id` = 商户 org，归属从对象推导）；`backend/models/models.go:87,98,99`（字段类型）。
- **差异核对（只读，已于 /analyze 执行）**：
  - `grep -n "effectiveOrgID" backend/handlers/user_rental.go` → 游客回退仅 `:433-439`、`:1040-1046` 两处，其余均为消费点；
  - `grep -rn "SiteID.String()" backend/handlers` → 仅这两处把站点 ID 当组织用（其余为日志/ID 输出）；
  - `grep -n "ContextKeyOrgID" backend/handlers/user_rental_1762_test.go` → 既有用例注入 org → 游客分支未被覆盖（缺陷不可见）。
- **损坏输入拒绝**：`instrument.OrgID == nil/""` 且 `site_id`/`current_site_id` 均无法反查到 `sites.org_id` → `400/40002` + `log.Printf` WARN（含 instrument id），**不得**以空串或站点 ID 继续下单；批量路径要求**全部**可解析后才落库。
- **工具盲区声明**：
  - `go test ./handlers` 需 Postgres 测试库（`docs/ops/test-db-lifecycle.md`）；本单只跑定向用例（`-run 'CreateOrder|BatchCreateOrder'`），全量由 pre-ready P5 baseline 差分承担；
  - R5 的存量排查 SQL **只出文本不执行**（DB 写入需 Owner 授权）；生产污染规模本单不核实（Out of Scope）；
  - grep 无法识别动态拼接赋值；以「两处回退串为 0 + `resolveOrderOrgID` 被两处调用」作为最小防漂移断言。
- **证据时序**：fix commit 之后重跑 `bash scripts/verify-2512.sh r1` … `r6`；`evidence.json` 的 `build/commit/dirty` 由脚本从 `git rev-parse` 派生，禁止手改。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 解析顺序：`instruments.org_id` → `site_id`/`current_site_id` 反查 `sites.org_id` → **解析失败即拒绝下单**（`400/40002`） | Architect | 本计划锁定（Issue 期望 1/2 明确要求） | 若 Owner 要求「失败时降级为商户 org」→ 需回查商户锚（另立） |
| D2 | 不改非游客分支（`effectiveOrgID = oid`）与 6 处消费点；本单只修回退链 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 存量 `orders.org_id` 污染**不在本单修数据**：只交付只读排查 SQL；修复需 Owner 授权（AGENTS.md DB 红线） | Architect | 本计划锁定 | 审计若要求治理 → 转 Ops/独立 Issue |
| D4 | 批量路径要求「全部乐器可解析」才落库（全有或全无），不部分成功 | Architect | 本计划锁定 | 若 Owner 要求部分成功 → 回 Architect |
| D5 | 生产污染规模抽样与 `instruments.org_id IS NULL` 分布 → 不在本单 | Architect | 本计划锁定（dev 无样本） | 审计若要求实测 → 转 Ops Follow-up |

## Out of Scope

- 存量数据修复（`orders.org_id` 域污染、`instruments.org_id` 回填）→ 需 Owner 授权，另立 Issue（本单只给只读 SQL）。
- S3 `warehouse.go:697,872`（`current_site_id` 写入 org 值）→ **#2511**。
- S5 `repair_service.go` → **#2513**（已 accepted）。
- 非游客分支的归属口径（员工代下单时 `orders.org_id = 操作者 oid`）→ 现状保留。
- 前端任何改动（H5/小程序/PC）；`docs/spec/api/api.md` 之外的文档。
- 批量下单的响应/事务语义重构（仅按 D4 保证「全有或全无」）。

## G0 Golden Acceptance

| 行 ID | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） | assertions（含负例） |
|---|---|---|---|---|
| R1 | tenant T；site A(`id=aID, org_id=aOrg`)，`aID≠aOrg`；乐器(`tenant=T, org_id=NULL, site_id=aID, stock_status=available`)；游客上下文（`tid/oid` 空，`SeedVerifiedCustomer`） | `POST /api/user/orders` | `docs/evidence/2512/R1-guest-org-from-site.txt` | `ORDER_ORG_MATCHES_SITE_ORG=1`、`SITE_ID_IS=aID`、`OLD_FALLBACK_IS_SITE_ID=1`、`RED_CONFIRMED=1`、`PRE_FIX_SITE_ID_AS_ORG_LINES>=1`（`--- pre-fix code ---` 段命中旧实现 `effectiveOrgID = instrument.SiteID.String()`）；负例：`ORDER_ORG_MATCHES_SITE_ORG=0`（即落库为 `aID`）→ FAIL |
| R2 | 同上；乐器(`org_id=NULL, site_id=NULL, current_site_id=bID`)，site B(`id=bID, org_id=bOrg`) | 同上 | `docs/evidence/2512/R2-current-site.txt` | `ORDER_ORG_MATCHES_CURRENT_SITE_ORG=1`、`RESOLVED_FROM_CURRENT_SITE=1`；负例：`ORDER_ORG_MATCHES_CURRENT_SITE_ORG=0`（落库为 `bID`）→ FAIL |
| R3 | 同上；乐器(`org_id=NULL, site_id=NULL, current_site_id=NULL`) | 同上 | `docs/evidence/2512/R3-reject.txt` | `STATUS=400`、`CODE=40002`、`ORDERS_AFTER=0`；负例：201/20000 且落库 → FAIL |
| R4 | 同上；批量两件（首件 `org_id=NULL, site_id=aID`）；另一批含不可解析件 | `POST /api/user/orders/batch` | `docs/evidence/2512/R4-batch.txt` | `BATCH_ORG_MATCHES_SITE_ORG=1`、`BATCH_ORGS_UNIQUE=1`、`BATCH_SITE_ID_AS_ORG=0`、`REJECT_STATUS=400`、`BATCH_ORDERS_AFTER=0`；负例：`BATCH_SITE_ID_AS_ORG=1` 或部分落库 → FAIL |
| R5 | 修后 `user_rental.go` + 只读 SQL | grep + 文本 | `docs/evidence/2512/R5-scan-sql.txt` | `SITE_ID_AS_ORG=0`、`CURRENT_SITE_AS_ORG=0`、`RESOLVER_CALLS=2`、`SQL_PRESENT=1`、`SQL_SET_SEMANTICS=1`（只读 SQL 含 `orders.org_id::text NOT IN (sites.org_id ∪ merchants.org_id)`）；负例：残留 `effectiveOrgID = instrument.SiteID.String()` 或缺集合语义查询 → FAIL |
| R6 | 修后工作树 | `cd backend` | `docs/evidence/2512/R6-build-regression.txt` | `BUILD_RC=0`、`REG_RC=0`（`CreateOrder`/`BatchCreateOrder` 既有用例通过）；负例：任一失败 → FAIL |
| R7 | 修后干净树 + 最新 Accepted Plan + evidence.json | `pre-ready-check.sh 2512` | `docs/evidence/2512/pre-ready-2512.json` | `jq -e '.overall == "PASS"'`（报告根键）、每行 `pass=true`、`dirty=false`、P13 在 `origin/issue-2512`；负例：`.overall != "PASS"` 或任一 P 失败 / evidence 缺行 → 不得 ready |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2512.sh r1 > docs/evidence/2512/R1-guest-org-from-site.txt` | `docs/evidence/2512/R1-guest-org-from-site.txt` | exit 0；`grep -q '^R1_OK$'` 且 `grep -q 'ORDER_ORG_MATCHES_SITE_ORG=1'` | `OLD_FALLBACK_IS_SITE_ID=1`、`RED_CONFIRMED=1`、`PRE_FIX_SITE_ID_AS_ORG_LINES>=1` | 落库为 `aID`（`ORDER_ORG_MATCHES_SITE_ORG=0`），或缺 `--- pre-fix code ---` 段 → `R1_FAIL` 且非 0 退出 | dev |
| R2 | `bash scripts/verify-2512.sh r2 > docs/evidence/2512/R2-current-site.txt` | `docs/evidence/2512/R2-current-site.txt` | exit 0；`grep -q '^R2_OK$'` 且 `grep -q 'ORDER_ORG_MATCHES_CURRENT_SITE_ORG=1'` | `RESOLVED_FROM_CURRENT_SITE=1` | 落库为 `bID`（`ORDER_ORG_MATCHES_CURRENT_SITE_ORG=0`）→ `R2_FAIL` 且非 0 退出 | dev |
| R3 | `bash scripts/verify-2512.sh r3 > docs/evidence/2512/R3-reject.txt` | `docs/evidence/2512/R3-reject.txt` | exit 0；`grep -q '^R3_OK$'` 且 `grep -q 'CODE=40002'` | `STATUS=400`、`ORDERS_AFTER=0` | 成功下单或落库 → `R3_FAIL` 且非 0 退出 | dev |
| R4 | `bash scripts/verify-2512.sh r4 > docs/evidence/2512/R4-batch.txt` | `docs/evidence/2512/R4-batch.txt` | exit 0；`grep -q '^R4_OK$'` 且 `grep -q 'BATCH_ORG_MATCHES_SITE_ORG=1'` | `BATCH_ORGS_UNIQUE=1`、`BATCH_SITE_ID_AS_ORG=0`、`REJECT_STATUS=400`、`BATCH_ORDERS_AFTER=0` | `BATCH_SITE_ID_AS_ORG=1` 或部分落库 → `R4_FAIL` 且非 0 退出 | dev |
| R5 | `bash scripts/verify-2512.sh r5 > docs/evidence/2512/R5-scan-sql.txt` | `docs/evidence/2512/R5-scan-sql.txt` | exit 0；`grep -q '^R5_OK$'` 且 `grep -q 'SITE_ID_AS_ORG=0'` | `CURRENT_SITE_AS_ORG=0`、`RESOLVER_CALLS=2`、`SQL_PRESENT=1`、`SQL_SET_SEMANTICS=1` | 残留站点 ID 兜底，或缺集合语义查询（`SQL_SET_SEMANTICS=0`） → `R5_FAIL` 且非 0 退出 | dev |
| R6 | `bash scripts/verify-2512.sh r6 > docs/evidence/2512/R6-build-regression.txt` | `docs/evidence/2512/R6-build-regression.txt` | exit 0；`grep -q '^R6_OK$'` 且 `grep -q 'BUILD_RC=0'` | `REG_RC=0` | 构建或定向回归失败 → `R6_FAIL` 且非 0 退出 | dev |
| R7 | `~/.config/opencode/scripts/pre-ready-check.sh 2512 --summary-file docs/evidence/2512/summary.md --changed-files docs/evidence/2512/changed-files.txt --evidence-file docs/evidence/2512/evidence.json --affected-cmd "bash scripts/verify-2512.sh r6" --full-gate-cmd "bash scripts/verify-2512.sh gate" --report docs/evidence/2512/pre-ready-2512.json` | `docs/evidence/2512/pre-ready-2512.json` | exit 0；`GATE_RESULT=PASS`，`jq -e '.overall == "PASS"'`，evidence 每行 `pass=true` | `dirty=false`、P13 提交在 `origin/issue-2512` | 任一 P 项失败 / evidence 缺行 / `dirty=true` → gate 非 0，禁止 ready | dev |

### Files（变更文件清单）

| 类别 | 路径 |
|---|---|
| Handler 修复 | `backend/handlers/user_rental.go` |
| 新增测试 | `backend/handlers/user_rental_2512_test.go` |
| 证据生成器（新增） | `scripts/verify-2512.sh` |
| 核心文档 | `docs/spec/api/api.md` |
| 证据（计划） | `docs/evidence/2512/plan.md` |
| 证据（摘要） | `docs/evidence/2512/summary.md` |
| 证据（清单） | `docs/evidence/2512/changed-files.txt` |
| 证据（合同汇总） | `docs/evidence/2512/evidence.json` |
| 证据 R1 | `docs/evidence/2512/R1-guest-org-from-site.txt` |
| 证据 R2 | `docs/evidence/2512/R2-current-site.txt` |
| 证据 R3 | `docs/evidence/2512/R3-reject.txt` |
| 证据 R4 | `docs/evidence/2512/R4-batch.txt` |
| 证据 R5 | `docs/evidence/2512/R5-scan-sql.txt` |
| 证据 R6 | `docs/evidence/2512/R6-build-regression.txt` |
| Gate 报告 | `docs/evidence/2512/pre-ready-2512.json` |

## Artifact Provenance

- 生成器：`scripts/verify-2512.sh`（`r1`–`r6` + `gate`），每个文本 artifact 首行必须是
  `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`（锚定本单 fix commit；证据收尾提交不改变锚点）。
- 生成树可追溯（Rev.2）：每个文本 artifact 在首行之后另起一行写 `GENERATED_AT_HEAD=<40 位 sha>`
  （生成时 `git rev-parse HEAD`），用于区分「锚点 commit」与「产物生成树」；**首行格式不得改动**
  （P14 依赖首行）。
- 逐条清单（全部位于 `docs/evidence/2512/`）：
  - `docs/evidence/2512/R1-guest-org-from-site.txt`
  - `docs/evidence/2512/R2-current-site.txt`
  - `docs/evidence/2512/R3-reject.txt`
  - `docs/evidence/2512/R4-batch.txt`
  - `docs/evidence/2512/R5-scan-sql.txt`
  - `docs/evidence/2512/R6-build-regression.txt`
  - `docs/evidence/2512/pre-ready-2512.json`（gate 报告，JSON 自带 `.provenance.{build,commit,dirty}`）
- `docs/evidence/2512/evidence.json` 字段口径：`build` 形如 `1.0.<7-40 位短码>`，
  `commit` 为 40 位 sha，`dirty:false`；evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$
- 证据重生成纪律：fix commit 之后统一重跑 `bash scripts/verify-2512.sh r1` … `r6`，禁止只改 `build/commit` 冒充新证据。

*Model: DeepSeek V4.1 Flash 1017*



