## Accepted Plan — #2508 网点管理员人员列表恒为空（`oid` ↔ `site_id` ID 域纠偏）

> Tags: bug, code:go, api-contract
> 关联：#2427（查询口径 / 空结果与失败区分）、#2447（同页面入口）、#2504（同页面 40303 文案）、#2065（三视图）、#2067（membership 锚定）、#2509（同类 ID 域缺陷，另单）

### 背景与现状核对（代码为据）

| 事实 | 位置 |
|---|---|
| `personnel.List` 的 site_admin 分支把 **JWT `oid`（组织 ID）** 传给 `personnelSiteView` | `backend/handlers/personnel.go:86` |
| `personnelSiteView` 把该值当作 **`site_members.site_id`（= `sites.id`）** 过滤 | `backend/handlers/personnel.go:219-227` |
| `sites.id`（PK）与 `sites.org_id`（网点自身的 IAM 组织 ID）是不同列；一个 IAM 组织经 `SyncOrganizations` 只创建一个 site 行 | `backend/models/models.go:469-472`；`backend/handlers/iam_proxy.go:758-772`；`docs/cases/cases.md:289` |
| 现有夹具把 `sites.org_id` 写成 `tenant_id`，并让 site actor 的 `oid = site.id`（注释「生产约定：site 级账号 JWT oid == site.id」）→ 缺陷被夹具掩盖 | `backend/handlers/personnel_2065_test.go:89-90,130-132` |
| 契约文档把错误口径写成了规格（`site_members.site_id = JWT oid`） | `docs/spec/api/api.md` §10.18.0（site_admin 行） |
| 「查询失败」已与「空结果」分离（#2427 D3：scan 错误 → `50000` + ERROR 日志） | `backend/handlers/personnel.go:97-103` |
| 同一 `oid` 在正确实现处的用法（对照证据）：`JOIN sites` / 子查询按 `org_id` 解析站点 | `backend/handlers/iam_proxy.go:197`、`backend/handlers/warehouse.go:52-56`、`backend/handlers/personnel.go:282` |

**根因**：ID 域混用。JWT `oid` = `sites.org_id`；`site_members.site_id` = `sites.id`。凡 `sites.id ≠ sites.org_id`（本项目全部网点皆如此），`WHERE sm.site_id = oid` 恒返回 0 行 → site_admin 人员列表恒空、无报错、无日志（静默错误）。

**测试为何没拦住**：夹具同时把 `sites.org_id` 设成 `tenant_id`、把 site actor 的 `oid` 设成 `site.id`，使「组织 ID」与「站点 ID」在测试数据里**同值**，两种谓词无法区分。

### 修复设计（最小、可测、语义显式）

1. `personnel.List`（`backend/handlers/personnel.go:81-86`）：site_admin 分支把 `oid` **与 `siteFilter`** 一起传入；`siteFilter` 的 UUID 校验与 `40303` 守卫保持不变。
2. `personnelSiteView(db, orgID, keyword, siteFilter)`（`personnel.go:219`）：
   - 过滤条件改为 **组织域**：`JOIN sites s ON s.id = sm.site_id WHERE s.org_id = ? AND sm.status = 'active'`；
   - `siteFilter != ""` 时追加 `sm.site_id = ?`（**与本组织求交**：跨组织的网点 ID → 空集，不报错、不泄漏）；
   - `Scan` 错误包上 `org` 上下文返回（不吞，保持 #2427 D3 口径）；
   - 解析出 0 个站点时 `log.Printf("[Personnel] WARN: ...")` 记录 `org`（让「静默 0 条」可观测，便于巡检发现同类配置问题）；
   - 取 `users` 时改用 `database.ExplicitTenantScopeOnly(ctx)`（与 #2067 merchant 视图同口径：**membership 权威**，`users.tenant_id` 不可靠；隔离由 `s.org_id = oid` 保证，因此关闭自动 tenant scope 不降低隔离强度）。
3. **不改** `personnelMerchantView` / `personnelSystemView`（`tid` / `rootOrg` 用法已是正确域，`personnel.go:153`、`:282`）。
4. 语义（写入 api.md）：一个组织下有多站点 → 返回该组织**全部站点**成员；**不递归**子网点（`sites.parent_id` 层级）——若产品要求递归，另立 Issue。

