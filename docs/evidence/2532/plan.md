## Accepted Plan — #2532 `SubmitAssessment` 回退分支：禁止把组织 ID 写入 `instruments.current_site_id`

> 前置（硬依赖）：#2511（S3 warehouse 同一缺陷；`resolveSiteIDByOrg` 解析函数由 #2511 单一实现，本单只消费不复刻）
> 同族登记：本单是 #2509「ID 域混用同族审计」在 S3 扫描中登记的第 6 处（`assessment` handler），#2509 仍在 holdon，等 S3/S4 与本单
> 关联：#1889（`current_site_id` 引入点）、#2526（`uuid.Nil` 域污染同族）、#2508（ID 域口径首个修复）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| `SubmitAssessment`（`hasDamage=true`）在解析不到操作者站点时，把 `order.OrgID` 写进 `instruments.current_site_id` | `backend/handlers/assessment.go:132-138`（回退赋值在 `:134-135`） |
| `instruments.current_site_id` 是 `sites.id` 域（`*uuid.UUID`），列定义带 `REFERENCES sites(id)` | `backend/models/models.go:99`；`backend/database/migrations/002_add_site_management.up.sql:29-31` |
| `orders.org_id` 是组织域（IAM `oid`），与 `sites.id` 不同键 | `backend/models/models.go:87`、`docs/cases/cases.md:289` |
| 主分支取值来自 `site_members.site_id`（正确域） | `backend/handlers/repair.go:620-630`（`resolveOperatorSiteID`） |
| 下游按 `current_site_id` 关联 `site_members` 做验收门；按 `sites` 反查网点展示 | `backend/handlers/repair.go:143/204/269/325`；`backend/handlers/public.go:278-284` |
| 现有用例只覆盖主分支（操作者有 `site_members`），回退分支零覆盖 | `backend/handlers/repair_site_1889_test.go:166-190`（`TestSubmitAssessment_Damaged_SetsCurrentSiteID`） |
| 同族残留：`grep -rnE 'current_site_id"\] = .*OrgID' backend/handlers/` = 3（本单 1 + #2511 的 warehouse 2） | `/analyze` 实测（2026-11-25） |
| `resolveSiteIDByOrg` 尚未入库（#2511 仍 `status:wip`，由另一节点执行） | `grep -rn "func resolveSiteIDByOrg" backend/handlers` → 无命中 |

**根因**：回退分支把「订单所属组织」直接当作「站点」写入，缺「解析失败」出口——组织值落进站点列后，Postgres 外键 `instruments_current_site_id_fkey` 直接 23503（接口 500）；若外键被历史绕过，验收门与网点展示都查不到站点（乐器卡「无法验收」）。

### 变更点

**A. 代码（`backend/handlers/assessment.go` 单文件，1 处）**

1. `SubmitAssessment` 回退分支改为「组织 → 站点反查」，解析失败**不写入**（保留原值）+ WARN：

```go
if siteID := resolveOperatorSiteID(h.db, middleware.GetUserID(c.Request.Context())); siteID != nil {
	instUpdates["current_site_id"] = *siteID
} else if resolved := resolveSiteIDByOrg(h.db, order.OrgID); resolved != "" {
	// #2511/#2532: order.OrgID 是组织域，必须先反查 sites.id；禁止直接写入
	instUpdates["current_site_id"] = resolved
} else {
	// #1889/#2511/#2532: 无法解析站点 → 保留原值，禁止写入跨域值
	log.Printf("[WARN] #2532: no site resolved for instrument %s entering repair (operator has no site membership; order org %q maps to no site)", order.InstrumentID, order.OrgID)
}
```

2. **禁止**在本单定义第二份解析函数（`resolveSiteIDByOrg` 的单一 owner 是 #2511）；若 #2511 落地时函数名/签名有变，只允许改调用点，不得在本文件新增 `func resolveSite...`。
3. 不改主分支语义（有 `site_members` → 操作者站点），不改其它 `current_site_id` 消费方，不改 `GetAssessmentData`。

**B. 测试（新增 `backend/handlers/assessment_2532_test.go`，自带夹具，不复用 `newFixture1889` 的「站点 org 与订单 org 同源即通过」假设）**

