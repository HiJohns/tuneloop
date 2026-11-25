## Accepted Plan — #2511 S3 归还验收：`instruments.current_site_id` 回退改为「`order.OrgID` → `sites.id` 反查」（禁止写组织 ID）

> Parent: #2509（同族 ID 域审计）｜关联：#1889（`current_site_id` 引入点）、#2510（S2）、#2512（S4，已 accepted）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| `InspectReturn`（定损入库）优先写操作者站点；**无法解析时把 `order.OrgID`（组织域）写入 `instruments.current_site_id`（`sites.id` 域）** | `backend/handlers/warehouse.go:694-700` |
| `AssessDamage` 同样的回退 | `backend/handlers/warehouse.go:869-875` |
| `current_site_id` 是 `sites.id`（`*uuid.UUID`）；`orders.org_id` 是组织域（`string`） | `backend/models/models.go:99`；`backend/models/models.go:87`（`instruments.org_id` 是 `*string`） |
| 下游以 `current_site_id` 关联 `site_members` 做验收门 | `backend/handlers/repair.go:143,204,269,325` |
| 下游以 `current_site_id` 反查 `sites` 展示网点 | `backend/handlers/public.go:278-284` |
| `resolveOperatorSiteID` 取值来自 `site_members.site_id`（正确域）；无成员 → `nil` | `backend/handlers/repair.go:620-630` |
| ID 域权威：`sites.org_id` = 组织 ID，`sites.id` = 站点主键 | `docs/cases/cases.md:289` |
| 现有测试只覆盖**主分支**（操作者有 site_members → 写操作者站点），回退分支零覆盖 | `backend/handlers/repair_site_1889_test.go:131-167`（`TestInspectReturn_Damaged_SetsCurrentSiteID`、`TestAssessDamage_SetsCurrentSiteID`） |

**根因**：`#1889` 的回退把「订单所属组织」直接当作「站点」写库——域不同（`sites.org_id` ≠ `sites.id`），一旦落库即污染：验收门查不到站点（乐器卡「无法验收」）、网点展示为空、与 S2（`instrument_loss`）叠加后行为不可预测。

### 变更点

**A. 代码（`backend/handlers/warehouse.go` + 一个解析 helper）**
1. 新增唯一解析函数（放 `warehouse.go` 顶部工具区，注释指向 #2511/#1889/#2509）：

```go
// resolveSiteIDByOrg 把组织 ID 反查为 sites.id（#2511）：禁止把 org 值写入 current_site_id。
// 策略（确定性）：恰好 1 个站点 → 用该站点；多个 → 优先锚点站点（sites.org_id == sites.tenant_id），
// 否则按 created_at 升序取最早；0 个或查询失败 → 返回 ""（调用方不写入，保留原值 + WARN）。
func resolveSiteIDByOrg(db *gorm.DB, orgID string) string
```
2. `InspectReturn`（`:694-700`）改为：

```go
if siteID := resolveOperatorSiteID(db, userID); siteID != nil {
	updates["current_site_id"] = *siteID
} else if resolved := resolveSiteIDByOrg(db, order.OrgID); resolved != "" {
	updates["current_site_id"] = resolved
} else {
	log.Printf("[WARN] #1889/#2511: no site resolved for instrument %s entering repair (operator has no site membership; order org %q maps to no site)", order.InstrumentID, order.OrgID)
}
```
3. `AssessDamage`（`:869-875`）同构替换。
4. 不改主分支语义（有 site_members → 操作者站点），不改其它 `current_site_id` 消费方。

