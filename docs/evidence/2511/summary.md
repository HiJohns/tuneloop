# Work Summary — #2511 S3：current_site_id 回退改为「order.OrgID → sites.id 反查」

Parent: #2509

## 修改内容

| # | 改动 | 文件 |
|---|------|------|
| 1 | 新增唯一解析函数 `resolveSiteIDByOrg(db, orgID)`：单站点直取；多站点优先锚点（sites.org_id == sites.tenant_id），否则 created_at 最早；0 个/查询失败 → ""（不写入 + WARN）。永不返回组织 ID | `backend/handlers/warehouse.go` |
| 2 | InspectReturn（定损入库）与 AssessDamage（定损评估）两处回退改为调用上述解析函数；无法解析时不写入（保留原值）并打印含 instrument id 与 org 值的 WARN；主分支（操作者 site_members.site_id）语义不变 | `backend/handlers/warehouse.go` |
| 3 | 新增 4 个用例：两处回退写站点 ID、不可解析保留原值、解析确定性（锚点/最早/未知）+ 主分支不变 | `backend/handlers/warehouse_2511_test.go` |
| 4 | 证据生成器 r1–r6 + gate（首行 provenance；r5 输出**只读**存量排查 SQL，不执行） | `scripts/verify-2511.sh` |
| 5 | Core Update：ID 域口径补「instruments.current_site_id 合法来源」与反查规则 | `docs/cases/cases.md` |

**Commit**：fix 220785f52 → evidence（本次）

## 验证层级

- 子任务（`Parent: #2509`）：只跑受影响包定向命令——`go build ./...` + `go test ./handlers -run 'TestInspectReturn2511|TestAssessDamage2511|TestResolveSiteIDByOrg2511|TestResolveOperatorSiteID|TestInspectReturn_|TestAssessDamage_|TestSubmitAssessment_'`（R6）；全量 gate 属父任务 #2509（本单不在子任务里重复 `go test ./...`）。
- 前端无改动。

## Remote

remote: origin/issue-2511@<evidence-commit>; fix=220785f52 pushed=yes; evidence=<evidence-commit> pushed=yes

## 红绿实证

**R1（回退写站点 ID）—— 缺陷版 FAIL（HTTP 500 + 外键违例）→ 修复版 PASS**

缺陷版（fix commit 父提交 1bd6b3e5a，临时 worktree，仅拷入 R1/R2/R3 用例，接同一测试库）：

