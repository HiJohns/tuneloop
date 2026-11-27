## Work Summary — #2620 [Bug] `/users/me/*` 身份自查询跳过自动租户 scope

### 修改内容

1. **身份句柄收口（2 文件 / 6 处，纯句柄替换）**：把 `/users/me/*` 家庭的 `db := database.GetDB().WithContext(ctx)` 改为 database.GetDB().WithContext(database.IdentityCtx(ctx))，每处加注释说明「身份自查询不适用租户过滤（#2078 同款口径）」。
   - `backend/handlers/wechat_bind.go`：`GenBindToken`（Issue 已复现的 404）、`Unbind`（同款 `/users/me/wechat-unbind`）、`ConfirmBind` / `MergeBind`（防御性收口：当前路由无鉴权、ctx 无 `tid`，行为不变，避免挂到鉴权组后复发）。
   - `backend/handlers/referral.go`：`GetPromoQR`（含同句柄 `Update("ref_code")`）、`ListReferrals`（含 `referrals` 行定位的 referee 查询——`referrals` 表无 `TenantID`，按 `tid` 过滤会静默丢被推荐人姓名）。
   - 不新增 helper、不改 `addTenantScope` 全局语义、不动业务分支/状态码/字段集合（D1/D8）。
2. **回归测试** `backend/handlers/self_query_tenant_scope_2620_test.go`：5 个子用例（`bind_gen` / `unbind` / `promo_qr` / `referrals` / `cross_tenant_scoped`）。夹具刻意构造预生产故障形态：`users.tenant_id = cacheTenant` ≠ JWT `tid = jwtTenant`；referee 行放在**第三个** tenant；上下文强制由 testutil.TestActor.InjectContext 构造（同时写 `database` 键，使自动租户回调真实执行——这是本缺陷此前的测试盲点）。
3. **证据生成器** `scripts/verify-2620.sh`（`r1` 端点直调 / `r2` 收口扫描 + 构建 + 编译冒烟 + gofmt）。
4. **审计登记** `docs/ops/tenant-id-audit.md` 追加 **A1.1 身份自查询例外（#2620）**：谓词、依据、已覆盖端点、未覆盖（→ #2621）、不适用情形。

### 涉及文件

- `backend/handlers/wechat_bind.go`
- `backend/handlers/referral.go`
- `backend/handlers/self_query_tenant_scope_2620_test.go`（新增）
- `scripts/verify-2620.sh`（新增）
- `docs/ops/tenant-id-audit.md`
- `docs/evidence/2620/plan.md`、`docs/evidence/2620/R1-self-query-tests.txt`、`docs/evidence/2620/R2-build-and-scan.txt`、`docs/evidence/2620/evidence.json`、`docs/evidence/2620/pre-ready-2620.json`、`docs/evidence/2620/summary.md`、`docs/evidence/2620/changed-files.txt`

### Commit

- fix: `0064ce146`（`fix(issue-2620): user self-queries bypass automatic tenant scope`）
- evidence: `docs(evidence): issue-2620 …`（fix commit 之后从该 commit 重新生成，`dirty=false`）

### 实现前差异核对（计划「Input Truth & Tool Preflight」逐条）

| 检查 | 期望 | 实测 |
|---|---|---|
| `grep -c 'WithContext(database.IdentityCtx(ctx))'` 两文件 | 修前 0 / 0 | 0 / 0 ✅（修后 4 / 2） |
| `grep -c 'WithContext(ctx)'` 两文件 | 修前 4 / 2 | 4 / 2 ✅（修后 0 / 0） |
| `grep -rn 'database.SetTenantID' backend/handlers/wechat_bind_test.go` | 修前 0 | 0 ✅（缺陷在旧单测里不可见的证据） |

### 验证层级（Issue #70）

