## Work Summary — #2512 S4 游客下单：`orders.org_id` 回退链改为「站点 → `sites.org_id`」反查（禁止写站点 ID）

### 修改内容

- `backend/handlers/user_rental.go`
  - 新增唯一解析函数 `resolveOrderOrgID(db, inst)`（第 30–49 行）：`instruments.org_id` → 按 `site_id` / `current_site_id` 反查 `sites.org_id` → 都解析不到返回 `""`（注释锚定 #2509/#2512 与 `docs/cases/cases.md:289`、`docs/spec/api/api.md:91` 的 ID 域口径）。
  - `CreateOrder` 游客分支（原 `:430-440`）：改为调用 `resolveOrderOrgID`；解析为空 → `log.Printf` WARN（含 instrument id）+ `400 / 40002`「乐器归属信息不完整，暂时无法下单，请联系门店」，**在落库前返回**。
  - `BatchCreateOrder`（原 `:1037-1047`）：先收集全部可用乐器到 `verified`，游客分支对**每条**解析 org；任一条为空 → `400 / 40002`（消息含乐器 ID）且**零落库**（D4 全有或全无）；解析成功时所有订单共用同一 org（`sites.org_id`）。
  - 非游客分支（`effectiveOrgID = oid`）与其余 6 处 `effectiveOrgID` 消费点未改动（D2）。
- `backend/handlers/user_rental_2512_test.go`（新增，262 行）：夹具 `sites.id ≠ sites.org_id`（#2508/#2510 口径）、游客上下文 = `ContextKeyUserID` 有值且 `TenantID`/`OrgID` 为空、用户经 `testutil.SeedVerifiedCustomer` 满足实名前置。
  - `TestCreateOrder2512_GuestOrgFromSite`（R1）、`TestCreateOrder2512_GuestOrgFromCurrentSite`（R2）、`TestCreateOrder2512_UnresolvableRejected`（R3）、`TestBatchCreateOrder2512_GuestOrgFromSiteAndReject`（R4）。
- `docs/spec/api/api.md` §6.2 / §6.3（Core Update）：补 `org_id` 派生顺序（`instruments.org_id` → `sites.org_id` 反查）、**禁止**写入 `sites.id`、不可解析 → `400 / 40002`；批量注明「全部可解析才落库，零落库」。
- `scripts/verify-2512.sh`（新增，214 行）：`r1`–`r6` + `gate`；每行首行写 PROVENANCE（`build`/`commit`/`dirty` 由 `git rev-parse` 派生），`gate` 供 `--full-gate-cmd` 使用。

Commit：`c8a2c5d4f`（fix，含 plan 快照与证据 harness）。

### 验证层级（Issue #70）

- 子任务（body 含 `Parent: #2509`）：**只跑定向用例**，不重复全量门禁。
  - 定向：`cd backend && go test ./handlers -run '^TestCreateOrder2512_GuestOrgFromSite$'` 等 4 个用例（均 PASS）。
  - 定向回归：`go test ./handlers -run 'CreateOrder|BatchCreateOrder' -count=1` → `REG_RC=0`。
  - 构建：`cd backend && go build ./...` → `BUILD_RC=0`。
  - 父任务 #2509 / 批次全量 `go test ./...` 由父批次 gate 承担（本单未重复执行）。
- 证据在 fix commit `c8a2c5d4f` 之后整体重新生成：R1–R6 首行均为 `# build=1.0.c8a2c5d4 # commit=c8a2c5d4f52b577ba72602aa9a11357c41ec3b57 # dirty=false`。

### Remote

- remote: origin/issue-2512; fix=c8a2c5d4f pushed=yes; evidence pushed=yes（见本评论发布时的 push 回执）

### 红绿实证

- 红（缺陷版语义）：修前 `effectiveOrgID = instrument.SiteID.String()` / `CurrentSiteID.String()` 直接把 `sites.id` 写入 `orders.org_id`。R1 用例显式打印 `OLD_FALLBACK_IS_SITE_ID=1`（旧回退值 `aID ≠ aOrg`）、`RED_CONFIRMED=1`（旧值 ≠ 实际落库值）——即缺陷版落库值为站点 ID。
  - 反向验证：若实现回退到旧逻辑（写 `sites.id`），`require.Equal(f.orgA, order.OrgID)` 与 `require.NotEqual(f.siteA, order.OrgID)` 立即失败 → `ORDER_ORG_MATCHES_SITE_ORG=0`、`R1_FAIL` 且非 0 退出；R2 同理（`bID` vs `bOrg`）。
- 绿（修复版）：`ORDER_ORG_MATCHES_SITE_ORG=1`（R1）、`RESOLVED_FROM_CURRENT_SITE=1`（R2）、`STATUS=400/CODE=40002/ORDERS_AFTER=0`（R3）、`BATCH_ORGS_UNIQUE=1/BATCH_ORDERS_AFTER=0`（R4）、`SITE_ID_AS_ORG=0/CURRENT_SITE_AS_ORG=0/RESOLVER_CALLS=2`（R5）、`BUILD_RC=0/REG_RC=0`（R6）。
- 假阴性反例（已在矩阵登记）：落库等于 `sites.id`、全空仍下单成功、批量部分落库、残留站点 ID 兜底 → 对应行 FAIL 并非 0 退出。

