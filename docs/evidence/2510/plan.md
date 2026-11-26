## Accepted Plan — #2510 丢失/找回归属校验的 ID 域纠偏（`oid` ↔ `current_site_id`）

> Tags: bug, code:go
> Parent: #2509（同族 ID 域审计）
> 关联：#1948（丢失/找回业务口径）、#1889（`current_site_id` 记录物理位置）、#2508（同族修复与夹具口径）、#2514（`List` 跨网点可见性，另单）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| 归属校验把 JWT `oid`（`sites.org_id`）与 `instruments.current_site_id`（`sites.id`）直接比较 | `backend/handlers/instrument_loss.go:47` |
| `current_site_id` 是 `sites.id` 域（`*uuid.UUID`，写入侧正确样例取自 `site_members.site_id`） | `backend/models/models.go:99`；`backend/handlers/warehouse.go:694-695` |
| `oid` 是 `sites.org_id` 域（ID 域权威：`docs/cases/cases.md:289`） | — |
| 用例要求「**网点员工**可做丢失登记 / 找回」→ 当前实现对网点账号恒拒，功能不可用 | `docs/cases/instrument-loss.md:13,93` |
| 路由：`POST /instruments/:id/lost`、`POST /instruments/:id/restore`、`GET /instrument-loss`，权限 `instrument:update|read`，且启用 `ExplicitTenantScope()`（关闭自动租户范围 → handler 必须显式隔离） | `backend/main.go:440-442`；`backend/middleware/explicit_tenant_scope.go:11-17` |
| `site_admin` / `site_member` 均持有 `instrument:update`（→ 故障对两类网点账号同样生效） | `backend/services/role_templates.go:49,63`；`:69`（site_member 亦然） |
| 测试夹具同样混淆 ID 域：`SeedSiteFK` 建 `sites.org_id = tenant_id`，staff actor 用 `OrgID: siteID` → 缺陷被测试掩盖 | `backend/handlers/testfixtures/seed.go:44`；`backend/handlers/instrument_loss_test.go:57-61` |

**根因**：`loadLossInstrument`（`instrument_loss.go:37-51`）用 `inst.CurrentSiteID.String() != oid` 判定「网点一致」，两个值是不同主键域；生产全部网点 `sites.id ≠ sites.org_id` → `Register` / `Restore` 对网点级操作者恒返回 `403/40300`（`instrument_loss.go:159-163`、`:340-344`）。

**为什么「网点一致」应保留语义**：#1889 的意图是「乐器**当前物理位置**的网点属于该操作者所在组织」，因此正确判定是「`current_site_id` 是 `oid` 组织下的站点之一」，既不是字符串相等，也不应改用 `instruments.org_id`（网点级乐器可能由商户级持有、`org_id` 为租户，见 `instrument_loss_test.go:82` 的既有夹具事实）。

### 修复设计（`backend/handlers/instrument_loss.go`）

1. `loadLossInstrument` 的网点判定改为**集合成员判定**：

```go
if oid := middleware.GetOrgID(ctx); oid != "" && inst.CurrentSiteID != nil {
    var n int64
    if err := db.Model(&models.Site{}).
        Where("id = ? AND org_id = ?", *inst.CurrentSiteID, oid).
        Count(&n).Error; err != nil {
        log.Printf("[InstrumentLoss] ERROR: resolve site org (site=%s, org=%s): %v", *inst.CurrentSiteID, oid, err)
        return nil, false
    }
    if n == 0 {
        log.Printf("[InstrumentLoss] WARN: access denied — instrument %s at site %s not in operator org %s", inst.ID, *inst.CurrentSiteID, oid)
        return nil, false
    }
}
```

2. 保持既有对外契约：仍返回 `403/40300 "instrument not found or access denied"`（前端已按该码处理，不在本单改状态码/文案），但**三种拒绝原因分别记 WARN/ERROR 日志**（not found / tenant mismatch / site-not-in-org），满足「不得静默当作不存在」。
3. `inst.CurrentSiteID == nil` → 不做网点级判定（仅租户校验），与现状一致（避免对本单外的空站点乐器产生新拒绝）。
4. **不改** `List`（`instrument_loss.go:388-410`）的可见范围；跨网点可见性问题另立（见 Out of Scope）。

