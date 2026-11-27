## Accepted Plan — #2620 [Bug] `/users/me/*` 身份自查询跳过自动租户 scope（本地缓存 tenant ≠ JWT tid 时「绑定微信」恒 404）

### 背景与现状核对（代码为据）

| # | 事实 | 位置 / 证据 |
|---|---|---|
| 1 | 自动租户 scope 只挂在 **Query / Delete** 回调上：模型含 `TenantID` 且 ctx 有 `tenant_id` → 自动追加 `WHERE tenant_id = <JWT tid>` | `backend/database/db.go:96-124`（`registerTenantCallbacks` / `addTenantScope`） |
| 2 | `users.tenant_id` 是 IAM 关系的**本地缓存**，可能与 JWT `tid` 不一致：添加成员建缓存时 `TenantID/OrgID = merchant.OrgID`（平台级商户即平台组织）且 `Role` 硬编码 `site_member` | Issue 正文「根因 2」表；`backend/handlers/merchant_member.go:170,251,303`（租户/组织）与 `:256,308`（`Role: "site_member"`） |
| 3 | 已存在的正确口径：**身份自查询要跳过自动 scope**。`GET/PUT /api/users/me` 已用 `database.GetDB().WithContext(database.IdentityCtx(ctx))` | `backend/handlers/user_staff.go:1425`、`:1696`（#2078 Phase 2） |
| 4 | `database.IdentityCtx(ctx)` = `context.WithValue(ctx, TenantIDKey, "")`（仅清空租户键，不动 org/user 键） | `backend/database/db.go:222-224` |
| 5 | **未收口点 A**：`GenBindToken` 用 `WithContext(ctx)` + `Where("iam_sub = ?", userID)` → 命中 404 `40004 user not found`（Issue 已复现） | `backend/handlers/wechat_bind.go:73,75` |
| 6 | **未收口点 B**：`Unbind` 同款 | `backend/handlers/wechat_bind.go:364,366` |
| 7 | **未收口点 C**：`GetPromoQR` 同款（且随后 `db.Model(&user).Update("ref_code", …)` 用同一句柄） | `backend/handlers/referral.go:24,28,35` |
| 8 | **未收口点 D**：`ListReferrals` 自查询同款；另 `db.First(&referee, "id = ?")` 也按 JWT tid 过滤 → 推荐人列表的**被推荐人姓名整体丢失**（`referrals` 表自身无 `TenantID`，故列表行在、姓名不在） | `backend/handlers/referral.go:81,85,101`；`models.Referral` = `backend/models/models.go:122-129`（无 `TenantID` 字段） |
| 9 | `ConfirmBind` / `MergeBind` 也按 `id = ?` 查 `users`，但路由在**无鉴权组**（`api.POST("/wechat-bind/confirm"|"/wechat-bind/merge")`），ctx 无 `tid` → 当前不触发；属「未来若挂鉴权组即复发」的同族点 | `backend/handlers/wechat_bind.go:188,190,286,288`；`backend/main.go:277-278` |
| 10 | **不受影响（不改）**：`POST /api/users/me/avatar` 只有 `UPDATE`（无 `models.User` 的 SELECT）；GORM v1.31.1 的 Update 回调链不含 query 回调，`addTenantScope` 不参与 | `backend/handlers/instrument_media.go:822`；`~/go/pkg/mod/gorm.io/gorm@v1.31.1/callbacks/callbacks.go:65-74` |
| 11 | **测试盲点**：既有 wechat-bind 测试只注入 `middleware.ContextKeyUserID`，**没有** `database.SetTenantID`，自动 scope 从未执行 → 该缺陷在单测里天然不可见 | `backend/handlers/wechat_bind_test.go:53-61`；对照 `backend/testutil/role_factory.go:57-73`（`TestActor.InjectContext` 同时写 `middleware` 与 `database` 两套键） |
| 12 | 同类但**不在本单**的直连自查询（积分 / 地址 / 人脸 / 支付计算 / 共享解析器）已另立 **#2621** | `[Bug] 其余身份自查询… #2621` |

**结论**：缺陷是「身份自查询被当成租户数据查询」。修复 = 把 `/users/me/*` 家庭中 6 处 `WithContext(ctx)` 句柄改为**身份句柄** `WithContext(database.IdentityCtx(ctx))`，并补上「本地缓存 tenant ≠ JWT tid」的回归测试（该形态此前从未被测试覆盖）。