### 语义变更（必须翻转的既有夹具/断言，强制清单）

| 文件:行 | 旧期望 | 新期望 | 理由 |
|---|---|---|---|
| `backend/handlers/personnel_2065_test.go:89` | `Site{ID: siteA, OrgID: f.tid}`（组织 ID 与 tenant 同值） | `Site{ID: siteA, OrgID: f.oid}` | 夹具必须反映生产 ID 域（1 组织 = 1 网点，且 `org_id ≠ id`），否则 #2508 缺陷在测试里不可见 |
| `backend/handlers/personnel_2065_test.go:90` | `Site{ID: siteB, OrgID: f.tid}` | `Site{ID: siteB, OrgID: f.siteBOrg}`（新增独立组织 ID） | 同上；siteB 与 siteA 不同组织，才能验证跨组织隔离 |
| `backend/handlers/personnel_2065_test.go:78` | `mkUser(siteMemberB, ..., f.siteB)`（`users.org_id` = 站点 ID） | `mkUser(siteMemberB, ..., f.siteBOrg)` | 同一口径：`users.org_id` 是组织 ID |
| `backend/handlers/personnel_2065_test.go:130-132` | 注释「生产约定：site 级账号 JWT oid == site.id」；`actor=site` 注入 `OrgID: f.siteA` | 注释改为「site 级账号 JWT `oid` == `sites.org_id`（≠ `sites.id`）」；注入 `OrgID: f.oid` | 被固化的错误契约必须同批纠正（AGENTS.md §语义变更） |
| `docs/spec/api/api.md` §10.18.0 site_admin 行 | 可见范围 = `site_members.site_id = JWT oid` | 可见范围 = `JOIN sites s ON s.id = sm.site_id WHERE s.org_id = JWT oid`，并补 ID 域注 | 文档把缺陷写成规格，必须纠正（Core Update） |

> 既有断言 `TestPersonnel2065_SiteView`（`personnel_2065_test.go:214-224`）的**期望值不变**，但在新夹具下：修复前必 FAIL（红），修复后 PASS（绿）。这是本单的红绿主线。

## Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据（命令/日志） |
|---|---|---|---|---|---|---|
| R1 根因（ID 域）：旧谓词恒空 | `backend/handlers/personnel_site_org_2508_test.go` | `go test ./handlers -run TestPersonnel2508_IDDomainRootCause`（经 `scripts/verify-2508.sh r1`） | 夹具 `sites.id ≠ sites.org_id`，org 下 2 名成员 | backend（Postgres 测试库） | 同一数据下 `BUGGY_ROWS=0`（`sm.site_id = oid`）、`FIXED_ROWS=2`（`s.org_id = oid`），`RED_CONFIRMED=1` | R1（合同 R1） |
| R2 site_admin 列表（主诉） | `backend/handlers/personnel.go` | `GET /api/admin/personnel`（测试路由，actor site_admin，`oid = sites.org_id`） | 本组织 2 名成员（含 1 名 `users.tenant_id` 全零） | backend | `code=20000`、`SITE_VIEW_ROWS=2`、含「网点A成员」与「零租户成员」、不含「网点B成员」「维修张师傅」「平台员工」「他商户成员」 | R2（合同 R2） |
| R3 跨组织隔离负例 | `backend/handlers/personnel.go` | 同上，actor `oid` 分别取 A/B 组织 | 两组织各有成员 | backend | A 视角 `LEAK=0`（无 B 成员/师傅/平台/他租户）、B 视角 `B_VIEW_ROWS=1`（仅本组织） | R3（合同 R3） |
| R4 旧行为消失（负例） | `backend/handlers/personnel.go` | 同上，actor `oid = sites.id`（旧错误口径） | 该站点存在，但其 `org_id ≠ id` | backend | `OLD_KEYING_ROWS=0`（按 `sm.site_id = oid` 取值不再命中）；`STATUS=200` | R4（合同 R4） |
| R5 多站点组织 + `site_id` 交集 | `backend/handlers/personnel.go` | 同上，`site_id=<站点 UUID>` / `site_id=<他组织站点>` | 同一组织下 2 个站点（A、A2），另有 B 组织站点 | backend | 无过滤 `MULTI_ROWS=3`；`site_id=A` → `FILTER_A_ROWS=2`；`site_id=B` → `FILTER_B_ROWS=0`；非法 `site_id=null` 仍 `40002` | R5（合同 R5） |
| R6 既有人员套件回归 | `backend/handlers/personnel_2065_test.go`、`backend/handlers/personnel_filter_2427_test.go` | `go test ./handlers -run TestPersonnel` | 全量人员用例（2065/2067/2072/2427/2508） | backend | `REGRESSION_PASS=1`（0 fail），merchant `direct` 与 `site_id` 过滤仍工作 | R6（合同 R6） |
| R7 构建 + 同类扫描 | `backend/handlers/personnel.go`、`scripts/verify-2508.sh` | `cd backend && go build ./...` + `grep` 扫描 | 修后工作树 | backend | `BUILD_RC=0`；`PERSONNEL_SITE_ID_MATCH=0`（本单已修）；同族其他命中固定为 `S2=1`、`S3=2`、`S4=2`、`S5=2`（已在 #2509 登记） | R7（合同 R7） |