4. 夹具约束：`sites.id (aID) ≠ sites.org_id (aOrg)` 且 `order.OrgID = aOrg`；操作者 `users` 行存在、claims `oid=aOrg`，但**无** `site_members` 行（`resolveOperatorSiteID` → nil，确定性进入回退分支）。
5. 用例：
   - `TestSubmitAssessment2532_FallbackWritesSiteNotOrg`：`POST /orders/:id/assessment`（`hasDamage=true`）→ `current_site_id == aID` 且 `!= aOrg`；输出 `STATUS=200`、`OPERATOR_SITE=none`、`RESOLVED_IS_SITE=1`、`CURRENT_IS_SITE=1`、`CURRENT_NOT_ORG=1`、`OLD_VALUE_IS_ORG=1`（旧表达式 `order.OrgID` 不属于 `sites.id` 集合）、`RED_CONFIRMED=1`；
   - `TestSubmitAssessment2532_UnresolvableKeepsValue`（负例）：订单 `OrgID` 无对应站点，乐器预置 `current_site_id = aID` → 提交后仍为 `aID`；输出 `KEPT_ORIGINAL=1`、`NO_CROSS_DOMAIN_WRITE=1`、`RESOLVED_EMPTY=1`、`OLD_WOULD_WRITE_ORG=1`；
   - `TestSubmitAssessment2532_PrimaryBranchUnchanged`（负例）：操作者**有** `site_members` → 写操作者站点；输出 `PRIMARY_EQUALS_OPERATOR_SITE=1`、`PRIMARY_NOT_ORG=1`。

**C. 证据**

6. 新增 `scripts/verify-2532.sh`（`r1`–`r4` + `gate` + `ledger`，首行 provenance）；`r4` 输出同类扫描计数（tree-wide 必须 0，依赖 #2511 已合入）。

- Tags: bug, code:go, scope:backend

### 语义变更（必须翻转的既有断言）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `backend/handlers/assessment.go:134-135`（回退分支，无既有断言） | 解析不到操作者站点时把 `order.OrgID` 写入 `current_site_id` | `order.OrgID` → `sites.id` 反查；不可解析**不写入** + WARN | 组织域值写入站点域列（FK 23503 / 验收门失配） |
| `backend/handlers/repair_site_1889_test.go:166-190`（`TestSubmitAssessment_Damaged_SetsCurrentSiteID`） | 主分支写操作者站点 | **保持不变**（同一期望，纳入 R4 定向回归） | 本单只改回退分支 |

> 无必须翻转的既有断言（旧行为未被任何用例锁定）；但按「语义变更必须翻转旧断言」纪律，本单**新增负例行**：`CURRENT_NOT_ORG=1`、`NO_CROSS_DOMAIN_WRITE=1`、`PRIMARY_NOT_ORG=1`——断言「旧行为不再发生」。

### Core Update