- 独立 Issue（无 Parent）：**定向** `go test ./handlers -run TestSelfQueryTenantScope2620`（5 子用例，一次性 PG 15433）+ `go build ./...` + `go test ./... -run '^$'` 编译冒烟 + `gofmt -l` 三文件。全量 `go test ./...` 按计划（Out of Scope）由 pre-ready P5 baseline 差分承担。
- 既有 wechat-bind 相关用例一并复跑：`go test ./handlers -run 'TestGenBindToken|TestConfirmBind|TestMergeBind|TestPollBindToken|TestUnbind' -count=1` → `ok`（0 翻转）。

### Remote

remote: origin/issue-2620; fix=0064ce146 pushed=yes; evidence 提交 pushed=yes

### 红绿实证

红（只落测试、未换句柄；同一命令）：`go test ./handlers -run TestSelfQueryTenantScope2620 -count=1 -v` → exit 1

```
=== RUN   TestSelfQueryTenantScope2620/bind_gen
    Messages:   	body={"code":40004,"message":"user not found"}
=== RUN   TestSelfQueryTenantScope2620/unbind
    Messages:   	body={"code":40004,"message":"user not found"}
=== RUN   TestSelfQueryTenantScope2620/promo_qr
    Messages:   	body={"code":40004,"message":"user not found"}
=== RUN   TestSelfQueryTenantScope2620/referrals
    Messages:   	body={"code":40004,"message":"user not found"}
    --- FAIL: TestSelfQueryTenantScope2620/bind_gen (0.00s)
    --- FAIL: TestSelfQueryTenantScope2620/unbind (0.00s)
    --- FAIL: TestSelfQueryTenantScope2620/promo_qr (0.00s)
    --- FAIL: TestSelfQueryTenantScope2620/referrals (0.00s)
    --- PASS: TestSelfQueryTenantScope2620/cross_tenant_scoped (0.00s)
```

绿（fix commit `0064ce146`）：`--- PASS: TestSelfQueryTenantScope2620` + 5 个子用例全部 PASS（artifact docs/evidence/2620/R1-self-query-tests.txt 内含 5 条 `--- PASS:` 行与 9 个 token）。

### Accepted Plan 对照（逐条完成）

| 计划项 | 状态 |
|---|---|
| 变更点 1 六处句柄替换（含 `ConfirmBind`/`MergeBind` 防御性收口） | ✅ |
| 变更点 2 回归测试 5 子用例 + 红绿实证 | ✅ |
| 变更点 3 `scripts/verify-2620.sh`（r1/r2，token 逐字） | ✅ |
| 变更点 4 `docs/ops/tenant-id-audit.md` A1.1 小节（不改既有计数） | ✅ |
| Decision Lock D1–D10 | ✅（D1 只用 IdentityCtx；D2 六处；D3 referee 同句柄；D4 测试强制 `TestActor`；D5 不重构共享解析器；D6 不做数据纠正；D7 不改审计计数；D8 不碰 avatar/UPDATE 型自写；D9 size M；D10 真机复验归部署侧） |
| Out of Scope 未越界 | ✅ 未改 `/user/*` 家庭、未改 middleware、未做回填 |

### Evidence Contract（2/2 行，全部 PASS）

| 行 | 命令 | 产物 | 关键 token | 状态 |
|---|---|---|---|---|
| R1 | `bash scripts/verify-2620.sh r1` | `docs/evidence/2620/R1-self-query-tests.txt` | GO_TEST_EXIT=0 / SELF2620_CASES_TOTAL=5 / SELF2620_CASES_FAIL=0 / SELF2620_BIND_GEN_HTTP=200 / SELF2620_UNBIND_HTTP=200 / SELF2620_PROMO_QR_HTTP=200 / SELF2620_REFERRALS_TOTAL=1 / SELF2620_REFEREE_NAME=1 / SELF2620_CROSS_TENANT_HIDDEN=1 + 5 条 `--- PASS: TestSelfQueryTenantScope2620/` | PASS |
| R2 | `bash scripts/verify-2620.sh r2` | `docs/evidence/2620/R2-build-and-scan.txt` | GO_BUILD_EXIT=0 / TEST_COMPILE_EXIT=0 / GOFMT_DIFF=0 / IDENTITY_CTX_SITES=6 / RESIDUAL_PLAIN_CTX_SITES=0 | PASS |