### 变更点

#### 1. 身份句柄收口（2 文件 / 6 处，纯句柄替换）

替换规则（逐点机械，不改任何判定分支 / 状态码 / 业务顺序）：

```go
// before
db := database.GetDB().WithContext(ctx)
// after（并在上一行写注释，说明身份查询不适用租户过滤）
// #2620：身份自查询——users.tenant_id 是 IAM 关系缓存，可能不等于 JWT tid；
// 查「我自己那行」不得套用自动租户 scope（#2078 同款口径）。
db := database.GetDB().WithContext(database.IdentityCtx(ctx))
```

| 序 | 文件:行 | 函数 | 查询 | 说明 |
|---|---|---|---|---|
| 1 | `backend/handlers/wechat_bind.go:73` | `GenBindToken` | `Where("iam_sub = ?", userID).First(&user)` | Issue 已复现的 404 |
| 2 | `backend/handlers/wechat_bind.go:188` | `ConfirmBind` | `Where("id = ?", userID).First(&user)` | 无鉴权路由，当前 no-op；防「挂鉴权组即复发」 |
| 3 | `backend/handlers/wechat_bind.go:286` | `MergeBind` | `Where("id = ?", userID).First(&user)` | 同上 |
| 4 | `backend/handlers/wechat_bind.go:364` | `Unbind` | `Where("iam_sub = ?", userID).First(&user)` | 与点 1 同款（`/users/me/wechat-unbind`） |
| 5 | `backend/handlers/referral.go:24` | `GetPromoQR` | `Where("iam_sub = ?", userID).First(&user)`；`:35` 用同一句柄 `Update("ref_code")` | `/users/me/promo-qrcode` |
| 6 | `backend/handlers/referral.go:81` | `ListReferrals` | `Where("iam_sub = ?", userID).First(&user)`；`:101` `First(&referee, "id = ?", r.RefereeID)` | `/users/me/referrals`；`:101` 必须同句柄，否则被推荐人姓名仍丢 |

**边界**：本单只改句柄。`iam_sub` 全局唯一（`backend/database/migrations/038_add_users_iam_sub.up.sql`：`CREATE UNIQUE INDEX users_iam_sub_key ON users(iam_sub)`），`referee` 行由「我自己 `referrals` 行」的 `referee_id` 定位，故身份句柄不会放宽任何**他人数据**的可见性。

#### 2. 回归测试（新增 1 文件，5 个子用例）

新增 `backend/handlers/self_query_tenant_scope_2620_test.go`，统一夹具：
- 库：`setupMockIAMAndDB(t)`（= `testfixtures.SetupTestDB`）；
- 身份：`testutil.TestActor{TenantID: jwtTenant, OrgID: jwtOrg, UserID: iamSub, Role: "ADMIN"}.InjectContext(...)` —— **必须**走 `TestActor`（同时写 `database.SetTenantID`，使 `addTenantScope` 真实生效，见背景第 11 条）；
- 数据形态：`users.tenant_id = cacheTenant`（≠ `jwtTenant`，均 `uuid.NewString()`），即预生产故障形态。

| 子用例（子测试名，逐字锁定） | 入口 | 期望（修后） | 修前（红样） |
|---|---|---|---|
| `bind_gen` | `WechatBindHandler.GenBindToken` | 200 + `data.token` 16 位十六进制 + `data.wxacode_base64` 非空 | 404 `40004 user not found` |
| `unbind` | `WechatBindHandler.Unbind` | 200 + `code=20000`（IAM mock 应答 PUT `/api/v1/users/…`） | 404 |
| `promo_qr` | `ReferralHandler.GetPromoQR` | 200 + `data.ref_code == iamSub[:8]` | 404 |
| `referrals` | `ReferralHandler.ListReferrals` | 200 + `total=1` + `list[0].name == referee.Name`（referee 行在**第三个** tenant 下） | 404；仅修自查询不修 referee 时 `total=1` 但 `list` 为空 |
| `cross_tenant_scoped`（**负例**） | 直连 GORM（`database.GetDB().WithContext(actorCtx)`） | 按他人 `iam_sub` 查 `models.User` → `gorm.ErrRecordNotFound`；`Model(&models.User{}).Find` 只返回本 tenant 行 | 修前后均 PASS（守卫：证明未全局关闭自动 scope） |