无。`instruments.current_site_id` 的合法写入来源行（`docs/cases/cases.md:289` 附近）由 #2511 的 Core Update 单一持有，本单不重复登记（避免同一口径两处 owner 漂移）；本单不改任何核心文档。

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 触发条件 | 评估人**无** `site_members`（商户直属员工/师傅/成员数据缺失）→ 提交有损评估（`hasDamage=true`）→ 回退分支把 `order.OrgID` 写入 `current_site_id` | `assessment.go:132-138` |
| 2 | 根因 | 回退把组织域值写入站点域列，且无「解析失败」出口（组织为空才 WARN，非空一律写脏值） | `models.go:99`、`migrations/002:29-31` |
| 3 | 为何长期未发现 | 既有用例只覆盖主分支；夹具 `sites.org_id == order.OrgID`，即使走回退也「看起来对」；`orders.org_id` 与 `sites.id` 在 dev 小数据里可能同值 | `repair_site_1889_test.go:166-190` |
| 4 | 同类扫描 | 同族：#2511（warehouse `:697`/`:872`，S3）、#2510（已 done）、#2513（已 done）、#2512（S4，analysis）、#2508（S1，已 done） | `/analyze` grep 实测 3 处残留 |
| 5 | 防复发（代码） | 复用唯一解析函数 `resolveSiteIDByOrg`（owner=#2511），禁止本文件新增解析实现；解析失败不写入 + WARN | 变更点 1–2；R4 `LOCAL_RESOLVER_DEF=0` |
| 6 | 防复发（测试） | 3 个用例：回退写站点（红绿）、不可解析保留原值、主分支不变 | 变更点 5 |
| 7 | 防复发（文档） | `current_site_id` 合法来源口径由 #2511 Core Update 单点持有；本单以依赖方式继承，不复制 | Core Update、`## Dependencies` |
| 8 | 假阴性反例 | 若回退仍写 `order.OrgID` → R1 `CURRENT_NOT_ORG=0`/`OLD_VALUE_IS_ORG=1` 断言 FAIL；若不可解析时覆盖原值为空/org → R2 FAIL；若主分支被回退覆盖 → R3 FAIL；若残留任何 `current_site_id"] = .*OrgID` → R4 `TREE_ORG_ASSIGN>0` FAIL | R1/R2/R3/R4 负例 |
| 9 | 回归面 | 主分支行为不变（R3 + R4 定向回归包含 `TestSubmitAssessment_Damaged_SetsCurrentSiteID`、`TestResolveOperatorSiteID`、`TestInspectReturn_*`、`TestAssessDamage_*`） | 变更点 5、R4 |
| 10 | 未修边界 | 存量 `current_site_id` 域污染数据修复（需 Owner 授权）；`assessment.go` 的 `orders`/`instruments` 更新未加 `tenant_id` 条件的隔离问题；warehouse 两处（#2511） | Out of Scope |

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志） |
|---|---|---|---|---|---|---|
| R1 回退分支写站点 ID（红绿） | `backend/handlers/assessment.go`、`backend/handlers/assessment_2532_test.go` | `POST /api/orders/:id/assessment` | 操作者无 `site_members`；`order.OrgID=aOrg` 且仅映射一个站点 `aID (aID≠aOrg)` | backend（Postgres 测试库） | `STATUS=200`、`OPERATOR_SITE=none`、`RESOLVED_IS_SITE=1`、`CURRENT_IS_SITE=1`、`CURRENT_NOT_ORG=1`、`OLD_VALUE_IS_ORG=1`、`RED_CONFIRMED=1` | R1 |
| R2 不可解析 → 保留原值（负例） | 同上 | `POST /api/orders/:id/assessment` | 订单 `OrgID` 无对应站点；乐器预置 `current_site_id=aID` | backend | `STATUS=200`、`KEPT_ORIGINAL=1`、`NO_CROSS_DOMAIN_WRITE=1`、`RESOLVED_EMPTY=1`、`OLD_WOULD_WRITE_ORG=1` | R2 |
| R3 主分支不变（负例） | 同上 | `POST /api/orders/:id/assessment` | 操作者有 `site_members`（`aID`） | backend | `STATUS=200`、`PRIMARY_EQUALS_OPERATOR_SITE=1`、`PRIMARY_NOT_ORG=1` | R3 |
| R4 同类扫描 + 构建 + 定向回归 | `backend/handlers/assessment.go`、`scripts/verify-2532.sh` | `bash scripts/verify-2532.sh r4`（`cd backend && go build ./...` + grep + 定向套件） | 修后工作树（#2511 已合入） | backend | `BUILD_RC=0`、`ASSESSMENT_ORG_ASSIGN=0`、`TREE_ORG_ASSIGN=0`、`RESOLVER_CALL=1`、`LOCAL_RESOLVER_DEF=0`、`WARN_LINE_PRESENT=1`、`REG_FAIL=0` | R4 |
| R5 pre-ready gate（四源 + 定向测试 + 构建） | `docs/evidence/2532/pre-ready-2532.json` | `pre-ready-check.sh 2532` | 修后干净树 + 最新 Accepted Plan | backend | `overall=PASS`、每行 `pass=true`、`dirty=false`、P13 在 `origin/issue-2532` | R5 |

### Input Truth & Tool Preflight

- **Source of truth（口径唯一）**：`docs/cases/cases.md:289`（`sites.org_id` = 组织 ID、`sites.id` = 站点主键）；`backend/models/models.go:99`（`current_site_id *uuid.UUID`）；`backend/database/migrations/002_add_site_management.up.sql:29-31`（`REFERENCES sites(id)`）；`backend/handlers/repair.go:620-630`（`resolveOperatorSiteID` 取 `site_members.site_id`）；#2511 Accepted Plan（`resolveSiteIDByOrg` 唯一实现 + 确定性解析规则：恰好 1 站用该站；多站优先锚点 `org_id == tenant_id`，否则 `created_at` 最早；0 站不写 + WARN）。
- **差异核对（只读，已于 /analyze 执行）**：
  - `grep -n "current_site_id" backend/handlers/assessment.go` → 仅 `:133`（主分支）与 `:135`（回退），无其它写点；
  - `grep -rnE 'current_site_id"\] = .*OrgID' backend/handlers/` → `assessment.go:135`、`warehouse.go:697`、`warehouse.go:872`（后两处归 #2511）；
  - `grep -rn "func resolveSiteIDByOrg" backend/handlers` → 0 命中（#2511 未入库 → 硬依赖）；
  - `grep -rn "TestSubmitAssessment" backend/handlers` → 仅 `repair_site_1889_test.go:166`（主分支；回退分支零覆盖）。
