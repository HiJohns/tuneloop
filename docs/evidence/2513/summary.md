# Work Summary — #2513 S5 维修服务 oid ↔ site_id 误用修复

- Fix commit（Round 1）: d7a3cecc2（`fix(issue-2513): repair service drops the oid/site_id ID-domain misuse`）
- Fix commit（Round 2，当前）: 27d9cf29e（`fix(issue-2513): align R7 assertion with the gate report schema`）
- Evidence artifacts（R1–R6）: 27d9cf29e（首行 provenance 由 `scripts/verify-2513.sh ledger` 锚定）
- Evidence ledger/gate/summary: 本摘要所在提交（`git log -1 --format=%H -- docs/evidence/2513/`）；分支 `origin/issue-2513`
- 涉及文件：backend/handlers/repair_service.go、backend/handlers/repair_service_2513_test.go（新增）、scripts/verify-2513.sh（新增/扩展 ledger）、docs/cases/repair-service.md

## Round 1 审计修复（Round 2 交付）

| 审计项 | 修复 | 证据 |
|---|---|---|
| Bug 1（Medium）R7 断言字段不存在（`.result` / `GATE_RESULT`） | R7 断言改为 `jq -e '.overall == "PASS"'`（gate 报告真实顶层键）；`actual` = `overall=PASS`；账本锚点改由 `bash scripts/verify-2513.sh ledger` 从最新 `fix(issue-2513)` commit 派生并写回，取消手工改 `build/commit` | `docs/evidence/2513/evidence.json`（R7 行）；`bash scripts/verify-2513.sh ledger` → `ARTIFACT_PROVENANCE=OK`、`LEDGER_ROWS=7/7`、`GATE_OVERALL=PASS`、`LEDGER_OK`；计划 Rev.2 勘误（断言文本对齐 schema，验收语义/工具未变） |
| Bug 2（Low）RS-05 响应形态描述与实现不符 | `docs/cases/repair-service.md` RS-05 改为「两者均为 `data.{list,total}`（`total = len(list)`、无分页游标）；差异仅在入参与上限（pending 固定 `done_repair`、无 200 上限；`scope=site` 支持 `status/start/end/sort`、`Limit(200)`、回填 `user_name`）」 | `docs/cases/repair-service.md:141-142`（fix commit diff） |
| Bug 3（Low）provenance 指针停在中间提交 | 摘要改为按角色列出：fix commit / artifacts / ledger+gate+summary（本摘要所在提交）；Issue 评论同步 | 本节头部指针 + `## Remote` |
| （建议，非阻断）R1 仅角色分支正向 | R1 增加商户直属员工（`Role=STAFF` + `oid=商户 org`）用例，覆盖 `repairServiceMerchantLevel` 的 DB 分支 → `MERCHANT_STAFF_ROWS=2` | `docs/evidence/2513/R1-pending-dispatch.txt`（`MERCHANT_STAFF_ROWS=2`）、计划 Rev.2 R1 行 |

## 修改内容

1. **新增唯一闸门** repairServiceMerchantLevel(db, ctx)：平台级业务角色（system_admin / platform_staff / merchant_admin）→ true；否则 merchants.org_id = oid 命中 → true；纯网点账号（oid 是网点组织/空）→ false。ListTasks 的既有内联判定抽出复用。
2. **ListPendingDispatch**：删除 site_id = oid（ID 域误用，恒空），改为与 scope=site&status=done_repair 同口径（RS-05）——闸门 + tenant_id IN repairServiceTenantScope，无 site 过滤。
3. **ListTasks scope=site**：删除 site_id 相等/为空的旧谓词，历史带 site_id 行不再被漏；闸门改用共享 helper。
4. **repairServiceStaffAllowed**：删除 rr.SiteID != oid 分支；跨商户隔离仍由 repairServiceTenantScope 承担。
5. **文档（Core Update）**：`docs/cases/repair-service.md` RS-API-2 补「site_id 不是可见性维度」、RS-05 补「pending-dispatch 与 scope=site 同口径（#2513）」。
6. **测试**：backend/handlers/repair_service_2513_test.go，夹具 sites.id ≠ sites.org_id ≠ merchants.org_id，4 用例覆盖 R1–R4（含旧谓词红证据计数与负例）。

## 验证层级

- 本单为**子任务**（Parent: #2509），只跑**定向**套件，不跑全量 go test ./...：
  - go build ./...（backend）→ BUILD_RC=0
  - go vet ./handlers/ → 无输出
  - 定向回归 ^(TestRepairService|TestListTasks|TestRepair) → PASS_COUNT=41、FAIL_COUNT=0
- 全量 import 与前端 gate 由父批次 #2509 承担（本单无前端改动）。

## 红绿实证（缺陷版 FAIL → 修复版 PASS）

复现方式：git checkout -- backend/handlers/repair_service.go（保留新测试）后运行 3 个正向用例：