- 外部依赖全部离线化：微信 token/wxacode 用既有 `newWxAPIStub`（`backend/handlers/wechat_bind_test.go:26-50`）+ `services.SetWxAPIBaseURLForTesting`；IAM 用 `httptest` mock（参照 `backend/handlers/update_current_user_test.go:33-58`）。
- **红绿实证**（强制）：先落测试（不含 6 处句柄替换）→ `bind_gen`/`unbind`/`promo_qr`/`referrals` 全 FAIL、`cross_tenant_scoped` PASS；再落替换 → 5/5 PASS。红样输出必须贴进 Work Summary，并写入 `evidence.json` 的 `negative_result`。

#### 3. 证据生成器（新增 `scripts/verify-2620.sh`）

沿用 `scripts/verify-2603.sh` 结构（provenance 首行 `# build=… # commit=… # dirty=…`、`heavy-run.sh` 取锁、一次性 PG `127.0.0.1:15433` 口径、`FIX_REF` 锚点默认 `git log --grep="^fix(issue-2620)"`）：

| 模式 | 行为 | 输出 token（逐字锁定） |
|---|---|---|
| `r1` | 跑 `go test ./handlers -run 'TestSelfQueryTenantScope2620' -count=1 -v` | `GO_TEST_EXIT=0` `SELF2620_CASES_TOTAL=5` `SELF2620_CASES_FAIL=0` `SELF2620_BIND_GEN_HTTP=200` `SELF2620_UNBIND_HTTP=200` `SELF2620_PROMO_QR_HTTP=200` `SELF2620_REFERRALS_TOTAL=1` `SELF2620_REFEREE_NAME=1` `SELF2620_CROSS_TENANT_HIDDEN=1` |
| `r2` | `go build ./...` + `go test ./... -run '^$' -count=1`（编译冒烟，非全量）+ `gofmt -l` 3 个 go 文件 + 静态扫描 2 个生产文件 | `GO_BUILD_EXIT=0` `TEST_COMPILE_EXIT=0` `GOFMT_DIFF=0` `IDENTITY_CTX_SITES=6` `RESIDUAL_PLAIN_CTX_SITES=0` |

其中：`IDENTITY_CTX_SITES` = 两文件中 `WithContext(database.IdentityCtx(ctx))` 的出现数（应为 6）；`RESIDUAL_PLAIN_CTX_SITES` = 两文件中 `WithContext(ctx)`（非 IdentityCtx）的出现数（应为 0）。

#### 4. 审计登记（1 文件）

`docs/ops/tenant-id-audit.md` 追加小节 **A1.1 身份自查询例外（#2620）**：
- 谓词：查询「我自己那行」（`users.iam_sub = JWT sub`，或由该行主键 / 我自己的 `referrals` 行定位的 `users` 行）**不适用**自动租户 scope，必须用 `database.IdentityCtx`；
- 依据：`users.tenant_id` 是 IAM 关系缓存，可与 JWT `tid` 不一致（#2078 / #2620）；
- 已覆盖：`/users/me`、`/users/me/wechat-bind`、`/users/me/wechat-unbind`、`/users/me/promo-qrcode`、`/users/me/referrals`；
- 未覆盖（→ #2621）：`/user/*` 家庭直连自查询与 `middleware.EnsureLocalUser`/`LocalUserID` 共享解析器。

> 该文档现有 `IdentityCtx 23 处` 计数字段本单**不改**（已因历史增量过期，计数口径随 P3 #2255 统一，见 Out of Scope）。

### 既有断言影响（0 翻转）

| 检查 | 结果 | 依据 |
|---|---|---|
| 既有测试断言「tenant 不匹配 → 404」 | **0 处** | 既有 wechat-bind 用例只注入 `ContextKeyUserID`，自动 scope 未生效（`backend/handlers/wechat_bind_test.go:53-61`），故不存在反向断言 |
| `TestGenBindToken_Success` / `_WxacodeFailure` / `_UserNotFound` | 保持通过 | 三者无 `database.SetTenantID`，`IdentityCtx` 对它们是无操作（tid 本就空） |
| `TestConfirmBind_Success` / `_NeedsMerge` / `_MissingParams` | 保持通过 | 同上；这两处只换句柄，行为不变 |
| `TestPollBindToken_States` | 不受影响 | 纯内存，无 DB |
| referral / promo 相关 handler 测试 | **不存在** | `grep -rln 'GetPromoQR\|ListReferrals' backend --include=*_test.go` → 0 命中 handler 测试（仅 `models.Referral` 的造数） |
| 需要翻转的语义断言 | **无** | 本单把「本不该 404 的自查询」修成 200，不改变任何成功路径语义 |