- **损坏输入拒绝**：`order.OrgID == ""` 或反查 0 站 → **不写** `current_site_id`（保留原值）+ WARN（含 instrument id 与 org 值）；多站点按 #2511 确定性规则，不取 map 顺序、不随机。
- **工具盲区声明**：
  - `go test ./handlers` 需 Postgres 测试库（`docs/ops/test-db-lifecycle.md`）；测试库由 GORM `AutoMigrate` 建表，**不一定**复刻生产的 `instruments_current_site_id_fkey`，故红证据用「旧值不属于 `sites.id` 集合」（`OLD_VALUE_IS_ORG=1`）而不是「FK 报 23503」；
  - 全量套件由 pre-ready P5 baseline 差分承担，合同行只跑定向用例 + `go build ./...`；
  - `grep` 为文本级：以「回退赋值特征串为 0 + 解析函数被调用 1 次 + 本文件无解析函数定义」作为最小防漂移断言（`TREE_ORG_ASSIGN=0` 依赖 #2511 已合入，未合入时 `/work` 必须 HOLD，禁止放宽断言）。
- **证据时序**：fix commit 之后重跑 `bash scripts/verify-2532.sh r1` … `r4`；`evidence.json` 的 `build/commit/dirty` 由 `bash scripts/verify-2532.sh ledger` 从最新 `fix(issue-2532)` commit 派生并写回（禁止手改）。

### Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 回退改为复用 **#2511 的 `resolveSiteIDByOrg`**（单一实现）；不可解析**不写入** + WARN | Architect | 本计划锁定（Issue「期望」/「证据要求」要求复用 #2511） | 若 Owner 要求「多站点一律不写」→ 回 Architect 简化（属 #2511 D1 同一决策） |
| D2 | `resolveSiteIDByOrg` 的名称/签名 owner 是 #2511；本单只消费，**禁止**在 `assessment.go` 新增同名/同类解析函数 | Architect | 本计划锁定 | #2511 落地形态不同（改名/改签名）→ 只改本单调用点并在计划 Rev 记录；语义若变化 → 回 Architect |
| D3 | 前置未满足（#2511 未 `done`）时 `/work` 必须 HOLD，**不得**先落地局部副本或临时内联反查 | Architect | 本计划锁定（`## Dependencies`） | 若 Owner 要求解耦 → 另立 Issue 把解析函数迁到共享文件（更大改动） |
| D4 | 测试库不复刻生产 FK，红证据形态 = 「旧值不属于 `sites.id` 集合」+「修复后值属于集合且 ≠ org」 | Architect | 本计划锁定 | 审计要求真实 FK 23503 证据 → 转 Ops/Probe（需带 FK 的库） |
| D5 | 存量 `current_site_id` 域污染排查/修复 → 不在本单（需 Owner 授权改数据） | Architect | 本计划锁定 | Owner 要求排查 → 只出只读 SQL 作为 Ops Follow-up |

### Out of Scope

- `warehouse.go:697`、`:872` 两处（归 #2511）。
- 存量 `current_site_id` 域污染数据修复（需 Owner 授权，另立 Ops）。
- `assessment.go` 中 `orders`/`instruments` 更新缺 `tenant_id` 条件的隔离加固（与 ID 域无关，另立）。
- `GetAssessmentData`（对比照片接口）与其 `damageLevel` 推导。
- 前端任何改动（H5/小程序/PC）。
- `docs/cases/cases.md` 的 `current_site_id` 合法来源行（#2511 Core Update 单点持有）。
- #2509 父任务验收与其余子任务（S3 #2511、S4 #2512）。

## G0 Golden Acceptance