**B. 测试（新增 `backend/handlers/warehouse_2511_test.go`，复用 `setup1889Fixture`/`newFixture1889` 的建单方式，但自带独立夹具）**
5. 夹具强制 `sites.id ≠ sites.org_id`（`newFixture1889` 已满足：`siteID` 独立生成、`sites.org_id = orgID`）。
6. 用例：
   - `TestInspectReturn2511_FallbackWritesSiteNotOrg`：操作者**无** `site_members` → 定损入库 → `instrument.current_site_id == sites.id`（`aID`）；红证据：旧逻辑值 = `order.OrgID`（`OLD_FALLBACK_IS_ORG=1`、`RED_CONFIRMED=1`）；
   - `TestAssessDamage2511_FallbackWritesSiteNotOrg`：同经 `/warehouse/orders/:id/damage`；
   - `TestInspectReturn2511_UnresolvableKeepsValue`：`order.OrgID` 映射不到站点，乐器预置 `current_site_id = aID` → 入库后仍为 `aID`（`NO_CROSS_DOMAIN_WRITE=1`、`KEPT_ORIGINAL=1`）；
   - `TestResolveSiteIDByOrg2511_Deterministic`：同 org 两个站点（其一为锚点 `org_id == tenant_id`）→ 命中锚点；无锚点时按 `created_at` 取最早（`ANCHOR_PREFERRED=1`、`EARLIEST_FALLBACK=1`、`EMPTY_FOR_UNKNOWN=1`）；
   - `TestInspectReturn2511_PrimaryBranchUnchanged`：操作者有成员 → 写操作者站点（负例：不得等于 `order.OrgID`）。

**C. 文档（口径留痕）**
7. `docs/cases/cases.md` ID 域小节（`:289` 附近）补一行：`instruments.current_site_id` 只能由「操作者 `site_members.site_id`」或「`order.OrgID` → `sites.id` 反查」得到，**任何路径不得直接写入组织 ID**（#2511）。

**D. 证据**
8. 新增 `scripts/verify-2511.sh`（`r1`–`r6` + `gate`，首行写 provenance）；`r5` 输出**只读**存量排查 SQL（不执行写操作）。

- Tags: bug, code:go, scope:backend

### Workload

- Size: M
- Basis: 1 个 handler 文件（1 新函数 + 2 处替换）+ 1 个新测试文件 + 1 个文档 + 1 个证据脚本 + 7 条证据行；无 migration、无前端改动
- Confidence: high

## Core Update