```
=== RUN   TestInspectReturn2511_FallbackWritesSiteNotOrg
    warehouse_2511_test.go:135:
        	Error:      	Not equal:
        	            	expected: 200
        	            	actual  : 500
        	Messages:   	{"code":50000,"message":"failed to update instrument status: ERROR: insert or update on table \"instruments\" violates foreign key constraint \"instruments_current_site_id_fkey\" (SQLSTATE 23503)"}
--- FAIL: TestInspectReturn2511_FallbackWritesSiteNotOrg (1.46s)
``

修复版（220785f52）：docs/evidence/2511/R1-inspect-return.txt → CURRENT_SITE_ID=aID、OLD_FALLBACK_IS_ORG=1（旧逻辑写 org 的取证标记）、RED_CONFIRMED=1、SITE_ID_IS=aID、R1_OK，且 `--- PASS`。

> 说明：缺陷版不仅「值不对」，还会因 instruments_current_site_id_fkey 直接 500 —— 说明该路径在真实库上会让「定损入库」整步失败（比原 Issue 描述的「僵尸状态」更严重），修复同时消除了这条 500。

## Accepted Plan 对照（8/8）

1. 新增 `resolveSiteIDByOrg`（策略：单站点 / 锚点优先 / 最早 / 空）→ 完成
2. `InspectReturn` 回退改反查，不写入时 WARN → 完成
3. `AssessDamage` 同构替换 → 完成
4. 主分支语义不变、其它 `current_site_id` 消费方不动 → 完成
5. 测试夹具强制 `sites.id ≠ sites.org_id`，5 类断言（两回退 / 不可解析 / 确定性 / 主分支）→ 完成
6. `docs/cases/cases.md` ID 域补合法来源 → 完成
7. `scripts/verify-2511.sh` r1–r6 + gate，r5 只读 SQL → 完成
8. 不做：存量数据修复（只出只读 SQL）、assessment handler（登记 deferral，见下）→ 完成

## Evidence Contract

| 行 | 结果 | 产物 |
|---|------|------|
| R1 | PASS → `CURRENT_SITE_ID=aID、OLD_FALLBACK_IS_ORG=1、RED_CONFIRMED=1、SITE_ID_IS=aID、R1_OK` | `docs/evidence/2511/R1-inspect-return.txt` |
| R2 | PASS → `CURRENT_SITE_ID=aID、NOT_ORG=1、R2_OK` | `docs/evidence/2511/R2-assess-damage.txt` |
| R3 | PASS → `KEPT_ORIGINAL=1、NO_CROSS_DOMAIN_WRITE=1、CURRENT_SITE_ID=aID、R3_OK` | `docs/evidence/2511/R3-unresolvable.txt` |
| R4 | PASS → `ANCHOR_PREFERRED=1、EARLIEST_FALLBACK=1、EMPTY_FOR_UNKNOWN=1、PRIMARY_EQUALS_OPERATOR_SITE=1、PRIMARY_NOT_ORG=1、R4_OK` | `docs/evidence/2511/R4-deterministic.txt` |
| R5 | PASS → `ORG_WRITTEN_TO_SITE=0、RESOLVER_CALLS=2、RESOLVER_DEF=1、SQL_PRESENT=1、R5_OK`（另打印只读 SQL 文本） | `docs/evidence/2511/R5-scan-sql.txt` |
| R6 | PASS → `BUILD_RC=0、REG_RC=0、R6_OK` | `docs/evidence/2511/R6-build-regression.txt` |
| R7 | PASS → gate `overall=PASS` | `docs/evidence/2511/pre-ready-2511.json` |

机器可判定记录：`docs/evidence/2511/evidence.json`（7 行，全部 pass=true，dirty=false，build 含 fix commit 短码 220785f52）。

## G0 Golden Acceptance

| 行 | artifact | assertions | 负例（未被触发） |
|---|----------|-----------|------------------|
| R1 | `docs/evidence/2511/R1-inspect-return.txt` | CURRENT_SITE_ID=aID／SITE_ID_IS=aID／OLD_FALLBACK_IS_ORG=1／RED_CONFIRMED=1 | 写入 aOrg → require.NotEqual 失败 → R1_FAIL |
| R2 | `docs/evidence/2511/R2-assess-damage.txt` | CURRENT_SITE_ID=aID／NOT_ORG=1 | 写入 aOrg → R2_FAIL |
| R3 | `docs/evidence/2511/R3-unresolvable.txt` | KEPT_ORIGINAL=1／NO_CROSS_DOMAIN_WRITE=1／CURRENT_SITE_ID=aID | 被覆盖为 org 或 NULL → R3_FAIL |
| R4 | `docs/evidence/2511/R4-deterministic.txt` | ANCHOR_PREFERRED=1／EARLIEST_FALLBACK=1／EMPTY_FOR_UNKNOWN=1／PRIMARY_EQUALS_OPERATOR_SITE=1／PRIMARY_NOT_ORG=1 | 主分支被 org 覆盖或解析不确定 → R4_FAIL |
| R5 | `docs/evidence/2511/R5-scan-sql.txt` | ORG_WRITTEN_TO_SITE=0／RESOLVER_CALLS=2／SQL_PRESENT=1 | 残留 `current_site_id"] = order.OrgID` → R5_FAIL |
| R6 | `docs/evidence/2511/R6-build-regression.txt` | BUILD_RC=0／REG_RC=0 | 任一既有用例失败 → R6_FAIL |
| R7 | `docs/evidence/2511/pre-ready-2511.json` | overall=PASS、每行 pass=true、dirty=false、P13 在 origin/issue-2511 | 任一 P 失败 / evidence 缺行 → gate 非 0 |