| 行 ID | fixture（输入/seed） | entry（生产入口） | artifact（路径/类型） | assertions（含负例） |
|---|---|---|---|---|
| R1 | tenant=T；site A(`id=aID, org_id=aOrg`)，`aID≠aOrg`；instrument(`org=aOrg`, `current_site_id=NULL`)；order(`org_id=aOrg`)；操作者 user(claims `tid=T, oid=aOrg`)**无** `site_members` | `POST /orders/:id/assessment`（`hasDamage=true`） | `docs/evidence/2532/R1-assessment-fallback.txt` | `STATUS=200`、`OPERATOR_SITE=none`、`RESOLVED_IS_SITE=1`、`CURRENT_IS_SITE=1`、`CURRENT_NOT_ORG=1`、`OLD_VALUE_IS_ORG=1`、`RED_CONFIRMED=1`；负例：`current_site_id == aOrg` 或 500 → FAIL |
| R2 | 同 R1，但 order(`org_id=orgB`，`orgB` 无任何站点)；instrument 预置 `current_site_id=aID` | `POST /orders/:id/assessment`（`hasDamage=true`） | `docs/evidence/2532/R2-unresolvable-keeps.txt` | `STATUS=200`、`KEPT_ORIGINAL=1`、`NO_CROSS_DOMAIN_WRITE=1`、`RESOLVED_EMPTY=1`、`OLD_WOULD_WRITE_ORG=1`；负例：`current_site_id` 变为 `orgB`/空 → FAIL |
| R3 | 同 R1，但操作者**有** `site_members(site_id=aID)` | `POST /orders/:id/assessment`（`hasDamage=true`） | `docs/evidence/2532/R3-primary-unchanged.txt` | `STATUS=200`、`PRIMARY_EQUALS_OPERATOR_SITE=1`、`PRIMARY_NOT_ORG=1`；负例：`current_site_id != aID` 或 == org → FAIL |
| R4 | 修后工作树（#2511 已合入） | `cd backend && go build ./...` + grep 扫描 + 定向套件 | `docs/evidence/2532/R4-scan-build.txt` | `BUILD_RC=0`、`ASSESSMENT_ORG_ASSIGN=0`、`TREE_ORG_ASSIGN=0`、`RESOLVER_CALL=1`、`LOCAL_RESOLVER_DEF=0`、`WARN_LINE_PRESENT=1`、`REG_FAIL=0`；负例：任意 `current_site_id"] = .*OrgID` 残留或本文件新增解析函数 → FAIL |
| R5 | 修后干净树 + 最新 Accepted Plan + evidence.json | `pre-ready-check.sh 2532` | `docs/evidence/2532/pre-ready-2532.json` | `overall=PASS`、每行 `pass=true`、`dirty=false`、P13 在 `origin/issue-2532`；负例：任一 P 项失败 / evidence 缺行 → 不得 ready |

### Evidence Contract

| 行 ID | 命令 | 产物 | 断言 | 期望 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2532.sh r1 > docs/evidence/2532/R1-assessment-fallback.txt` | `docs/evidence/2532/R1-assessment-fallback.txt` | exit 0；`grep -q '^R1_OK$'` 且 `grep -q 'CURRENT_NOT_ORG=1'` 且 `grep -q 'RED_CONFIRMED=1'` | `STATUS=200`、`RESOLVED_IS_SITE=1`、`CURRENT_IS_SITE=1`、`OLD_VALUE_IS_ORG=1` | `current_site_id` 仍等于 `order.OrgID`（`CURRENT_NOT_ORG=0`）或 500 → `R1_FAIL` 非 0 退出 | dev |
| R2 | `bash scripts/verify-2532.sh r2 > docs/evidence/2532/R2-unresolvable-keeps.txt` | `docs/evidence/2532/R2-unresolvable-keeps.txt` | exit 0；`grep -q '^R2_OK$'` 且 `grep -q 'KEPT_ORIGINAL=1'` | `NO_CROSS_DOMAIN_WRITE=1`、`RESOLVED_EMPTY=1`、`OLD_WOULD_WRITE_ORG=1` | 不可解析时覆盖原值为 org/空（`KEPT_ORIGINAL=0`）→ `R2_FAIL` 非 0 退出 | dev |
| R3 | `bash scripts/verify-2532.sh r3 > docs/evidence/2532/R3-primary-unchanged.txt` | `docs/evidence/2532/R3-primary-unchanged.txt` | exit 0；`grep -q '^R3_OK$'` 且 `grep -q 'PRIMARY_EQUALS_OPERATOR_SITE=1'` | `PRIMARY_NOT_ORG=1`、`STATUS=200` | 主分支被回退覆盖为组织 ID（`PRIMARY_NOT_ORG=0`）→ `R3_FAIL` 非 0 退出 | dev |
| R4 | `bash scripts/verify-2532.sh r4 > docs/evidence/2532/R4-scan-build.txt` | `docs/evidence/2532/R4-scan-build.txt` | exit 0；`grep -q '^R4_OK$'` 且 `grep -q 'BUILD_RC=0'` | `ASSESSMENT_ORG_ASSIGN=0`、`TREE_ORG_ASSIGN=0`、`RESOLVER_CALL=1`、`LOCAL_RESOLVER_DEF=0`、`WARN_LINE_PRESENT=1`、`REG_FAIL=0` | 残留 `current_site_id"] = .*OrgID` 或本文件重定义解析函数 → `R4_FAIL` 非 0 退出 | dev |
| R5 | `~/.config/opencode/scripts/pre-ready-check.sh 2532 --summary-file docs/evidence/2532/summary.md --changed-files docs/evidence/2532/changed-files.txt --evidence-file docs/evidence/2532/evidence.json --affected-cmd "bash scripts/verify-2532.sh r4" --full-gate-cmd "bash scripts/verify-2532.sh gate" --report docs/evidence/2532/pre-ready-2532.json` | `docs/evidence/2532/pre-ready-2532.json` | exit 0；`jq -e '.overall == "PASS"' docs/evidence/2532/pre-ready-2532.json` 且 evidence 每行 `pass=true` | `overall=PASS`、`dirty=false`、P13 提交在 `origin/issue-2532` | 任一 P 项失败 / evidence 缺行 / `dirty=true` → gate 非 0，禁止 ready | dev |