### Accepted Plan 对照（8/8 变更点，n/n）

1. 变更点 1：新增 `resolveOrderOrgID` 唯一解析函数 → 完成（`user_rental.go:30-49`，R5 `RESOLVER_CALLS=2`）。
2. 变更点 2：`CreateOrder` 游客分支改用解析函数 + 不可解析 `400/40002` + WARN → 完成。
3. 变更点 3：`CreateBatchOrders` 同口径 + 全有或全无（D4）→ 完成。
4. 变更点 4：不改非游客分支与 6 处消费点 → 完成（diff 仅回退链两处）。
5. 变更点 5：测试夹具 `sites.id ≠ sites.org_id`、游客上下文空 tid/oid、实名前置 → 完成。
6. 变更点 6：R1–R4 四个用例 → 完成。
7. 变更点 7：`docs/spec/api/api.md` §6.2/§6.3 → 完成（Core Update）。
8. 变更点 8：`scripts/verify-2512.sh`（r1–r6 + gate，r5 只读 SQL 不执行）→ 完成。

### Pre-ready gate

- 命令（R7）：

  ```
  ~/.config/opencode/scripts/pre-ready-check.sh 2512 \
    --summary-file docs/evidence/2512/summary.md \
    --changed-files docs/evidence/2512/changed-files.txt \
    --evidence-file docs/evidence/2512/evidence.json \
    --affected-cmd "bash scripts/verify-2512.sh r6" \
    --full-gate-cmd "bash scripts/verify-2512.sh gate" \
    --fix-commit c8a2c5d4f \
    --report docs/evidence/2512/pre-ready-2512.json
  ```

- 报告：`docs/evidence/2512/pre-ready-2512.json`（`overall=PASS`、零 FAIL）。
- 关键输出：P2（矩阵 7 行全 PASS）、P3（证据路径均存在）、P6（改动文件均在计划 Files 内）、P7（契约三向核对）、P12（Evidence Contract 7 行完整）、P13（fix 与证据在 `origin/issue-2512`）、P14、P17、P18 全 PASS。
- 子任务只跑受影响包：`--affected-cmd "bash scripts/verify-2512.sh r6"`（构建 + `CreateOrder|BatchCreateOrder` 定向回归）；`--full-gate-cmd` 为同一 harness 的 `gate`（`go build ./...` + `go test ./handlers -timeout 30m`，30m ≥ 实测），未在子任务内重复父批次全量。

### Evidence Contract（7/7 PASS）

- R1 → PASS → `docs/evidence/2512/R1-guest-org-from-site.txt`（`ORDER_ORG_MATCHES_SITE_ORG=1`、`OLD_FALLBACK_IS_SITE_ID=1`、`RED_CONFIRMED=1`）
- R2 → PASS → `docs/evidence/2512/R2-current-site.txt`（`RESOLVED_FROM_CURRENT_SITE=1`）
- R3 → PASS → `docs/evidence/2512/R3-reject.txt`（`STATUS=400`、`CODE=40002`、`ORDERS_AFTER=0`）
- R4 → PASS → `docs/evidence/2512/R4-batch.txt`（`BATCH_ORGS_UNIQUE=1`、`REJECT_STATUS=400`、`BATCH_ORDERS_AFTER=0`）
- R5 → PASS → `docs/evidence/2512/R5-scan-sql.txt`（`SITE_ID_AS_ORG=0`、`CURRENT_SITE_AS_ORG=0`、`RESOLVER_CALLS=2`、`SQL_PRESENT=1`）
- R6 → PASS → `docs/evidence/2512/R6-build-regression.txt`（`BUILD_RC=0`、`REG_RC=0`）
- R7 → PASS → `docs/evidence/2512/pre-ready-2512.json`
- 账本：`docs/evidence/2512/evidence.json`（schema v1，build=`1.0.c8a2c5d4`、dirty=false、plan_sha256=`9e7ca42277b7…`）。

### G0 Golden Acceptance

- R1：fixture site A(`id=aID, org_id=aOrg`, `aID≠aOrg`) + 乐器(`org_id=NULL, site_id=aID`) + 游客上下文；入口 `POST /api/user/orders`；断言 `ORDER_ORG=aOrg`（负例 `aID` → FAIL）。
- R2：fixture 乐器(`org_id=NULL, site_id=NULL, current_site_id=bID`)；入口同上；断言 `ORDER_ORG=bOrg`（负例 `bID` → FAIL）。
- R3：fixture 乐器三者全空；入口同上；断言 `STATUS=400`、`CODE=40002`、`ORDERS_AFTER=0`（负例 201 且落库 → FAIL）。
- R4：批量两件（`site_id=aID`）→ `BATCH_ORG=aOrg`、`BATCH_ORGS_UNIQUE=1`；另一批含不可解析件 → `400/40002` 且 `BATCH_ORDERS_AFTER=0`。
- R5：`user_rental.go` 静态扫描 `SITE_ID_AS_ORG=0`、`CURRENT_SITE_AS_ORG=0`、`RESOLVER_CALLS=2`；只读 SQL 文本存在（`SQL_PRESENT=1`，未执行）。
- R6：`go build ./...` = 0、`CreateOrder|BatchCreateOrder` 定向回归 = 0。
- R7：gate `overall=PASS`、零 FAIL、`dirty=false`、P13 在 `origin/issue-2512`。