## Input Truth & Tool Preflight

- **Source of truth（口径唯一）**：
  - ID 域：`docs/cases/cases.md:289`（`sites.org_id` = 网点自身的 IAM 组织 ID）+ `backend/models/models.go:469-472`（`ID` 与 `OrgID` 两列）+ `backend/handlers/iam_proxy.go:758-772`（一个 IAM 组织 → 一个 site 行）。
  - 视图语义：#2065（三视图范围）与 #2067（membership 权威，`users.tenant_id` 不可靠）。
  - 空结果 vs 失败：#2427 D3。
- **差异核对（实施前先跑，结果入 R1/R7 artifact）**：
  - `grep -n "personnelSiteView" backend/handlers/*.go` → 恰好 1 个调用点 + 1 个定义（`personnel.go:86`、`:219`）。
  - `grep -n "GetOrgID" backend/handlers/personnel.go` → 只有 `:53`（`oid`）与 `:87`/`:92` 的守卫，确认无第二处 ID 域消费。
  - `grep -rn "site_id = ?" backend/handlers` → 逐条判定「绑定的值来自 `GetOrgID` 还是 `sites.id`」；本单只需确认 `personnel.go:224` 命中且修复后为 0（其余命中清单见 R7 + #2509）。
- **损坏输入拒绝**：`site_id` 非 UUID（含字面 `null`）→ `40002`（沿用 #2427 R3）；`oid` 为空 → `40303`（沿用现有守卫）；`oid` 指向无站点组织 → 空列表 + WARN 日志（不得 500，不得静默无痕迹）。
- **工具盲区（显式声明）**：
  - 测试夹具依赖真实 Postgres（`testfixtures.SetupTestDB` → `database.LoadConfig`）；无测试库时 `TUNELOOP_TEST_ALLOW_SKIP=1` 会 skip —— 因此 `verify-2508.sh` **不得**设置该变量，skip 视为 FAIL。
  - GORM 自动 tenant scope 对 `db.Table("site_members AS sm")`（无 Schema）不注入；隔离由 `s.org_id` 承担，`users` 查询显式关闭自动 scope（D4）。
  - 生产 5 条成员的实际行数无法在 analyze 阶段读取（本机不做 DB 查询）；R2 断言基于夹具的确定行数，预生产 5 条的人工核对按 D5 归部署侧。
- **影响面预检**：`personnelSiteView` 仅被 `personnel.List` 的 site_admin 分支调用；merchant/system 视图零改动；前端 `/staff` 契约（`name`/`site_id`/`direct`/`page`/`page_size`）不变。

## Decision Lock