```text
--- FAIL: TestRepairService2513_PendingDispatchMerchantSeesLegacy
    MERCHANT_ROWS=0      ← 旧谓词 site_id = oid 恒空
--- FAIL: TestRepairService2513_ListTasksSiteIncludesLegacy
    SITE_SCOPE_ROWS=1    ← 旧谓词漏掉历史带 site_id 行
--- FAIL: TestRepairService2513_StaffAllowedLegacySiteRow
    ALLOWED_LEGACY=0     ← 旧分支 rr.SiteID != oid 恒拒
FAIL	tuneloop-backend/handlers	4.517s
```

修复版（本提交）同一命令：

```text
MERCHANT_ROWS=2      MERCHANT_STAFF_ROWS=2   LEGACY_VISIBLE=1   OLD_SITE_EQ_OID_ROWS=0
SITE_SCOPE_ROWS=2    OLD_FILTER_ROWS=1
ALLOWED_LEGACY=1     QUOTE_STATUS=200   OLD_DIRECT_OID_COMPARE=0
OTHER_TENANT_ROWS=0  OTHER_TENANT_ALLOWED=0  SITE_STAFF_ROWS=0  SITE_STAFF_PENDING_ROWS=0
PASS
```

## Accepted Plan 对照（6/6 变更点 + 1 文档）

| 计划项 | 状态 | 证据 |
|---|---|---|
| 变更点 1 ListPendingDispatch 同口径 | PASS | docs/evidence/2513/R1-pending-dispatch.txt |
| 变更点 2 新增 repairServiceMerchantLevel | PASS | docs/evidence/2513/R6-build-scan.txt（MERCHANT_GATE_SHARED=1，2 处调用） |
| 变更点 3 ListTasks scope=site 去 site 谓词 | PASS | docs/evidence/2513/R2-list-tasks-site.txt、docs/evidence/2513/R6-build-scan.txt |
| 变更点 4 repairServiceStaffAllowed 去 ID 域比较 | PASS | docs/evidence/2513/R3-staff-allowed.txt |
| 变更点 5 测试 4 用例 | PASS | docs/evidence/2513/R1-pending-dispatch.txt / R2 / R3 / R4 |
| 变更点 6 文档 Core Update | PASS | docs/cases/repair-service.md（见 fix commit diff） |
| 负例（跨租户/网点账号） | PASS | docs/evidence/2513/R4-negative.txt |

## 矩阵 n/n（7/7）

| 变更点 | 状态 | 证据路径 |
|---|---|---|
| R1 待发回清单：商户层级见历史带 site 行 | PASS | docs/evidence/2513/R1-pending-dispatch.txt |
| R2 任务列表 scope=site 不再漏历史行 | PASS | docs/evidence/2513/R2-list-tasks-site.txt |
| R3 单条守卫：历史行可操作 | PASS | docs/evidence/2513/R3-staff-allowed.txt |
| R4 负例：跨租户 / 网点账号口径 | PASS | docs/evidence/2513/R4-negative.txt |
| R5 既有维修服务套件回归 | PASS | docs/evidence/2513/R5-regression.txt |
| R6 构建 + 同类扫描 | PASS | docs/evidence/2513/R6-build-scan.txt |
| R7 pre-ready gate（四源 + 定向测试 + 构建） | PASS（overall=PASS） | docs/evidence/2513/pre-ready-2513.json |

## Evidence Contract（7/7，见 docs/evidence/2513/evidence.json）

| 行 | 命令 | 结果 | 产物 |
|---|---|---|---|
| R1 | `bash scripts/verify-2513.sh r1` | PASS（R1_OK、MERCHANT_ROWS=2、MERCHANT_STAFF_ROWS=2） | docs/evidence/2513/R1-pending-dispatch.txt |
| R2 | `bash scripts/verify-2513.sh r2` | PASS（R2_OK、SITE_SCOPE_ROWS=2） | docs/evidence/2513/R2-list-tasks-site.txt |
| R3 | `bash scripts/verify-2513.sh r3` | PASS（R3_OK、ALLOWED_LEGACY=1） | docs/evidence/2513/R3-staff-allowed.txt |
| R4 | `bash scripts/verify-2513.sh r4` | PASS（R4_OK、OTHER_TENANT_ROWS=0） | docs/evidence/2513/R4-negative.txt |
| R5 | `bash scripts/verify-2513.sh r5` | PASS（R5_OK、FAIL_COUNT=0） | docs/evidence/2513/R5-regression.txt |
| R6 | `bash scripts/verify-2513.sh r6` | PASS（R6_OK、BUILD_RC=0） | docs/evidence/2513/R6-build-scan.txt |
| R7 | pre-ready-check.sh 2513（含 --summary-file / --changed-files / --evidence-file / --report） | PASS（`jq -e '.overall == "PASS"'` 可执行且通过；bootstrap 后重跑 gate PASS） | docs/evidence/2513/pre-ready-2513.json |