### Files（变更文件清单）

- `backend/handlers/wechat_bind.go`
- `backend/handlers/referral.go`
- `backend/handlers/self_query_tenant_scope_2620_test.go`
- `scripts/verify-2620.sh`
- `docs/ops/tenant-id-audit.md`
- `docs/evidence/2620/plan.md`
- `docs/evidence/2620/R1-self-query-tests.txt`
- `docs/evidence/2620/R2-build-and-scan.txt`
- `docs/evidence/2620/evidence.json`
- `docs/evidence/2620/pre-ready-2620.json`

### Verification Matrix

| 变更点 | 文件/组件 | 入口/路由 | 触发状态 | 目标平台 | 期望结果 | 证据 |
|---|---|---|---|---|---|---|
| R1 身份自查询不再被租户过滤（4 端点）+ 跨租户隔离仍生效（负例） | `backend/handlers/self_query_tenant_scope_2620_test.go`、`backend/handlers/wechat_bind.go`、`backend/handlers/referral.go` | `POST /api/users/me/wechat-bind`、`POST /api/users/me/wechat-unbind`、`GET /api/users/me/promo-qrcode`、`GET /api/users/me/referrals`（handler 直调） | `users.tenant_id = cacheTenant` ≠ ctx `tid = jwtTenant`；referee 行在第三 tenant | backend（一次性 PG 15433） | `SELF2620_CASES_TOTAL=5`、`SELF2620_CASES_FAIL=0`、四端点 HTTP 200、`SELF2620_REFERRALS_TOTAL=1`、`SELF2620_REFEREE_NAME=1`、`SELF2620_CROSS_TENANT_HIDDEN=1` | R1 |
| R2 句柄收口全覆盖 + 构建/格式/编译冒烟 | `scripts/verify-2620.sh`、`backend/handlers/wechat_bind.go`、`backend/handlers/referral.go` | `bash scripts/verify-2620.sh r2` | 修后工作树 | backend | `IDENTITY_CTX_SITES=6`、`RESIDUAL_PLAIN_CTX_SITES=0`、`GO_BUILD_EXIT=0`、`TEST_COMPILE_EXIT=0`、`GOFMT_DIFF=0` | R2 |

### G0 Golden Acceptance

| 行 | fixture | entry | artifact | assertions | negative |
|---|---|---|---|---|---|
| R1 | `testfixtures.SetupTestDB` + `users` 行（`iam_sub=iam-sub-2620`、`tenant_id=cacheTenant`）+ `TestActor{TenantID: jwtTenant≠cacheTenant, UserID: iam-sub-2620}` + `referrals` 1 行（referrer=自己、referee 在 tenantC）+ 微信 stub + IAM mock | `TestSelfQueryTenantScope2620/{bind_gen,unbind,promo_qr,referrals,cross_tenant_scoped}`（handler 直调，走真实 GORM 租户回调） | `docs/evidence/2620/R1-self-query-tests.txt` | `GO_TEST_EXIT=0`、`SELF2620_CASES_TOTAL=5`、`SELF2620_CASES_FAIL=0`、`SELF2620_BIND_GEN_HTTP=200`、`SELF2620_UNBIND_HTTP=200`、`SELF2620_PROMO_QR_HTTP=200`、`SELF2620_REFERRALS_TOTAL=1`、`SELF2620_REFEREE_NAME=1`、`SELF2620_CROSS_TENANT_HIDDEN=1`；artifact 含 5 条 `--- PASS: TestSelfQueryTenantScope2620/<子用例>` | 任一子用例返回 404/`40004 user not found`、`ref_code` 缺失、referee 姓名缺失（`list` 空）、或 `cross_tenant_scoped` 能查到他人行 → 任一即 FAIL |
| R2 | 修后工作树（2 个生产文件 + 3 个 go 文件含测试） | `bash scripts/verify-2620.sh r2` | `docs/evidence/2620/R2-build-and-scan.txt` | `GO_BUILD_EXIT=0`、`TEST_COMPILE_EXIT=0`、`GOFMT_DIFF=0`、`IDENTITY_CTX_SITES=6`、`RESIDUAL_PLAIN_CTX_SITES=0` | 残留任一 `WithContext(ctx)` 自查询（`RESIDUAL_PLAIN_CTX_SITES>0`）、漏改某处（`IDENTITY_CTX_SITES<6`）、构建/编译冒烟失败、gofmt 有差异 → 任一即 FAIL |