| ID | 决策项 | 责任人 | 锁定条件 | 未锁定时的处理 |
|---|---|---|---|---|
| D1 | site_admin 可见范围 = **`JOIN sites s ON s.id = sm.site_id WHERE s.org_id = JWT oid`**（组织域），一个组织多站点时返回**全部站点**成员；**不递归** `sites.parent_id` 子网点 | Architect | 本计划锁定（#2508 期望 1/2） | 若产品要求递归子网点 → 回 Architect 另立 Issue（涉及 `getDescendantSiteIDs` 口径） |
| D2 | `site_id` 查询参数对 site_admin **生效**，语义为「与本组织求交」：跨组织站点 ID → 空集（不报错、不泄漏） | Architect | 本计划锁定（#2427「筛选必须生效」同族） | 若要求 site_admin 忽略该参数 → 回 Architect（需同步 api.md） |
| D3 | `direct` 仍为 **merchant 视图专属**参数（api.md §10.18.0 Query 已如此规定）；site 视图不消费。不改 PC 页面控件可见性 | Architect | 本计划锁定 | 若 Owner 要求按角色隐藏「只看直属/网点」控件 → 另立前端 Issue（不在本单） |
| D4 | site 视图取 `users` 关闭自动 tenant scope（`ExplicitTenantScopeOnly`），与 #2067 merchant 视图同口径；隔离由 `s.org_id = oid` + `sm.status='active'` 成员关系承担 | Architect | 本计划锁定 | 审计若要求保留自动 scope → 必须同时给出「零租户成员不得丢」的替代方案（拒绝则两项冲突） |
| D5 | 预生产「5 条成员」真机/接口核对由部署侧回执承担（Developer 无生产访问权） | Architect / ops | 关闭时创建 `[Ops-Followup of #2508]`（仅 `waitForConfirm` 标签，无 `status:*`） | 未回执不阻断代码验收，但不得当作已验收 |
| D6 | 本单**不含** `oid ↔ site_id` 其他 4 处同类缺陷（`instrument_loss` / `warehouse` / `user_rental` / `repair_service`），已在 #2509 登记为独立任务 | Architect | 本计划锁定（#2508 期望 5 要求「扫描并核对」，未要求同单修全部） | 若要求合并 → 回 Architect 拆分（跨 4 个业务域，超出本单 3 文件边界） |

## Out of Scope

- `personnelMerchantView` / `personnelSystemView` 的任何改动（`tid` / 根组织用法已是正确 ID 域）。
- 递归子网点（`sites.parent_id`）成员可见性（D1）。
- PC 页面 `/staff` 的控件可见性与交互（含按角色隐藏「只看直属」/网点下拉、#2447 入口优化）——本单只修后端口径；`direct` 语义按 D3 保持文档既有约定。
- `oid ↔ site_id` 其他 4 处同类缺陷（S2-S5）→ #2509。
- `sites` 软删除语义（`sites.deleted_at` 为 `*time.Time`，merchant 视图同样未过滤）——保持与既有视图一致，不在本单纯净度扩张。
- 错误文案 / 错误码映射（#2504 处理 `40303`）。
- 预生产真实数据核对与小程序/PC 真机走查（D5，归 Ops-Followup）。