## G0 Golden Acceptance

| 行 | artifact | assertions（含负例） |
|---|---|---|
| R1 | docs/evidence/2513/R1-pending-dispatch.txt | MERCHANT_ROWS=2、MERCHANT_STAFF_ROWS=2（STAFF 走 DB 分支）、LEGACY_VISIBLE=1、OLD_SITE_EQ_OID_ROWS=0；负例：仍 0 行 → R1_FAIL |
| R2 | docs/evidence/2513/R2-list-tasks-site.txt | SITE_SCOPE_ROWS=2、OLD_FILTER_ROWS=1；负例：仍 1 行 → R2_FAIL |
| R3 | docs/evidence/2513/R3-staff-allowed.txt | ALLOWED_LEGACY=1、QUOTE_STATUS=200、OLD_DIRECT_OID_COMPARE=0；负例：仍 403 → R3_FAIL |
| R4 | docs/evidence/2513/R4-negative.txt | OTHER_TENANT_ROWS=0、OTHER_TENANT_ALLOWED=0、SITE_STAFF_ROWS=0、SITE_STAFF_PENDING_ROWS=0；负例：>0 → R4_FAIL |
| R5 | docs/evidence/2513/R5-regression.txt | REGRESSION_PASS=1、FAIL_COUNT=0、PASS_COUNT=41；负例：任一 FAIL → 非 0 |
| R6 | docs/evidence/2513/R6-build-scan.txt | BUILD_RC=0、SITE_ID_EQ_ORG=0、SITE_FILTER_IN_SERVICE=0、MERCHANT_GATE_SHARED=1；负例：残留 → R6_FAIL |

## 证据来源

| session / 命令 | repo | build_version | fix commit | artifact |
|---|---|---|---|---|
| bash scripts/verify-2513.sh r1..r6 | tuneloop | 1.0.27d9cf29（首行 provenance） | 27d9cf29e | docs/evidence/2513/R1-pending-dispatch.txt 等 6 份 |
| bash scripts/verify-2513.sh ledger | tuneloop | 1.0.27d9cf29 | 27d9cf29e | docs/evidence/2513/evidence.json（build/commit/dirty 派生，非手改） |

- 证据均在 **fix commit 之后**、干净树上重新生成（git status --porcelain --untracked-files=no 为空 → dirty=false），无手工修改的 artifact。

## 端到端验收（生产入口 replay）

- 入口：真实 Gin 路由 GET /repair-services/pending-dispatch、GET /repair-services?scope=site、POST /repair-services/:id/quote（httptest 打到 NewRepairServiceHandler()，走 loadRepairService + repairServiceStaffAllowed + repairServiceTenantScope 全链路），DB 为真实 Postgres 测试库（非 sqlite）。
- 行级断言：rrLegacy（site_id=aID）在两端点均出现（LEGACY_VISIBLE=1）；quote 对 rrQuote 返回 20000 且状态由 pending_quote 转移（用例断言 20000 + 落库更新）。
- 负例：他租户商户 0 行 / false；网点账号 0 行（#2122 保持）。

## Dependency Order / 验收规则

- 前置层：父任务 #2509（分批集成，本单不依赖其它子任务代码）；计划 Dependencies = 无。
- 本单**未**修改任何验收工具/规则（scripts/verify-2513.sh 为本单新增的证据生成器，不参与他人验收）。
- 未引入延期项；Out of Scope 项（响应形态统一、历史数据清理、生产抽样 D5）按计划保留，无新增待办。

## Remote

- remote: origin/issue-2513; fix=27d9cf29e pushed=yes（历史 fix=d7a3cecc2）；evidence artifacts=27d9cf29e 锚点；ledger/gate/summary=本摘要所在提交（见 `git log -1 --format=%H -- docs/evidence/2513/`）

## Pre-ready gate

- 报告：docs/evidence/2513/pre-ready-2513.json
- 结果：`RESULT: PASS`（关键输出如下）。

```text
$ ~/.config/opencode/scripts/pre-ready-check.sh 2513 --summary-file docs/evidence/2513/summary.md \
    --changed-files docs/evidence/2513/changed-files.txt \
    --evidence-file docs/evidence/2513/evidence.json \
    --affected-cmd "bash scripts/verify-2513.sh r5" \
    --full-gate-cmd "bash scripts/verify-2513.sh gate" --gate-lock-scope host \
    --report docs/evidence/2513/pre-ready-2513.json
P1 PASS  P2 PASS  P3 PASS  P4 PASS  P12 PASS  P14 PASS  P17 PASS
P5 N/A (sub-task: full gate belongs to the parent/batch gate)  P6 PASS  P11 PASS  P13 PASS
P15/P16/P7/P8/P9/P10 N/A
P18 WARN tooling_commit missing (legacy bundle)
REPORT: docs/evidence/2513/pre-ready-2513.json
RESULT: PASS
```

*Model: DeepSeek V4.1 Flash 1017*