| 文档路径 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/cases/cases.md` | ID 域小节只定义「`sites.org_id` = 组织 ID / `sites.id` = 站点主键」，未写 `current_site_id` 的合法写入来源 | 补一行：`instruments.current_site_id` 仅可由操作者 `site_members.site_id` 或 `order.OrgID` → `sites.id` 反查写入，禁止直接写组织 ID（#2511） | 代码曾以「order 的组织」顶替站点；把合法来源写进口径文档可防复发 |

## Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 触发条件 | 归还验收人**无** `site_members`（商户直属员工/师傅/缺成员数据）→ 定损入库 → `current_site_id` 被写成 `order.OrgID` | `warehouse.go:694-700`、`:869-875` |
| 2 | 根因 | 回退把组织域值写入站点域列；无「解析失败」出口（只在不写时 WARN 一次，另一分支写脏值） | `models.go:99`、`repair.go:620-630` |
| 3 | 为何长期未发现 | 现有用例只覆盖「操作者有站点」主分支；夹具中 `sites.org_id` 与 `order.OrgID` 同值，即便走到回退也「看起来对」 | `repair_site_1889_test.go:131-167` |
| 4 | 同类扫描 | S2 `instrument_loss.go:47`（已 accepted #2515）、S4 `user_rental.go:433-439`（已 accepted #2512）、S5 `repair_service.go`（已 accepted #2513） | #2509 |
| 5 | 防复发（代码） | 唯一解析函数 `resolveSiteIDByOrg`（两处复用）；解析失败**不写入**并 WARN | 变更点 1–3 |
| 6 | 防复发（测试） | 5 个用例：两处回退、不可解析保留原值、多站点确定性、主分支不变 | 变更点 5–6 |
| 7 | 防复发（文档） | `cases.md` 写清 `current_site_id` 合法来源 | Core Update |
| 8 | 假阴性反例 | 若任一分支仍写 `order.OrgID`、或不可解析时覆盖原值为空/org → R1/R3 FAIL | R1/R3 负例 |
| 9 | 回归面 | 主分支（操作者站点）行为不变；`SubmitAssessment` 路径（`assessment` handler 自有逻辑）不在本单改动 | R4/R6 |
| 10 | 未修边界 | 存量 `current_site_id` 域污染清理（需 Owner 授权）→ 本单只出**只读**排查 SQL；`assessment` handler 若存在同类写法 → 另见 Out of Scope | R5、Out of Scope |

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志/截图） |
|---|---|---|---|---|---|---|
| R1 定损入库回退写站点 ID（红绿） | `backend/handlers/warehouse.go`、`backend/handlers/warehouse_2511_test.go` | `PUT /api/warehouse/orders/:id/return-inspect` | 操作者无 `site_members`；`order.OrgID = aOrg`；`sites(aID, aOrg)` | backend（Postgres 测试库） | `CURRENT_SITE_ID=aID`、`SITE_ID_IS=aID`、`OID_IS=aOrg`、`OLD_FALLBACK_IS_ORG=1`、`RED_CONFIRMED=1` | R1 |
| R2 定损评估回退写站点 ID | 同上 | `PUT /api/warehouse/orders/:id/damage` | 同上 | backend | `CURRENT_SITE_ID=aID`、`NOT_ORG=1` | R2 |
| R3 不可解析 → 保留原值（负例） | 同上 | `return-inspect` | `order.OrgID` 无对应站点；乐器预置 `current_site_id=aID` | backend | `KEPT_ORIGINAL=1`、`NO_CROSS_DOMAIN_WRITE=1`、`CURRENT_SITE_ID=aID` | R3 |
| R4 多站点确定性 + 主分支不变（负例） | 同上 | 解析函数 + `return-inspect` | 同 org 两站点（含锚点）/ 无锚点；操作者有成员 | backend | `ANCHOR_PREFERRED=1`、`EARLIEST_FALLBACK=1`、`EMPTY_FOR_UNKNOWN=1`、`PRIMARY_EQUALS_OPERATOR_SITE=1`、`PRIMARY_NOT_ORG=1` | R4 |
| R5 同类扫描 + 只读存量 SQL | `scripts/verify-2511.sh` | grep + SQL 文本 | 修后工作树 | backend | `ORG_WRITTEN_TO_SITE=0`、`RESOLVER_CALLS=2`、`SQL_PRESENT=1` | R5 |
| R6 构建 + 定向回归 | `scripts/verify-2511.sh` | `cd backend` 构建 + 定向套件 | 修后工作树 | backend | `BUILD_RC=0`、`REG_RC=0`（`1889`/`InspectReturn`/`AssessDamage`/`ResolveOperatorSiteID` 既有用例） | R6 |
| R7 pre-ready gate（四源 + 定向测试 + 构建） | `docs/evidence/2511/pre-ready-2511.json` | `pre-ready-check.sh 2511` | 修后干净树 + 最新 Accepted Plan | backend | `GATE_RESULT=PASS`、P12 完整、P13 在 `origin/issue-2511` | R7 |

## Input Truth & Tool Preflight

- **Source of truth（口径唯一）**：`docs/cases/cases.md:289`（ID 域定义）；`backend/models/models.go:99`（`current_site_id` 为 `*uuid.UUID`）；`backend/handlers/repair.go:620-630`（`resolveOperatorSiteID` 取值为 `site_members.site_id`）；#1889 意图（记录乐器**物理位置**供验收门核对）。
- **差异核对（只读，已于 /analyze 执行）**：
  - `grep -rn "current_site_id" backend/handlers/warehouse.go` → 写点仅 `:695,:697,:870,:872`（两处主分支 + 两处回退）；
  - `grep -rn "current_site_id" backend/handlers/*.go | grep -v warehouse` → 其余均为读侧（`repair.go` 验收门、`public.go` 展示、`api.go` 输出）；
  - `grep -rn "SiteID\b" backend/handlers/assessment*.go` → 检查 `SubmitAssessment` 是否另有同类写法（结果在 R5 artifact 打印；若命中则按 D3 处理）。
- **损坏输入拒绝**：`resolveSiteIDByOrg` 查询失败/0 行 → 返回空 → 调用方**不写** `current_site_id`（保留原值）+ WARN（含 instrument id 与 org 值）；多站点 → 确定性规则（锚点优先，其次 `created_at` 最早），不得随机/取 map 顺序。
- **工具盲区声明**：
  - `go test ./handlers` 需 Postgres 测试库（`docs/ops/test-db-lifecycle.md`）；本单只跑定向套件，全量由 pre-ready P5 baseline 差分承担；
  - R5 的存量排查 SQL **只出文本不执行**（数据修复需 Owner 授权）；生产污染规模本单不核实（Out of Scope）；
  - grep 为文本级：以「回退分支特征串 = 0 + `resolveSiteIDByOrg` 被两处调用」作为最小防漂移断言。
- **证据时序**：fix commit 之后重跑 `bash scripts/verify-2511.sh r1` … `r6`；`evidence.json` 的 `build/commit/dirty` 由脚本从 `git rev-parse` 派生，禁止手改。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 回退解析规则：`order.OrgID` → `sites.id` 反查；**恰好 1 个**用该站点，**多个**优先锚点（`org_id == tenant_id`）否则 `created_at` 最早，**0 个不写入** + WARN | Architect | 本计划锁定（Issue 期望 1/2 允许「取首个」或「不写 + WARN」，此处合并为确定性规则） | 若 Owner 要求「多站点一律不写」→ 回 Architect 简化 |
| D2 | 主分支（操作者 `site_members.site_id`）语义不变；不覆盖为 org 值 | Architect | 本计划锁定 | 阻塞实现 |
| D3 | 存量 `current_site_id` 污染**不修数据**：只交付只读排查 SQL；数据修复需 Owner 授权（AGENTS.md 红线） | Architect | 本计划锁定 | 审计若要求治理 → 转 Ops/独立 Issue |
| D4 | 若 `SubmitAssessment`（`assessment` handler）存在同类写法 → **不在本单修**，登记到本单 Review 报告的 deferral 并在有用户可见影响时另立 Issue | Architect | 本计划锁定（本单范围限定 warehouse 两处） | 若 R5 扫描命中且属缺陷 → 回 Architect 决定是否纳入 |
| D5 | 生产污染规模与 `instruments.current_site_id NOT IN (SELECT id FROM sites)` 行数 → 不在本单 | Architect | 本计划锁定 | 审计若要求实测 → 转 Ops Follow-up |

## Out of Scope

- 存量数据修复（`instruments.current_site_id` 域污染）→ 需 Owner 授权，另立 Issue（本单只给只读 SQL）。
- S2 `instrument_loss.go`（#2515，已 accepted）、S4 `user_rental.go`（#2512，已 accepted）、S5 `repair_service.go`（#2513，已 accepted）。
- `assessment` handler（`SubmitAssessment`）若存在同类写法 → 本单只扫描登记（D4）。
- `current_site_id` 的**读侧**语义调整（验收门 `repair.go:143/204/269/325`、`public.go` 展示）→ 不改。
- 前端任何改动（H5/小程序/PC）。
- 站点数据治理（同 org 多站点的唯一性约束/清理）→ 另议。

## G0 Golden Acceptance

| 行 ID | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） | assertions（含负例） |
|---|---|---|---|---|
| R1 | tenant T；site A(`id=aID, org_id=aOrg`)、`aID≠aOrg≠T`；order(`org_id=aOrg`)；操作者无 `site_members`；乐器(`current_site_id=NULL`) | `PUT /warehouse/orders/:id/return-inspect`（`condition=damaged`） | `docs/evidence/2511/R1-inspect-return.txt` | `CURRENT_SITE_ID=aID`、`OLD_FALLBACK_IS_ORG=1`、`RED_CONFIRMED=1`；负例：写入 `aOrg` → FAIL |
| R2 | 同 R1（另一张订单） | `PUT /warehouse/orders/:id/damage` | `docs/evidence/2511/R2-assess-damage.txt` | `CURRENT_SITE_ID=aID`、`NOT_ORG=1`；负例：写入 `aOrg` → FAIL |
| R3 | 同 R1，但 `order.OrgID` = 无站点组织；乐器预置 `current_site_id=aID` | `return-inspect` | `docs/evidence/2511/R3-unresolvable.txt` | `KEPT_ORIGINAL=1`、`NO_CROSS_DOMAIN_WRITE=1`、`CURRENT_SITE_ID=aID`；负例：被覆盖为 org 或 NULL → FAIL |
| R4 | 同 org 两站点（含锚点 `org_id==tenant_id`）/ 无锚点两组；操作者有成员场景 | `resolveSiteIDByOrg` + `return-inspect` | `docs/evidence/2511/R4-deterministic.txt` | `ANCHOR_PREFERRED=1`、`EARLIEST_FALLBACK=1`、`EMPTY_FOR_UNKNOWN=1`、`PRIMARY_EQUALS_OPERATOR_SITE=1`；负例：主分支被 org 覆盖 → FAIL |
| R5 | 修后 `warehouse.go` + 只读 SQL | grep + 文本 | `docs/evidence/2511/R5-scan-sql.txt` | `ORG_WRITTEN_TO_SITE=0`、`RESOLVER_CALLS=2`、`SQL_PRESENT=1`；负例：残留 `updates["current_site_id"] = order.OrgID` → FAIL |
| R6 | 修后工作树 | `cd backend` | `docs/evidence/2511/R6-build-regression.txt` | `BUILD_RC=0`、`REG_RC=0`；负例：任一既有用例失败 → FAIL |
| R7 | 修后干净树 + 最新 Accepted Plan + evidence.json | `pre-ready-check.sh 2511` | `docs/evidence/2511/pre-ready-2511.json` | `GATE_RESULT=PASS`、每行 `pass=true`、`dirty=false`、P13 在 `origin/issue-2511`；负例：任一 P 失败/evidence 缺行 → 不得 ready |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2511.sh r1 > docs/evidence/2511/R1-inspect-return.txt` | `docs/evidence/2511/R1-inspect-return.txt` | exit 0；`grep -q '^R1_OK$'` 且 `grep -q 'SITE_ID_IS=aID'` | `CURRENT_SITE_ID=aID`、`OLD_FALLBACK_IS_ORG=1`、`RED_CONFIRMED=1` | 写入 `aOrg` → `R1_FAIL` 且非 0 退出 | dev |
| R2 | `bash scripts/verify-2511.sh r2 > docs/evidence/2511/R2-assess-damage.txt` | `docs/evidence/2511/R2-assess-damage.txt` | exit 0；`grep -q '^R2_OK$'` 且 `grep -q 'NOT_ORG=1'` | `CURRENT_SITE_ID=aID` | 写入 `aOrg` → `R2_FAIL` 且非 0 退出 | dev |
| R3 | `bash scripts/verify-2511.sh r3 > docs/evidence/2511/R3-unresolvable.txt` | `docs/evidence/2511/R3-unresolvable.txt` | exit 0；`grep -q '^R3_OK$'` 且 `grep -q 'KEPT_ORIGINAL=1'` | `NO_CROSS_DOMAIN_WRITE=1`、`CURRENT_SITE_ID=aID` | 原值被覆盖 → `R3_FAIL` 且非 0 退出 | dev |
| R4 | `bash scripts/verify-2511.sh r4 > docs/evidence/2511/R4-deterministic.txt` | `docs/evidence/2511/R4-deterministic.txt` | exit 0；`grep -q '^R4_OK$'` 且 `grep -q 'ANCHOR_PREFERRED=1'` | `EARLIEST_FALLBACK=1`、`EMPTY_FOR_UNKNOWN=1`、`PRIMARY_EQUALS_OPERATOR_SITE=1` | 主分支被 org 覆盖或解析不确定 → `R4_FAIL` 且非 0 退出 | dev |
| R5 | `bash scripts/verify-2511.sh r5 > docs/evidence/2511/R5-scan-sql.txt` | `docs/evidence/2511/R5-scan-sql.txt` | exit 0；`grep -q '^R5_OK$'` 且 `grep -q 'ORG_WRITTEN_TO_SITE=0'` | `RESOLVER_CALLS=2`、`SQL_PRESENT=1` | 残留 org→site 写入 → `R5_FAIL` 且非 0 退出 | dev |
| R6 | `bash scripts/verify-2511.sh r6 > docs/evidence/2511/R6-build-regression.txt` | `docs/evidence/2511/R6-build-regression.txt` | exit 0；`grep -q '^R6_OK$'` 且 `grep -q 'BUILD_RC=0'` | `REG_RC=0` | 构建/回归失败 → `R6_FAIL` 且非 0 退出 | dev |
| R7 | `~/.config/opencode/scripts/pre-ready-check.sh 2511 --summary-file docs/evidence/2511/summary.md --changed-files docs/evidence/2511/changed-files.txt --evidence-file docs/evidence/2511/evidence.json --affected-cmd "bash scripts/verify-2511.sh r6" --full-gate-cmd "bash scripts/verify-2511.sh gate" --report docs/evidence/2511/pre-ready-2511.json` | `docs/evidence/2511/pre-ready-2511.json` | exit 0；`GATE_RESULT=PASS`，`jq -e '.result == "PASS"'`，evidence 每行 `pass=true` | `dirty=false`、P13 提交在 `origin/issue-2511` | 任一 P 项失败 / evidence 缺行 / `dirty=true` → gate 非 0，禁止 ready | dev |

### Files（变更文件清单）

| 类别 | 路径 |
|---|---|
| Handler 修复 | `backend/handlers/warehouse.go` |
| 新增测试 | `backend/handlers/warehouse_2511_test.go` |
| 证据生成器（新增） | `scripts/verify-2511.sh` |
| 口径文档 | `docs/cases/cases.md` |
| 证据（计划） | `docs/evidence/2511/plan.md` |
| 证据（摘要） | `docs/evidence/2511/summary.md` |
| 证据（清单） | `docs/evidence/2511/changed-files.txt` |
| 证据（合同汇总） | `docs/evidence/2511/evidence.json` |
| 证据 R1 | `docs/evidence/2511/R1-inspect-return.txt` |
| 证据 R2 | `docs/evidence/2511/R2-assess-damage.txt` |
| 证据 R3 | `docs/evidence/2511/R3-unresolvable.txt` |
| 证据 R4 | `docs/evidence/2511/R4-deterministic.txt` |
| 证据 R5 | `docs/evidence/2511/R5-scan-sql.txt` |
| 证据 R6 | `docs/evidence/2511/R6-build-regression.txt` |
| Gate 报告 | `docs/evidence/2511/pre-ready-2511.json` |

## Artifact Provenance

- 生成器：`scripts/verify-2511.sh`（`r1`–`r6` + `gate`），每个文本 artifact 首行必须是
  `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`（锚定本单 fix commit；证据收尾提交不改变锚点）。
- 逐条清单（全部位于 `docs/evidence/2511/`）：
  - `docs/evidence/2511/R1-inspect-return.txt`
  - `docs/evidence/2511/R2-assess-damage.txt`
  - `docs/evidence/2511/R3-unresolvable.txt`
  - `docs/evidence/2511/R4-deterministic.txt`
  - `docs/evidence/2511/R5-scan-sql.txt`
  - `docs/evidence/2511/R6-build-regression.txt`
  - `docs/evidence/2511/pre-ready-2511.json`（gate 报告，JSON 自带 `.provenance.{build,commit,dirty}`）
- `docs/evidence/2511/evidence.json` 字段口径：`build` 形如 `1.0.<7-40 位短码>`，
  `commit` 为 40 位 sha，`dirty:false`；evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$
- 证据重生成纪律：fix commit 之后统一重跑 `bash scripts/verify-2511.sh r1` … `r6`，禁止只改 `build/commit` 冒充新证据。

*Model: DeepSeek V4.1 Flash 1017*