## G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `setupPersonnel2065`（`sites.id ≠ sites.org_id`；org A 下 siteA 有 2 名成员） | `bash scripts/verify-2508.sh r1` | `docs/evidence/2508/R1-id-domain.txt` | `BUGGY_ROWS=0`、`FIXED_ROWS=2`、`RED_CONFIRMED=1` | `BUGGY_ROWS>0` → 夹具未区分 ID 域（红例不成立）→ FAIL |
| R2 | site_admin actor（`oid` = siteA 的 `org_id`） | `bash scripts/verify-2508.sh r2` | `docs/evidence/2508/R2-site-view.txt` | `CODE=20000`、`SITE_VIEW_ROWS=2`、`HAS_SITE_A_MEMBER=1`、`HAS_ZERO_TENANT=1`、`LEAK_SITE_B=0`、`LEAK_TECH=0`、`LEAK_PLATFORM=0`、`LEAK_OTHER_TENANT=0` | 任一行泄漏或 `SITE_VIEW_ROWS=0` → FAIL |
| R3 | 同夹具，actor `oid` = 组织 A / 组织 B | `bash scripts/verify-2508.sh r3` | `docs/evidence/2508/R3-isolation.txt` | `A_ONLY=1`（A 视角仅 A 成员）、`B_ONLY=1`（B 视角仅 B 成员）、`CROSS_LEAK=0` | 两个组织互相可见任一成员 → FAIL |
| R4 | actor `oid` = `sites.id`（旧错误口径） | `bash scripts/verify-2508.sh r4` | `docs/evidence/2508/R4-old-behavior-gone.txt` | `OLD_KEYING_ROWS=0`、`STATUS=200` | `OLD_KEYING_ROWS>0` → 仍在按 `sm.site_id = oid` 取值（旧行为未消失）→ FAIL |
| R5 | org A 下 siteA + siteA2 各 1 名成员 + 零租户成员；org B 站点 1 名 | `bash scripts/verify-2508.sh r5` | `docs/evidence/2508/R5-multisite-filter.txt` | `MULTI_ROWS=3`、`FILTER_A_ROWS=2`、`FILTER_B_ROWS=0`、`INVALID_SITE_CODE=40002` | `MULTI_ROWS=2`（漏掉第二站点）或 `FILTER_B_ROWS>0`（跨组织泄漏）→ FAIL |
| R6 | 既有人员套件（2065/2067/2072/2427）+ 本单用例 | `bash scripts/verify-2508.sh r6` | `docs/evidence/2508/R6-regression.txt` | `REGRESSION_PASS=1`、`FAIL_COUNT=0`、含 `TestPersonnel2065_SiteView` PASS 行 | 任一人人员用例 FAIL → FAIL |
| R7 | 修后工作树 | `bash scripts/verify-2508.sh r7` | `docs/evidence/2508/R7-build-scan.txt` | `BUILD_RC=0`、`PERSONNEL_SITE_ID_MATCH=0`、`S2=1`、`S3=2`、`S4=2`、`S5=2` | `PERSONNEL_SITE_ID_MATCH>0`（本单残留）或同族命中数与 #2509 清单不符 → FAIL |

## Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2508.sh r1` | `docs/evidence/2508/R1-id-domain.txt` | exit 0；grep `BUGGY_ROWS=0`、`FIXED_ROWS=2`、`RED_CONFIRMED=1` | PASS | 输出 `BUGGY_ROWS=1` 或更大 → FAIL | dev |
| R2 | `bash scripts/verify-2508.sh r2` | `docs/evidence/2508/R2-site-view.txt` | exit 0；grep `SITE_VIEW_ROWS=2`、`HAS_SITE_A_MEMBER=1`、`HAS_ZERO_TENANT=1`、`LEAK_SITE_B=0`、`LEAK_TECH=0`、`LEAK_PLATFORM=0`、`LEAK_OTHER_TENANT=0` | PASS | 任一 `LEAK_*=1` 或 `SITE_VIEW_ROWS=0` → FAIL | dev |
| R3 | `bash scripts/verify-2508.sh r3` | `docs/evidence/2508/R3-isolation.txt` | exit 0；grep `A_ONLY=1`、`B_ONLY=1`、`CROSS_LEAK=0` | PASS | `CROSS_LEAK=1` → FAIL | dev |
| R4 | `bash scripts/verify-2508.sh r4` | `docs/evidence/2508/R4-old-behavior-gone.txt` | exit 0；grep `OLD_KEYING_ROWS=0`、`STATUS=200` | PASS | `OLD_KEYING_ROWS=1` 或更大 → FAIL | dev |
| R5 | `bash scripts/verify-2508.sh r5` | `docs/evidence/2508/R5-multisite-filter.txt` | exit 0；grep `MULTI_ROWS=3`、`FILTER_A_ROWS=2`、`FILTER_B_ROWS=0`、`INVALID_SITE_CODE=40002` | PASS | `MULTI_ROWS=2` 或 `FILTER_B_ROWS=1` → FAIL | dev |
| R6 | `bash scripts/verify-2508.sh r6` | `docs/evidence/2508/R6-regression.txt` | exit 0；grep `REGRESSION_PASS=1`、`FAIL_COUNT=0`、`TEST=TestPersonnel2065_SiteView` 且该行 `PASS` | PASS | 任一 `--- FAIL` 或 `FAIL_COUNT=1` → FAIL | dev |
| R7 | `bash scripts/verify-2508.sh r7` | `docs/evidence/2508/R7-build-scan.txt` | exit 0；grep `BUILD_RC=0`、`PERSONNEL_SITE_ID_MATCH=0`、`S2=1`、`S3=2`、`S4=2`、`S5=2` | PASS | `BUILD_RC=1` 或 `PERSONNEL_SITE_ID_MATCH=1` → FAIL | dev |