### Files（变更文件清单）

| 类别 | 路径 |
|---|---|
| Handler 修复 | `backend/handlers/assessment.go` |
| 新增测试 | `backend/handlers/assessment_2532_test.go` |
| 证据生成器（新增） | `scripts/verify-2532.sh` |
| 证据（计划快照） | `docs/evidence/2532/plan.md` |
| 证据（摘要） | `docs/evidence/2532/summary.md` |
| 证据（清单） | `docs/evidence/2532/changed-files.txt` |
| 证据（合同汇总） | `docs/evidence/2532/evidence.json` |
| 证据 R1 | `docs/evidence/2532/R1-assessment-fallback.txt` |
| 证据 R2 | `docs/evidence/2532/R2-unresolvable-keeps.txt` |
| 证据 R3 | `docs/evidence/2532/R3-primary-unchanged.txt` |
| 证据 R4 | `docs/evidence/2532/R4-scan-build.txt` |
| Gate 报告 | `docs/evidence/2532/pre-ready-2532.json` |

## Artifact Provenance

- 生成器：`scripts/verify-2532.sh`（`r1`–`r4` + `gate` + `ledger`），每个文本 artifact 首行必须是
  `# build=1.0.<短码> # commit=<40 位 sha> # dirty=false`（锚定本单最新 `fix(issue-2532)` commit；证据收尾提交不改变锚点）。
- 逐条清单（全部位于 `docs/evidence/2532/`）：
  - `docs/evidence/2532/R1-assessment-fallback.txt`
  - `docs/evidence/2532/R2-unresolvable-keeps.txt`
  - `docs/evidence/2532/R3-primary-unchanged.txt`
  - `docs/evidence/2532/R4-scan-build.txt`
  - `docs/evidence/2532/pre-ready-2532.json`（gate 报告，JSON 自带 `.provenance.{build,commit,dirty}`）
- `docs/evidence/2532/evidence.json` 字段口径：`build` 形如 `1.0.<7-40 位短码>`，`commit` 为 40 位 sha，`dirty:false`；evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$
- 证据重生成纪律：fix commit 之后统一重跑 `bash scripts/verify-2532.sh r1` … `r4`，禁止只改 `build/commit` 冒充新证据。

### Workload

- Size: S
- Basis: 1 个 handler 文件（1 处回退分支）+ 1 个新测试文件（3 用例）+ 1 个证据脚本 + 5 条证据行；无 migration、无前端、无文档改动、无新增解析函数
- Confidence: high（解析规则与函数 owner 已由 #2511 计划锁定；夹具与用例形态有 `repair_site_1889_test.go` 先例）

## Dependencies

- #2511

### 关联（非硬依赖）

- `#2509`：同族 ID 域审计父任务（本单为其 S3 扫描登记项；父任务仍 holdon）。
- `#1889`：`current_site_id` 记录物理位置的引入点（本处注释来源）。
- `#2526`：同族「nil UUID 域污染」缺陷（数据侧）。

*Model: DeepSeek V4.1 Flash 1017*