### 契约三向核对

| 维度 | 内容 | 一致性 |
|---|---|---|
| Plan / API | 计划要求回退不得写 org、必须反查 sites.id，不可解析保留原值 + WARN | warehouse.go 两处调用同一 `resolveSiteIDByOrg`；R1/R2/R3 实测覆盖 |
| docs | `docs/cases/cases.md` 补 ID 域合法来源与反查策略（单站点/锚点/最早/空） | 与 `resolveSiteIDByOrg` 实现逐条对应 |
| handler / frontend | 无前端改动；写点唯一（warehouse 两处） | R5 断言 `ORG_WRITTEN_TO_SITE=0、RESOLVER_CALLS=2、RESOLVER_DEF=1` |

## Dependency Order / 验收规则

- 父任务 #2509；无前置硬依赖。同族 S2/S4/S5 已各自 accepted，本单只改 warehouse 两处，未触碰它们。
- 本单未修改任何既有验收工具/判据；`scripts/verify-2511.sh` 为本 Issue 新增证据脚本。
- **deferral**：backend/handlers/assessment.go:135（SubmitAssessment）存在**同一写法**（R5 `ASSESSMENT_SAME_CLASS_HITS=1`）。按计划 D4 本单不修，已当天另立 **#2532**（bug + status:todo，含红绿与扫描要求）并回链本单。
- 存量 `current_site_id` 域污染：按 D3 只交付**只读**排查 SQL（R5 artifact 内），不执行任何数据写入。

## 矩阵 n/n

| 矩阵行 | 状态 | 证据 |
|---|---|---|
| R1 定损入库回退写站点 ID（红绿） | PASS | `docs/evidence/2511/R1-inspect-return.txt` |
| R2 定损评估回退写站点 ID | PASS | `docs/evidence/2511/R2-assess-damage.txt` |
| R3 不可解析 → 保留原值（负例） | PASS | `docs/evidence/2511/R3-unresolvable.txt` |
| R4 多站点确定性 + 主分支不变（负例） | PASS | `docs/evidence/2511/R4-deterministic.txt` |
| R5 同类扫描 + 只读存量 SQL | PASS | `docs/evidence/2511/R5-scan-sql.txt` |
| R6 构建 + 定向回归 | PASS | `docs/evidence/2511/R6-build-regression.txt` |
| R7 pre-ready gate（四源 + 定向测试 + 构建） | PASS | `docs/evidence/2511/pre-ready-2511.json` |

七行全 PASS，无 N/A 行。

## 证据真实性

| artifact | 生成时 HEAD | dirty | 机制 |
|---|---|---|---|
| R1–R4 | 220785f52 | false | 真实 Postgres 测试库（testfixtures.SetupTestDB），真实 handler 路由（`PUT /warehouse/orders/:id/return-inspect、/damage`） |
| R5 | 220785f52 | false | 文本级结构断言（写点清零 + 解析函数被两处调用 + 只读 SQL 文本） |
| R6 | 220785f52 | false | 重命令经 heavy-run（host 锁）：`go build ./...` + 定向回归 |
| R7 | 220785f52 | false | pre-ready gate（P12/P14 校验 evidence 与产物） |

- `p5 说明`：pre-ready 的 P5 在**洁净 worktree** 内运行（`git worktree add`），该目录没有机器本地 `backend/.env`，DB 相关用例在 HEAD 与 baseline 上表现一致，故 P5 结果为「HEAD 失败集 ⊆ baseline」或 WARN；后端权威验证由 affected-cmd（R6，真实 worktree）承担。
- 无跨项目证据引用，故不需要额外的证据来源表与第三方审计汇总段。

## 人工 / 部署侧

| 项 | 责任人 | 回执要求 |
|---|---|---|
| 存量 `current_site_id` 域污染排查与修复 | Owner / Ops | 用 R5 artifact 的只读 SQL 统计行数；修复需 Owner 授权（AI 不写库） |
| #2532（assessment.go 同族缺陷）排期 | Architect / Owner | 另立 Issue 已建，待进入流水线 |