## Artifact Provenance

| artifact | first_line |
|---|---|
| `docs/evidence/2508/R1-id-domain.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2508/R2-site-view.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2508/R3-isolation.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2508/R4-old-behavior-gone.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2508/R5-multisite-filter.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2508/R6-regression.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |
| `docs/evidence/2508/R7-build-scan.txt` | `# build=1.0.<fix> # commit=<sha> # dirty=false` |

evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

## Bug Reflection Matrix

| # | 缺陷面 symptom | 触发条件 trigger | 根因 root cause | 测试盲区 test blind spot | 同类路径 same-class | 修复层 fix layer | 红绿 red-green | 预防 prevention |
|---|---|---|---|---|---|---|---|---|
| B1 | site_admin 人员列表恒空，无报错 | `sites.id ≠ sites.org_id` 且站点账号进入 `/staff` | `personnelSiteView` 把 JWT `oid` 当 `sm.site_id`（ID 域混用） | 夹具把 `sites.org_id` 设成 `tenant_id`、actor `oid` 设成 `site.id`，两域同值不可区分 | `personnel.go:224`（本单）；`instrument_loss`/`warehouse`/`user_rental`/`repair_service`（#2509） | handler SQL 谓词（组织域 JOIN） | R1（旧谓词 0 行）→ R2（新谓词 2 行） | 夹具强制 `org_id ≠ id` + R4 反向断言 |
| B2 | 契约文档把错误口径写成规格，后续实现按文档复现缺陷 | 开发/审计读 api.md §10.18.0 | 文档未区分 `sites.org_id` 与 `sites.id` | 无「文档 ↔ 代码 ID 域」交叉断言 | api.md §10.18.0 其他行（merchant/system 口径正确） | Core Update（文档） | R2 的 `oid = org_id` fixture 与文档一致 | Core Update 同批落地；D1 落字 |
| B3 | 夹具用「同值 ID」让错误谓词通过 | 任何按 ID 域过滤的新用例 | 测试数据构造图省事（`OrgID: f.tid`、`oid: f.siteA`） | 夹具本身是盲区源头 | `personnel_2065_test.go` 全文件 | 测试夹具 | 修复前 `TestPersonnel2065_SiteView` 红 → 修复后绿 | 夹具注释写明 ID 域（personnel_2065_test.go:130） |
| B4 | 零租户成员（`users.tenant_id` 全零）在 site 视图被自动 scope 滤掉 | membership 有效但 users 行租户号不可靠（#2067 已为 merchant 视图修过） | `users` 查询走自动 tenant scope | 无零租户成员 × site 视图用例 | merchant 视图已修（#2067）；system 视图 tid 为空不触发 | handler 查询范围（`ExplicitTenantScopeOnly`） | R2 `HAS_ZERO_TENANT=1` | D4 口径复用 #2067 |
| B5 | 数据无异常但列表空，巡检/监控无法发现 | 恒空返回而非错误 | 空结果与失败未区分痕迹（#2427 已修错误路径，但「0 站点组织」无痕） | 无 0 站点 + WARN 断言 | 其他「恒空」查询（#2427 D3 家族） | handler 可观测性（WARN 日志） | R1/R2 覆盖语义；WARN 由 R7 静态 grep 断言 | 同类「过滤后恒空」须留可观测痕迹 |
| B6 | `site_id` 下拉选了别组织网点仍显示本组织成员 | site_admin 使用网点下拉 | site 视图忽略 `siteFilter` | 无 site_admin × site_id 用例 | merchant 视图 siteFilter（已实现） | handler 参数消费 | R5 `FILTER_B_ROWS=0` | D2 落字 + api.md Query 说明 |
| B7 | 修复方向搞反（把 `oid` 塞进 `site_id` 列的写入侧） | 需要「组织 → 站点」转换的实现 | 两列语义相近、命名不带域标记 | 无写入侧反向断言 | `warehouse.go:697/872`、`user_rental.go:437/1044`（#2509） | 本单只读侧；写入侧另单 | R7 同族扫描计数固定 | #2509 逐项红绿 + 全仓 ID 域扫描 |
| B8 | 修完 A 组织，B 组织越权可见 | 谓词写成「tenant + 任意 site」 | 组织域过滤写漏（只剩 tenant） | 无跨组织负例 | `personnelSystemView` 中转网点（正确） | handler SQL 谓词 | R3 `CROSS_LEAK=0` | 负例矩阵（R3/R5）固定进合同 |
| B9 | 语义变了但旧注释/旧断言留着，下一轮照旧写法复现 | 只改代码不改夹具注释与文档 | 语义变更清单缺失 | 旧断言固化错误契约 | `personnel_2065_test.go:130` 注释、api.md §10.18.0 | 同批翻转（本计划语义变更表） | R6 回归 + 语义变更表逐条核对 | AGENTS.md §语义变更 强制清单 |