### Evidence Contract

| 行 ID | 命令 | 产物（仓库可见路径） | 断言（机器可判定） | 期望值 | 负例 | 责任人 |
|---|---|---|---|---|---|---|
| R1 | `bash scripts/verify-2620.sh r1 > docs/evidence/2620/R1-self-query-tests.txt` | `docs/evidence/2620/R1-self-query-tests.txt` | exit 0 且含 `GO_TEST_EXIT=0` `SELF2620_CASES_TOTAL=5` `SELF2620_CASES_FAIL=0` `SELF2620_BIND_GEN_HTTP=200` `SELF2620_UNBIND_HTTP=200` `SELF2620_PROMO_QR_HTTP=200` `SELF2620_REFERRALS_TOTAL=1` `SELF2620_REFEREE_NAME=1` `SELF2620_CROSS_TENANT_HIDDEN=1`；且含 5 条 `--- PASS: TestSelfQueryTenantScope2620/` 子用例行 | PASS（9 项 token 全为期望值 + 5 条 PASS 子用例） | 任一端点 404 `40004 user not found`、referee 姓名缺失、跨租户行可见、子用例数 <5 → FAIL | dev |
| R2 | `bash scripts/verify-2620.sh r2 > docs/evidence/2620/R2-build-and-scan.txt` | `docs/evidence/2620/R2-build-and-scan.txt` | exit 0 且含 `GO_BUILD_EXIT=0` `TEST_COMPILE_EXIT=0` `GOFMT_DIFF=0` `IDENTITY_CTX_SITES=6` `RESIDUAL_PLAIN_CTX_SITES=0` | PASS（5 项 token 全为期望值） | 残留 `WithContext(ctx)`、漏改句柄（<6）、构建失败、编译冒烟失败、gofmt 差异 → FAIL | dev |

### Artifact Provenance

- 生成器 `scripts/verify-2620.sh`（`r1`/`r2`）；每个文本 artifact 首行 `# build=1.0.<fix短码> # commit=<40 位 sha> # dirty=false`，其后 `GENERATED_AT_HEAD=<40 位 sha>`；证据必须在 fix commit **之后**生成（`FIX_REF` 可覆盖锚点，默认 `git log --grep="^fix(issue-2620)"`）。
- 重命令（`go test` / `go build` / `gofmt`）一律经 `~/.config/opencode/scripts/heavy-run.sh` 取锁；handler 测试用一次性 PostgreSQL（`127.0.0.1:15433`，`#2341` 口径），**不**使用 `jobmaster-postgres`。
- 逐条清单（全部位于 `docs/evidence/2620/`）：
  - `docs/evidence/2620/R1-self-query-tests.txt`
  - `docs/evidence/2620/R2-build-and-scan.txt`
- gate：`~/.config/opencode/scripts/pre-ready-check.sh 2620 --summary-file tmp/pre-ready-2620-summary.md --changed-files tmp/pre-ready-2620-changed.txt --evidence-file docs/evidence/2620/evidence.json --report docs/evidence/2620/pre-ready-2620.json`
- evidence.json build schema: ^1\.0\.[0-9a-f]{7,40}$

### Input Truth & Tool Preflight

- **Source of truth**：
  - 收口点清单 = `grep -n 'WithContext(ctx)' backend/handlers/wechat_bind.go backend/handlers/referral.go`（修前 6 处全部非 IdentityCtx；修后同命令 0 处）；
  - 身份语义依据 = `backend/database/db.go:96-124`（Query/Delete 自动注入）与 `:222-224`（`IdentityCtx` 只清空 tenant 键）；
  - 既有正确口径 = `backend/handlers/user_staff.go:1425,1696`（`/users/me` 已用 IdentityCtx，本单对齐它）；
  - 测试能否触发缺陷 = `backend/testutil/role_factory.go:57-73`（`TestActor` 同时写 `middleware` 与 `database` 两套 context key）。