### 语义变更（必须翻转的既有夹具/断言，强制清单）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `backend/handlers/instrument_loss_test.go:44-46`（`SeedSiteFK` 之后） | 站点 `org_id = tenant_id`（ID 域同值） | 显式补一行把该站点 `org_id` 更新为**独立组织 ID** `siteOrg`（≠ `siteID`、≠ `tenantID`） | 夹具必须区分 ID 域，否则 #2510 缺陷不可见（#2508 同口径） |
| `backend/handlers/instrument_loss_test.go:57-61` | staff actor `OrgID: siteID`（把站点 ID 当组织 ID） | `OrgID: siteOrg` | 生产约定：网点账号 JWT `oid` = `sites.org_id` |
| `backend/handlers/instrument_loss_test.go:48-53` | `users.org_id = siteID`（把站点 ID 写进用户组织列） | `users.org_id = siteOrg` | 同一口径（`users.org_id` 是组织域） |

> 既有 `TestInstrumentLoss_*` 各用例**断言值不变**；夹具改完后：修复前全部返回 403（红），修复后恢复原语义（绿）。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志） |
|---|---|---|---|---|---|---|
| R1 根因（ID 域）：旧谓词不成立、新判定成立 | `backend/handlers/instrument_loss_test.go` | `scripts/verify-2510.sh r1`（内部定向用例 `TestInstrumentLoss2510_IDDomainRootCause`） | 夹具站点 `id ≠ org_id` | backend（Postgres 测试库） | `OLD_PREDICATE_MATCH=0`、`NEW_RULE_MATCH=1`、`RED_CONFIRMED=1` | R1（合同 R1） |
| R2 主诉：网点账号可登记丢失 | `backend/handlers/instrument_loss.go` | `POST /instruments/:id/lost` | staff(oid=siteOrg) + 本组织站点乐器 | backend | `STATUS=200`、`CODE=20000`、`LOSS_RECORD_CREATED=1`、`INSTRUMENT_LOST=1` | R2（合同 R2） |
| R3 主诉：网点账号可执行找回 | `backend/handlers/instrument_loss.go` | `POST /instruments/:id/restore` | 已有未结丢失记录，staff(oid=siteOrg) | backend | `STATUS=200`、`RESTORED_DAMAGED=0`、`INSTRUMENT_AVAILABLE=1` | R3（合同 R3） |
| R4 跨组织拒绝（负例） | `backend/handlers/instrument_loss.go` | 同 `/lost`、`/restore` | 乐器在当前网点属**他组织**站点 | backend | `LOST_STATUS=403`、`RESTORE_STATUS=403`、`LEAK_RECORDS=0`（未落库、未改动乐器） | R4（合同 R4） |
| R5 旧行为消失（负例） | `backend/handlers/instrument_loss.go` | 同上，actor `oid = sites.id`（旧错误口径） | 该站点存在且 `org_id ≠ id` | backend | `OLD_OID_ALLOWED=0`（两接口均 403），不再按「字符串相等」放行 | R5（合同 R5） |
| R6 既有丢失套件回归 | `backend/handlers/instrument_loss_test.go` | `scripts/verify-2510.sh r6`（内部定向套件 `TestInstrumentLoss`） | 全部 `TestInstrumentLoss*`（含 LS-03/03a/03b/05/06 结算链路） | backend | `REGRESSION_PASS=1`、`FAIL_COUNT=0` | R6（合同 R6） |
| R7 构建 + 同类扫描 | `backend/handlers/instrument_loss.go`、`scripts/verify-2510.sh` | `cd backend && go build ./...` + `grep` | 修后工作树 | backend | `BUILD_RC=0`；`OID_SITE_STRING_COMPARE=0`（本文件已无 `CurrentSiteID...!= oid` 比较）；`LIST_SCOPE_OID=0`（本单未改 List，另单 #2514） | R7（合同 R7） |

## Input Truth & Tool Preflight

- **Source of truth（口径唯一）**：
  - ID 域：`docs/cases/cases.md:289`（`sites.org_id` = 网点自身 IAM 组织 ID）+ `models.go:99`（`current_site_id` 为 `sites.id`）。
  - 业务意图：#1889（`current_site_id` = 乐器物理位置）→ 网点判定 = 「该站点属于操作者组织」。
  - 角色能力：`docs/cases/instrument-loss.md:93`（网点员工可登记/找回）+ `role_templates.go:49,63`（持有 `instrument:update`）。
- **差异核对（实施前跑，结果入 R1/R7 artifact）**：
  - `grep -n "GetOrgID" backend/handlers/instrument_loss.go` → 仅 `:47`（唯一 ID 域消费点；`List` 不消费 `oid`）。
  - `grep -n "loadLossInstrument(c" backend/handlers/instrument_loss.go` → `:159`（Register）、`:340`（Restore）。
  - `grep -c "CurrentSiteID" backend/handlers/instrument_loss.go` → 修复后应为 1（仅判定处），不得残留字符串比较。