### Workload

- Size: S
- Basis: 1 个生产文件（`personnel.go`，约 25 行改动）+ 2 个测试文件（1 改夹具 + 1 新增用例）+ 1 个证据脚本 + 1 份契约文档；无 migration、无前端、无新依赖
- Confidence: high

### Files（变更文件清单）

- `backend/handlers/personnel.go`
- `backend/handlers/personnel_2065_test.go`
- `backend/handlers/personnel_site_org_2508_test.go`
- `backend/handlers/personnel_filter_2427_test.go`（R6 回归引用，不改动）
- `scripts/verify-2508.sh`
- `docs/spec/api/api.md`
- `docs/evidence/2508/plan.md`
- `docs/evidence/2508/evidence.json`
- `docs/evidence/2508/pre-ready-2508.json`
- `docs/evidence/2508/R1-id-domain.txt`
- `docs/evidence/2508/R2-site-view.txt`
- `docs/evidence/2508/R3-isolation.txt`
- `docs/evidence/2508/R4-old-behavior-gone.txt`
- `docs/evidence/2508/R5-multisite-filter.txt`
- `docs/evidence/2508/R6-regression.txt`
- `docs/evidence/2508/R7-build-scan.txt`

## Core Update

| 文档 | 当前状态 | 修改内容 | 原因 |
|---|---|---|---|
| `docs/spec/api/api.md` §10.18.0（人员列表三视图，site_admin 行 + Query 段 + 隔离段） | site_admin 行写「**仅本网点成员**（`site_members.site_id = JWT oid`）」；Query 段写 `site_id`「**merchant 视图**按网点过滤」；隔离段未提 ID 域 | ① site_admin 行改为「`JOIN sites s ON s.id = sm.site_id WHERE s.org_id = JWT oid`（JWT `oid` 是**网点自身的 IAM 组织 ID**，与 `site_members.site_id`（`sites.id`）不是同一域）；一个组织多站点时返回全部站点成员，不递归子网点；不含维修师傅」；② Query 段 `site_id` 补「site_admin 亦生效，语义为与本组织求交（跨组织站点 ID → 空集）」；③ 隔离段补一行 ID 域注（`sites.org_id` vs `sites.id`）与「0 站点组织返回空列表并记 WARN」 | 现有文档把缺陷（`site_members.site_id = JWT oid`）写成规格，是按文档复现缺陷的源头；#2508 期望 1/2 要求语义落字 |

## Dependencies

- 无

### 关联（非硬依赖）

- `#2427`（查询口径 + 空结果/失败区分）：本单沿用其 `site_id` UUID 校验与 `50000` 语义。
- `#2067`（membership 锚定）：本单把「users.tenant 不可靠」口径对称到 site 视图（D4）。
- `#2509`（同族 ID 域缺陷 4 处）：本单只做扫描登记（D6），不修。
- 部署侧：预生产 `site_admin` 打开 `/staff` 应见 5 条成员 → 关闭时以 `[Ops-Followup of #2508]`（仅 `waitForConfirm`）承载。

*Model: DeepSeek V4.1 Flash 1017*