### Verification Matrix n/n

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 游客下单 org 取 sites.org_id（红绿） | PASS | docs/evidence/2512/R1-guest-org-from-site.txt |
| R2 current_site_id 回退同样按 org 反查 | PASS | docs/evidence/2512/R2-current-site.txt |
| R3 不可解析 → 拒绝且不落库（负例） | PASS | docs/evidence/2512/R3-reject.txt |
| R4 批量下单同口径（含拒绝不落库） | PASS | docs/evidence/2512/R4-batch.txt |
| R5 同类扫描 + 只读存量排查 SQL | PASS | docs/evidence/2512/R5-scan-sql.txt |
| R6 构建 + 定向回归 | PASS | docs/evidence/2512/R6-build-regression.txt |
| R7 pre-ready gate（四源 + 定向测试 + 构建） | PASS | docs/evidence/2512/pre-ready-2512.json |

### 契约三向核对

- **Plan/API**：计划 D1 锁定派生顺序 `instruments.org_id` → `sites.org_id`（按 `site_id`/`current_site_id` 反查）→ 解析失败 `400/40002`；实现与之逐条一致（`user_rental.go:30-49`、`:450-462`、`:1049-1082`），未引入任何 `sites.id` 写入（R5 `SITE_ID_AS_ORG=0`/`CURRENT_SITE_AS_ORG=0`）。
- **docs**：`docs/spec/api/api.md` §6.2/§6.3 写明的派生顺序、`sites.id` 禁令、`400/40002` 拒绝语义与批量「全有或全无」与实现一致；字段名（`org_id`、`tenant_id`、`site_id`、`current_site_id`）与 `backend/models/models.go:87,98,99` 类型（`*string` / `*uuid.UUID`）对照无误。
- **handler/frontend**：本单无前端改动；`orders.org_id` 的消费点（`warehouse.go:52-56`、`merchant_order.go`、`repair_service.go`）仍在 `sites.org_id` 域内，契约未变更，无跨端字段漂移。

### Dependency Order / 验收规则

- 无前置层依赖（计划 `## Dependencies` 未声明）；父任务 #2509 同族 S1/S2 已修，S3（#2511）与 S5（#2513）为独立单，不同文件，无先后冲突。
- 本单**未修改**任何验收工具/规则（`pre-ready-check.sh`、`matrix_parser.sh`、`plan-lint.sh`、`evidence-artifacts.sh` 均未改）；未修改计划矩阵或 Evidence Contract 行。
- 无连续两轮失败的验收行（本轮首次交付）。

### 同族扫描（同类路径对称扫描义务）

- `grep -n "effectiveOrgID" backend/handlers/user_rental.go`：游客回退仅 `CreateOrder` 与 `BatchCreateOrder` 两处，均已替换为 `resolveOrderOrgID`；其余 6 处为消费点（订单落库、结算 breakdown、支付期限），未改动（D2）。
- `grep -rn "SiteID.String()\|CurrentSiteID.String()" backend/handlers`：本文件内不再有任何「站点 ID 赋给 org」的形态（R5 断言为 0）；其余命中为日志/ID 输出。
- 同族其它文件（`warehouse.go:697,872` → #2511、`repair_service.go` → #2513）按计划 Out of Scope 未触碰。

### Issue 问题点覆盖

1. 回退链改为 `instrument.OrgID` → `sites.org_id` 反查 → 仍为空拒绝下单（不写站点 ID）→ 完成（`resolveOrderOrgID` + 两处替换）。
2. 拒绝时错误码/文案明确（`40002` 家族）+ WARN 含 instrument id → 完成（`CreateOrder`/`BatchCreateOrders` 均 `log.Printf` + `40002`）。
3. 红绿实证（夹具 `instruments.org_id` 空但 `site_id` 有值 → `orders.org_id == sites.org_id`；全空 → 400 且不落库）→ 完成（R1/R3，负例字段 `OLD_FALLBACK_IS_SITE_ID`/`RED_CONFIRMED`）。
4. 存量排查只读 SQL（不自动修数据，需 Owner 授权）→ 完成（R5 `SQL_PRESENT=1`，脚本内仅打印文本，D3）。
5. 同类扫描 `grep -rn "effectiveOrgID\|org_id.*=.*SiteID" backend/handlers` 并写入证据 → 完成（R5 + 上节清单）。

### 人工/部署侧

- 存量数据修复（`orders.org_id` 域污染清理、`instruments.org_id` 回填）不在本单（D3/D5，需 Owner 授权与生产库访问）；本单只交付只读排查 SQL。
- 生产污染规模抽样（D5）由部署侧/Ops 承担，Developer 无生产访问权。

*Model: DeepSeek V4.1 Flash 1017*