- **损坏输入拒绝**：`inst.CurrentSiteID == nil` → 跳过网点判定；`oid` 为空（商户/平台账号）→ 跳过（保持现状）；`sites` 查询失败 → 返回拒绝并记 ERROR（fail-closed，不得放行）。
- **工具盲区（显式声明）**：
  - 测试需要真实 Postgres（`testfixtures.SetupTestDB`）；`verify-2510.sh` **不得**设置 `TUNELOOP_TEST_ALLOW_SKIP`，skip 视为 FAIL。
  - `SeedSiteFK` 是共享 helper（`seed.go:38-48`），本单**不修改它**（会影响其他套件），只在 loss 夹具内更新本用例站点的 `org_id`。
  - 生产真实数据（预生产网点账号登记丢失）无法在 analyze 阶段执行 → 部署侧回执由 D5 承载。
- **影响面预检**：`loadLossInstrument` 只被 `Register` / `Restore` 调用；`List`、结算链路（`lossRentToDateCents`/`effectiveCashPaid`）、`M-08` 债务冲销无改动。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | 网点判定语义 = 「`current_site_id` 是操作者组织（`oid`）下站点之一」；`current_site_id IS NULL` → 只做租户校验（沿用现状） | Architect | 本计划锁定（#1889 物理位置意图） | 若要求「无站点归属即拒绝」→ 回 Architect（会影响纯库存乐器流程） |
| D2 | 不改对外状态码/文案：仍 `403/40300`，拒绝原因仅进日志（3 类 WARN/ERROR） | Architect | 本计划锁定（前端已按 40300 处理；改码属 #2504 文案域） | 若 Owner 要求 404/403 细分 → 另立 api-contract Issue |
| D3 | **不改** `SeedSiteFK`（共享 helper）；只在 loss 夹具内为本用例站点设置独立 `org_id` | Architect | 本计划锁定 | 若审计要求统一修 helper → 另立 Issue（会波及多套件夹具，需全量 gate） |
| D4 | `GET /api/instrument-loss` 的跨网点可见性（当前仅按 `tenant_id` 过滤，网点账号可见全商户丢失记录）**不在本单**，另立 #2514 | Architect | 本计划锁定 | 若要求同单修 → 回 Architect（需先裁定「丢失记录的站点归属」口径） |

## Out of Scope