- **实施前差异核对（先跑，结果入 Work Summary）**：
  - `grep -c 'WithContext(database.IdentityCtx(ctx))' backend/handlers/wechat_bind.go backend/handlers/referral.go` → 修前 **0 / 0**，修后 **4 / 2**；
  - `grep -c 'WithContext(ctx)' backend/handlers/wechat_bind.go backend/handlers/referral.go` → 修前 **4 / 2**，修后 **0 / 0**；
  - `grep -rn 'database.SetTenantID' backend/handlers/wechat_bind_test.go` → 修前 **0**（缺陷在单测里不可见的证据）。
- **损坏输入 / 边界拒绝规则**：
  - 匿名（`GetUserID` 空）仍走原 404/401 分支，**不得**因本修复变成 200；
  - 真正缺失的本地行（`iam_sub` 查不到）仍必须 404 `40004`（`TestGenBindToken_UserNotFound` 守卫）；
  - 跨租户**数据**查询（`users` 列表、`tenant_id`/`org_id` 过滤）必须继续被自动 scope 限制（`cross_tenant_scoped` 负例守卫）。
- **工具盲区**：
  - handler 直调测试不经 nginx / IAM 真实实例；真实扫码绑定（二维码出码 + 小程序扫码）归部署侧复验；
  - `cross_tenant_scoped` 只能证明 `users` 表的自动 scope 未被全局关闭，不能替代其它模型的隔离用例（`backend/database/tenant_scope_2261_test.go` 继续覆盖回调层）；
  - `TestSelfQueryTenantScope2620` 依赖一次性 PG；不可达时必须 `exit 2`/FAIL，**禁止** 静默跳过；
  - 全量 `go test ./...` 不作合同行（P5 baseline 差分承担），`r2` 只跑 `-run '^$'` 编译冒烟。
- **契约影响**：无 API 契约变更（状态码/字段/错误码集合不变，仅修正本不该出现的 404）；`docs/spec/api/api.md` 无需修改，故无 `## Core Update`。

### Decision Lock

| ID | 项 | 裁定 |
|---|---|---|
| D1 | 修复口径 = **身份句柄** `database.GetDB().WithContext(database.IdentityCtx(ctx))`（与 `/users/me` 的 #2078 实现同款），**不新增** helper、不改 `addTenantScope` 全局行为、不按表/按列做隐式豁免 | 本计划锁定 |
| D2 | 收口范围 = `/users/me/*` 家庭 **6 处**（含 `ConfirmBind`/`MergeBind` 两处防御性收口：今日无 `tid`，行为不变，但避免路由调整后复发） | 本计划锁定 |
| D3 | `ListReferrals` 的 referee 查询（`referral.go:101`）同样走身份句柄：`referrals` 表无 `TenantID`，推荐关系是「我的子图」，按 JWT tid 过滤会静默丢姓名 | 本计划锁定 |
| D4 | 回归测试**必须**用 `testutil.TestActor.InjectContext`（写 `database.SetTenantID`），否则自动 scope 不执行、用例退化为无效绿 | 本计划锁定 |
| D5 | 不新增 helper、不重构 `middleware.EnsureLocalUser`/`LocalUserID`（31 个调用点）→ **#2621**，本单只保证 `/users/me/*` 可用 | #2621 |
| D6 | 本单**不做**数据纠正（`users.tenant_id/role` 缓存行回填）→ #2567 / #2527；本单只保证自查询可用 | 本计划锁定 |
| D7 | 不改 `docs/ops/tenant-id-audit.md` 的既有计数/分类（口径随 P3 #2255 统一），只追加 A1.1 谓词小节 | 本计划锁定 |
| D8 | 不触碰 `backend/handlers/instrument_media.go:822`（avatar 只有 UPDATE，非本缺陷路径）与其余 `UPDATE` 型自写（背景第 10 条） | 本计划锁定 |
| D9 | 工作量由 Analyze 重估为 **M**（product 2 文件 + 测试 1 + 证据脚本 1 + 文档 1，2 条证据行）；Issue 创建时的 `size:S` 以本计划为准，`accept` 会写 `size:M` | 本计划锁定 |
| D10 | 预生产真机复验（`linwx1978@yahoo.com` 点「绑定微信」出码）归部署侧 → closing 时建 `[Ops-Followup of #2620]`（`waitForConfirm`，无 `status:*`） | 本计划锁定 |

### Out of Scope