依据 `docs/evidence/2620/evidence.json`（schema_version=1、issue=2620、build=1.0.0064ce14、commit=0064ce14…、dirty=false、plan_sha256=87052a23…）。

### G0 Golden Acceptance

| 行 | artifact | assertions | negative |
|---|---|---|---|
| R1 | `docs/evidence/2620/R1-self-query-tests.txt` | 9 项 token 全为期望值 + 5 条子用例 PASS 行；首两行 provenance（build/commit/dirty、GENERATED_AT_HEAD） | 任一端点 404 `40004 user not found`、referee 姓名缺失、跨租户行可见、子用例数 <5 → FAIL（红样已复现四端点 404） |
| R2 | `docs/evidence/2620/R2-build-and-scan.txt` | 5 项 token 全为期望值（含 per-file 4+2=6 明细） | 残留 `WithContext(ctx)`、漏改句柄（<6）、构建/编译冒烟失败、gofmt 差异 → FAIL |

### Verification Matrix（矩阵 2/2）

| 变更点 | 状态 | 证据 |
|---|---|---|
| R1 身份自查询不再被租户过滤（4 端点）+ 跨租户隔离仍生效（负例） | PASS | `docs/evidence/2620/R1-self-query-tests.txt` |
| R2 句柄收口全覆盖 + 构建/格式/编译冒烟 | PASS | `docs/evidence/2620/R2-build-and-scan.txt` |

### 证据真实性 / 持久化

- 两份 artifact 位于 `docs/evidence/2620/`（随 `docs(evidence)` 提交），首行 provenance `# build=1.0.0064ce14 # commit=0064ce14639b07ba16e23b8053f695ce69b09d45 # dirty=false`，与 evidence.json 一致；证据在 fix commit **之后**生成。
- R1（handler 直调 + 一次性 PG）与 R2（静态扫描 + 构建）是两个不同断言面，无复用。
- 一次性 PG 库名 `tuneloop_test_2620`（127.0.0.1:15433），未触碰 dev `jobmaster-postgres` 与预生产/生产库。

### 测试保真

- 上下文由 testutil.TestActor.InjectContext 构造：同时写 database.SetTenantID/SetOrgID 与 middleware 包 两套 key，`addTenantScope` 真实执行——旧单测只写 middleware.ContextKeyUserID，这正是缺陷此前不可见的原因（已在测试文件头注释说明）。
- `cross_tenant_scoped` 负例同时断言「他人行 `ErrRecordNotFound`」与「`Find` 只返回本 `tid` 租户行（len=1 且为该行）」，证明修复没有变成全局关闭自动 scope。
- `referrals` 子用例把 referee 放在第三个 tenant 下，只有 referee 查询也用身份句柄才能拿到姓名（`total=1` 且 list[0].name 正确）。

### 依赖顺序 / 验收规则

- 计划 `## Dependencies`：无；未修改任何既有验收工具/规则（新增 `scripts/verify-2620.sh` 为本单证据生成器）。
- 既有断言 0 翻转（计划「既有断言影响」逐条复核：wechat-bind 三个既有用例无 database.SetTenantID，`IdentityCtx` 对它们等价于无操作，实测仍通过）。
- 未出现连续两轮验收失败；无延期项。

### 人工 / 部署侧（不由 Developer 完成）

| 事项 | 负责方 | 回执要求 |
|---|---|---|
| 预生产真机复验：linwx1978@yahoo.com 点「绑定微信」出码、扫码完成绑定（D10） | 运维 / Owner | 关单时按 `[Ops-Followup of #2620]`（`waitForConfirm`，无 `status:*`）另记；需回评操作时间与结果 |

*Model: deepseek/deepseek-flash*