- `GET /api/instrument-loss` 的网点级可见范围（D4 → #2514）。
- `instruments.current_site_id` 的写入侧污染修复（`warehouse.go` 回退写入 `order.OrgID`）→ #2511（Parent #2509）。
- 站点集合判定逻辑抽成共享 helper（当前仅 1 处消费，遵循 Rule of Three；若 #2513 也需要同款判定，再在彼单评估下沉）。
- `40300` 文案/错误码映射（#2504 域）。
- 预生产真实数据验证与真机走查（D5，关闭时 `[Ops-Followup of #2510]`）。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | 站点 `id=siteID, org_id=siteOrg`（`siteID ≠ siteOrg`），乐器 `current_site_id=siteID`，actor `oid=siteOrg` | `bash scripts/verify-2510.sh r1` | `docs/evidence/2510/R1-id-domain.txt` | `OLD_PREDICATE_MATCH=0`、`NEW_RULE_MATCH=1`、`RED_CONFIRMED=1` | `OLD_PREDICATE_MATCH=1` → 夹具未区分 ID 域（红例不成立）→ FAIL |
| R2 | 同上 + 可租乐器、无未结丢失记录 | `bash scripts/verify-2510.sh r2` | `docs/evidence/2510/R2-register.txt` | `STATUS=200`、`CODE=20000`、`LOSS_RECORD_CREATED=1`、`INSTRUMENT_LOST=1` | `STATUS=403` → 网点账号仍被拒 → FAIL |
| R3 | 已存在未结丢失记录 + 乐器 `stock_status=lost` | `bash scripts/verify-2510.sh r3` | `docs/evidence/2510/R3-restore.txt` | `STATUS=200`、`RESTORED_DAMAGED=0`、`INSTRUMENT_AVAILABLE=1` | `STATUS=403` → FAIL |
| R4 | 乐器 `current_site_id` = 他组织站点（`otherOrg ≠ siteOrg`） | `bash scripts/verify-2510.sh r4` | `docs/evidence/2510/R4-cross-org.txt` | `LOST_STATUS=403`、`RESTORE_STATUS=403`、`LEAK_RECORDS=0`、`STOCK_STATUS_UNCHANGED=1` | 任一 `200` 或 `LEAK_RECORDS>0` → 跨组织越权 → FAIL |
| R5 | actor `oid = siteID`（旧错误口径），站点 `org_id=siteOrg` | `bash scripts/verify-2510.sh r5` | `docs/evidence/2510/R5-old-behavior-gone.txt` | `OLD_OID_LOST_STATUS=403`、`OLD_OID_RESTORE_STATUS=403`、`OLD_OID_ALLOWED=0` | `OLD_OID_ALLOWED=1` → 仍按字符串相等放行（旧行为未消失）→ FAIL |
| R6 | 既有 `TestInstrumentLoss*` 全量 | `bash scripts/verify-2510.sh r6` | `docs/evidence/2510/R6-regression.txt` | `REGRESSION_PASS=1`、`FAIL_COUNT=0`、`TESTS_RUN>=6` | 任一 `--- FAIL` → FAIL |
| R7 | 修后工作树 | `bash scripts/verify-2510.sh r7` | `docs/evidence/2510/R7-build-scan.txt` | `BUILD_RC=0`、`OID_SITE_STRING_COMPARE=0`、`LIST_SCOPE_OID=0` | `BUILD_RC=1` 或 `OID_SITE_STRING_COMPARE=1` → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2510.sh r1` | `docs/evidence/2510/R1-id-domain.txt` | exit 0；grep `OLD_PREDICATE_MATCH=0`、`NEW_RULE_MATCH=1`、`RED_CONFIRMED=1` | PASS | `OLD_PREDICATE_MATCH=1` → FAIL | dev |
| R2 | `bash scripts/verify-2510.sh r2` | `docs/evidence/2510/R2-register.txt` | exit 0；grep `STATUS=200`、`LOSS_RECORD_CREATED=1`、`INSTRUMENT_LOST=1` | PASS | `STATUS=403` → FAIL | dev |
| R3 | `bash scripts/verify-2510.sh r3` | `docs/evidence/2510/R3-restore.txt` | exit 0；grep `STATUS=200`、`RESTORED_DAMAGED=0`、`INSTRUMENT_AVAILABLE=1` | PASS | `STATUS=403` → FAIL | dev |
| R4 | `bash scripts/verify-2510.sh r4` | `docs/evidence/2510/R4-cross-org.txt` | exit 0；grep `LOST_STATUS=403`、`RESTORE_STATUS=403`、`LEAK_RECORDS=0` | PASS | `LOST_STATUS=200` 或 `LEAK_RECORDS=1` → FAIL | dev |
| R5 | `bash scripts/verify-2510.sh r5` | `docs/evidence/2510/R5-old-behavior-gone.txt` | exit 0；grep `OLD_OID_ALLOWED=0`、`OLD_OID_LOST_STATUS=403`、`OLD_OID_RESTORE_STATUS=403` | PASS | `OLD_OID_ALLOWED=1` → FAIL | dev |
| R6 | `bash scripts/verify-2510.sh r6` | `docs/evidence/2510/R6-regression.txt` | exit 0；grep `REGRESSION_PASS=1`、`FAIL_COUNT=0` | PASS | `FAIL_COUNT=1` → FAIL | dev |
| R7 | `bash scripts/verify-2510.sh r7` | `docs/evidence/2510/R7-build-scan.txt` | exit 0；grep `BUILD_RC=0`、`OID_SITE_STRING_COMPARE=0`、`LIST_SCOPE_OID=0` | PASS | `BUILD_RC=1` 或 `OID_SITE_STRING_COMPARE=1` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2510/R1-id-domain.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2510/R2-register.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2510/R3-restore.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2510/R4-cross-org.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2510/R5-old-behavior-gone.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2510/R6-regression.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2510/R7-build-scan.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 symptom | 触发条件 trigger | 根因 root cause | 测试盲区 test blind spot | 同类路径 same-class | 修复层 fix layer | 红绿 red-green | 预防 prevention |
|---|---|---|---|---|---|---|---|---|
| B1 | 网点账号登记丢失/找回恒 403，功能不可用 | `sites.id ≠ sites.org_id` 且 `current_site_id` 有值 | `loadLossInstrument` 字符串比较 `current_site_id` 与 `oid` | 夹具 `org_id = tenant_id`、actor `oid = siteID`（两域同值） | `warehouse.go:697/872`、`user_rental.go:437/1044`、`repair_service.go:1832/1883/336`（#2509 子任务） | handler 归属判定（集合成员） | R1（旧为 0）→ R2/R3（新为 200） | 夹具强制 `id ≠ org_id`（D1 + 语义变更表） |
| B2 | 拒绝与「不存在」不可区分，排障无据 | 任一拒绝分支 | 三个分支共用同一返回、无日志 | 无日志断言 | `personnel`（#2427 D3 已分 ERROR） | handler 可观测性 | R4/R5 走拒绝路径并落 WARN（R7 扫描确认） | 拒绝分支必须留痕（D2） |
| B3 | 测试夹具混淆 ID 域，缺陷长期绿灯 | 使用 `SeedSiteFK` + `OrgID: siteID` 的套件 | shared helper 用 `tenant_id` 充当 `org_id` 图省事 | 全套 loss 用例 | `personnel_2065_test.go`、`SeedSiteFK` 其他消费者 | 测试夹具（本单局部） | R1 红 → R2 绿 | D3：不盲改共享 helper；新用例必须显式 `org_id` |
| B4 | 修复方向搞错：改用 `instruments.org_id` 判定 | 网点级乐器由商户级持有（`org_id = tenant`） | 想当然把「组织一致」等价于「乐器 org_id 一致」 | 无「商户级持有 + 网点在库」用例 | 与 #2512（orders.org_id 回退）同族 | 判定语义 | R2 使用 `OrgID: tenant` 的既有夹具形态 → 若按 org_id 判定将 FAIL | D1 落字 + 夹具保留 `OrgID: tenant` |
| B5 | 空站点乐器被新检查误拒 | `current_site_id IS NULL` | 新判定未处理 nil | 无 nil 站点用例 | — | handler 判定分支 | R6 回归覆盖既有纯库存用例 | D1：nil → 只做租户校验 |
| B6 | 查询失败被当成「无权限」而放行/拒绝不明 | `sites` 查询报错 | 错误分支处理缺失 | 无 DB 错误注入用例 | `personnel` #2427 D3 | handler 错误处理 | R7 静态确认无 `_ =` 吞错 | fail-closed + ERROR 日志（D2 同族） |
| B7 | `List` 跨网点可见（网点账号看他店丢失记录） | site 账号调用 `GET /instrument-loss` | `List` 只按 `tenant_id` 过滤，未消费 `oid` | 无跨网点 List 用例 | `personnel`（本单主诉的同族可见性） | 本单不修（D4） | 不在本单（#2514 承载红绿） | 另立 Issue 落地，不口头延期 |
| B8 | 修完一个角色仍坏另一个（site_member vs site_admin） | 两角色均持 `instrument:update` | 只测一个角色 | 无角色矩阵用例 | `role_templates.go:49,63,69` | 测试夹具覆盖 | R2/R3 用 staff 角色（site_member），R6 覆盖既有 admin 路径 | 角色矩阵写入矩阵行（R2/R3 注明角色） |
| B9 | 语义变了但既有夹具注释/期望照旧 | 只改代码不改夹具 | 语义变更清单缺失 | 旧夹具固化错误契约 | `instrument_loss_test.go:44-61` | 同批翻转（本计划语义变更表） | R6 回归 + 语义变更表逐条核对 | 语义变更表强制（AGENTS.md） |

### Workload

- Size: S
- Basis: 1 个生产文件（`instrument_loss.go`，约 15 行改动）+ 1 个测试文件（夹具 3 处 + 4 个新用例）+ 1 个证据脚本；无 migration、无前端、无新依赖
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/instrument_loss.go`
- `backend/handlers/instrument_loss_test.go`
- `scripts/verify-2510.sh`
- `docs/evidence/2510/plan.md`
- `docs/evidence/2510/evidence.json`
- `docs/evidence/2510/pre-ready-2510.json`
- `docs/evidence/2510/R1-id-domain.txt`
- `docs/evidence/2510/R2-register.txt`
- `docs/evidence/2510/R3-restore.txt`
- `docs/evidence/2510/R4-cross-org.txt`
- `docs/evidence/2510/R5-old-behavior-gone.txt`
- `docs/evidence/2510/R6-regression.txt`
- `docs/evidence/2510/R7-build-scan.txt`

## Core Update

无需修改核心文档：`docs/cases/instrument-loss.md:93` 已规定「网点员工可做丢失登记/找回」，当前代码是**违背**该规格，本单使实现回归文档口径（非规格演进）。

## Dependencies

- 无

### 关联（非硬依赖）

- Parent #2509：同族 ID 域审计（本单为 S2）；`#2511`/`#2512`/`#2513` 为其余子任务，互无硬依赖。
- #2508：确立「夹具必须 `sites.id ≠ sites.org_id`」口径（本单沿用）。
- #2514：`List` 跨网点可见性（D4 转出）。
- 部署侧：预生产网点账号登记丢失/找回的真机回执 → 关闭时 `[Ops-Followup of #2510]`（仅 `waitForConfirm`）。

*Model: DeepSeek V4.1 Flash 1017*