- `/user/*` 家庭直连自查询（积分 / 地址 / 人脸采集与核验 / 支付计算 / 押金减免 / 待处理订单 / customer_role）→ **#2621**（已在本次 analyze 创建，`status:todo`）。
- 共享解析器 `middleware.EnsureLocalUser` / `middleware.LocalUserID`（`backend/middleware/iam.go:561,588,611`，31 个调用点）→ **#2621**。
- `users.tenant_id/org_id/role` 缓存行纠正 / 回填 → #2567 / #2527；`admin_uid` 与 nil UUID 行 → #2546 / #2526。
- `tenant_id` 体系的整体废弃（回调/context/查询口径）→ P3 #2255。
- `docs/ops/tenant-id-audit.md` 的 `IdentityCtx 23 处` 计数与模型分类刷新 → 随 #2255。
- 前端（PC / H5 / weapp）零改动：前端只展示后端错误码，本单不改任何接口契约。
- `POST /api/users/me/avatar` 与其它 `UPDATE` 型自写：非本缺陷路径（背景第 10 条）。
- 全量 `go test ./...` 作为合同行（由 pre-ready P5 baseline 差分承担）。
- 预生产真机复验 → `[Ops-Followup of #2620]`（closing 时创建）。

### Dependencies

- 无

### Workload

- Size: M
- Basis: 实现 2 文件（6 处句柄）+ 回归测试 1 文件（5 子用例）+ 证据脚本 1 + 审计文档 1 小节；2 条证据行（R1/R2）；无 migration、无新端点、无跨仓依赖、无前端
- Confidence: high

### Bug Reflection Matrix

| # | 维度 | 结论 | 证据 |
|---|---|---|---|
| 1 | 症状（symptom） | 商户管理员账号点「绑定微信」→ `404 {"code":40004,"message":"user not found"}`；同账号其它能力正常（token 有效） | Issue 正文（预生产 1.0.102 实测）；`backend/handlers/wechat_bind.go:75-78` |
| 2 | 触发条件（trigger） | `users.tenant_id`（IAM 关系缓存）≠ JWT `tid`，且该自查询使用带 `tid` 的句柄 | 背景第 1–2、5 条；`backend/database/db.go:96-124` |
| 3 | 根因（root cause） | 把「按 JWT `sub` 查我自己那行」当成租户数据查询：`addTenantScope` 给 `users` 查询追加 `tenant_id = tid` | `backend/database/db.go:96-124`；对比已修好的 `user_staff.go:1425` |
| 4 | 测试盲点（test blind spot） | 既有 wechat-bind 用例只注入 `middleware.ContextKeyUserID`，未注入 `database.SetTenantID` → 自动 scope 从不执行，缺陷在单测里不可见 | `backend/handlers/wechat_bind_test.go:53-61`；`backend/testutil/role_factory.go:57-73` |
| 5 | 同类扫描（same-class paths） | 同族直连自查询另见 8 文件（积分/地址/人脸/支付计算/押金减免/待处理订单/customer_role/共享解析器）→ **#2621**；本单只收 `/users/me/*` 的 6 处 | 背景第 12 条、Out of Scope |
| 6 | 修复层（fix layer） | 查询层单点收口：句柄换 `IdentityCtx`；不动业务分支、不动 `addTenantScope` 的全局语义 | 变更点 1 |
| 7 | 红绿实证（red-green） | 红：只落测试时 4 子用例 FAIL（404 `40004 user not found`）；绿：换句柄后 5/5 PASS | 变更点 2；R1 的 `negative_result` |
| 8 | 防复发（prevention） | ① 静态收口断言 `RESIDUAL_PLAIN_CTX_SITES=0` / `IDENTITY_CTX_SITES=6`；② 跨租户负例 `SELF2620_CROSS_TENANT_HIDDEN=1` 证明未放松隔离；③ 审计文档写入「身份自查询例外」谓词；④ 测试强制走 `TestActor` 注入 `database` 键 | R1/R2；`docs/ops/tenant-id-audit.md` A1.1 |
| 9 | 影响面（blast radius） | 仅 4 个 `/users/me/*` 端点的自查询命中率 + 2 处无鉴权路由的等价句柄；成功路径/错误码/字段集合不变；他人数据可见性不变 | 既有断言影响（0 翻转）、R1 负例 |
| 10 | 未修边界（deferred） | 其余自查询与共享解析器 → #2621；缓存行纠正 → #2567/#2527；`tenant_id` 废弃 → #2255；真机复验 → `[Ops-Followup of #2620]` | Out of Scope、D5/D6/D10 |

*Model: deepseek/deepseek-flash*

